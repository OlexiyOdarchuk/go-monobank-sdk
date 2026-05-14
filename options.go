package monobank

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
)

const defaultBaseURL = "https://api.monobank.ua"

// Option configures [Client]. Pass it to [New] (and likewise to
// sub-package factories — [personal.New], [bank.New] etc., which
// forward options to New). Additive design: new options are added
// without breaking existing call sites.
type Option func(*Client)

// WithHTTPClient sets a custom *http.Client (handy for timeouts,
// custom transports, proxies). nil or omission falls back to the
// standard &http.Client{} with no timeout.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient == nil {
			return
		}
		c.http = httpClient
	}
}

// WithHTTPDoer accepts any [HTTPDoer]. Useful for plugging in
// middleware (circuit breakers, custom transports, test fakes).
// nil is ignored.
func WithHTTPDoer(d HTTPDoer) Option {
	return func(c *Client) {
		if d == nil {
			return
		}
		c.http = d
	}
}

// WithRoundTripper installs a custom [http.RoundTripper] without
// touching the rest of the embedded *http.Client settings (timeout,
// Cookie jar, redirect policy). This is the standard middleware
// slot: OpenTelemetry / Datadog / Prometheus / custom auth-refresh
// or circuit-breaker logic that does not require rewriting the whole
// HTTP stack.
//
//	type loggingRT struct{ next http.RoundTripper }
//	func (l loggingRT) RoundTrip(r *http.Request) (*http.Response, error) {
//	    log.Println("→", r.Method, r.URL.Path)
//	    return l.next.RoundTrip(r)
//	}
//
//	cli := personal.New(token, monobank.WithRoundTripper(
//	    loggingRT{next: http.DefaultTransport},
//	))
//
// Compose by ordinary wrapping: build the middleware chain from the
// outside in, or via a helper function. nil is ignored.
//
// Option order: [WithRoundTripper] MUST come AFTER [WithHTTPClient]
// (otherwise WithHTTPClient overwrites the transport). If you pass
// your own http.Client that already has the right Transport, use
// [WithHTTPClient] alone, without [WithRoundTripper].
func WithRoundTripper(rt http.RoundTripper) Option {
	return func(c *Client) {
		if rt == nil {
			return
		}
		// If c.http is a standard *http.Client, clone it and replace
		// Transport. Otherwise build a new client with the default
		// timeout.
		if hc, ok := c.http.(*http.Client); ok {
			cloned := *hc
			cloned.Transport = rt
			c.http = &cloned
			return
		}
		c.http = &http.Client{Transport: rt}
	}
}

// WithAuth attaches an [auth.Authorizer] to the client. Sub-packages
// (personal, corporate, business, acquiring) use it to plug in their
// authorization scheme (X-Token, ECDSA signature, etc.). nil is
// ignored — the default stays [auth.Public] (no authorization).
func WithAuth(a auth.Authorizer) Option {
	return func(c *Client) {
		if a == nil {
			return
		}
		c.auth = a
	}
}

// WithLogger attaches a *slog.Logger to the client. When set, the
// SDK logs:
//
//   - Debug "monobank: sending request" — before every HTTP call
//     (method, url).
//   - Debug "monobank: http response" — successful response (method,
//     url, status, duration).
//   - Warn "monobank: http error" — transport failure (method, url,
//     duration, err).
//
// The logger fires per attempt — retries produce multiple records.
// nil is ignored. Default: do not log.
//
// CAUTION (PII): at Debug level the full request URL goes into the
// log, including the accountID path segment for
// /personal/statement/{acc}/... Do not enable Debug in production,
// or wire up a handler-side filter in your own slog.Handler (banking
// secrecy). Info/Warn are safe.
//
//	cli := personal.New(token, monobank.WithLogger(slog.Default()))
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) {
		if l == nil {
			return
		}
		c.logger = l
	}
}

// WithRequestHook installs a callback invoked before every HTTP
// request (including each retry). The request has been resolved to
// its full URL and has the authorization headers set — the hook may
// add its own (for example, OpenTelemetry trace context,
// X-Correlation-Id). nil is ignored.
//
//	cli := personal.New(token, monobank.WithRequestHook(func(r *http.Request) {
//	    r.Header.Set("X-Correlation-Id", uuid.NewString())
//	}))
func WithRequestHook(fn func(*http.Request)) Option {
	return func(c *Client) {
		if fn == nil {
			return
		}
		c.onReq = fn
	}
}

// WithResponseHook installs a callback invoked after every HTTP
// response (success and failure). It runs IMMEDIATELY after
// http.Doer.Do, before parsing and before the expected-status check.
// resp may be nil (if err != nil); err may be nil (when everything
// is fine). Useful for metrics (per-attempt latency, error counts).
// nil is ignored.
//
//	cli := personal.New(token, monobank.WithResponseHook(func(r *http.Response, err error) {
//	    if r != nil {
//	        metrics.Counter("mono.responses", "status", strconv.Itoa(r.StatusCode)).Inc()
//	    }
//	}))
func WithResponseHook(fn func(*http.Response, error)) Option {
	return func(c *Client) {
		if fn == nil {
			return
		}
		c.onResp = fn
	}
}

// WithBaseURL overrides the default base URL
// (https://api.monobank.ua). Handy for testing against httptest.Server,
// a recorded proxy, or alternative hosts (corp-api.monobank.ua is
// applied automatically inside [business.New], no need to set it
// here).
//
// SECURITY: if uri uses a non-https scheme AND the host is not a
// loopback, the client remembers [ErrInsecureBaseURL] and returns
// it from the very first [Client.Do]. Loopback covers the literal
// hostname "localhost" plus any IP for which
// [net.IP.IsLoopback] is true (127.0.0.0/8 and ::1 — not just
// 127.0.0.1). This guards against accidentally deploying with a
// staging config that sends X-Token in cleartext. Opt out
// deliberately via [WithInsecureBaseURL].
//
// Option order does NOT matter: [New] applies WithInsecureBaseURL
// first in a separate pass before evaluating the base-URL guard.
func WithBaseURL(uri string) Option {
	return func(c *Client) {
		if isInsecureBaseURL(uri) && !c.allowInsecureBaseURL {
			c.optErr = fmt.Errorf("%w: got %q", ErrInsecureBaseURL, uri)
			return
		}
		c.SetBaseURL(uri)
	}
}

// WithInsecureBaseURL deliberately allows an http:// URL on a
// non-loopback host in [WithBaseURL]. Useful for a recorded MITM
// proxy used for debugging (mitmproxy, burp) or staging setups
// behind a VPN where https is overkill. The default is false; turn
// it on only if you understand that the token will travel in
// cleartext and that is acceptable on your network.
//
// Option order is irrelevant — [New] resolves WithInsecureBaseURL
// in a dedicated pass before any other option runs.
func WithInsecureBaseURL(allow bool) Option {
	return func(c *Client) {
		c.allowInsecureBaseURL = allow
	}
}

// isInsecureBaseURL reports whether the scheme is not https and the
// host is not a loopback. Loopback means literal "localhost", or an
// IP for which [net.IP.IsLoopback] is true (127.0.0.0/8 and ::1) —
// not only "127.0.0.1" and "::1" exactly.
func isInsecureBaseURL(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil || u == nil {
		return false
	}
	if u.Scheme == "https" || u.Scheme == "" {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return false
	}
	return true
}

// WithRetry enables automatic retry for transient failures (5xx and
// 429). Backoff is exponential with full jitter; Retry-After from the
// response is honored when present.
//
// attempts == 0 inherits the defaults (4 attempts, base 500ms,
// max 30s). attempts <= 1 explicitly disables retry. Non-positive
// baseDelay / maxDelay inherit the defaults.
//
// Only transient failures (5xx, 429) are retried; 4xx errors come
// back as [APIError] without a retry, because they are genuine
// client-side errors.
func WithRetry(attempts int, baseDelay, maxDelay time.Duration) Option {
	return func(c *Client) {
		if attempts == 0 {
			c.retry = defaultRetry
			return
		}
		c.retry.maxAttempts = attempts
		if baseDelay > 0 {
			c.retry.baseDelay = baseDelay
		} else if c.retry.baseDelay == 0 {
			c.retry.baseDelay = defaultRetry.baseDelay
		}
		if maxDelay > 0 {
			c.retry.maxDelay = maxDelay
		} else if c.retry.maxDelay == 0 {
			c.retry.maxDelay = defaultRetry.maxDelay
		}
	}
}

// WithUserAgent overrides the User-Agent the SDK sets on every
// request (default is [UserAgent]). Helpful so Mono support can tell
// your service apart from other SDK users:
//
//	cli := personal.New(token,
//	    monobank.WithUserAgent("acme-receipts/2.1.0 "+monobank.UserAgent()),
//	)
//
// An empty string is ignored (the SDK default is kept).
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		if ua == "" {
			return
		}
		c.userAgent = ua
	}
}

// WithUnsafeRetries enables automatic retries for POST/PATCH without
// an Idempotency-Key header. By default such methods are not retried,
// because a 502/504 from the load balancer can arrive AFTER upstream
// has already processed the request — a retry then creates a
// duplicate operation (for example, two invoices via
// [acquiring.Client.CreateInvoice]).
//
// Mono accepts Idempotency-Key for every mutating endpoint where it
// makes sense (see [business.NewIdempotencyKey], which is set
// automatically in [business.Client.PreparePayment] /
// [business.Client.CreateSalaryRegistry]). For the remaining POST
// methods, if you are sure the endpoint is idempotent on Mono's side
// or are happy to live with duplicates, set WithUnsafeRetries(true).
func WithUnsafeRetries(enabled bool) Option {
	return func(c *Client) {
		c.unsafeRetries = enabled
	}
}

// WithRateLimiter sets a client-side throttle. [RateLimiter.Wait]
// is called on EVERY attempt, including each retry — so a burst of
// retries after a 502/429 cannot blow past the limiter the moment
// the upstream recovers. nil is ignored.
//
// Mono has strict limits (for example, /personal/client-info is one
// call per 60 s); without a limiter the SDK relies solely on
// server-side 429 plus [WithRetry] backoff. A local limiter helps
// avoid getting 429-ed right away.
//
//	lim := monobank.NewLimiter(time.Minute, 1)
//	cli := personal.New(token, monobank.WithRateLimiter(lim))
//
// You can drop in any *golang.org/x/time/rate.Limiter — its Wait(ctx)
// signature matches [RateLimiter].
func WithRateLimiter(l RateLimiter) Option {
	return func(c *Client) {
		if l == nil {
			return
		}
		c.limiter = l
	}
}

// New returns the base [Client] assembled from the supplied options.
// Without [WithAuth] the client uses [auth.Public] (no-op) — fine for
// the public bank endpoints via the bank sub-package, but not for
// personal, corporate, business or acquiring (which require real
// authorization; they normally call New themselves, adding their own
// [auth.Authorizer]).
//
//	c := monobank.New(
//	    monobank.WithHTTPClient(myHTTP),
//	    monobank.WithRetry(5, 0, 0),
//	)
//
// Options are applied in two passes so [WithInsecureBaseURL] takes
// effect regardless of where it appears in the list. Each option
// runs twice — they are expected to be pure setters; if you wrote a
// custom Option with side effects, scope them to a single pass.
func New(opts ...Option) Client {
	base, _ := url.Parse(defaultBaseURL)
	c := Client{
		http:    &http.Client{},
		auth:    auth.Public{},
		baseURL: base,
	}
	// First pass: discover allowInsecureBaseURL on a throwaway probe
	// so the real apply can already see it when WithBaseURL fires.
	probe := c
	for _, opt := range opts {
		opt(&probe)
	}
	c.allowInsecureBaseURL = probe.allowInsecureBaseURL
	// Second pass: apply all options on the real client. optErr from
	// the probe is discarded; only the real pass counts.
	for _, opt := range opts {
		opt(&c)
	}
	return c
}
