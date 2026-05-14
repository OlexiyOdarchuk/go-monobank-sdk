// Package acquiring is the Go client for monobank's acquiring API
// (api.monobank.ua/api/merchant/*). It covers:
//
//   - invoices, holds, QR cash desks, tokenized cards (wallet);
//   - recurring payments (subscriptions: create/edit/remove/
//     status/list/payments);
//   - the monopay button: importing/removing/listing merchant keys;
//   - payment splitting (split receivers) and T2P terminals
//     (terminal-on-a-smartphone);
//   - periodic statements, fiscal checks, receipts, submerchants.
//
// Authorization is a single X-Token issued for a specific merchant.
// This token is NOT the Personal API token and NOT the business
// (corp-api) token — keep them separate.
//
// Webhook verification uses ECDSA-SHA256 with NIST P-256 (an ASN.1
// DER signature in the X-Sign header). The key from [Client.PubKey]
// arrives as base64(PEM(SPKI)); [ServerKey.Public] / [ParsePubKey]
// strips both wrappers and returns a ready *ecdsa.PublicKey for
// [VerifyWebhook].
//
// Typical scenarios:
//
//   - Single-step debit: [Client.CreateInvoice] with
//     PaymentType: PaymentDebit → show inv.PageURL → wait for the
//     webhook or poll [Client.InvoiceStatus].
//   - Auth-then-capture: [Client.CreateInvoice] with PaymentType:
//     PaymentHold → the client pays → status "hold" →
//     [Client.FinalizeInvoice] captures part or all of the authorized
//     amount.
//   - Recurring via tokenization: the first invoice with
//     SaveCardData.SaveCard; the success webhook brings back
//     WalletData.CardToken — subsequent charges go via
//     [Client.WalletPayment].
//   - Subscriptions (recurring payments):
//     [Client.SubscriptionCreate] → the client pays the first time →
//     the bank charges the rest automatically per interval. Listen
//     on WebHookURLs.ChargeURL / StatusURL.
//   - QR cash desks: [Client.QRList] / [Client.QRDetails] /
//     [Client.QRResetAmount] for terminal-like scenarios.
//   - Refunds: [Client.CancelInvoice] (full or partial).
//   - Reconciliation: [Client.Statement] for a period; CancelList
//     in each row carries the refund history.
//
// The direct-PAN flows ([Client.PaymentDirect], [Client.SyncPayment])
// require PCI DSS scope — the merchant must operate, or proxy
// through, a certified environment that may accept raw card data.
package acquiring

import (
	"log/slog"
	"net/http"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

// BaseURL is the acquiring API host. Override via
// [monobank.WithBaseURL] in tests.
const BaseURL = "https://api.monobank.ua"

// Client talks to api.monobank.ua/api/merchant/*. It is a wrapper
// around [monobank.Client] for HTTP plumbing (retry, transport,
// error mapping) plus typed methods and DTOs for acquiring.
type Client struct {
	c monobank.Client
}

// New returns a [Client] authorized with the given X-Token. Extra
// options (HTTP client, retry policy) are forwarded to
// [monobank.New].
//
//	cli := acquiring.New(os.Getenv("MONO_ACQUIRING_TOKEN"))
//	out, err := cli.MerchantDetails(ctx)
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

// TokenAuth implements [auth.Authorizer] for acquiring X-Token
// authorization.
type TokenAuth struct {
	Token string
}

// SetAuth adds X-Token to the outgoing request. A nil request is a
// no-op.
func (a TokenAuth) SetAuth(r *http.Request) error {
	if r == nil {
		return nil
	}
	r.Header.Set("X-Token", a.Token)
	return nil
}

// LogValue hides the token in slog output.
func (a TokenAuth) LogValue() slog.Value {
	return slog.StringValue("acquiring.TokenAuth{Token:***}")
}
