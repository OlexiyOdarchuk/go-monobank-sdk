package acquiring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// ErrNilRequest повертається з мутаційних endpoint-ів, коли передано
// nil body.
var ErrNilRequest = errors.New("request body is nil")

// CreateInvoice створює новий інвойс і повертає його id та URL для
// показу клієнту (форма оплати Mono). PaymentType визначає сценарій:
// PaymentDebit — одразу списання, PaymentHold — автентифікація з
// подальшою [Client.FinalizeInvoice].
// https://api.monobank.ua/docs/acquiring.html#tag/Merchant-account/paths/~1api~1merchant~1invoice~1create/post
func (c *Client) CreateInvoice(ctx context.Context, in *CreateInvoiceRequest) (*CreateInvoiceResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/invoice/create", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out CreateInvoiceResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// InvoiceStatus повертає поточний стан інвойсу: статус (created /
// processing / hold / success / failure / reversed / expired), деталі
// картки і платіжної системи, історію повернень. Використовуй для
// поллінгу, якщо WebHookURL не налаштований.
// https://api.monobank.ua/docs/acquiring.html#tag/Merchant-account/paths/~1api~1merchant~1invoice~1status/get
func (c *Client) InvoiceStatus(ctx context.Context, invoiceID string) (*InvoiceStatusResponse, error) {
	q := url.Values{}
	q.Set("invoiceId", invoiceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/merchant/invoice/status?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out InvoiceStatusResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelInvoice скасовує (або частково повертає) оплачений інвойс.
// Передай Amount < початкової суми для часткового повернення;
// порожній Amount — повне повернення. ExtRef — необов'язковий
// ідентифікатор у твоїй системі (потрапить у статус операції).
// https://api.monobank.ua/docs/acquiring.html#tag/Merchant-account/paths/~1api~1merchant~1invoice~1cancel/post
func (c *Client) CancelInvoice(ctx context.Context, in *CancelRequest) (*CancelResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/invoice/cancel", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out CancelResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// FinalizeInvoice фіналізує (списує) hold-інвойс — захоплює раніше
// заавторизовану суму. Можна передати Amount менше за початкову — тоді
// з картки знімається тільки вказана сума, решта заавторизованих коштів
// розблоковується (часткова фіналізація).
// https://api.monobank.ua/docs/acquiring.html#tag/Holds/paths/~1api~1merchant~1invoice~1finalize/post
func (c *Client) FinalizeInvoice(ctx context.Context, in *FinalizeRequest) (*FinalizeResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/invoice/finalize", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out FinalizeResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveInvoice інвалідовує інвойс, який ще не оплачений. Після цього
// сторінка оплати перестає працювати. Для скасування ВЖЕ оплаченого
// інвойсу використовуй [Client.CancelInvoice] (повернення коштів).
// https://api.monobank.ua/docs/acquiring.html#tag/Merchant-account/paths/~1api~1merchant~1invoice~1remove/post
func (c *Client) RemoveInvoice(ctx context.Context, invoiceID string) error {
	body, err := json.Marshal(RemoveRequest{InvoiceID: invoiceID})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/invoice/remove", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}

// FiscalChecks повертає фіскальні чеки, прив'язані до інвойсу
// (через Checkbox або Monopay). Один інвойс може мати кілька чеків:
// продаж (sale) і повернення (return).
// https://api.monobank.ua/docs/acquiring.html#tag/Merchant-account/paths/~1api~1merchant~1invoice~1fiscal-checks/get
func (c *Client) FiscalChecks(ctx context.Context, invoiceID string) ([]FiscalCheck, error) {
	q := url.Values{}
	if invoiceID != "" {
		q.Set("invoiceId", invoiceID)
	}
	uri := "/api/merchant/invoice/fiscal-checks"
	if s := q.Encode(); s != "" {
		uri += "?" + s
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out FiscalChecksResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.Checks, nil
}

// Receipt повертає base64-кодовану PDF-квитанцію для інвойсу. Якщо
// email непорожній — банк ще й надсилає копію поштою.
// https://api.monobank.ua/docs/acquiring.html#tag/Merchant-account/paths/~1api~1merchant~1invoice~1receipt/get
func (c *Client) Receipt(ctx context.Context, invoiceID, email string) (*ReceiptResponse, error) {
	q := url.Values{}
	q.Set("invoiceId", invoiceID)
	if email != "" {
		q.Set("email", email)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/merchant/invoice/receipt?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out ReceiptResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// PaymentDirect списує з картки за реквізитами (сирі PAN/exp/CVV).
// УВАГА: вимагає PCI DSS-сертифікацію — твоє оточення має право
// обробляти дані карток. Якщо ні — використовуй [Client.CreateInvoice]
// (там Mono бере на себе збір даних картки).
// https://api.monobank.ua/docs/acquiring.html#tag/Vyklyki-dlya-mercha-z-rozshyrenym-dostupom/paths/~1api~1merchant~1invoice~1payment-direct/post
func (c *Client) PaymentDirect(ctx context.Context, in *PaymentDirectRequest) (*PaymentDirectResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/invoice/payment-direct", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out PaymentDirectResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// SyncPayment — синхронна оплата для ApplePay/GooglePay та токенізованих
// PAN-потоків. Вимагає розширеного скоупу мерчанта (PCI DSS). На відміну
// від [Client.PaymentDirect] (сирий PAN+CVV), тут payload — це токени,
// які приходять прямо з Apple Pay / Google Pay SDK.
// https://api.monobank.ua/docs/acquiring.html#tag/Vyklyki-dlya-mercha-z-rozshyrenym-dostupom/paths/~1api~1merchant~1invoice~1sync-payment/post
func (c *Client) SyncPayment(ctx context.Context, in *SyncPaymentRequest) (*SyncPaymentResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/invoice/sync-payment", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out SyncPaymentResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}
