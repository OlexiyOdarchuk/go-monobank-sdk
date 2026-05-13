package business

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Accounts повертає всі рахунки компанії (UAH, USD, EUR тощо — по
// одному рядку на валюту).
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

// Account повертає один рахунок за його IBAN. Зручно, коли треба
// швидкий поточний баланс на конкретному рахунку без вантаження
// всього списку через [Client.Accounts].
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

// AccountBalances повертає історію щоденних балансів рахунку за період.
// dateFrom / dateTo — дати у ISO-8601 (YYYY-MM-DD), межі включно. Поле
// IsFinal у відповіді каже, чи можуть ще відбутися зміни балансу на
// дату (для останньої дати в межах робочого дня — false).
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
