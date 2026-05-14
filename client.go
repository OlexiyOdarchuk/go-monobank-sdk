package monobank

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
)

// Client-level errors.
var (
	// ErrEmptyRequest indicates that [Client.Do] received a nil request.
	ErrEmptyRequest = errors.New("empty request")
	// ErrInvalidURL indicates that the client's baseURL is not valid
	// (usually never happens because [New] always sets a default; it
	// can occur after [Client.SetBaseURL] with an invalid string).
	ErrInvalidURL = errors.New("invalid URL")
	// ErrInsecureBaseURL indicates that [WithBaseURL] received a
	// non-https URL for a non-loopback host. This guards against
	// accidentally sending tokens in cleartext. For tests via httptest
	// or a custom localhost proxy, use a loopback host or opt in via
	// [WithInsecureBaseURL].
	ErrInsecureBaseURL = errors.New("base URL must be https for non-loopback hosts")
)

// Sentinel errors for the common HTTP statuses. [APIError.Is]
// implements errors.Is against them, giving convenient detection:
//
//	if errors.Is(err, monobank.ErrUnauthorized) { /* token expired */ }
//	if errors.Is(err, monobank.ErrTooManyRequests) { /* back off */ }
//
// On top of the sentinels, the full [APIError] is still reachable via
// errors.As (status code, ErrorDescription, raw body).
var (
	// ErrUnauthorized is HTTP 401: the token is invalid or expired.
	ErrUnauthorized = errors.New("monobank: unauthorized (401)")
	// ErrForbidden is HTTP 403: the token lacks rights for the endpoint.
	ErrForbidden = errors.New("monobank: forbidden (403)")
	// ErrNotFound is HTTP 404: endpoint or entity does not exist.
	ErrNotFound = errors.New("monobank: not found (404)")
	// ErrTooManyRequests is HTTP 429: rate limit exceeded.
	ErrTooManyRequests = errors.New("monobank: too many requests (429)")
)

// HTTPDoer is the minimal subset of *http.Client that [Client]
// depends on. Any transport that implements this interface (the
// standard client, a custom round-tripper, a test fake) plugs in via
// [WithHTTPDoer].
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// APIError is returned when a monobank HTTP response does not match
// any of the statuses the caller expected. It captures the method,
// full URL, received and expected status codes, plus the first 256
// characters of the body for diagnostics.
//
// If the response body is JSON of the shape {"errorDescription": "..."}
// (the standard Mono error format for the personal/corporate/business/
// acquiring APIs), the [APIError.ErrorDescription] field holds the
// parsed message; otherwise it is empty and the original bytes remain
// in [APIError.Body].
type APIError struct {
	Method              string
	URL                 string
	StatusCode          int
	ExpectedStatusCodes []int
	// ErrorDescription is the value of the errorDescription field from
	// the JSON body of the Mono response, when the body could be
	// parsed; otherwise empty.
	ErrorDescription string
	Body             []byte
}

func (e *APIError) Error() string {
	detail := e.ErrorDescription
	if detail == "" {
		detail = truncate(e.Body, 256)
	}
	return fmt.Sprintf("%s %s: HTTP %d (expected %v): %s",
		e.Method, e.URL, e.StatusCode, e.ExpectedStatusCodes, detail)
}

// Is enables errors.Is(apiErr, monobank.ErrUnauthorized) and similar
// checks for the common HTTP statuses. Other status codes map only
// to [APIError] itself — use errors.As for full access.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrUnauthorized:
		return e.StatusCode == http.StatusUnauthorized
	case ErrForbidden:
		return e.StatusCode == http.StatusForbidden
	case ErrNotFound:
		return e.StatusCode == http.StatusNotFound
	case ErrTooManyRequests:
		return e.StatusCode == http.StatusTooManyRequests
	}
	return false
}

// errorBody is the JSON shape of the error that Mono returns from the
// personal/corporate/business/acquiring APIs. Other sub-packages (for
// example, installment) have their own formats and use their own
// error types.
type errorBody struct {
	ErrorDescription string `json:"errorDescription"`
}

// parseErrorDescription extracts errorDescription from a JSON body.
// Returns "" if the body is not JSON or the field is missing or empty.
func parseErrorDescription(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var e errorBody
	if err := json.Unmarshal(body, &e); err != nil {
		return ""
	}
	return e.ErrorDescription
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// Client is the base HTTP transport for every monobank surface. Each
// sub-package (bank, personal, corporate, business, acquiring)
// composes [Client] with an [auth.Authorizer] from the auth package
// and the base URL tailored to its API. Routine code does not usually
// construct this type directly — use the sub-package factories
// ([personal.New], [bank.New] etc.).
type Client struct {
	http      HTTPDoer
	auth      auth.Authorizer
	baseURL   *url.URL
	retry     retryPolicy
	limiter   RateLimiter
	userAgent string

	// unsafeRetries allows retrying POST/PATCH without an
	// Idempotency-Key. Default is false: methods that are not provably
	// idempotent are retried only when the Idempotency-Key header is
	// set (otherwise a retry loop after 502/504 can create a duplicate
	// operation).
	unsafeRetries bool

	// allowInsecureBaseURL is the bypass for [WithInsecureBaseURL]. By
	// default, an http:// URL pointing at a non-loopback host is
	// rejected with [ErrInsecureBaseURL] on the first Do.
	allowInsecureBaseURL bool

	// optErr holds the error produced while applying an Option (for
	// example, insecure base URL). It is returned from the first Do
	// so the error does not get lost between constructor and call.
	optErr error

	logger *slog.Logger
	onReq  func(*http.Request)
	onResp func(*http.Response, error)
}

// SetBaseURL overrides the base URL of an already-constructed client.
// If uri does not parse as a URL, the previous value is kept. For
// routine code use [WithBaseURL] when constructing through [New];
// this method exists for sub-packages that assemble [Client]
// incrementally.
func (c *Client) SetBaseURL(uri string) {
	u, err := url.Parse(uri)
	if err != nil || u == nil {
		return
	}
	c.baseURL = u
}

// Close stops the background resources attached to the client
// (currently the sweeper goroutine of [KeyedLimiter], when such a
// limiter was passed via [WithRateLimiter]). Safe to call on a client
// that does not need Close (returns nil).
//
// Implements [io.Closer], so the standard defer pattern just works:
//
//	cli := personal.New(token, monobank.WithRateLimiter(klim))
//	defer cli.Close()
//
// Without Close, the sweeper goroutine of [KeyedLimiter] stays alive
// until the process exits (a leak in tests, but normal in long-running
// services with a single global client).
func (c Client) Close() error {
	if closer, ok := c.limiter.(interface{ Stop() }); ok {
		closer.Stop()
	}
	return nil
}

// Do executes req against c.baseURL and decodes the response into v.
// The number of expected status codes is arbitrary; the default is
// http.StatusOK. If the response has a different code, returns
// [*APIError].
//
// The type of v selects the decoding mode:
//   - nil — the body is simply read and discarded;
//   - *[]byte — the raw body bytes are written into v;
//   - io.Writer — the body is copied to the Writer;
//   - otherwise — decoded as JSON into v.
//
// Transient failures (5xx, 429) are retried per [WithRetry]
// (honoring Retry-After). Context cancellation exits immediately.
//
// The method is exported so sub-packages (bank, personal, corporate,
// business, acquiring) share one HTTP plumbing (retry, base-URL
// resolution, error mapping) instead of reimplementing it. Pass an
// *http.Request with a path-only URL — it is resolved against the
// configured base URL.
func (c Client) Do(req *http.Request, v any, expectedStatusCodes ...int) error {
	if c.optErr != nil {
		return c.optErr
	}
	if req == nil {
		return ErrEmptyRequest
	}
	if c.baseURL == nil {
		return ErrInvalidURL
	}
	if len(expectedStatusCodes) == 0 {
		expectedStatusCodes = []int{http.StatusOK}
	}

	target, err := url.Parse(req.URL.String())
	if err != nil {
		return fmt.Errorf("parse request URL: %w", err)
	}
	req.URL = c.baseURL.ResolveReference(target)
	if req.Header.Get("Content-Type") == "" && req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if req.Header.Get("User-Agent") == "" {
		ua := c.userAgent
		if ua == "" {
			ua = UserAgent()
		}
		req.Header.Set("User-Agent", ua)
	}

	if c.auth != nil {
		if err := c.auth.SetAuth(req); err != nil {
			return fmt.Errorf("SetAuth: %w", err)
		}
	}

	// Read the body once and set GetBody so retries can re-read it.
	// http.NewRequest sets GetBody itself for
	// *bytes.Reader/*bytes.Buffer/*strings.Reader; here we cover the
	// case of an arbitrary io.Reader.
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		buf, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return fmt.Errorf("read request body: %w", err)
		}
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(buf)), nil
		}
		req.Body = io.NopCloser(bytes.NewReader(buf))
		if req.ContentLength == 0 {
			req.ContentLength = int64(len(buf))
		}
	}

	if c.limiter != nil {
		if err := c.limiter.Wait(req.Context()); err != nil {
			return err
		}
	}

	attemptFn := func() error {
		return c.attempt(req, v, expectedStatusCodes)
	}
	if !c.shouldRetry(req) {
		return attemptFn()
	}
	return c.retry.run(req.Context(), attemptFn)
}

// shouldRetry reports whether req may be automatically retried on a
// transient failure. GET/HEAD/PUT/DELETE/OPTIONS are always retried
// (idempotent per HTTP semantics). POST/PATCH only when the caller
// explicitly set Idempotency-Key (the bank deduplicates) or via
// [WithUnsafeRetries].
func (c Client) shouldRetry(req *http.Request) bool {
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions:
		return true
	case http.MethodPost, http.MethodPatch:
		return c.unsafeRetries || req.Header.Get("Idempotency-Key") != ""
	default:
		return false
	}
}

func (c Client) attempt(req *http.Request, v any, expectedStatusCodes []int) error {
	// Reset the body before each attempt — http.Transport fully
	// consumes Body during the previous Do.
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return fmt.Errorf("get request body: %w", err)
		}
		req.Body = body
	}

	if c.onReq != nil {
		c.onReq(req)
	}
	if c.logger != nil {
		c.logger.Debug("monobank: sending request",
			slog.String("method", req.Method),
			slog.String("url", req.URL.String()))
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	dur := time.Since(start)

	if c.onResp != nil {
		c.onResp(resp, err)
	}
	if c.logger != nil {
		if err != nil {
			c.logger.Warn("monobank: http error",
				slog.String("method", req.Method),
				slog.String("url", req.URL.String()),
				slog.Duration("duration", dur),
				slog.Any("err", err))
		} else {
			c.logger.Debug("monobank: http response",
				slog.String("method", req.Method),
				slog.String("url", req.URL.String()),
				slog.Int("status", resp.StatusCode),
				slog.Duration("duration", dur))
		}
	}

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if !slices.Contains(expectedStatusCodes, resp.StatusCode) {
		body, _ := io.ReadAll(resp.Body)
		apiErr := &APIError{
			Method:              req.Method,
			URL:                 req.URL.String(),
			StatusCode:          resp.StatusCode,
			ExpectedStatusCodes: expectedStatusCodes,
			ErrorDescription:    parseErrorDescription(body),
			Body:                body,
		}
		if isTransientStatus(resp.StatusCode) {
			return &transientStatus{
				code:       resp.StatusCode,
				retryAfter: parseRetryAfter(resp.Header),
				cause:      apiErr,
			}
		}
		return apiErr
	}

	// Empty body (204 No Content or Content-Length: 0) — do not try to
	// JSON-decode, because json.Decoder.Decode on an empty stream
	// returns io.EOF. For DELETE methods this is a valid success
	// response.
	if resp.StatusCode == http.StatusNoContent || resp.ContentLength == 0 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	switch out := v.(type) {
	case nil:
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	case *[]byte:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response body: %w", err)
		}
		*out = body
		return nil
	case io.Writer:
		if _, err := io.Copy(out, resp.Body); err != nil {
			return fmt.Errorf("copy response body: %w", err)
		}
		return nil
	default:
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		return nil
	}
}

// isTransientStatus reports whether an HTTP status code is worth retrying.
func isTransientStatus(code int) bool {
	return code == http.StatusTooManyRequests || (code >= 500 && code <= 599)
}
