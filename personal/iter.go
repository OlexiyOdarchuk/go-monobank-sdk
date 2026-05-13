package personal

import (
	"context"
	"iter"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// TransactionsRangeIter — streaming-варіант [Client.TransactionsRange].
// Замість того щоб накопичувати всі транзакції в memory і повернути
// одним зрізом, ітератор віддає по одній транзакції за час, тягнучи
// наступне 31-денне вікно лише коли поточне вичерпане.
//
//	for tx, err := range cli.TransactionsRangeIter(ctx, accID, from, to) {
//	    if err != nil { return err }
//	    process(tx)
//	}
//
// Виграш — пам'ять на квартальних/річних діапазонах. Rate-limit той
// самий (1 виклик на 60с на акаунт), просто не блокується на повному
// завантаженні.
//
// Якщо to нульовий або раніше за from — ітератор віддає нуль yields і
// зупиняється без помилки.
func (c *Client) TransactionsRangeIter(ctx context.Context, accountID string,
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
			chunk, err := c.Transactions(ctx, accountID, cursor, end)
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
