package business

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// PreparePayment створює чернетку платежу і відправляє її у flow
// підпису компанії (через її бізнес-кабінет / mobile). Повернутий
// [PaymentPrepared.ID] — ключ для опитування стану через
// [Client.PaymentState]. idempotencyKey обов'язковий: новий UUID v4 на
// кожну логічну спробу — повтор з тим самим ключем безпечний.
// https://corp-api.monobank.ua/docs/#operation/prepare-payment
func (c *Client) PreparePayment(ctx context.Context, idempotencyKey string,
	in *PaymentRequest) (*PaymentPrepared, error) {

	if in == nil {
		return nil, ErrNilRequest
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

// PaymentState повертає стан життєвого циклу платежу за внутрішнім id
// Mono (DRAFT, DECLINED або IN_STATEMENT — фінальний стан передбачає,
// що шукай результат вже у виписці через [Client.Operation]).
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

// PaymentStateByReference шукає платіж за externalReference, який ти
// додав при [Client.PreparePayment]. Зручно, коли мережа впала після
// PreparePayment і ти не знаєш id, але знаєш свій reference.
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
