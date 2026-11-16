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
// the amount-entry mode: "merchant" (the merchant sets the amount —
// via [Client.CreateInvoice] with QrID), "client" (the client enters
// it), "fix" (a fixed amount).
// https://monobank.ua/api-docs/acquiring/methods/qr/get--api--merchant--qr--list
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
// https://monobank.ua/api-docs/acquiring/methods/qr/get--api--merchant--qr--details
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

// QRResetAmount removes the payment amount currently set on a QR cash
// desk — handy when the amount was set by mistake, or the client did
// not pay and the desk needs to be released. The docs do not restrict
// it by AmountType. If the amount is already being paid, the bank
// rejects the reset with 400 [CodeOrderInProgress].
// https://monobank.ua/api-docs/acquiring/methods/qr/post--api--merchant--qr--reset-amount
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
