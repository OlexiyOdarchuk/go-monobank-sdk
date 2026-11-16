package acquiring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/money"
)

// SubscriptionStatus is the state of a recurring payment.
type SubscriptionStatus string

// Possible SubscriptionStatus values.
const (
	SubscriptionActive    SubscriptionStatus = "active"
	SubscriptionCancelled SubscriptionStatus = "cancelled"
)

// SubscriptionAction is an operation on a recurring payment. At the
// time of writing, the API supports only "cancel".
type SubscriptionAction string

// Possible SubscriptionAction values.
const (
	SubscriptionCancel SubscriptionAction = "cancel"
)

// SubscriptionPaymentStatus is the status of a single subscription
// charge.
type SubscriptionPaymentStatus string

// Possible SubscriptionPaymentStatus values.
const (
	SubscriptionPaymentNew     SubscriptionPaymentStatus = "new"
	SubscriptionPaymentSuccess SubscriptionPaymentStatus = "success"
	SubscriptionPaymentFailed  SubscriptionPaymentStatus = "failed"
)

// SubscriptionWebHookURLs is a pair of callbacks for subscription
// events.
//
//	ChargeUrl is notified about each charge, successful or failed.
//	StatusUrl is notified about the subscription's status.
//
// Mono documents neither the body schema nor a signature (x-sign) for
// these callbacks, so the SDK ships no type for them and cannot
// authenticate them. Treat a callback only as a trigger to read the
// current state with [Client.SubscriptionStatus] and
// [Client.SubscriptionPayments].
type SubscriptionWebHookURLs struct {
	ChargeURL string `json:"chargeUrl,omitempty"`
	StatusURL string `json:"statusUrl,omitempty"`
}

// SubscriptionCreateRequest is the body of
// POST /api/merchant/subscription/create.
//
// Interval is a string in the form "{N}{unit}" where unit is one
// of d (day), w (week), m (month), y (year). Mono's documented
// examples are exactly:
//
//	"1d"  — daily
//	"2w"  — every two weeks
//	"1m"  — monthly
//	"1y"  — yearly
//
// The docs give only these examples and no further rules (whether N
// may be 0 or how large it may be is not stated). The SDK does not
// pre-validate the format — stick to the documented shape.
type SubscriptionCreateRequest struct {
	Amount      int64                    `json:"amount"`
	Currency    currency.Code            `json:"ccy,omitempty"`
	RedirectURL string                   `json:"redirectUrl,omitempty"`
	WebHookURLs *SubscriptionWebHookURLs `json:"webHookUrls,omitempty"`
	Interval    string                   `json:"interval"`
	Validity    int64                    `json:"validity,omitempty"`
}

// SubscriptionCreateResponse is the response of /subscription/create.
type SubscriptionCreateResponse struct {
	SubscriptionID string `json:"subscriptionId"`
	PageURL        string `json:"pageUrl"`
}

// SubscriptionEditRequest is the body of
// POST /api/merchant/subscription/edit. RefundAmount, when set, refunds
// that amount along with the cancellation; when omitted, nothing is
// refunded and only the cancel is performed.
type SubscriptionEditRequest struct {
	SubscriptionID string             `json:"subscriptionId"`
	Action         SubscriptionAction `json:"action"`
	RefundAmount   int64              `json:"refundAmount,omitempty"`
}

// SubscriptionRemoveRequest is the body of
// POST /api/merchant/subscription/remove.
type SubscriptionRemoveRequest struct {
	SubscriptionID string `json:"subscriptionId"`
}

// SubscriptionSummary is the success/failure counter over the
// lifetime of a subscription.
type SubscriptionSummary struct {
	TotalPaid   int `json:"totalPaid"`
	TotalFailed int `json:"totalFailed"`
}

// SubscriptionWalletData is the tokenized card a subscription is
// bound to.
type SubscriptionWalletData struct {
	CardToken          string       `json:"cardToken"`
	WalletID           string       `json:"walletId"`
	Status             WalletStatus `json:"status"`
	FailureDescription string       `json:"failureDescription,omitempty"`
}

// SubscriptionStatusResponse is the response of
// GET /api/merchant/subscription/status.
//
// The API exposes no "paid until" date. NextChargeDate is documented
// only as the date of the next charge; read right after a successful
// charge, it marks where the period just paid for ends. Store it at
// that point: after a failed charge or a cancellation it no longer
// tells you that.
type SubscriptionStatusResponse struct {
	SubscriptionID string             `json:"subscriptionId"`
	Status         SubscriptionStatus `json:"status"`
	// StartDate is the date of the first charge.
	StartDate string `json:"startDate"`
	// EndDate is described inconsistently: the status endpoint calls
	// it the date the subscription was cancelled, the list endpoint
	// "the end date of the subscription", and both samples set it on an
	// active subscription. Do not use it for access control.
	EndDate  string        `json:"endDate,omitempty"`
	Amount   int64         `json:"amount"`
	Currency currency.Code `json:"ccy"`
	Interval string        `json:"interval"`
	// NextChargeDate is the date of the next scheduled charge.
	NextChargeDate   string                 `json:"nextChargeDate,omitempty"`
	CancellationDesc string                 `json:"cancellationDesc,omitempty"`
	Summary          SubscriptionSummary    `json:"summary"`
	WalletData       SubscriptionWalletData `json:"walletData"`
}

// AmountMoney returns Amount as a typed [money.Money] with Currency
// attached — consistent with the money.Money used elsewhere in the
// SDK, since the wire ships subscription amounts as a bare minor-unit
// int64 alongside a separate ccy.
func (s SubscriptionStatusResponse) AmountMoney() money.Money {
	return money.New(s.Amount, s.Currency)
}

// Pagination is the standard page wrapper for subscription and
// payment lists.
type Pagination struct {
	TotalItems   int `json:"totalItems"`
	ItemsPerPage int `json:"itemsPerPage"`
	CurrentPage  int `json:"currentPage"`
	TotalPages   int `json:"totalPages"`
}

// SubscriptionPayment is a single subscription charge (an element
// of /payments).
//
// The API returns no per-charge identifier. Use (subscription ID,
// ChargedAt) as the idempotency key, and update Status under that key:
// a charge reported as "new" may later become "success" or "failed".
type SubscriptionPayment struct {
	Amount    int64                     `json:"amount"`
	Currency  currency.Code             `json:"ccy"`
	Status    SubscriptionPaymentStatus `json:"status"`
	ChargedAt string                    `json:"chargedAt"`
}

// AmountMoney returns Amount as a typed [money.Money] with Currency
// attached.
func (p SubscriptionPayment) AmountMoney() money.Money {
	return money.New(p.Amount, p.Currency)
}

// SubscriptionPaymentsResponse is the response of
// GET /api/merchant/subscription/payments.
type SubscriptionPaymentsResponse struct {
	Payments   []SubscriptionPayment `json:"payments"`
	Pagination Pagination            `json:"pagination"`
}

// SubscriptionListItem is a single entry in a subscription list.
// Dates are RFC 3339 date-times.
type SubscriptionListItem struct {
	SubscriptionID string `json:"subscriptionId"`
	Amount         int64  `json:"amount"`
	Interval       string `json:"interval"`
	// StartDate is documented here as the subscription's start date
	// (on the status endpoint: the date of the first charge).
	StartDate string `json:"startDate"`
	// Created is when the subscription was created.
	Created string `json:"created"`
	// NextChargeDate is the next charge date.
	NextChargeDate string `json:"nextChargeDate,omitempty"`
	// EndDate: see [SubscriptionStatusResponse.EndDate].
	EndDate string             `json:"endDate,omitempty"`
	Status  SubscriptionStatus `json:"status"`
}

// AmountMoney returns Amount as a typed [money.Money]. The list
// endpoint does not ship a per-row currency, so Code is left unset
// (zero); the amount's Minor value is still exact. Fetch the
// subscription's status for the currency-qualified amount.
func (i SubscriptionListItem) AmountMoney() money.Money {
	return money.New(i.Amount, 0)
}

// SubscriptionsListResponse is the response of
// GET /api/merchant/subscription/list.
type SubscriptionsListResponse struct {
	List       []SubscriptionListItem `json:"list,omitempty"`
	Pagination Pagination             `json:"pagination"`
}

// SubscriptionListOptions holds filters / pagination for
// [Client.SubscriptionList]. DateFrom is required (pass a non-zero
// time, otherwise the API returns 400).
type SubscriptionListOptions struct {
	Status   SubscriptionStatus
	Limit    int
	Page     int
	DateFrom time.Time
	DateTo   time.Time
}

// SubscriptionPaymentsOptions holds filters / pagination for
// [Client.SubscriptionPayments]. SubscriptionID and DateFrom are
// required.
type SubscriptionPaymentsOptions struct {
	SubscriptionID string
	Limit          int
	Page           int
	DateFrom       time.Time
	DateTo         time.Time
}

// SubscriptionCreate creates a recurring payment. The user must pay
// the first charge on PageURL; afterwards Mono charges Amount every
// Interval (for example "1m") against the tokenized card on its own.
// WebHookURLs.ChargeURL/StatusURL are notified about events; see
// [SubscriptionWebHookURLs] for how to handle them.
func (c *Client) SubscriptionCreate(ctx context.Context, in *SubscriptionCreateRequest) (*SubscriptionCreateResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/subscription/create", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out SubscriptionCreateResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// SubscriptionEdit manages an existing subscription. The only
// action currently supported is [SubscriptionCancel], which stops
// further charges. Already completed charges stay paid unless
// RefundAmount > 0 is passed, in which case that amount is refunded
// along with the cancellation.
func (c *Client) SubscriptionEdit(ctx context.Context, in *SubscriptionEditRequest) error {
	if in == nil {
		return ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/subscription/edit", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}

// SubscriptionRemove invalidates a subscription that has not been
// charged yet (for example, one whose first payment was never made).
// To stop a subscription that already has charges, use
// [Client.SubscriptionEdit] with [SubscriptionCancel], optionally with
// RefundAmount.
func (c *Client) SubscriptionRemove(ctx context.Context, subscriptionID string) error {
	if subscriptionID == "" {
		return ErrEmptyID
	}
	body, err := json.Marshal(SubscriptionRemoveRequest{SubscriptionID: subscriptionID})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/subscription/remove", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}

// SubscriptionStatus returns the subscription's current state:
// dates, charge counters, the bound card. Use for reconciliation
// against your own DB.
func (c *Client) SubscriptionStatus(ctx context.Context, subscriptionID string) (*SubscriptionStatusResponse, error) {
	if subscriptionID == "" {
		return nil, ErrEmptyID
	}
	q := url.Values{}
	q.Set("subscriptionId", subscriptionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/merchant/subscription/status?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out SubscriptionStatusResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// SubscriptionList returns a page of the merchant's subscriptions
// for the given period. opts.DateFrom is required. Limit/Page drive
// pagination (API defaults: limit=20, page=1).
func (c *Client) SubscriptionList(ctx context.Context, opts SubscriptionListOptions) (*SubscriptionsListResponse, error) {
	if opts.DateFrom.IsZero() {
		return nil, fmt.Errorf("SubscriptionList: DateFrom is required")
	}
	q := url.Values{}
	// The docs ask for RFC 3339 and their example carries an offset
	// (2024-06-26T18:12:44+03:00). Sending UTC is the same instant in
	// the same format; normalise so the query does not depend on the
	// caller's location.
	q.Set("dateFrom", opts.DateFrom.UTC().Format(time.RFC3339))
	if !opts.DateTo.IsZero() {
		q.Set("dateTo", opts.DateTo.UTC().Format(time.RFC3339))
	}
	if opts.Status != "" {
		q.Set("status", string(opts.Status))
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Page > 0 {
		q.Set("page", strconv.Itoa(opts.Page))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/merchant/subscription/list?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out SubscriptionsListResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// SubscriptionPayments returns a page of charges for a specific
// subscription. opts.SubscriptionID and opts.DateFrom are required.
func (c *Client) SubscriptionPayments(ctx context.Context, opts SubscriptionPaymentsOptions) (*SubscriptionPaymentsResponse, error) {
	if opts.SubscriptionID == "" {
		return nil, fmt.Errorf("SubscriptionPayments: SubscriptionID is required")
	}
	if opts.DateFrom.IsZero() {
		return nil, fmt.Errorf("SubscriptionPayments: DateFrom is required")
	}
	q := url.Values{}
	q.Set("subscriptionId", opts.SubscriptionID)
	// Same UTC normalisation as SubscriptionList.
	q.Set("dateFrom", opts.DateFrom.UTC().Format(time.RFC3339))
	if !opts.DateTo.IsZero() {
		q.Set("dateTo", opts.DateTo.UTC().Format(time.RFC3339))
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Page > 0 {
		q.Set("page", strconv.Itoa(opts.Page))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/merchant/subscription/payments?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out SubscriptionPaymentsResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// SubscriptionListAll lazily iterates every page of
// [Client.SubscriptionList]. Useful for a full pass without
// managing Page by hand.
//
//	opts := acquiring.SubscriptionListOptions{DateFrom: from, Limit: 50}
//	for s, err := range cli.SubscriptionListAll(ctx, opts) {
//	    if err != nil { return err }
//	    process(s)
//	}
func (c *Client) SubscriptionListAll(ctx context.Context, opts SubscriptionListOptions) iter.Seq2[SubscriptionListItem, error] {
	return func(yield func(SubscriptionListItem, error) bool) {
		page := opts.Page
		if page < 1 {
			page = 1
		}
		for {
			if err := ctx.Err(); err != nil {
				_ = yield(SubscriptionListItem{}, err)
				return
			}
			cur := opts
			cur.Page = page
			resp, err := c.SubscriptionList(ctx, cur)
			if err != nil {
				_ = yield(SubscriptionListItem{}, err)
				return
			}
			for _, s := range resp.List {
				if !yield(s, nil) {
					return
				}
			}
			if len(resp.List) == 0 || page >= resp.Pagination.TotalPages {
				return
			}
			page++
		}
	}
}

// SubscriptionPaymentsAll lazily iterates every page of
// [Client.SubscriptionPayments] for a specific subscription.
func (c *Client) SubscriptionPaymentsAll(ctx context.Context, opts SubscriptionPaymentsOptions) iter.Seq2[SubscriptionPayment, error] {
	return func(yield func(SubscriptionPayment, error) bool) {
		page := opts.Page
		if page < 1 {
			page = 1
		}
		for {
			if err := ctx.Err(); err != nil {
				_ = yield(SubscriptionPayment{}, err)
				return
			}
			cur := opts
			cur.Page = page
			resp, err := c.SubscriptionPayments(ctx, cur)
			if err != nil {
				_ = yield(SubscriptionPayment{}, err)
				return
			}
			for _, p := range resp.Payments {
				if !yield(p, nil) {
					return
				}
			}
			if len(resp.Payments) == 0 || page >= resp.Pagination.TotalPages {
				return
			}
			page++
		}
	}
}
