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

func TestTransactionsRangeIter_zeroToReturnsZeroYields(t *testing.T) {
	c, _ := New(stubMaker{})
	for _, err := range c.TransactionsRangeIter(context.Background(), "rq", "acc", time.Now(), time.Time{}) {
		t.Fatal("expected zero yields, got", err)
	}
}
