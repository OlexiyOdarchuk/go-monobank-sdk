package bank

import "context"

// API is the interface of the public bank endpoints (no
// authorization). It exists separately from *[Client] so callers can
// mock it via mockgen/testify-mock in their own tests. [Client]
// implements this interface (verified by the compile-time assert
// below).
type API interface {
	Rates(ctx context.Context) (Rates, error)
	ServerKey(ctx context.Context) (*ServerKey, error)
}

// Compile-time assert: *Client satisfies [API].
var _ API = (*Client)(nil)
