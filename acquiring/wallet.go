package acquiring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Wallet повертає список карток, збережених під заданим walletID.
// Передай "" — отримаєш всі картки мерчанта. WalletID зазвичай — це
// твій user-ID (одна людина — один wallet із кількома її картками).
// https://api.monobank.ua/docs/acquiring.html#tag/Tokenization/paths/~1api~1merchant~1wallet/get
func (c *Client) Wallet(ctx context.Context, walletID string) ([]WalletCard, error) {
	q := url.Values{}
	if walletID != "" {
		q.Set("walletId", walletID)
	}
	uri := "/api/merchant/wallet"
	if s := q.Encode(); s != "" {
		uri += "?" + s
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out WalletResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.Wallet, nil
}

// DeleteCard видаляє токенізовану картку з гаманця. Після цього її
// CardToken стає недійсним для [Client.WalletPayment].
// https://api.monobank.ua/docs/acquiring.html#tag/Tokenization/paths/~1api~1merchant~1wallet~1card/delete
func (c *Client) DeleteCard(ctx context.Context, cardToken string) error {
	q := url.Values{}
	if cardToken != "" {
		q.Set("cardToken", cardToken)
	}
	uri := "/api/merchant/wallet/card"
	if s := q.Encode(); s != "" {
		uri += "?" + s
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, uri, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}

// WalletPayment списує з раніше токенізованої картки (CardToken).
// InitiationKind ("merchant" або "client") важливий для compliance:
// "merchant" — повторне списання за вашою ініціативою (recurring),
// "client" — клієнт явно дав згоду тут і зараз.
// https://api.monobank.ua/docs/acquiring.html#tag/Tokenization/paths/~1api~1merchant~1wallet~1payment/post
func (c *Client) WalletPayment(ctx context.Context, in *WalletPaymentRequest) (*WalletPaymentResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/wallet/payment", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out WalletPaymentResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}
