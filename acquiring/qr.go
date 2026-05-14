package acquiring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// QRList lists every QR cash desk for this merchant. AmountType is
// the amount-entry mode: "merchant" (the merchant sets it via
// QRSetAmount), "client" (the client enters it in the Mono mobile
// app), "fix" (a fixed amount).
// https://api.monobank.ua/docs/acquiring.html#tag/QR-kasy/paths/~1api~1merchant~1qr~1list/get
func (c *Client) QRList(ctx context.Context) ([]QR, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/merchant/qr/list", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out QRList
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.List, nil
}

// QRDetails returns the details of a single QR cash desk (current
// invoice, amount, currency — if a payment is pending).
// https://api.monobank.ua/docs/acquiring.html#tag/QR-kasy/paths/~1api~1merchant~1qr~1details/get
func (c *Client) QRDetails(ctx context.Context, qrID string) (*QRDetails, error) {
	if qrID == "" {
		return nil, ErrEmptyID
	}
	q := url.Values{}
	q.Set("qrId", qrID)
	uri := "/api/merchant/qr/details?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out QRDetails
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// QRResetAmount resets the expected amount on a QR cash desk whose
// AmountType is "merchant" or "client". Handy when the merchant set
// the amount by mistake, or the client did not pay and the terminal
// needs to be "released".
// https://api.monobank.ua/docs/acquiring.html#tag/QR-kasy/paths/~1api~1merchant~1qr~1reset-amount/post
func (c *Client) QRResetAmount(ctx context.Context, qrID string) error {
	if qrID == "" {
		return ErrEmptyID
	}
	body, err := json.Marshal(ResetAmountRequest{QrID: qrID})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/qr/reset-amount", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}
