package installment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Standard base URLs for the three installment environments.
const (
	BaseURLSandbox    = "https://u2-demo-ext.mono.st4g3.com"
	BaseURLStage      = "https://u2-ext.mono.st4g3.com"
	BaseURLProduction = "https://u2.monobank.com.ua"
)

// HeaderStoreID is the name of the store-identifier HTTP header.
const HeaderStoreID = "store-id"

// HeaderSignature is the name of the signature HTTP header.
const HeaderSignature = "signature"

// ErrNilRequest is returned from mutating methods when body == nil.
var ErrNilRequest = errors.New("installment: request body is nil")

// ErrCallbackSignatureMismatch is returned from
// [Client.VerifyCallback] when HMAC-SHA256 of the body keyed with
// the merchant secret does not match the value of the signature
// header. Use this sentinel to distinguish a forged callback from
// other Verify failures.
var ErrCallbackSignatureMismatch = errors.New("installment: callback signature mismatch")

// ErrCallbackBadLength is returned from [Client.VerifyCallback]
// when the signature header has the wrong length (base64 of a
// 32-byte HMAC-SHA256 is always 44 characters). Splitting this
// case from [ErrCallbackSignatureMismatch] lets callers tell a
// malformed request apart from a forgery attempt — useful for
// security telemetry.
var ErrCallbackBadLength = errors.New("installment: callback signature has wrong length")

// ErrEmptyOrderID is returned when an order ID argument is empty.
var ErrEmptyOrderID = errors.New("installment: order ID is empty")

// ErrEmptyDate is returned when a date argument is empty / zero.
var ErrEmptyDate = errors.New("installment: date is empty")

// ErrEmptyPhone is returned when a phone argument is empty.
var ErrEmptyPhone = errors.New("installment: phone is empty")

// ErrInvalidPhone is returned when a phone does not look like a
// monobank-acceptable number (must start with + and contain only
// digits afterwards).
var ErrInvalidPhone = errors.New("installment: phone must start with + and contain only digits")

// ErrEmptyStoreID is returned by [New] when storeID == "".
var ErrEmptyStoreID = errors.New("installment: storeID is empty")

// ErrEmptySecret is returned by [New] when secret == "".
var ErrEmptySecret = errors.New("installment: secret is empty")

// ErrInsecureBaseURL is returned by [WithBaseURL] / [New] when the
// configured base URL has an http:// scheme on a non-loopback host.
// The installment secret is the HMAC key for outgoing requests and
// for callback verification — over plain http anyone who captures a
// (body, signature) pair can forge subsequent callbacks. Loopback
// (localhost / 127.0.0.1 / ::1) is allowed for tests.
var ErrInsecureBaseURL = errors.New("installment: base URL must be https for non-loopback hosts")

// MaxJSONResponseBytes caps the JSON response size for installment
// API calls. Mono's JSON envelopes for orders, validation, and
// reports are well under 100 KiB in practice; a 1 MiB cap guards
// against a malicious or glitched proxy returning gigabyte JSON
// bodies into io.ReadAll without exhausting heap.
const MaxJSONResponseBytes = 1 << 20

// MaxPDFResponseBytes caps the PDF response size for installment
// endpoints that return application/pdf (guarantee letters,
// receipts). Guarantee letters typically run in the low MB; 50 MiB
// is the same generous ceiling that used to apply to every response
// — it is fine for binary, but is far too lax for JSON.
const MaxPDFResponseBytes = 50 << 20

// MaxResponseBytes preserved for backward compatibility — the
// per-endpoint constants above are what code should reach for
// instead.
//
// Deprecated: use [MaxJSONResponseBytes] or [MaxPDFResponseBytes].
const MaxResponseBytes = MaxPDFResponseBytes

// APIError is the server-error shape (response body
// {"message": "..."}).
type APIError struct {
	StatusCode int
	Message    string `json:"message"`
	TraceID    string
}

// Error implements error.
func (e *APIError) Error() string {
	if e.TraceID != "" {
		return fmt.Sprintf("installment: %d: %s (trace=%s)", e.StatusCode, e.Message, e.TraceID)
	}
	return fmt.Sprintf("installment: %d: %s", e.StatusCode, e.Message)
}

// Option is a functional option for [New].
type Option func(*Client)

// WithBaseURL overrides the base URL (default is production).
//
// SECURITY: http:// scheme on a non-loopback host is rejected at
// [New] time with [ErrInsecureBaseURL]. The installment secret is
// the HMAC key for both outgoing requests and callback verification
// — if anyone captures a (body, signature) pair over plain http they
// can forge subsequent callbacks. Loopback (localhost / 127.0.0.1 /
// ::1) is permitted for tests via httptest.
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithInsecureBaseURL deliberately allows an http:// URL on a
// non-loopback host. Useful for a recorded MITM proxy or staging
// behind a VPN where https is overkill. Default false — turn on
// only if you understand the secret will travel in the clear.
//
// Option order does NOT matter — [New] validates the final base
// URL after applying every option.
func WithInsecureBaseURL(allow bool) Option {
	return func(c *Client) { c.allowInsecureBaseURL = allow }
}

// WithHTTPClient installs a custom http.Client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.h = h } }

// Client is the installment API client. Build one via [New].
type Client struct {
	h                    *http.Client
	baseURL              string
	storeID              string
	secret               []byte
	allowInsecureBaseURL bool
}

// New returns a client with a 30s default timeout, pointed at
// production. For tests use [WithBaseURL] with [BaseURLSandbox] or
// [BaseURLStage].
//
// New returns an error when storeID or secret is empty, or when the
// configured base URL has an http:// scheme on a non-loopback host
// (opt out with [WithInsecureBaseURL]).
//
// CAUTION: the default is production. Forgetting [WithBaseURL] in a
// test environment makes the first call hit the live API. Sandbox
// and production secrets differ, so authentication will fail on
// mismatch — but if your test code holds a production secret, a real
// operation will be performed. Always pass an explicit BaseURL in
// non-prod code.
//
//	cli, err := installment.New("test_store_with_confirm", "secret_98765432--123-123",
//	    installment.WithBaseURL(installment.BaseURLSandbox))
//	if err != nil { ... }
func New(storeID, secret string, opts ...Option) (*Client, error) {
	if storeID == "" {
		return nil, ErrEmptyStoreID
	}
	if secret == "" {
		return nil, ErrEmptySecret
	}
	c := &Client{
		h:       &http.Client{Timeout: 30 * time.Second},
		baseURL: BaseURLProduction,
		storeID: storeID,
		secret:  []byte(secret),
	}
	for _, o := range opts {
		o(c)
	}
	if isInsecureBaseURL(c.baseURL) && !c.allowInsecureBaseURL {
		return nil, fmt.Errorf("%w: got %q", ErrInsecureBaseURL, c.baseURL)
	}
	return c, nil
}

// isInsecureBaseURL reports whether u has a non-https scheme AND
// is not pointed at a loopback host. Mirrors monobank.isInsecureBaseURL
// (kept local to avoid a circular import).
func isInsecureBaseURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil || parsed == nil {
		return false
	}
	if parsed.Scheme == "https" || parsed.Scheme == "" {
		return false
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return false
	}
	return true
}

// LogValue is the slog serializer that hides the store secret.
// Without it, `slog.Info("cli", "installment", cli)` would dump the
// raw byte slice of the secret as the field value.
func (c *Client) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("storeID", c.storeID),
		slog.String("secret", "***"),
		slog.String("baseURL", c.baseURL),
	)
}

// Sign computes base64(HMAC-SHA256(body, secret)). Exported for
// tests and for verifying incoming callback requests (the signature
// in the signature header is computed the same way).
//
// Important: the signature covers ONLY the request body, not any
// request headers, query parameters, path, method, or timestamp.
// Mono uses this primitive both for outgoing requests (the SDK
// computes it before sending) and for incoming callbacks (the SDK
// verifies it via [Client.VerifyCallback]). If you need replay
// protection on callbacks, deduplicate at your application layer
// — for example by (order_id, state) or by Mono's own callback
// nonce when one is supplied — because two different requests with
// identical bodies will produce identical signatures.
func (c *Client) Sign(body []byte) string {
	mac := hmac.New(sha256.New, c.secret)
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyCallback returns nil when HMAC-SHA256(body, secret) matches
// the value of the signature header received in an incoming
// callback. Otherwise it returns one of two sentinels:
//   - [ErrCallbackBadLength] — the header has the wrong length
//     (base64 of a 32-byte HMAC-SHA256 is always 44 characters).
//   - [ErrCallbackSignatureMismatch] — the length is right but
//     the HMAC does not match.
//
// Splitting the two cases makes it cheap to tell a malformed
// request apart from a forgery attempt in security logs and
// alerting rules. Callers that do not care about the distinction
// can either inspect both sentinels with errors.Is or fall back to
// "any non-nil err means reject".
//
// Call VerifyCallback before processing the body, otherwise
// anyone can send a fake request. Wrap the request body in
// [http.MaxBytesReader] before calling so an attacker cannot
// stream gigabytes into ReadAll. The HMAC comparison is
// constant-time via [hmac.Equal].
func (c *Client) VerifyCallback(body []byte, signatureHeader string) error {
	const wantLen = 44 // base64.StdEncoding.EncodedLen(sha256.Size)
	if len(signatureHeader) != wantLen {
		return ErrCallbackBadLength
	}
	want := c.Sign(body)
	if !hmac.Equal([]byte(want), []byte(signatureHeader)) {
		return ErrCallbackSignatureMismatch
	}
	return nil
}

// doJSON performs a POST with a JSON body, signs it, checks the
// expected status, and decodes the response into out (when
// out != nil).
func (c *Client) doJSON(ctx context.Context, path string, in, out any, wantStatus int) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("installment: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("installment: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(HeaderStoreID, c.storeID)
	req.Header.Set(HeaderSignature, c.Sign(body))
	resp, err := c.h.Do(req)
	if err != nil {
		return fmt.Errorf("installment: do request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxJSONResponseBytes))
	if err != nil {
		return fmt.Errorf("installment: read response body: %w", err)
	}
	if resp.StatusCode != wantStatus {
		return decodeAPIError(resp, respBody)
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("installment: decode response: %w", err)
	}
	return nil
}

// doPDF performs a POST with a JSON body and returns the raw
// response body (PDF). PDFs (guarantee letters, receipts) can run
// into a few MB legitimately, so this path uses the larger
// [MaxPDFResponseBytes] cap rather than the JSON cap.
func (c *Client) doPDF(ctx context.Context, path string, in any) ([]byte, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("installment: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("installment: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/pdf")
	req.Header.Set(HeaderStoreID, c.storeID)
	req.Header.Set(HeaderSignature, c.Sign(body))
	resp, err := c.h.Do(req)
	if err != nil {
		return nil, fmt.Errorf("installment: do request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxPDFResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("installment: read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// Error responses are JSON, even on PDF endpoints. Keep the
		// PDF cap here because Mono never sends a multi-MB error
		// payload — saving a per-path branch.
		return nil, decodeAPIError(resp, respBody)
	}
	return respBody, nil
}

func decodeAPIError(resp *http.Response, body []byte) error {
	e := &APIError{StatusCode: resp.StatusCode, TraceID: resp.Header.Get("Trace-Id")}
	_ = json.Unmarshal(body, e)
	if e.Message == "" {
		e.Message = string(body)
	}
	return e
}
