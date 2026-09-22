package acquiring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/money"
)

// Terminal is a single T2P (terminal-to-phone) terminal running on
// a merchant employee's smartphone.
type Terminal struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Terminal string `json:"terminal"`
}

// TerminalListResponse wraps the /t2p/terminal/list response.
type TerminalListResponse struct {
	List []Terminal `json:"list"`
}

// T2PPaymentType tells a T2P charge apart from its refund. The wire
// values differ from [PaymentType] (debit/hold), which describes how
// an invoice captures money rather than which way it moves.
type T2PPaymentType string

// Possible T2PPaymentType values.
const (
	T2PPurchase T2PPaymentType = "purchase"
	T2PRefund   T2PPaymentType = "refund"
)

// T2PStatus is the state of a T2P payment. It is deliberately not
// [ProcessingStatus], whose failure value is "failure".
//
// The docs contradict themselves about the failure value and the SDK
// does not paper over it. The callback table prose says the status is
// "success або failed", yet the sample payload right beside it sends
// "rejected", and the status endpoint's page also speaks of rejected
// plus an unenumerated family of "*_not_completed" values, with
// pending until the payment settles. All four constants are declared
// and any unknown value is preserved verbatim.
//
// So do not test against a single failure constant. Treat anything
// that is neither [T2PStatusSuccess] nor [T2PStatusPending] as "not
// successful", and anything you do not recognize as not yet final.
type T2PStatus string

// Possible T2PStatus values.
const (
	T2PStatusPending T2PStatus = "pending"
	T2PStatusSuccess T2PStatus = "success"
	// T2PStatusRejected is what the docs' own failure sample sends.
	T2PStatusRejected T2PStatus = "rejected"
	// T2PStatusFailed is what the callback table's prose names. No
	// sample in the docs uses it.
	T2PStatusFailed T2PStatus = "failed"
)

// T2PPaymentStatusResponse is the result of
// GET /api/merchant/t2p/terminal/payment/external/status. The docs
// state outright that the body is identical to the one the bank POSTs
// to callbackSuccess/callbackFail — that is the point of the
// endpoint, so an integrator need not run a webhook server at all.
// The field set below therefore follows the documented callback
// table; monobank publishes no machine-readable schema for either.
//
// MaskedPan, CardMask, ApprovalCode, RRN, ResponseCode and
// CountryCard are filled in only once the bank has built the
// statement entry — see [Client.T2PPaymentStatus] for the timing.
// On a failed payment most of the other fields may be absent and
// ErrorMessage carries the reason.
//
// https://monobank.ua/api-docs/acquiring/integrations/t2p-ios/get--api--merchant--t2p--terminal--payment--external--status
type T2PPaymentStatusResponse struct {
	// Bank is a constant "АТ Універсал Банк".
	Bank string `json:"bank"`
	// Terminal is the terminal id (MT%) that took the payment.
	Terminal    string         `json:"terminal"`
	PaymentType T2PPaymentType `json:"paymentType"`
	Amount      money.Money    `json:"amount"`
	// Currency is the alpha-3 code ("UAH"). This endpoint is the odd
	// one out — elsewhere in acquiring `ccy` is the ISO 4217 number,
	// hence [currency.Code]. UnmarshalJSON resolves it onto
	// Amount.Code, so prefer that for comparisons.
	//
	// Decoded with [currency.Parse] rather than [currency.FromAlpha3]:
	// Parse accepts both spellings, so a switch to the numeric form
	// would not silently drop the code.
	Currency string    `json:"ccy"`
	Status   T2PStatus `json:"status"`
	// InternalPaymentID is monobank's internal id of the payment,
	// ExternalPaymentID echoes back the `id` the integrator generated
	// when creating it, and TransactionID identifies the resulting
	// banking transaction.
	InternalPaymentID string `json:"internalPaymentId"`
	ExternalPaymentID string `json:"externalPaymentId"`
	TransactionID     string `json:"transactionId"`
	MaskedPan         string `json:"maskedPan,omitempty"`
	// CardMask is the card brand despite its name. Its casing is not
	// settled: the docs table writes "Mastercard"/"Visa" while the
	// sample response on the same page sends "visa". Compare
	// case-insensitively, and do not reach for [PaymentSystem] — the
	// values would not line up.
	CardMask     string `json:"cardMask,omitempty"`
	ApprovalCode string `json:"approvalCode,omitempty"`
	RRN          string `json:"rrn,omitempty"`
	// DataTime is the transaction timestamp. The misspelling is the
	// bank's — the wire field really is `dataTime`.
	DataTime string `json:"dataTime"`
	// ResponseCode is the authorization response code; "1" means
	// success. Typical declines: "06" PIN/3DS required, "22"
	// acquiring limit exceeded, "51" card expired, "53" wrong PIN,
	// "59" insufficient funds, "80" wrong CVV. The docs carry the
	// full table.
	ResponseCode string `json:"responseCode,omitempty"`
	// CountryCard is the card issuer's country.
	CountryCard  string `json:"countryCard,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// UnmarshalJSON attaches Currency to Amount.Code. An unrecognized
// code leaves Amount.Code zero rather than failing the whole response
// — the amount itself is still usable.
func (t *T2PPaymentStatusResponse) UnmarshalJSON(data []byte) error {
	type raw T2PPaymentStatusResponse
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*t = T2PPaymentStatusResponse(r)
	if code, ok := currency.Parse(t.Currency); ok {
		t.Amount.Code = code
	}
	return nil
}

// Terminals returns the list of T2P terminals (terminal on a phone)
// for this merchant. For a merchant without terminals the list may
// be empty, or simply absent from the response.
func (c *Client) Terminals(ctx context.Context) ([]Terminal, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/merchant/t2p/terminal/list", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out TerminalListResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.List, nil
}

// T2PPaymentStatus looks up a T2P payment by externalPaymentId — the
// id the integrator itself put in the `id` field when creating the
// payment. The bank keeps payments queryable for the last 90 days;
// beyond that, and for an id it never saw, it answers 404
// ([IsNotFound]).
//
// This is a fallback, not the main channel. The primary way to learn
// the outcome is the callbackSuccess/callbackFail webhook; poll only
// when that webhook was missed or has not arrived in time. The docs
// are explicit about the budget: no more than one request per 5
// seconds for a single externalPaymentId. When polling in a loop,
// back off exponentially (1 → 2 → 5 → 10 → 30 s) and cap the whole
// wait (two minutes is the suggested bound) — past that, treat the
// payment as undetermined and reconcile it out of band instead of
// hammering the endpoint.
//
// A success is not immediately complete: MaskedPan, CardMask,
// ApprovalCode, RRN, ResponseCode and CountryCard only appear once
// the bank has built the statement entry. The docs advise repeating
// the request 10–30 seconds after the payment reaches Status ==
// [T2PStatusSuccess] if you need those fields — treat their absence
// before then as "not ready", not as an error.
//
// See [T2PStatus] for the disagreement between the two status lists
// the docs publish.
func (c *Client) T2PPaymentStatus(ctx context.Context, externalPaymentID string) (*T2PPaymentStatusResponse, error) {
	if externalPaymentID == "" {
		return nil, ErrEmptyID
	}
	q := url.Values{}
	q.Set("externalPaymentId", externalPaymentID)
	uri := "/api/merchant/t2p/terminal/payment/external/status?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out T2PPaymentStatusResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}
