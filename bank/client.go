package bank

import (
	"context"
	"fmt"
	"net/http"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

// Client is a wrapper around the base [monobank.Client] that exposes
// two public (unauthorized) endpoints: currency rates and the server
// key for webhook verification. Typically a caller creates one
// [Client] per application and shares it between Rates calls and as
// a KeyProvider for [webhook.Handler].
type Client struct {
	c monobank.Client
}

// New returns a [Client] for the unauthorized endpoints. Options
// (HTTP client, retry policy, base URL for tests) are forwarded to
// the base [monobank.New].
//
//	cli := bank.New()
//	rates, err := cli.Rates(ctx)
func New(opts ...monobank.Option) *Client {
	return &Client{c: monobank.New(opts...)}
}

// Close releases the client's background resources (see
// [monobank.Client.Close]).
func (c *Client) Close() error { return c.c.Close() }

// Rates fetches the current exchange-rate table from /bank/currency.
// Mono rate-limits this endpoint — cache the result for a minute or
// two. Docs: https://api.monobank.ua/docs/#operation/getCurrency
func (c *Client) Rates(ctx context.Context) (Rates, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/bank/currency", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out Rates
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out, nil
}

// ServerKey fetches the bank's current public key from /bank/sync.
// It is used to verify the signature of incoming webhooks. Cache the
// result and refetch only when an incoming X-Key-Id stops matching
// [ServerKey.ID]. [webhook.Handler] does this automatically.
// Docs: https://api.monobank.ua/docs/#operation/getServerKey
func (c *Client) ServerKey(ctx context.Context) (*ServerKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/bank/sync", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var raw bankSyncResponse
	if err := c.c.Do(req, &raw, http.StatusOK); err != nil {
		return nil, err
	}
	return raw.asServerKey()
}
