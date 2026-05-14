package business

import (
	"context"
	"iter"
	"time"
)

// ContactsAll returns an iter.Seq2 iterator over every payroll
// contact in the company. Pages are pulled lazily one by one via
// [Client.Contacts] with step pageSize (0 → API default). If ctx is
// canceled or one of the calls returns an error, the iterator yields
// (Contact{}, err) and stops.
//
//	for c, err := range cli.ContactsAll(ctx, 0) {
//	    if err != nil { return err }
//	    process(c)
//	}
//
// To stop early, use an ordinary break.
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

// StatementAll lazily paginates operations within [from, to] on
// account. On each step it pulls a page via [Client.Statement] with
// pageSize (0 → API default) using `direction=DOWN` from `to` back
// in time; the internal cursor shifts the upper bound to the time
// of the oldest item returned. If to is zero, it iterates from `now`
// backwards.
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
			// DOWN direction: the last item is the oldest; shift the
			// upper bound one second earlier to avoid duplicates.
			oldest := page[len(page)-1].Time.Time
			next := oldest.Add(-time.Second)
			if !next.Before(cursorTo) {
				return
			}
			cursorTo = next
		}
	}
}

// SearchContactsAll is the [Client.ContactsAll] counterpart for
// full-text search: it lazily iterates over every contact that
// matches query (INN, IBAN, document number, full name, PAN) via
// [Client.SearchContacts].
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
