// Package personal is the client for monobank's Personal Open API:
// a single individual, authorized by one X-Token issued at
// https://api.monobank.ua/.
//
// Every response type comes from the [bank] sub-package (ClientInfo,
// Account, Jar, Transaction…) — they are shared with the corporate
// client, because the bank returns the same shapes regardless of the
// authorization scheme.
//
// Rate limits: Mono throttles /personal/client-info to one call per
// 60 s; /personal/statement to one call per account per 60 s. Other
// endpoints are throttled too — handle 429 with backoff
// ([monobank.WithRetry] does this automatically via Retry-After).
//
// The statement window is at most 31 days per call. For wider ranges
// use [Client.TransactionsRange] — it paginates transparently.
package personal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// Client is the Open API client with a personal token.
type Client struct {
	c monobank.Client
}

// New returns a [Client] with the given personal token. Extra options
// (HTTP client, retry policy, base URL for tests) are forwarded to
// the base [monobank.New].
//
//	cli := personal.New(os.Getenv("MONO_TOKEN"))
//	info, err := cli.ClientInfo(ctx)
func New(token string, opts ...monobank.Option) *Client {
	base := []monobank.Option{monobank.WithAuth(auth.NewPersonal(token))}
	return &Client{c: monobank.New(append(base, opts...)...)}
}

// Close releases the client's background resources (see
// [monobank.Client.Close]). Safe to defer right after [New].
func (c *Client) Close() error { return c.c.Close() }

// ClientInfo returns what the bank knows about the authorized user
// (name, accounts, jars). Rate limit: 1 call per 60 s.
// https://api.monobank.ua/docs/#tag/Klientski-personalni-dani/paths/~1personal~1client-info/get
func (c *Client) ClientInfo(ctx context.Context) (*bank.ClientInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/personal/client-info", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out bank.ClientInfo
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// Transactions returns statement entries for account accountID
// within [from, to] (inclusive). Mono accepts at most 31 days per
// call; for wider ranges use [Client.TransactionsRange]. Rate limit:
// 1 call per account per 60 s.
// https://api.monobank.ua/docs/#tag/Klientski-personalni-dani/paths/~1personal~1statement~1{account}~1{from}~1{to}/get
func (c *Client) Transactions(ctx context.Context, accountID string, from, to time.Time) (bank.Transactions, error) {
	uri := "/personal/statement/" + url.PathEscape(accountID) +
		"/" + strconv.FormatInt(from.Unix(), 10) +
		"/" + strconv.FormatInt(to.Unix(), 10)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out bank.Transactions
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out, nil
}

// TransactionsRange returns the statement for an arbitrary range,
// slicing it into consecutive 31-day windows (Mono's per-call limit)
// and concatenating the results in chronological order.
//
// If to is zero or earlier than from, returns nil, nil (no error).
// Mind the rate limit: 1 call per account per 60 s — for weekly
// ranges that is 1 request, for quarterly it can be 3-4.
func (c *Client) TransactionsRange(ctx context.Context, accountID string, from, to time.Time) (bank.Transactions, error) {
	if to.IsZero() || !to.After(from) {
		return nil, nil
	}

	var all bank.Transactions
	for cursor := from; cursor.Before(to); {
		end := cursor.Add(bank.MaxStatementWindow)
		if end.After(to) {
			end = to
		}
		chunk, err := c.Transactions(ctx, accountID, cursor, end)
		if err != nil {
			return nil, fmt.Errorf("range %s..%s: %w", cursor.Format(time.RFC3339), end.Format(time.RFC3339), err)
		}
		all = append(all, chunk...)
		cursor = end
	}
	return all, nil
}

// webhookRequest is the body of POST /personal/webhook.
type webhookRequest struct {
	WebHookURL string `json:"webHookUrl"`
}

// SetWebHook subscribes the given URI to StatementItem events. Mono
// pings the URI with a GET right after subscription to check that it
// is alive (respond with 200). Pass an empty string to unsubscribe.
// https://api.monobank.ua/docs/#tag/Klientski-personalni-dani/paths/~1personal~1webhook/post
func (c *Client) SetWebHook(ctx context.Context, uri string) error {
	body, err := json.Marshal(webhookRequest{WebHookURL: uri})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/personal/webhook", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}
