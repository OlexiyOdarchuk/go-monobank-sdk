package acquiring

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// MerchantDetails returns the merchant profile: id, name, EDRPOU
// (Ukrainian business registration code). Handy for smoke-testing
// the token.
// https://api.monobank.ua/docs/acquiring.html#tag/Vyklyki-dlya-mercha/paths/~1api~1merchant~1details/get
func (c *Client) MerchantDetails(ctx context.Context) (*MerchantDetails, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/merchant/details", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out MerchantDetails
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// Employees lists the merchant's active employees (for example,
// tip recipients). IDs from this list go into
// CreateInvoiceRequest.TipsEmployeeID.
// https://api.monobank.ua/docs/acquiring.html#tag/Vyklyki-dlya-mercha/paths/~1api~1merchant~1employee~1list/get
func (c *Client) Employees(ctx context.Context) ([]Employee, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/merchant/employee/list", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out EmployeeList
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.List, nil
}

// PubKey returns the merchant public key (a base64-encoded PEM x.509
// ECDSA key on NIST P-256) used to verify acquiring webhook
// signatures. Parse the Key field via [ServerKey.Public] or
// [ParsePubKey] and cache it — the bank rotates the key rarely, but
// it can change. Unlike /bank/sync, this is a separate key for the
// acquiring webhooks.
// https://api.monobank.ua/docs/acquiring.html#tag/Vyklyki-dlya-mercha/paths/~1api~1merchant~1pubkey/get
func (c *Client) PubKey(ctx context.Context) (*ServerKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/merchant/pubkey", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out ServerKey
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// Submerchants lists the submerchants configured under this
// merchant. Subm-Code is used in the Statement filter and in
// CreateInvoice.Code.
// https://api.monobank.ua/docs/acquiring.html#tag/Vyklyki-dlya-mercha/paths/~1api~1merchant~1submerchant~1list/get
func (c *Client) Submerchants(ctx context.Context) ([]Submerchant, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/merchant/submerchant/list", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out SubmerchantList
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.List, nil
}

// Statement returns the statement for the period for this merchant.
// A zero to means "up to now". code (optional) filters by submerchant.
// Each row carries CancelList — the refund history for a specific
// invoice.
// https://api.monobank.ua/docs/acquiring.html#tag/Vyplaty-ta-zvirky/paths/~1api~1merchant~1statement/get
func (c *Client) Statement(ctx context.Context, from, to time.Time, code string) ([]StatementInvoice, error) {
	q := url.Values{}
	q.Set("from", strconv.FormatInt(from.Unix(), 10))
	if !to.IsZero() {
		q.Set("to", strconv.FormatInt(to.Unix(), 10))
	}
	if code != "" {
		q.Set("code", code)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/merchant/statement?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out StatementResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.List, nil
}
