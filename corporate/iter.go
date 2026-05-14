package corporate

import (
	"context"
	"iter"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// TransactionsRangeIter is the streaming variant of
// [Client.TransactionsRange]. Same shape as the personal client, but
// requires requestID (the client must have approved access, see
// [Client.Auth]).
//
//	for tx, err := range cli.TransactionsRangeIter(ctx, reqID, accID, from, to) {
//	    if err != nil { return err }
//	    process(tx)
//	}
//
// If to is zero or earlier than from, yields nothing without an error.
func (c *Client) TransactionsRangeIter(ctx context.Context, requestID, accountID string,
	from, to time.Time) iter.Seq2[bank.Transaction, error] {

	return func(yield func(bank.Transaction, error) bool) {
		if to.IsZero() || !to.After(from) {
			return
		}
		for cursor := from; !cursor.After(to); {
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
			// inclusive [from, to] on both sides — bump by +1s to
			// avoid double-yielding the boundary transaction.
			next := end.Add(time.Second)
			if !next.After(cursor) {
				return
			}
			cursor = next
		}
	}
}
