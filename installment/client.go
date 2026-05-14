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
	"net/http"
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
// [Client.VerifyCallback] when the value of the signature header
// does not match the expected one.
var ErrCallbackSignatureMismatch = errors.New("installment: callback signature mismatch")

// MaxResponseBytes is the cap on response size beyond which the body
// is truncated. Guards against OOM when a malicious or glitched
// proxy returns enormous bodies. PDF documents (payslips, invoices)
// can reach a few MB; 50 MiB leaves plenty of headroom.
const MaxResponseBytes = 50 << 20

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
// CAUTION (security): pass only https URLs in non-local environments.
// The installment secret travels in the body's HMAC signature — if
// someone intercepts a (body, signature) pair over http they can
// forge a callback (since the secret is the HMAC key). Unlike root
// [monobank.WithBaseURL], there is no logger here for a runtime
// warning, so an http scheme is accepted silently. Localhost /
// 127.0.0.1 is fine for tests via httptest.
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithHTTPClient installs a custom http.Client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.h = h } }

// Client is the installment API client. Build one via [New].
type Client struct {
	h       *http.Client
	baseURL string
	storeID string
	secret  []byte
}

// New returns a client with a 30s default timeout, pointed at
// production. For tests use [WithBaseURL] with [BaseURLSandbox] or
// [BaseURLStage].
//
// CAUTION: the default is production. Forgetting [WithBaseURL] in a
// test environment makes the first call hit the live API. Sandbox
// and production secrets differ, so authentication will fail on
// mismatch — but if your test code holds a production secret, a real
// operation will be performed. Always pass an explicit BaseURL in
// non-prod code.
//
//	cli := installment.New("test_store_with_confirm", "secret_98765432--123-123",
//	    installment.WithBaseURL(installment.BaseURLSandbox))
func New(storeID, secret string, opts ...Option) *Client {
	c := &Client{
		h:       &http.Client{Timeout: 30 * time.Second},
		baseURL: BaseURLProduction,
		storeID: storeID,
		secret:  []byte(secret),
	}
	for _, o := range opts {
		o(c)
	}
	return c
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
func (c *Client) Sign(body []byte) string {
	mac := hmac.New(sha256.New, c.secret)
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyCallback returns nil when HMAC-SHA256(body, secret) matches
// the value of the signature header received in an incoming
// callback; otherwise it returns [ErrCallbackSignatureMismatch].
// Call it before processing the body, otherwise anyone can send a
// fake request.
//
// The implementation rejects a signature of incorrect length
// (base64 of a 32-byte HMAC-SHA256 is always 44 characters) BEFORE
// computing the HMAC — this guards against a CPU-DoS where an
// attacker sends a gigabyte body with an empty or arbitrary
// signature. Regardless of length, the final comparison is
// constant-time via [hmac.Equal].
//
// Also wrap the request body in [http.MaxBytesReader] before calling
// VerifyCallback to cap the upper bound on body reads.
func (c *Client) VerifyCallback(body []byte, signatureHeader string) error {
	const wantLen = 44 // base64.StdEncoding.EncodedLen(sha256.Size)
	if len(signatureHeader) != wantLen {
		return ErrCallbackSignatureMismatch
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
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes))
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
// response body (PDF).
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
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("installment: read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
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
