package personal

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
		// 2 транзакції на вікно.
		body := `[` +
			`{"id":"` + segs[3] + `-a","time":` + strconv.FormatInt(from, 10) + `,"amount":100,"currencyCode":980},` +
			`{"id":"` + segs[3] + `-b","time":` + strconv.FormatInt(from, 10) + `,"amount":200,"currencyCode":980}` +
			`]`
		_, _ = w.Write([]byte(body))
	})

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(70 * 24 * time.Hour) // 3 вікна по 31 день

	var n int
	for tx, err := range c.TransactionsRangeIter(context.Background(), "acc", from, to) {
		require.NoError(t, err)
		assert.NotEmpty(t, tx.ID)
		n++
	}
	assert.Equal(t, 6, n, "3 вікна × 2 транзакції = 6 yields")
	assert.Equal(t, int32(3), hits.Load(), "3 HTTP-виклики")
}

func TestTransactionsRangeIter_breakStopsEarly(t *testing.T) {
	var hits atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`[{"id":"a","time":1,"amount":1},{"id":"b","time":2,"amount":2}]`))
	})

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(90 * 24 * time.Hour) // 3 вікна, але вийдемо після 1-ї tx

	var n int
	for range c.TransactionsRangeIter(context.Background(), "acc", from, to) {
		n++
		if n == 1 {
			break
		}
	}
	assert.Equal(t, 1, n)
	assert.Equal(t, int32(1), hits.Load(), "після break не має робити нові HTTP-виклики")
}

func TestTransactionsRangeIter_zeroOrNegativeRange(t *testing.T) {
	c := New("x")
	now := time.Now()
	for _, err := range c.TransactionsRangeIter(context.Background(), "acc", now, time.Time{}) {
		t.Fatal("expected zero yields, got", err)
	}
	for _, err := range c.TransactionsRangeIter(context.Background(), "acc", now, now.Add(-time.Hour)) {
		t.Fatal("expected zero yields, got", err)
	}
}

func TestTransactionsRangeIter_propagatesError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Hour)

	var seenErr error
	var n int
	for _, err := range c.TransactionsRangeIter(context.Background(), "acc", from, to) {
		n++
		if err != nil {
			seenErr = err
			break
		}
	}
	require.Error(t, seenErr)
	assert.Equal(t, 1, n, "ітератор має зупинитись на помилці першого вікна")
}
