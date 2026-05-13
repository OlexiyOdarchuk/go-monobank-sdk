package personal

import (
	"context"
	"iter"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// API — інтерфейс Personal Open API клієнта. Існує окремо від
// *[Client], щоб користувачі могли мокувати його через
// mockgen/testify-mock у власних тестах. Сам [Client] цей інтерфейс
// реалізує (перевіряється compile-time assert-ом нижче).
type API interface {
	ClientInfo(ctx context.Context) (*bank.ClientInfo, error)
	Transactions(ctx context.Context, accountID string, from, to time.Time) (bank.Transactions, error)
	TransactionsRange(ctx context.Context, accountID string, from, to time.Time) (bank.Transactions, error)
	TransactionsRangeIter(ctx context.Context, accountID string, from, to time.Time) iter.Seq2[bank.Transaction, error]
	SetWebHook(ctx context.Context, uri string) error
}

// Compile-time assert: *Client задовольняє [API].
var _ API = (*Client)(nil)
