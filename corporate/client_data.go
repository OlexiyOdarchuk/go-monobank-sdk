package corporate

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/bank"
)

// ClientInfo returns the data of the client whose access was granted
// via requestID. Per the spec, "if the client has not granted the
// right to read their personal data, only the list of accounts is
// returned" — so Accounts comes back regardless of [auth.PermPI],
// which is what fills Name. The spec does not say how [auth.PermSt]
// or [auth.PermFOP] change the response.
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
// access was granted via requestID. The spec caps one call at "31
// days + 1 hour (2682000 seconds)"; the SDK's range helpers slice by
// the more conservative [bank.MaxStatementWindow] — for wider ranges
// use [Client.TransactionsRange].
//
// A zero to omits the {to} path segment; per the spec the bank then
// uses the current time ("if not exist, will be using current
// time").
// https://api.monobank.ua/docs/corporate.html#tag/Klyentski-personalni-dani/paths/~1personal~1statement~1{account}~1{from}~1{to}/get
func (c *Client) Transactions(ctx context.Context, requestID, accountID string, from, to time.Time) (bank.Transactions, error) {
	uri := "/personal/statement/" + url.PathEscape(accountID) +
		"/" + strconv.FormatInt(from.Unix(), 10)
	if !to.IsZero() {
		uri += "/" + strconv.FormatInt(to.Unix(), 10)
	}

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
// arbitrary range, slicing it into consecutive
// [bank.MaxStatementWindow] windows and concatenating the results.
// A zero to means "until now", as in [Client.Transactions]. If to is
// not after from, it returns nil, nil.
//
// On error the transactions fetched before the failure are returned
// together with the error, so a caller can keep them and resume from
// the last one.
//
// Unlike the personal API, the corporate spec states no per-call rate
// limit for the statement, so consecutive windows are not paced.
//
// Mono's /personal/statement is inclusive on both ends, so the next
// window starts at end+1s — a transaction at exactly the boundary
// would otherwise appear in two chunks.
func (c *Client) TransactionsRange(ctx context.Context, requestID, accountID string, from, to time.Time) (bank.Transactions, error) {
	if to.IsZero() {
		to = c.clock()
	}
	if !to.After(from) {
		return nil, nil
	}
	var all bank.Transactions
	for cursor := from; !cursor.After(to); {
		end := cursor.Add(bank.MaxStatementWindow)
		if end.After(to) {
			end = to
		}
		if _, err := bank.DrainWindow(cursor, end,
			func(f, t time.Time) (bank.Transactions, error) {
				return c.Transactions(ctx, requestID, accountID, f, t)
			},
			func(tx bank.Transaction) bool { all = append(all, tx); return true },
		); err != nil {
			return all, fmt.Errorf("range %s..%s: %w", cursor.Format(time.RFC3339), end.Format(time.RFC3339), err)
		}
		next := end.Add(time.Second)
		if !next.After(cursor) {
			break
		}
		cursor = next
	}
	return all, nil
}
