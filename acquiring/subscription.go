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

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
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
//	ChargeUrl receives the state of each charge (success/failure).
//	StatusUrl receives state changes of the subscription itself
//	(active/cancelled).
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
// N must be a positive integer; "0d", "-1m" and similar are
// rejected by the bank with a 400. The SDK does not pre-validate
// the format — pass exactly what the bank's documentation shows.
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
// POST /api/merchant/subscription/edit. Set RefundAmount only if you
// want to refund along with cancellation; otherwise only a cancel
// is performed.
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
type SubscriptionStatusResponse struct {
	SubscriptionID   string                 `json:"subscriptionId"`
	Status           SubscriptionStatus     `json:"status"`
	StartDate        string                 `json:"startDate"`
	EndDate          string                 `json:"endDate,omitempty"`
	Amount           int64                  `json:"amount"`
	Currency         currency.Code          `json:"ccy"`
	Interval         string                 `json:"interval"`
	NextChargeDate   string                 `json:"nextChargeDate,omitempty"`
	CancellationDesc string                 `json:"cancellationDesc,omitempty"`
	Summary          SubscriptionSummary    `json:"summary"`
	WalletData       SubscriptionWalletData `json:"walletData"`
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
type SubscriptionPayment struct {
	Amount    int64                     `json:"amount"`
	Currency  currency.Code             `json:"ccy"`
	Status    SubscriptionPaymentStatus `json:"status"`
	ChargedAt string                    `json:"chargedAt"`
}

// SubscriptionPaymentsResponse is the response of
// GET /api/merchant/subscription/payments.
type SubscriptionPaymentsResponse struct {
	Payments   []SubscriptionPayment `json:"payments"`
	Pagination Pagination            `json:"pagination"`
}

// SubscriptionListItem is a single entry in a subscription list.
type SubscriptionListItem struct {
	SubscriptionID string             `json:"subscriptionId"`
	Amount         int64              `json:"amount"`
	Interval       string             `json:"interval"`
	StartDate      string             `json:"startDate"`
	Created        string             `json:"created"`
	NextChargeDate string             `json:"nextChargeDate,omitempty"`
	EndDate        string             `json:"endDate,omitempty"`
	Status         SubscriptionStatus `json:"status"`
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
// Listen on WebHookURLs.ChargeURL/StatusURL for events.
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
// action currently supported is [SubscriptionCancel]. Pass
// RefundAmount > 0 to cancel and refund the last charge at the same
// time.
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

// SubscriptionRemove invalidates a subscription. This is the hard
// off-switch (no refund). To cancel with a refund use
// [Client.SubscriptionEdit] with [SubscriptionCancel] + RefundAmount.
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
	// Mono expects UTC timestamps; t.Format(time.RFC3339) on a local
	// time.Time emits the local TZ offset, which the bank rejects on
	// some endpoints. Normalise to UTC explicitly.
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
	q.Set("dateFrom", opts.DateFrom.Format(time.RFC3339))
	if !opts.DateTo.IsZero() {
		q.Set("dateTo", opts.DateTo.Format(time.RFC3339))
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
