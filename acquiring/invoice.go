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

// ErrNilRequest is returned from mutating endpoints when a nil body
// is passed.
var ErrNilRequest = errors.New("request body is nil")

// CreateInvoice creates a new invoice and returns its id and the URL
// to show the client (Mono's checkout page). PaymentType selects the
// scenario: PaymentDebit captures immediately, PaymentHold
// authorizes followed by [Client.FinalizeInvoice].
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

// InvoiceStatus returns the invoice's current state: status (created
// / processing / hold / success / failure / reversed / expired),
// card and payment-system details, and the refund history. Use it
// for polling when WebHookURL is not configured.
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

// CancelInvoice cancels (or partially refunds) a paid invoice. Pass
// Amount < the original amount for a partial refund; an empty
// Amount is a full refund. ExtRef is an optional identifier in your
// system (it ends up in the operation status).
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

// FinalizeInvoice finalizes (captures) a hold invoice — captures the
// previously authorized amount. You may pass Amount less than the
// original — only that amount is then captured from the card, the
// remainder of the authorized funds is released (partial
// finalization).
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

// RemoveInvoice invalidates an unpaid invoice. After that the
// checkout page stops working. To cancel an ALREADY paid invoice
// use [Client.CancelInvoice] (refund).
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

// FiscalChecks returns the fiscal checks attached to an invoice (via
// Checkbox or Monopay). One invoice may have multiple checks: a sale
// and a return.
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

// Receipt returns the base64-encoded PDF receipt for an invoice. If
// email is non-empty, the bank also sends a copy by email.
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

// PaymentDirect charges a card by its raw details (raw PAN/exp/CVV).
// CAUTION: requires PCI DSS certification — your environment must
// be allowed to handle card data. If not, use [Client.CreateInvoice]
// (where Mono collects the card data for you).
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

// SyncPayment is a synchronous charge for Apple Pay/Google Pay and
// tokenized PAN flows. Requires extended merchant scope (PCI DSS).
// Unlike [Client.PaymentDirect] (raw PAN+CVV), here the payload is
// tokens that come straight from the Apple Pay / Google Pay SDK.
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
