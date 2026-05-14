package business

import (
	"context"
	"iter"
	"time"
)

// ContactsAll повертає iter.Seq2-ітератор по всіх зарплатних контактах
// компанії. Сторінки тягнуться лінько по черзі через [Client.Contacts]
// з кроком pageSize (0 → дефолт API). Якщо ctx скасовується або один
// з викликів повертає помилку — ітератор віддає (Contact{}, err) і
// зупиняється.
//
//	for c, err := range cli.ContactsAll(ctx, 0) {
//	    if err != nil { return err }
//	    process(c)
//	}
//
// Перервати раніше — звичайний break.
func (c *Client) ContactsAll(ctx context.Context, pageSize int) iter.Seq2[Contact, error] {
	return func(yield func(Contact, error) bool) {
		offset := 0
		for {
			if err := ctx.Err(); err != nil {
				_ = yield(Contact{}, err)
				return
			}
			page, err := c.Contacts(ctx, pageSize, offset)
			if err != nil {
				_ = yield(Contact{}, err)
				return
			}
			for _, contact := range page.Contacts {
				if !yield(contact, nil) {
					return
				}
			}
			if !page.HasMore || len(page.Contacts) == 0 {
				return
			}
			offset += len(page.Contacts)
		}
	}
}

// StatementAll лінько пагінує операції за період [from, to] на рахунку
// account. На кожному кроці тягне сторінку через [Client.Statement] із
// pageSize (0 → дефолт API), використовуючи `direction=DOWN` від `to`
// у минуле; проміжний курсор зсуває верхню межу до моменту найстарішої
// віддачі. Якщо to нульовий — ітерує від `now` назад.
//
//	for op, err := range cli.StatementAll(ctx, "acc-1", from, time.Time{}, 500) {
//	    if err != nil { return err }
//	    process(op)
//	}
func (c *Client) StatementAll(ctx context.Context, account string, from, to time.Time, pageSize int) iter.Seq2[StatementItem, error] {
	return func(yield func(StatementItem, error) bool) {
		cursorTo := to
		if cursorTo.IsZero() {
			cursorTo = time.Now()
		}
		for {
			if err := ctx.Err(); err != nil {
				_ = yield(StatementItem{}, err)
				return
			}
			if !cursorTo.After(from) {
				return
			}
			page, err := c.Statement(ctx, account, from, cursorTo, StatementDown, pageSize)
			if err != nil {
				_ = yield(StatementItem{}, err)
				return
			}
			if len(page) == 0 {
				return
			}
			for _, item := range page {
				if !yield(item, nil) {
					return
				}
			}
			// DOWN-direction: остання — найстаріша; зсуваємо верхню
			// межу на 1 секунду раніше за неї, щоб уникнути дублів.
			oldest := page[len(page)-1].Time.Time
			next := oldest.Add(-time.Second)
			if !next.Before(cursorTo) {
				return
			}
			cursorTo = next
		}
	}
}

// SearchContactsAll — аналог [Client.ContactsAll] для повнотекстового
// пошуку: лінько ітерує по всіх контактах, що матчать query (ІПН, IBAN,
// номер документа, ПІБ, PAN), через [Client.SearchContacts].
func (c *Client) SearchContactsAll(ctx context.Context, query string, pageSize int) iter.Seq2[Contact, error] {
	return func(yield func(Contact, error) bool) {
		offset := 0
		for {
			if err := ctx.Err(); err != nil {
				_ = yield(Contact{}, err)
				return
			}
			page, err := c.SearchContacts(ctx, query, pageSize, offset)
			if err != nil {
				_ = yield(Contact{}, err)
				return
			}
			for _, contact := range page.Contacts {
				if !yield(contact, nil) {
					return
				}
			}
			if !page.HasMore || len(page.Contacts) == 0 {
				return
			}
			offset += len(page.Contacts)
		}
	}
}
