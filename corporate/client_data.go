package corporate

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// ClientInfo returns the data of the client whose access was granted
// via requestID. Which fields are populated depends on the
// permissions [Client.Auth] requested: PermPI fills Name; PermSt
// fills Accounts and unlocks statement reads; PermFOP exposes FOP
// (sole-proprietor) accounts.
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

// Transactions returns the statement for one of the accounts whose
// access was granted via requestID. Same 31-day window limit as
// [personal.Client.Transactions] — for wider ranges use
// [Client.TransactionsRange].
// https://api.monobank.ua/docs/corporate.html#tag/Klyentski-personalni-dani/paths/~1personal~1statement~1{account}~1{from}~1{to}/get
func (c *Client) Transactions(ctx context.Context, requestID, accountID string, from, to time.Time) (bank.Transactions, error) {
	uri := "/personal/statement/" + url.PathEscape(accountID) +
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

// TransactionsRange paginates [Client.Transactions] over an
// arbitrary range, slicing it into consecutive 31-day windows and
// concatenating the results. If to is zero or earlier than from,
// it returns nil, nil.
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
