package corporate

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// ClientInfo повертає дані клієнта, доступ до якого було надано за
// requestID. Які саме поля заповнено — залежить від permissions, які
// запитував [Client.Auth]: PermPI наповнить Name; PermSt — Accounts і
// дозволить читати виписки; PermFOP — рахунки ФОП.
// https://api.monobank.ua/docs/corporate.html#tag/Klyentski-personalni-dani/paths/~1personal~1client-info/get
func (c *Client) ClientInfo(ctx context.Context, requestID string) (*bank.ClientInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/personal/client-info", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out bank.ClientInfo
	if err := c.do(req, c.authMaker.New(requestID), &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// Transactions повертає виписку по одному з рахунків, доступ до яких
// видано через requestID. Той самий ліміт 31-денного вікна, що й у
// [personal.Client.Transactions] — для ширших діапазонів є
// [Client.TransactionsRange].
// https://api.monobank.ua/docs/corporate.html#tag/Klyentski-personalni-dani/paths/~1personal~1statement~1{account}~1{from}~1{to}/get
func (c *Client) Transactions(ctx context.Context, requestID, accountID string, from, to time.Time) (bank.Transactions, error) {
	uri := "/personal/statement/" + accountID +
		"/" + strconv.FormatInt(from.Unix(), 10) +
		"/" + strconv.FormatInt(to.Unix(), 10)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out bank.Transactions
	if err := c.do(req, c.authMaker.New(requestID), &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out, nil
}

// TransactionsRange пагінує [Client.Transactions] на довільному
// діапазоні, нарізаючи його на послідовні 31-денні вікна і зчіплюючи
// результати. Якщо to нульовий або раніше за from — повертає nil, nil.
func (c *Client) TransactionsRange(ctx context.Context, requestID, accountID string, from, to time.Time) (bank.Transactions, error) {
	if to.IsZero() || !to.After(from) {
		return nil, nil
	}
	var all bank.Transactions
	for cursor := from; cursor.Before(to); {
		end := cursor.Add(bank.MaxStatementWindow)
		if end.After(to) {
			end = to
		}
		chunk, err := c.Transactions(ctx, requestID, accountID, cursor, end)
		if err != nil {
			return nil, fmt.Errorf("range %s..%s: %w", cursor.Format(time.RFC3339), end.Format(time.RFC3339), err)
		}
		all = append(all, chunk...)
		cursor = end
	}
	return all, nil
}
