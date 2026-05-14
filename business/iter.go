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
//
// Same-second overflow: when an entire page falls into a single
// second AND fills pageSize, the iterator does NOT shift the upper
// bound by -1s (that would lose the rest of the items on that
// second). Instead it keeps cursorTo equal to that second, refetches
// the same window, and skips already-yielded IDs via an internal
// "seen" set. The set is cleared as soon as cursorTo moves to an
// earlier second, so memory stays bounded by a single second's
// worth of operations.
func (c *Client) StatementAll(ctx context.Context, account string, from, to time.Time, pageSize int) iter.Seq2[StatementItem, error] {
	return func(yield func(StatementItem, error) bool) {
		cursorTo := to
		if cursorTo.IsZero() {
			cursorTo = time.Now()
		}
		// seen holds IDs already yielded at exactly cursorSecond. As
		// soon as we shift cursorTo to an earlier second, seen is
		// cleared — same-second collisions on different seconds do
		// not interfere with each other.
		seen := make(map[string]struct{})
		var cursorSecond time.Time
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

			newlyYielded := 0
			for _, item := range page {
				if _, dup := seen[item.ID]; dup {
					continue
				}
				if !yield(item, nil) {
					return
				}
				newlyYielded++
			}

			oldest := page[len(page)-1].Time.Time
			newest := page[0].Time.Time

			// Same-second overflow: every item in this page is at the
			// same second AND the page is full. Stay on the same
			// second, track seen IDs, refetch — without -1s shift we
			// would otherwise drop the remainder of that second.
			//
			// pageSize == 0 means "API default" — treat any
			// non-progressing same-second response as overflow so we
			// don't lose data.
			sameSecond := oldest.Equal(newest)
			fullPage := pageSize <= 0 || len(page) >= pageSize
			if sameSecond && fullPage {
				// Loop guard: if we already saw every ID in this page
				// (newlyYielded == 0), the API can't give us more for
				// this second — stop rather than spin forever.
				if newlyYielded == 0 {
					return
				}
				if !oldest.Equal(cursorSecond) {
					seen = map[string]struct{}{}
					cursorSecond = oldest
				}
				for _, item := range page {
					seen[item.ID] = struct{}{}
				}
				cursorTo = oldest
				continue
			}

			// Normal case: shift the upper bound one second earlier
			// to avoid re-yielding the oldest item on the next call.
			seen = map[string]struct{}{}
			cursorSecond = time.Time{}
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
