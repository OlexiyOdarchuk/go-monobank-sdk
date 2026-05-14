package acquiring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Wallet returns the list of cards stored under the given walletID.
// Pass "" to receive every merchant card. WalletID is usually your
// user-ID (one person — one wallet with their cards).
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

// DeleteCard removes a tokenized card from a wallet. Afterwards its
// CardToken becomes invalid for [Client.WalletPayment].
// https://api.monobank.ua/docs/acquiring.html#tag/Tokenization/paths/~1api~1merchant~1wallet~1card/delete
func (c *Client) DeleteCard(ctx context.Context, cardToken string) error {
	if cardToken == "" {
		return ErrEmptyID
	}
	q := url.Values{}
	q.Set("cardToken", cardToken)
	uri := "/api/merchant/wallet/card?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, uri, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}

// WalletPayment charges a previously tokenized card (CardToken).
// InitiationKind ("merchant" or "client") matters for compliance:
// "merchant" — a repeat charge initiated by you (recurring),
// "client" — the client explicitly consented here and now.
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
