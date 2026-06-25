package bank

import "time"

// StatementMaxRows is the maximum number of transactions a single
// /personal/statement (or the corporate equivalent) response contains.
// Mono caps the result set at this many rows, ordered newest-first, and
// exposes no offset cursor: when a response comes back full, older
// transactions in the requested window were truncated and have to be
// fetched again with a smaller `to`.
//
// This limit is absent from the official OpenAPI spec; it is documented
// by the community:
// https://github.com/andrew-demb/monobank-api-community-docs
const StatementMaxRows = 500

// StatementFetcher fetches a single statement window [from, to]
// (inclusive). It is the per-client closure [DrainWindow] calls one or
// more times to work around the [StatementMaxRows] cap.
type StatementFetcher func(from, to time.Time) (Transactions, error)

// DrainWindow yields every transaction in [from, to], transparently
// working around Mono's [StatementMaxRows]-per-response cap.
//
// Mono returns transactions newest-first and silently truncates the
// response to the newest StatementMaxRows rows without an offset cursor.
// When a response is full, DrainWindow re-anchors the upper bound to the
// oldest second it just saw and fetches again, deduplicating by
// transaction id so the overlap on that boundary second is not
// re-yielded. This mirrors the same-second-safe strategy
// [github.com/OlexiyOdarchuk/go-monobank-sdk/business.Client.StatementAll]
// uses — shifting `to` by a whole second instead would drop any rows
// sharing the boundary second past the cut.
//
// Each transaction is passed to yield once, newest-first; yield reports
// whether to continue (the [iter.Seq] convention). DrainWindow returns
// cont=false when yield stopped iteration early, and a non-nil error
// only when a fetch fails.
//
// The seen-id set is bounded to a single second's worth of rows (it is
// reset whenever the boundary second moves earlier). The only data this
// cannot reach is more than StatementMaxRows transactions sharing one
// exact second: there is no API cursor to page within a second, so once
// the cap's worth of that second is drained the loop guard stops rather
// than spinning. For a per-account statement that is unreachable in
// practice.
func DrainWindow(from, to time.Time, fetch StatementFetcher, yield func(Transaction) bool) (cont bool, err error) {
	// seen holds ids already yielded at exactly boundarySecond. Rows at
	// newer seconds cannot reappear (cursorTo excludes them); rows at
	// older seconds have not been seen yet — so only the boundary second
	// needs remembering, and seen resets when that second moves.
	seen := make(map[string]struct{})
	var boundarySecond time.Time

	for cursorTo := to; ; {
		chunk, err := fetch(from, cursorTo)
		if err != nil {
			return true, err
		}
		if len(chunk) == 0 {
			return true, nil
		}

		newly := 0
		for _, tx := range chunk {
			if _, dup := seen[tx.ID]; dup {
				continue
			}
			if !yield(tx) {
				return false, nil
			}
			newly++
		}

		// Below the cap: the window is fully drained.
		if len(chunk) < StatementMaxRows {
			return true, nil
		}

		// No progress on a full page means more than the cap share the
		// boundary second; the remainder is unreachable. Stop.
		if newly == 0 {
			return true, nil
		}

		// Full page: rows may remain at or before the oldest row (chunk
		// is newest-first, so that is the last). Re-anchor to its second
		// and remember the ids already emitted there.
		oldest := chunk[len(chunk)-1].Time.Time
		if !oldest.Equal(boundarySecond) {
			seen = make(map[string]struct{})
			boundarySecond = oldest
		}
		for _, tx := range chunk {
			if tx.Time.Time.Equal(oldest) {
				seen[tx.ID] = struct{}{}
			}
		}
		cursorTo = oldest
	}
}
