// Package openbanking is the Go client for monobank's Open Banking
// API — the Ukrainian PSD2 / NBU implementation of the Berlin Group
// NextGenPSD2 framework (openbanking.mono.bank/ext). It covers:
//
//   - account-access consents: create, start authorisation, read,
//     read status, revoke;
//   - account information (AIS): account list, balances, paginated
//     statements;
//   - payment initiation (PIS): create, read, read status, cancel,
//     start authorisation, list authorisations, read authorisation
//     status;
//   - the sandbox-only test-certificate desk.
//
// # Authorization
//
// There is no token. The caller is authenticated by the QWAC
// certificate it presents in the TLS handshake, so every request must
// go out over a mutually-authenticated connection. Build the
// transport with [NewMTLSHTTPClient] (or
// [NewMTLSHTTPClientFromFiles]) and hand it to the client via
// [monobank.WithHTTPClient]. Without a valid QWAC the bank drops the
// connection during the handshake — the failure surfaces as a
// transport error, never as an HTTP status, so do not expect a 401 to
// tell you the certificate is wrong.
//
// The only Berlin Group protocol header the SDK adds on its own is
// X-Request-ID, which the spec marks mandatory on all fifteen
// production operations; Accept, Content-Type and User-Agent come
// from the shared client. It is
// filled with a fresh UUIDv4 per call by [RequestIDAuth]; use
// [ContextWithRequestID] to pin a specific value when correlating
// with your own tracing or with a support ticket.
//
// # Typical scenarios
//
//   - Account information: [Client.CreateConsent] listing the exact
//     accounts and rights → [Client.StartConsentAuthorisation] →
//     send the PSU to _links.scaRedirect → poll
//     [Client.ConsentStatus] until [ConsentValid] → call
//     [Client.Accounts], [Client.Balances], [Client.Transactions]
//     with that consent id.
//   - Payment initiation: [Client.CreatePayment] →
//     [Client.StartPaymentAuthorisation] → the PSU confirms (by
//     redirect, or in their own app when the approach is
//     [SCADecoupled]) → poll [Client.PaymentStatus] to
//     [StatusAcceptedCreditSettlementCompleted].
//   - Housekeeping: [Client.RevokeConsent] when the customer
//     unsubscribes, [Client.CancelPayment] while a payment is still
//     cancellable.
//   - Onboarding in the sandbox:
//     [Client.RequestTestCertificate] →
//     [Client.TestCertificateStatus] until the CSR is approved.
//
// # Response headers
//
// Two pieces of protocol data arrive in headers rather than bodies:
// ASPSP-SCA-Approach (which approach the bank chose) and Location
// (the URI of a created resource). The shared [monobank.Client]
// decodes bodies, so read them with a response hook:
//
//	var approach string
//	cli := openbanking.New(
//		monobank.WithHTTPClient(tls),
//		monobank.WithResponseHook(func(resp *http.Response, err error) {
//			if resp != nil {
//				approach = resp.Header.Get(openbanking.HeaderASPSPSCAApproach)
//			}
//		}),
//	)
//
// For the common case the body is enough: for a payment
// authorisation the spec states that _links.scaRedirect is present
// only under the [SCARedirect] approach. It does not repeat that
// guarantee for a consent authorisation, so treat the link there as
// present-or-not rather than as a signal of the approach.
//
// # Rate limits
//
// A consent declares frequencyPerDay — a budget the TPP commits to
// itself. The spec does not document how the bank reacts when it is
// exhausted: the account-information operations declare only
// 200/400/401/500, and the single 429 in the whole spec belongs to
// [Client.RequestTestCertificate]. Budget on your side rather than
// waiting for a status code to tell you.
package openbanking

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
)

// Base URLs of the three environments the spec publishes. Production
// is the default of [New].
//
// The hosts are given without the "/ext" prefix that the spec's
// server URLs carry: the shared [monobank.Client] resolves each
// request against the base URL as an absolute path, so the prefix
// lives in the request paths instead. Overriding the base URL with
// [monobank.WithBaseURL] therefore takes a bare scheme+host.
const (
	// BaseURLProduction is the live Open Banking host.
	BaseURLProduction = "https://openbanking.mono.bank"
	// BaseURLStage is the staging host a test QWAC is issued for.
	BaseURLStage = "https://openbanking-ext.mono.st4g3.com"
	// BaseURLSandbox is the host that serves the test-certificate
	// desk ([Client.RequestTestCertificate] and
	// [Client.TestCertificateStatus]). The spec exposes it purely for
	// that purpose and does not say whether the production operations
	// answer there.
	BaseURLSandbox = "https://ob-ext.mono.st4g3.com"
)

// pathPrefix is the "/ext" segment every operation of this API sits
// under. See the base-URL constants for why it is not part of them.
const pathPrefix = "/ext"

// Input-validation sentinels. They are all raised before anything is
// written to the socket: the bank answers a missing mandatory header
// or path segment with an opaque 400, and a wasted round-trip against
// a consent's frequencyPerDay budget is worse than a local error.
//
// The checks cover headers, query parameters and path segments only.
// Mandatory fields *inside* a request body are left to the bank, the
// same way the other packages of this SDK treat them — an incomplete
// [CreateConsentRequest] or [CreatePaymentRequest] goes out and comes
// back as a 400 describing what was missing.
var (
	// ErrEmptyID is returned when a path-parameter identifier
	// (consentId, accountId, paymentId, authorisationId,
	// paymentProduct) is empty.
	ErrEmptyID = errors.New("openbanking: empty identifier")
	// ErrNilRequest is returned by methods that take a request body
	// when it is nil.
	ErrNilRequest = errors.New("openbanking: request body is nil")
	// ErrMissingConsentID is returned when [AccountOptions.ConsentID]
	// is empty — the Consent-ID header is mandatory on every
	// account-information call.
	ErrMissingConsentID = errors.New("openbanking: Consent-ID is required")
	// ErrMissingPSUIPAddress is returned when
	// [InitiationOptions].PSU.IPAddress is empty — the
	// PSU-IP-Address header is mandatory when creating a consent or a
	// payment.
	ErrMissingPSUIPAddress = errors.New("openbanking: PSU-IP-Address is required")
	// ErrMissingBookingStatus is returned when
	// [TransactionsOptions.BookingStatus] is empty — the
	// bookingStatus query parameter is mandatory.
	ErrMissingBookingStatus = errors.New("openbanking: bookingStatus is required")
	// ErrNoClientCert is returned by [NewMTLSHTTPClient] when the
	// certificate or key PEM is empty. Starting a client without a
	// QWAC only defers the failure to the first TLS handshake.
	ErrNoClientCert = errors.New("openbanking: client certificate and key are required")
	// ErrInvalidCA is returned by [NewMTLSHTTPClient] when
	// [MTLSConfig.RootCAsPEM] contains no parsable certificate.
	ErrInvalidCA = errors.New("openbanking: no certificates found in RootCAsPEM")
)

// Client talks to the monobank Open Banking API. It wraps
// [monobank.Client] for the HTTP plumbing (base-URL resolution,
// retries, rate limiting, error mapping, hooks) and adds the typed
// methods and DTOs of the Berlin Group surface.
type Client struct {
	c monobank.Client
}

// New returns a [Client] aimed at [BaseURLProduction].
//
// Authentication happens in the TLS handshake, so the one option that
// is not optional is the transport:
//
//	httpCli, err := openbanking.NewMTLSHTTPClientFromFiles(
//		"qwac.pem", "qwac.key", "")
//	if err != nil {
//		return err
//	}
//	cli := openbanking.New(
//		monobank.WithBaseURL(openbanking.BaseURLStage),
//		monobank.WithHTTPClient(httpCli),
//	)
//	defer cli.Close()
//
// Extra options are forwarded to [monobank.New]. The default
// authorizer is [RequestIDAuth]; replace it via [monobank.WithAuth]
// only to control how X-Request-ID is generated.
func New(opts ...monobank.Option) *Client {
	base := []monobank.Option{
		monobank.WithBaseURL(BaseURLProduction),
		monobank.WithAuth(RequestIDAuth{}),
	}

	return &Client{c: monobank.New(append(base, opts...)...)}
}

// Close releases the client's background resources (see
// [monobank.Client.Close]).
func (c *Client) Close() error { return c.c.Close() }

// requestIDKey is the context key carrying a caller-chosen
// X-Request-ID.
type requestIDKey struct{}

// ContextWithRequestID pins the X-Request-ID of every call made with
// the returned context, instead of letting [RequestIDAuth] mint one.
//
// Use it to stitch a bank-side request to your own trace, or to
// repeat a call under the id a support ticket refers to. The value
// should be a UUID — the spec types the header as one — and must be
// unique per call, so do not reuse one context across unrelated
// requests.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// requestIDFromContext returns the pinned id, if any.
func requestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)

	return id
}

// RequestIDAuth implements [auth.Authorizer] for an API whose only
// per-request header is X-Request-ID. The credentials themselves live
// in the TLS handshake, so this type carries no secret — it exists
// because the header is mandatory on fifteen of the seventeen
// operations and forgetting it turns every call into a 400.
//
// The generated id stays stable across the SDK's internal retries:
// SetAuth runs once per [monobank.Client.Do], not once per attempt,
// so a retried request reaches the bank under the id it was first
// sent with.
type RequestIDAuth struct {
	// NewRequestID overrides the generator. The zero value uses a
	// random UUIDv4, which is what the spec's format asks for.
	NewRequestID func() string
}

// SetAuth adds X-Request-ID to the outgoing request unless the caller
// already set one (directly, or through [ContextWithRequestID]). It
// also sets Accept: application/json, because the standard library
// sends no Accept header at all and the bank is free to content
// negotiate its way to something else.
func (a RequestIDAuth) SetAuth(r *http.Request) error {
	if r == nil {
		return nil
	}
	if r.Header.Get(HeaderRequestID) == "" {
		id := requestIDFromContext(r.Context())
		if id == "" {
			id = a.newID()
		}
		r.Header.Set(HeaderRequestID, id)
	}
	if r.Header.Get("Accept") == "" {
		r.Header.Set("Accept", "application/json")
	}

	return nil
}

// newID returns the configured generator's value, or a UUIDv4.
func (a RequestIDAuth) newID() string {
	if a.NewRequestID != nil {
		return a.NewRequestID()
	}

	return newUUIDv4()
}

// LogValue keeps slog output terse and stable.
func (a RequestIDAuth) LogValue() slog.Value {
	return slog.StringValue("openbanking.RequestIDAuth{}")
}

// newUUIDv4 formats 16 random bytes as a version-4 UUID. The SDK
// carries no UUID dependency, and this is the only place that needs
// one.
func newUUIDv4() string {
	var b [16]byte
	// crypto/rand.Read fills b entirely or crashes the process; it
	// never reports a partial read, so there is no error to handle.
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// badPathSegment reports whether s cannot stand as one URL path
// segment. Beyond the empty string it rejects "." and "..": those are
// the two values url.PathEscape leaves untouched (it escapes "/" but
// not a dot), and the path is later resolved against the base URL,
// where Go collapses dot-segments. A bare ".." would therefore walk
// the request one level up and send it to a different resource than
// the caller named — for example DELETE /ext/v2/consents/ instead of
// DELETE /ext/v2/consents/account-access/{id}. Identifiers regularly
// come from an application's own end users, so this is checked rather
// than assumed.
func badPathSegment(s string) bool {
	return s == "" || s == "." || s == ".."
}

// MTLSConfig describes the QWAC the TPP presents to the bank.
type MTLSConfig struct {
	// CertPEM is the PEM-encoded client certificate chain. Include
	// the intermediates: the bank validates the chain up to the
	// qualified trust-service provider that issued the QWAC.
	CertPEM []byte
	// KeyPEM is the PEM-encoded private key of CertPEM.
	KeyPEM []byte
	// RootCAsPEM optionally replaces the system trust store used to
	// verify the *bank's* certificate. Leave it nil in production;
	// it exists for staging, where the host may be served by a
	// private CA.
	RootCAsPEM []byte
	// Timeout is the per-request timeout of the returned client.
	// Zero selects [DefaultTimeout].
	Timeout time.Duration
}

// LogValue implements [slog.LogValuer] so that logging the config
// cannot spill the QWAC private key. Sizes and presence are enough to
// debug a misconfigured path; the key material never is.
func (c MTLSConfig) LogValue() slog.Value {
	key := "not set"
	if len(c.KeyPEM) > 0 {
		key = "***"
	}
	return slog.GroupValue(
		slog.Int("certPEMBytes", len(c.CertPEM)),
		slog.String("keyPEM", key),
		slog.Bool("customRootCAs", len(c.RootCAsPEM) > 0),
		slog.Duration("timeout", c.Timeout),
	)
}

// DefaultTimeout is the request timeout [NewMTLSHTTPClient] applies
// when [MTLSConfig.Timeout] is zero. It is deliberately finite: an
// http.Client with no timeout can hang a worker forever on a stalled
// TLS handshake, which is exactly how a revoked QWAC tends to fail.
const DefaultTimeout = 30 * time.Second

// NewMTLSHTTPClient builds the *http.Client this API requires: one
// that presents the QWAC client certificate during the TLS
// handshake. Pass the result to [monobank.WithHTTPClient].
//
// Authentication is the handshake — there is no token and no
// Authorization header. A missing, expired or revoked certificate
// makes the bank tear the connection down before any HTTP is
// exchanged, so the error you get back is a *url.Error wrapping a TLS
// alert, not a [monobank.APIError] with a 401.
func NewMTLSHTTPClient(cfg MTLSConfig) (*http.Client, error) {
	if len(cfg.CertPEM) == 0 || len(cfg.KeyPEM) == 0 {
		return nil, ErrNoClientCert
	}
	cert, err := tls.X509KeyPair(cfg.CertPEM, cfg.KeyPEM)
	if err != nil {
		return nil, fmt.Errorf("openbanking: load client certificate: %w", err)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		// The bank negotiates TLS 1.2 or better; pinning the floor
		// here keeps a caller's older default from weakening it.
		MinVersion: tls.VersionTLS12,
	}
	if len(cfg.RootCAsPEM) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cfg.RootCAsPEM) {
			return nil, ErrInvalidCA
		}
		tlsCfg.RootCAs = pool
	}

	// Clone the standard transport so the connection pooling, proxy
	// and HTTP/2 settings match what the rest of the SDK gets.
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("openbanking: http.DefaultTransport is not *http.Transport")
	}
	tr := transport.Clone()
	tr.TLSClientConfig = tlsCfg

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	return &http.Client{Transport: tr, Timeout: timeout}, nil
}

// NewMTLSHTTPClientFromFiles is [NewMTLSHTTPClient] reading the PEMs
// off disk. caFile may be empty, in which case the system trust store
// verifies the bank's certificate.
func NewMTLSHTTPClientFromFiles(certFile, keyFile, caFile string) (*http.Client, error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("openbanking: read certificate: %w", err)
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("openbanking: read key: %w", err)
	}
	cfg := MTLSConfig{CertPEM: certPEM, KeyPEM: keyPEM}
	if caFile != "" {
		caPEM, caErr := os.ReadFile(caFile)
		if caErr != nil {
			return nil, fmt.Errorf("openbanking: read CA bundle: %w", caErr)
		}
		cfg.RootCAsPEM = caPEM
	}

	return NewMTLSHTTPClient(cfg)
}
