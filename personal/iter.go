package personal

import (
	"context"
	"iter"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// TransactionsRangeIter is the streaming variant of
// [Client.TransactionsRange]. Instead of accumulating every
// transaction in memory and returning them as a single slice, the
// iterator yields one transaction at a time and only pulls the next
// 31-day window when the current one is exhausted. Windows that exceed
// Mono's 500-rows-per-response cap are drained transparently (see
// [bank.DrainWindow]).
//
//	for tx, err := range cli.TransactionsRangeIter(ctx, accID, from, to) {
//	    if err != nil { return err }
//	    process(tx)
//	}
//
// The win is memory on quarterly / annual ranges. The rate limit is
// the same (1 call per 60 s per account), it just no longer blocks
// on the full download.
//
// If to is zero or earlier than from, the iterator yields nothing
// and stops without an error.
func (c *Client) TransactionsRangeIter(ctx context.Context, accountID string,
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
			cont, err := bank.DrainWindow(cursor, end,
				func(f, t time.Time) (bank.Transactions, error) {
					return c.Transactions(ctx, accountID, f, t)
				},
				func(tx bank.Transaction) bool { return yield(tx, nil) },
			)
			if err != nil {
				_ = yield(bank.Transaction{}, err)
				return
			}
			if !cont {
				return
			}
			// Mono's /personal/statement is inclusive on both ends —
			// the next window starts at end+1s to avoid yielding the
			// boundary transaction twice.
			next := end.Add(time.Second)
			if !next.After(cursor) {
				return
			}
			cursor = next
		}
	}
}
