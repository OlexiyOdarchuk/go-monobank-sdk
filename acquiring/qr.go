package acquiring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// QRList перелічує всі QR-каси цього мерчанта. AmountType — режим
// введення суми: "merchant" (мерчант задає через QRSetAmount),
// "client" (клієнт вводить сам у мобільному застосунку Mono),
// "fix" (зафіксована сума).
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

// QRDetails повертає деталі одного QR-каси (поточний інвойс, сума,
// валюта — якщо є чекаюча оплата).
// https://api.monobank.ua/docs/acquiring.html#tag/QR-kasy/paths/~1api~1merchant~1qr~1details/get
func (c *Client) QRDetails(ctx context.Context, qrID string) (*QRDetails, error) {
	q := url.Values{}
	if qrID != "" {
		q.Set("qrId", qrID)
	}
	uri := "/api/merchant/qr/details"
	if s := q.Encode(); s != "" {
		uri += "?" + s
	}
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

// QRResetAmount скидає очікувану суму на QR-касі з AmountType
// "merchant" або "client". Корисно, коли мерчант помилково задав суму
// або клієнт не оплатив і треба «звільнити» термінал.
// https://api.monobank.ua/docs/acquiring.html#tag/QR-kasy/paths/~1api~1merchant~1qr~1reset-amount/post
func (c *Client) QRResetAmount(ctx context.Context, qrID string) error {
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
