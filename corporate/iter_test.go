package corporate

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionsRangeIter_streams(t *testing.T) {
	var hits atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		segs := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		from, _ := strconv.ParseInt(segs[3], 10, 64)
		body := `[` +
			`{"id":"` + segs[3] + `-a","time":` + strconv.FormatInt(from, 10) + `,"amount":1,"currencyCode":980}` +
			`]`
		_, _ = w.Write([]byte(body))
	})

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(70 * 24 * time.Hour)

	var n int
	for tx, err := range c.TransactionsRangeIter(context.Background(), "rq-1", "acc", from, to) {
		require.NoError(t, err)
		assert.NotEmpty(t, tx.ID)
		n++
	}
	assert.Equal(t, 3, n)
	assert.Equal(t, int32(3), hits.Load())
}

// A zero to means "until now"; with from at the clock's now the range
// is empty, so nothing is requested and nothing is yielded.
func TestTransactionsRangeIter_zeroToAtNowYieldsNothing(t *testing.T) {
	c := newTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("no request expected for an empty range")
	})
	now := time.Unix(1_700_000_000, 0)
	c.now = func() time.Time { return now }
	for _, err := range c.TransactionsRangeIter(context.Background(), "rq", "acc", now, time.Time{}) {
		t.Fatal("expected zero yields, got", err)
	}
}
