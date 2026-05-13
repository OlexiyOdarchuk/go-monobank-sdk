package corporate

import (
	"context"
	"iter"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// TransactionsRangeIter — streaming-варіант [Client.TransactionsRange].
// Аналогічно personal-клієнту, але потребує requestID (схвалення доступу
// клієнтом, див. [Client.Auth]).
//
//	for tx, err := range cli.TransactionsRangeIter(ctx, reqID, accID, from, to) {
//	    if err != nil { return err }
//	    process(tx)
//	}
//
// Якщо to нульовий або раніше за from — нуль yields без помилки.
func (c *Client) TransactionsRangeIter(ctx context.Context, requestID, accountID string,
	from, to time.Time) iter.Seq2[bank.Transaction, error] {

	return func(yield func(bank.Transaction, error) bool) {
		if to.IsZero() || !to.After(from) {
			return
		}
		for cursor := from; cursor.Before(to); {
			if err := ctx.Err(); err != nil {
				_ = yield(bank.Transaction{}, err)
				return
			}
			end := cursor.Add(bank.MaxStatementWindow)
			if end.After(to) {
				end = to
			}
			chunk, err := c.Transactions(ctx, requestID, accountID, cursor, end)
			if err != nil {
				_ = yield(bank.Transaction{}, err)
				return
			}
			for _, tx := range chunk {
				if !yield(tx, nil) {
					return
				}
			}
			cursor = end
		}
	}
}
