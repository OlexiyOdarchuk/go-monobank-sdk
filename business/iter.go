package business

import (
	"context"
	"iter"
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
