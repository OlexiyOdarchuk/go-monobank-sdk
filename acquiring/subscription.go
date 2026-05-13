package acquiring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// SubscriptionStatus — стан регулярного платежу.
type SubscriptionStatus string

// Possible SubscriptionStatus values.
const (
	SubscriptionActive    SubscriptionStatus = "active"
	SubscriptionCancelled SubscriptionStatus = "cancelled"
)

// SubscriptionAction — операція над регулярним платежем.
// На момент написання API підтримує лише "cancel".
type SubscriptionAction string

// Possible SubscriptionAction values.
const (
	SubscriptionCancel SubscriptionAction = "cancel"
)

// SubscriptionPaymentStatus — статус одного списання за підпискою.
type SubscriptionPaymentStatus string

// Possible SubscriptionPaymentStatus values.
const (
	SubscriptionPaymentNew     SubscriptionPaymentStatus = "new"
	SubscriptionPaymentSuccess SubscriptionPaymentStatus = "success"
	SubscriptionPaymentFailed  SubscriptionPaymentStatus = "failed"
)

// SubscriptionWebHookURLs — пара колбеків для подій підписки.
//
//	ChargeUrl — на нього шлється стан кожного списання (success/failure).
//	StatusUrl — на нього шлеться зміна стану самої підписки (active/cancelled).
type SubscriptionWebHookURLs struct {
	ChargeURL string `json:"chargeUrl,omitempty"`
	StatusURL string `json:"statusUrl,omitempty"`
}

// SubscriptionCreateRequest — тіло POST /api/merchant/subscription/create.
// Interval — рядок виду "{N}{одиниця}": "1d", "2w", "1m", "1y".
type SubscriptionCreateRequest struct {
	Amount      int64                    `json:"amount"`
	Ccy         int                      `json:"ccy,omitempty"`
	RedirectURL string                   `json:"redirectUrl,omitempty"`
	WebHookURLs *SubscriptionWebHookURLs `json:"webHookUrls,omitempty"`
	Interval    string                   `json:"interval"`
	Validity    int64                    `json:"validity,omitempty"`
}

// SubscriptionCreateResponse — відповідь /subscription/create.
type SubscriptionCreateResponse struct {
	SubscriptionID string `json:"subscriptionId"`
	PageURL        string `json:"pageUrl"`
}

// SubscriptionEditRequest — тіло POST /api/merchant/subscription/edit.
// RefundAmount передавай тільки якщо хочеш повернути кошти разом зі
// скасуванням; інакше відбувається лише cancel.
type SubscriptionEditRequest struct {
	SubscriptionID string             `json:"subscriptionId"`
	Action         SubscriptionAction `json:"action"`
	RefundAmount   int64              `json:"refundAmount,omitempty"`
}

// SubscriptionRemoveRequest — тіло POST /api/merchant/subscription/remove.
type SubscriptionRemoveRequest struct {
	SubscriptionID string `json:"subscriptionId"`
}

// SubscriptionSummary — лічильники успіхів/невдач за весь час життя
// підписки.
type SubscriptionSummary struct {
	TotalPaid   int `json:"totalPaid"`
	TotalFailed int `json:"totalFailed"`
}

// SubscriptionWalletData — токенізована картка, на яку прив'язана
// підписка.
type SubscriptionWalletData struct {
	CardToken          string `json:"cardToken"`
	WalletID           string `json:"walletId"`
	Status             string `json:"status"` // new | created | failed
	FailureDescription string `json:"failureDescription,omitempty"`
}

// SubscriptionStatusResponse — відповідь GET /api/merchant/subscription/status.
type SubscriptionStatusResponse struct {
	SubscriptionID   string                 `json:"subscriptionId"`
	Status           SubscriptionStatus     `json:"status"`
	StartDate        string                 `json:"startDate"`
	EndDate          string                 `json:"endDate,omitempty"`
	Amount           int64                  `json:"amount"`
	Ccy              int                    `json:"ccy"`
	Interval         string                 `json:"interval"`
	NextChargeDate   string                 `json:"nextChargeDate,omitempty"`
	CancellationDesc string                 `json:"cancellationDesc,omitempty"`
	Summary          SubscriptionSummary    `json:"summary"`
	WalletData       SubscriptionWalletData `json:"walletData"`
}

// Pagination — стандартна сторінка-обгортка у списках підписок та платежів.
type Pagination struct {
	TotalItems   int `json:"totalItems"`
	ItemsPerPage int `json:"itemsPerPage"`
	CurrentPage  int `json:"currentPage"`
	TotalPages   int `json:"totalPages"`
}

// SubscriptionPayment — одне списання за підпискою (елемент із /payments).
type SubscriptionPayment struct {
	Amount    int64                     `json:"amount"`
	Ccy       int                       `json:"ccy"`
	Status    SubscriptionPaymentStatus `json:"status"`
	ChargedAt string                    `json:"chargedAt"`
}

// SubscriptionPaymentsResponse — відповідь GET /api/merchant/subscription/payments.
type SubscriptionPaymentsResponse struct {
	Payments   []SubscriptionPayment `json:"payments"`
	Pagination Pagination            `json:"pagination"`
}

// SubscriptionListItem — один запис у списку підписок.
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

// SubscriptionsListResponse — відповідь GET /api/merchant/subscription/list.
type SubscriptionsListResponse struct {
	List       []SubscriptionListItem `json:"list,omitempty"`
	Pagination Pagination             `json:"pagination"`
}

// SubscriptionListOptions — фільтри/сторінкування для [Client.SubscriptionList].
// DateFrom обов'язковий (передавай ненульовий час, інакше API поверне 400).
type SubscriptionListOptions struct {
	Status   SubscriptionStatus
	Limit    int
	Page     int
	DateFrom time.Time
	DateTo   time.Time
}

// SubscriptionPaymentsOptions — фільтри/сторінкування для [Client.SubscriptionPayments].
// SubscriptionID та DateFrom обов'язкові.
type SubscriptionPaymentsOptions struct {
	SubscriptionID string
	Limit          int
	Page           int
	DateFrom       time.Time
	DateTo         time.Time
}

// SubscriptionCreate створює регулярний платіж. Користувач має оплатити
// перший списанням на PageURL; далі Mono самостійно списує Amount кожен
// Interval (наприклад "1m") за токенізованою карткою. Слухай вебхуки на
// WebHookURLs.ChargeURL/StatusURL, щоб дізнаватись про події.
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

// SubscriptionEdit керує існуючою підпискою. Поки що єдина дія —
// [SubscriptionCancel]. Передай RefundAmount > 0, щоб одночасно скасувати
// та повернути кошти за останнє списання.
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

// SubscriptionRemove інвалідує підписку. Це жорсткий варіант відключення
// (без refund). Для скасування з поверненням використовуй
// [Client.SubscriptionEdit] з [SubscriptionCancel] + RefundAmount.
func (c *Client) SubscriptionRemove(ctx context.Context, subscriptionID string) error {
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

// SubscriptionStatus повертає поточний стан підписки: дати, лічильники
// списань, прив'язану картку. Використовуй для звіряння з власною БД.
func (c *Client) SubscriptionStatus(ctx context.Context, subscriptionID string) (*SubscriptionStatusResponse, error) {
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

// SubscriptionList повертає сторінку підписок мерчанта за вказаний період.
// opts.DateFrom обов'язковий. Limit/Page керують сторінкуванням
// (дефолти на стороні API: limit=20, page=1).
func (c *Client) SubscriptionList(ctx context.Context, opts SubscriptionListOptions) (*SubscriptionsListResponse, error) {
	if opts.DateFrom.IsZero() {
		return nil, fmt.Errorf("SubscriptionList: DateFrom is required")
	}
	q := url.Values{}
	q.Set("dateFrom", opts.DateFrom.Format(time.RFC3339))
	if !opts.DateTo.IsZero() {
		q.Set("dateTo", opts.DateTo.Format(time.RFC3339))
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

// SubscriptionPayments повертає сторінку списань за конкретною підпискою.
// opts.SubscriptionID та opts.DateFrom обов'язкові.
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
