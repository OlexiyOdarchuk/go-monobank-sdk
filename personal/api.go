package personal

import (
	"context"
	"iter"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// API is the interface of the Personal Open API client. It exists
// separately from *[Client] so callers can mock it via
// mockgen/testify-mock in their own tests. [Client] implements this
// interface (verified by the compile-time assert below).
type API interface {
	ClientInfo(ctx context.Context) (*bank.ClientInfo, error)
	Transactions(ctx context.Context, accountID string, from, to time.Time) (bank.Transactions, error)
	TransactionsRange(ctx context.Context, accountID string, from, to time.Time) (bank.Transactions, error)
	TransactionsRangeIter(ctx context.Context, accountID string, from, to time.Time) iter.Seq2[bank.Transaction, error]
	SetWebHook(ctx context.Context, uri string) error
}

// Compile-time assert: *Client satisfies [API].
var _ API = (*Client)(nil)
