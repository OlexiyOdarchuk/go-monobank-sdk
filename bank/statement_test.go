package bank

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vtopc/epoch"
)

// fakeStatement is a backing store of transactions ordered newest-first,
// served exactly the way Mono's /personal/statement behaves: filtered to
// [from, to] (inclusive, second resolution), newest-first, capped to
// StatementMaxRows rows with no offset cursor.
type fakeStatement struct {
	all     Transactions // newest-first
	fetches int
}

func newFakeStatement(n int, newest time.Time, step time.Duration) *fakeStatement {
	txs := make(Transactions, n)
	for i := range txs {
		txs[i] = Transaction{
			ID:   strconv.Itoa(i),
			Time: epoch.NewSeconds(newest.Add(-time.Duration(i) * step)),
		}
	}
	return &fakeStatement{all: txs}
}

func (f *fakeStatement) fetch(from, to time.Time) (Transactions, error) {
	f.fetches++
	var out Transactions
	for _, tx := range f.all { // already newest-first
		t := tx.Time.Time.Unix()
		if t >= from.Unix() && t <= to.Unix() {
			out = append(out, tx)
			if len(out) == StatementMaxRows {
				break
			}
		}
	}
	return out, nil
}

func collect(t *testing.T, fs *fakeStatement, from, to time.Time) Transactions {
	t.Helper()
	var got Transactions
	cont, err := DrainWindow(from, to, fs.fetch, func(tx Transaction) bool {
		got = append(got, tx)
		return true
	})
	require.NoError(t, err)
	require.True(t, cont)
	return got
}

func TestDrainWindow_BelowCap_SingleFetch(t *testing.T) {
	newest := time.Unix(1_700_000_000, 0)
	fs := newFakeStatement(StatementMaxRows-1, newest, time.Minute)

	got := collect(t, fs, newest.Add(-time.Hour*24*40), newest.Add(time.Minute))

	assert.Len(t, got, StatementMaxRows-1)
	assert.Equal(t, 1, fs.fetches, "below the cap must not trigger extra fetches")
}

func TestDrainWindow_OverCap_DrainsAll(t *testing.T) {
	const n = StatementMaxRows*2 + 250 // 1250: needs 3 fetches
	newest := time.Unix(1_700_000_000, 0)
	fs := newFakeStatement(n, newest, time.Minute)

	got := collect(t, fs, newest.Add(-time.Duration(n+10)*time.Minute), newest.Add(time.Minute))

	require.Len(t, got, n, "every transaction in the window must be drained")
	assert.Equal(t, 3, fs.fetches)

	// Newest-first, strictly descending, no duplicates.
	seen := make(map[string]bool, n)
	for i, tx := range got {
		assert.False(t, seen[tx.ID], "duplicate id %s", tx.ID)
		seen[tx.ID] = true
		if i > 0 {
			assert.False(t, tx.Time.Time.After(got[i-1].Time.Time),
				"order broken at %d", i)
		}
	}
}

func TestDrainWindow_EarlyStop(t *testing.T) {
	newest := time.Unix(1_700_000_000, 0)
	fs := newFakeStatement(StatementMaxRows*2, newest, time.Minute)

	var got Transactions
	cont, err := DrainWindow(newest.Add(-time.Hour*48), newest.Add(time.Minute),
		fs.fetch,
		func(tx Transaction) bool { got = append(got, tx); return len(got) < 10 },
	)
	require.NoError(t, err)
	assert.False(t, cont, "yield returning false must report cont=false")
	assert.Len(t, got, 10)
	assert.Equal(t, 1, fs.fetches, "early stop must not fetch the next page")
}

func TestDrainWindow_FetchError(t *testing.T) {
	want := assert.AnError
	cont, err := DrainWindow(time.Unix(0, 0), time.Unix(100, 0),
		func(_, _ time.Time) (Transactions, error) { return nil, want },
		func(Transaction) bool { return true },
	)
	assert.ErrorIs(t, err, want)
	_ = cont
}

// TestDrainWindow_SameSecondOverflowWithinReach is the case the naive
// "shift to by -1s" approach would silently drop: a full page whose
// oldest row shares its second with a few rows past the page cut, but
// the second as a whole holds fewer than the cap. Dedup must recover
// them instead of losing them.
func TestDrainWindow_SameSecondOverflowWithinReach(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	const tail = 3 // rows sharing the oldest second, beyond the page cut
	txs := make(Transactions, StatementMaxRows+tail)
	for i := range StatementMaxRows - 1 {
		txs[i] = Transaction{ID: strconv.Itoa(i), Time: epoch.NewSeconds(t0.Add(-time.Duration(i) * time.Second))}
	}
	// Last (StatementMaxRows-1) of the page plus `tail` extras all land
	// on the same second, so the first page cuts mid-second.
	boundary := t0.Add(-time.Duration(StatementMaxRows-1) * time.Second)
	for i := StatementMaxRows - 1; i < StatementMaxRows+tail; i++ {
		txs[i] = Transaction{ID: strconv.Itoa(i), Time: epoch.NewSeconds(boundary)}
	}
	fs := &fakeStatement{all: txs}

	got := collect(t, fs, t0.Add(-time.Hour), t0.Add(time.Second))

	require.Len(t, got, StatementMaxRows+tail, "boundary-second overflow must be recovered, not dropped")
	assert.Equal(t, 2, fs.fetches)
	seen := make(map[string]bool, len(got))
	for _, tx := range got {
		assert.False(t, seen[tx.ID], "duplicate id %s", tx.ID)
		seen[tx.ID] = true
	}
}

// TestDrainWindow_SaturatedSecond documents the only unreachable case:
// when more than the cap share one exact second, rows past the cap cannot
// be paged (no offset cursor), so the loop guard stops instead of spinning.
func TestDrainWindow_SaturatedSecond(t *testing.T) {
	at := time.Unix(1_700_000_000, 0)
	fs := newFakeStatement(StatementMaxRows+100, at, 0) // all share the same second

	got := collect(t, fs, at, at)

	assert.Len(t, got, StatementMaxRows, "cannot page within a single second")
	assert.Equal(t, 2, fs.fetches, "guard stops after the re-anchored refetch makes no progress")
}
