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
// The docs mark walletId as optional but do not say what an empty one
// returns; the SDK then omits the parameter.
//
// Pass the walletId the bank returned in [WalletData] once its status
// is "created", not the one you sent: if the card was already
// tokenized, the bank keeps the walletId of the first token and
// ignores yours (see [SaveCardData.WalletID]).
// https://monobank.ua/api-docs/acquiring/extras/tokens/get--api--merchant--wallet
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
// https://monobank.ua/api-docs/acquiring/extras/tokens/delete--api--merchant--wallet--card
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

// WalletPayment charges a previously tokenized card (CardToken), or a
// raw Google Pay / Apple Pay token (GToken / AToken, per the wallet
// guides). InitiationKind ("merchant" or "client") is required by the
// schema: "merchant" — a merchant-initiated charge the client does not
// confirm (for example, recurring); "client" — the client pays with a
// saved card on demand. PaymentType accepts debit (default) or hold.
//
// Set the X-Cms / X-Cms-Version headers with [WithCMS] if you build a
// payment module for a CMS.
// https://monobank.ua/api-docs/acquiring/extras/tokens/post--api--merchant--wallet--payment
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
