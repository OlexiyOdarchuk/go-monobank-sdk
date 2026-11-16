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
// account, pulling pages via [Client.Statement] with pageSize (0 → API
// default). If to is zero, the window ends now.
//
//	for op, err := range cli.StatementAll(ctx, "acc-1", from, time.Time{}, 500) {
//	    if err != nil { return err }
//	    process(op)
//	}
//
// Page order: the spec describes direction=DOWN (the API default) as
// "спочатку повертати старі операції" (oldest first), while earlier
// SDK versions assumed newest first, and the live behavior has not
// been confirmed (see TestIntegration_Statement_directionOrder). The
// iterator therefore does not depend on it: it reads the order from
// each page and narrows the window from the side the page came from —
// the upper bound down to the oldest item for a newest-first page, the
// lower bound up to the newest item for an oldest-first page. A page
// whose items share one second does not reveal an order; until one
// does, newest first is assumed. Items are yielded in the order the
// API returns them.
//
// The window keeps the boundary second (both bounds are inclusive), so
// operations sharing a second with the page edge are not lost; the IDs
// already yielded at that second are skipped when they come back.
//
// Same-second overflow: the endpoint has no offset, so when a single
// second holds more operations than one page, only the first page of
// them is reachable. The iterator then steps past that second and
// carries on with the rest of the window rather than stopping.
func (c *Client) StatementAll(ctx context.Context, account string, from, to time.Time, pageSize int) iter.Seq2[StatementItem, error] {
	return func(yield func(StatementItem, error) bool) {
		lo, hi := from, to
		if hi.IsZero() {
			hi = time.Now()
		}
		// order is the page order seen so far: -1 newest first,
		// +1 oldest first, 0 not known yet.
		order := 0
		// seen holds the IDs already yielded at boundary, the second
		// the window was last narrowed to.
		seen := make(map[string]struct{})
		var boundary time.Time
		for {
			if err := ctx.Err(); err != nil {
				_ = yield(StatementItem{}, err)
				return
			}
			if !hi.After(lo) {
				return
			}
			page, err := c.Statement(ctx, account, lo, hi, StatementDown, pageSize)
			if err != nil {
				_ = yield(StatementItem{}, err)
				return
			}
			if len(page) == 0 {
				return
			}

			oldest, newest := page[0].Time.Time, page[0].Time.Time
			fresh := 0
			for _, item := range page {
				ts := item.Time.Time
				if ts.Before(oldest) {
					oldest = ts
				}
				if ts.After(newest) {
					newest = ts
				}
				if _, dup := seen[item.ID]; dup {
					continue
				}
				if !yield(item, nil) {
					return
				}
				fresh++
			}
			switch first, last := page[0].Time.Time, page[len(page)-1].Time.Time; {
			case first.After(last):
				order = -1
			case first.Before(last):
				order = 1
			}
			newestFirst := order <= 0

			// pageSize <= 0 means "API default": the page size is
			// unknown, so any page may be full.
			full := pageSize <= 0 || len(page) >= pageSize
			if fresh == 0 {
				if !full {
					// Only the boundary items came back: the window
					// is exhausted.
					return
				}
				// A full page of items already yielded: one second
				// holds more than a page. Step past it.
				if newestFirst {
					next := oldest.Add(-time.Second)
					if !next.Before(hi) {
						return
					}
					hi = next
				} else {
					next := newest.Add(time.Second)
					if !next.After(lo) {
						return
					}
					lo = next
				}
				clear(seen)
				boundary = time.Time{}
				continue
			}

			b := newest
			if newestFirst {
				b = oldest
				if b.After(hi) {
					return // the server answered outside the window
				}
				hi = b
			} else {
				if b.Before(lo) {
					return
				}
				lo = b
			}
			if !b.Equal(boundary) {
				clear(seen)
				boundary = b
			}
			for _, item := range page {
				if item.Time.Equal(b) {
					seen[item.ID] = struct{}{}
				}
			}
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
