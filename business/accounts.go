package business

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Accounts returns every account of the company (UAH, USD, EUR etc.
// — one row per currency).
// https://corp-api.monobank.ua/docs/#operation/get-all-accounts
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/ext/v1/accounts", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out []Account
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out, nil
}

// Account returns a single account by its IBAN. Handy when you need
// the current balance for a specific account without fetching the
// full list via [Client.Accounts].
// https://corp-api.monobank.ua/docs/#operation/get-account
func (c *Client) Account(ctx context.Context, iban string) (*Account, error) {
	uri := "/ext/v1/accounts/" + url.PathEscape(iban)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out Account
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// AccountBalances returns the history of daily account balances for
// a period. dateFrom / dateTo are ISO-8601 dates (YYYY-MM-DD), bounds
// inclusive. The IsFinal field in the response indicates whether the
// balance for that date may still change (for the last date within
// the business day it is false).
// https://corp-api.monobank.ua/docs/#operation/get-account-balances
func (c *Client) AccountBalances(ctx context.Context, iban, dateFrom, dateTo string) ([]BalancePoint, error) {
	q := url.Values{}
	q.Set("dateFrom", dateFrom)
	q.Set("dateTo", dateTo)
	uri := "/ext/v1/accounts/" + url.PathEscape(iban) + "/balances?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out []BalancePoint
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out, nil
}
