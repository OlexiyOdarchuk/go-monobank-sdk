package business

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// PreparePayment creates a payment draft and sends it into the
// company's signing flow (via the business cabinet / mobile). The
// returned [PaymentPrepared.ID] is the key for polling the state via
// [Client.PaymentState]. idempotencyKey is required: use a new
// UUID v4 per logical attempt — repeating with the same key is safe.
// https://corp-api.monobank.ua/docs/#operation/prepare-payment
func (c *Client) PreparePayment(ctx context.Context, idempotencyKey string,
	in *PaymentRequest) (*PaymentPrepared, error) {

	if in == nil {
		return nil, ErrNilRequest
	}
	if idempotencyKey == "" {
		return nil, ErrIdempotencyKeyRequired
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/ext/v1/payment/prepare", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Idempotency-Key", idempotencyKey)

	var out PaymentPrepared
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// PaymentState returns the lifecycle state of a payment by its
// internal Mono id (DRAFT, DECLINED, or IN_STATEMENT — the final
// state implies the result should be looked up in the statement via
// [Client.Operation]).
// https://corp-api.monobank.ua/docs/#operation/get-payment-state
func (c *Client) PaymentState(ctx context.Context, id string) (*PaymentState, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/ext/v1/payment/"+url.PathEscape(id)+"/state", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out PaymentState
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// PaymentStateByReference looks up a payment by the
// externalReference you added in [Client.PreparePayment]. Handy when
// the network failed after PreparePayment and you do not know the id
// but do know your reference.
// https://corp-api.monobank.ua/docs/#operation/get-payment-state-by-external-reference
func (c *Client) PaymentStateByReference(ctx context.Context, externalReference string) (*PaymentState, error) {
	q := url.Values{}
	q.Set("externalReference", externalReference)
	uri := "/ext/v1/payment/state?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out PaymentState
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}
