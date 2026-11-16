// Package personal is the client for monobank's Personal Open API:
// a single individual, authorized by one X-Token issued at
// https://api.monobank.ua/.
//
// Every response type comes from the [bank] sub-package (ClientInfo,
// Account, Jar, Transaction…) — they are shared with the corporate
// client, because the bank returns the same shapes regardless of the
// authorization scheme.
//
// Rate limits: the spec allows /personal/client-info and
// /personal/statement "не частіше ніж 1 раз у 60 секунд" each. It does
// not say whether the statement limit counts per account or per token,
// and it documents no Retry-After header. [monobank.WithRetry] caps
// every sleep at its maxDelay (30 s by default), so on its own it does
// not wait out a 60-second limit — use [monobank.WithRateLimiter], or
// rely on the pacing [Client.TransactionsRange] applies between its
// own requests ([monobank.WithStatementPacing]).
//
// The spec allows a statement window of up to 31 days + 1 hour
// (2682000 s) per call; the SDK uses 31-day windows
// ([bank.MaxStatementWindow]). For wider ranges use
// [Client.TransactionsRange] — it paginates transparently.
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

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/auth"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/bank"
)

// Client is the Open API client with a personal token.
type Client struct {
	c monobank.Client

	// now and sleep are test seams for the range helpers; nil means
	// time.Now and a context-aware timer.
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
}

func (c *Client) clock() time.Time {
	if c.now != nil {
		return c.now()
	}

	return time.Now()
}

func (c *Client) pause(ctx context.Context, d time.Duration) error {
	if c.sleep != nil {
		return c.sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// statementFetcher returns the fetch closure the range helpers hand to
// [bank.DrainWindow]. It spaces consecutive requests by the client's
// [monobank.Client.StatementPacing]; the first request is not delayed.
func (c *Client) statementFetcher(ctx context.Context, accountID string) bank.StatementFetcher {
	every := c.c.StatementPacing()
	var last time.Time

	return func(from, to time.Time) (bank.Transactions, error) {
		if every > 0 && !last.IsZero() {
			if d := every - c.clock().Sub(last); d > 0 {
				if err := c.pause(ctx, d); err != nil {
					return nil, err
				}
			}
		}
		last = c.clock()

		return c.Transactions(ctx, accountID, from, to)
	}
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
// (name, accounts, jars). Rate limit per the spec: "не частіше ніж 1
// раз у 60 секунд".
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
// within [from, to] (inclusive). accountID is an account or jar id
// from [bank.ClientInfo], or "0" for the default account. A zero to
// omits the {to} segment, which the spec defines as "until now". The
// spec allows at most 31 days + 1 hour per call; for wider ranges use
// [Client.TransactionsRange]. Rate limit: 1 call per 60 s.
// https://api.monobank.ua/docs/#tag/Klientski-personalni-dani/paths/~1personal~1statement~1{account}~1{from}~1{to}/get
func (c *Client) Transactions(ctx context.Context, accountID string, from, to time.Time) (bank.Transactions, error) {
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
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out, nil
}

// TransactionsRange returns the statement for an arbitrary range,
// slicing it into consecutive 31-day windows ([bank.MaxStatementWindow])
// and concatenating the results window by window, oldest window first
// (newest-first within each window).
//
// A zero to means "until now". If to is not after from, returns nil,
// nil (no error).
//
// The spec allows one statement call per 60 s, so consecutive requests
// of one range call are spaced by [monobank.WithStatementPacing]
// (default [monobank.DefaultStatementPacing]): a weekly range is 1
// request, a quarterly one 3-4 requests and about as many minutes.
// The wait honors ctx.
//
// On error the transactions fetched before the failure are returned
// together with the error, so a caller can keep them and resume from
// the last one.
//
// Consecutive windows do not overlap on the boundary second: Mono's
// /personal/statement is inclusive on both ends, so the next cursor
// is end+1s, not end. Without that shift a transaction landing
// exactly on a window boundary would appear in two chunks.
func (c *Client) TransactionsRange(ctx context.Context, accountID string, from, to time.Time) (bank.Transactions, error) {
	if to.IsZero() {
		to = c.clock()
	}
	if !to.After(from) {
		return nil, nil
	}

	fetch := c.statementFetcher(ctx, accountID)
	var all bank.Transactions
	for cursor := from; !cursor.After(to); {
		end := cursor.Add(bank.MaxStatementWindow)
		if end.After(to) {
			end = to
		}
		if _, err := bank.DrainWindow(cursor, end, fetch,
			func(tx bank.Transaction) bool { all = append(all, tx); return true },
		); err != nil {
			return all, fmt.Errorf("range %s..%s: %w", cursor.Format(time.RFC3339), end.Format(time.RFC3339), err)
		}
		next := end.Add(time.Second)
		if !next.After(cursor) {
			// safety against int64 overflow on very large windows
			break
		}
		cursor = next
	}
	return all, nil
}

// webhookRequest is the body of POST /personal/webhook.
type webhookRequest struct {
	WebHookURL string `json:"webHookUrl"`
}

// SetWebHook subscribes the given URI to StatementItem events.
//
// Per the spec, Mono first sends a GET to the URI and the server must
// answer "строго HTTP статус-кодом 200"; only then do POST events
// start. Each event must be answered with 200 within 5 s, otherwise
// Mono retries after 60 and 600 s, and if the third attempt also gets
// no answer the webhook is switched off — after which it has to be set
// again with this call.
//
// The spec does not describe unsubscribing and does not say what an
// empty uri does; the SDK sends it as given. Avoid passing "" unless
// you intend to clear the webhook.
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
