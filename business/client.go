// Package business is the Go client for corp-api.monobank.ua, the
// "API for legal-entity accounts". It is a different API from the
// Corporate Open API (sub-package [corporate]): different host,
// simpler authorization (a single X-Token), different surface (the
// company's own accounts/payments rather than delegated client data).
//
// Authorization is a single X-Token header issued from the web
// cabinet at https://web.monobank.ua/?modal=tokens. No ECDSA
// signatures.
//
// The 23 endpoints group into six topics:
//
//   - Accounts: [Client.Accounts], [Client.Account],
//     [Client.AccountBalances]
//   - Statement: [Client.Statement] (paginated), [Client.Operation]
//     (a single operation)
//   - Payments: [Client.PreparePayment], [Client.PaymentState],
//     [Client.PaymentStateByReference]
//   - Payroll contacts: [Client.Contacts], [Client.SearchContacts],
//     [Client.ContactByID], [Client.CreateContact],
//     [Client.DeleteContact], [Client.DeleteContactsBatch]
//   - Salary registries: [Client.CreateSalaryRegistry],
//     [Client.SalaryRegistryTypes], [Client.SalaryRegistryStatus]
//   - Payslips: [Client.UploadPayslips], [Client.DeletePayslips],
//     [Client.ImportStatus], [Client.DeleteImport],
//     [Client.SendPayslipsToMobile], [Client.PayslipPDF]
//
// Idempotency: mutating endpoints with Idempotency-Key
// (PreparePayment, CreateSalaryRegistry) expect a fresh UUID v4 for
// each logical attempt — repeating with the same key is safe.
//
// Rate limits: corp-api tracks quotas per company. The current
// remaining count is in the X-Rate-Limit-Remaining header; 429
// responses carry X-Rate-Limit-Retry-After-Seconds.
package business

import (
	"log/slog"
	"net/http"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

// BaseURL is the default corp-api.monobank.ua host. Override via
// [monobank.WithBaseURL] when creating a client for tests.
const BaseURL = "https://corp-api.monobank.ua"

// Client talks to corp-api.monobank.ua. It is a thin wrapper around
// [monobank.Client] — retry, transport, and error decoding do not
// need to be reimplemented; this package only adds the domain types
// and methods.
type Client struct {
	c monobank.Client
}

// New returns a [Client] authorized with the given X-Token. Extra
// options (HTTP client, retry policy etc.) are forwarded to
// [monobank.New].
//
//	cli := business.New(os.Getenv("MONO_BUSINESS_TOKEN"))
//	accs, err := cli.Accounts(ctx)
func New(token string, opts ...monobank.Option) *Client {
	base := []monobank.Option{
		monobank.WithBaseURL(BaseURL),
		monobank.WithAuth(TokenAuth{Token: token}),
	}
	return &Client{c: monobank.New(append(base, opts...)...)}
}

// Close releases the client's background resources (see
// [monobank.Client.Close]).
func (c *Client) Close() error { return c.c.Close() }

// TokenAuth implements [auth.Authorizer] for corp-api X-Token
// authorization. Beyond X-Token, it sets the mandatory
// `Accept: application/json` header that corp-api expects on every
// request.
type TokenAuth struct {
	Token string
}

// SetAuth adds X-Token and Accept to the request. A nil request is a
// no-op.
func (a TokenAuth) SetAuth(r *http.Request) error {
	if r == nil {
		return nil
	}
	r.Header.Set("X-Token", a.Token)
	if r.Header.Get("Accept") == "" {
		r.Header.Set("Accept", "application/json")
	}
	return nil
}

// LogValue hides the token in slog output.
func (a TokenAuth) LogValue() slog.Value {
	return slog.StringValue("business.TokenAuth{Token:***}")
}
