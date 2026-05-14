package business

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContactsAll_paginatesUntilExhausted(t *testing.T) {
	// 3 сторінки: 0-9, 10-19, 20-24; на третій hasMore=false.
	var calls atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		require.Equal(t, 10, limit)

		batchSize := 10
		hasMore := true
		switch offset {
		case 0:
			// 10 records
		case 10:
			// 10 records
		case 20:
			batchSize = 5
			hasMore = false
		default:
			t.Fatalf("unexpected offset %d", offset)
		}

		var items []string
		for i := 0; i < batchSize; i++ {
			items = append(items, fmt.Sprintf(`{"id":"c-%d","fullName":"User %d"}`, offset+i, offset+i))
		}
		body := fmt.Sprintf(`{"hasMore":%t,"contacts":[%s]}`, hasMore, strings.Join(items, ","))
		_, _ = w.Write([]byte(body))
	})

	var got []string
	for c, err := range c.ContactsAll(context.Background(), 10) {
		require.NoError(t, err)
		got = append(got, c.ID)
	}

	require.Len(t, got, 25)
	assert.Equal(t, "c-0", got[0])
	assert.Equal(t, "c-24", got[24])
	assert.Equal(t, int32(3), calls.Load())
}

func TestContactsAll_propagatesError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"errorDescription":"down"}`))
	})

	var iters int
	for contact, err := range c.ContactsAll(context.Background(), 0) {
		iters++
		require.Error(t, err)
		assert.Empty(t, contact.ID)
		break // first error stops iteration anyway
	}
	assert.Equal(t, 1, iters, "ітератор має одразу зупинитись на помилці")
}

func TestContactsAll_breakStopsPagination(t *testing.T) {
	var calls atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"hasMore":true,"contacts":[{"id":"a"},{"id":"b"},{"id":"c"}]}`))
	})

	var n int
	for _, err := range c.ContactsAll(context.Background(), 0) {
		require.NoError(t, err)
		n++
		if n == 2 {
			break
		}
	}
	assert.Equal(t, 2, n)
	assert.Equal(t, int32(1), calls.Load(), "після break не має робити нові HTTP-виклики")
}

func TestContactsAll_contextCancel(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"hasMore":true,"contacts":[{"id":"a"}]}`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var iters int
	for _, err := range c.ContactsAll(ctx, 0) {
		iters++
		if iters == 1 {
			cancel() // скасувати після першого
			continue
		}
		// другий-третій yield мають побачити err == context.Canceled.
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
		return
	}
}

// StatementAll лінько пагінує операції через DOWN-курсор.
func TestStatementAll_paginatesDownByTime(t *testing.T) {
	from := time.Unix(1_700_000_000, 0)
	to := time.Unix(1_700_009_000, 0)

	// Симулюємо 3 сторінки по 2 елементи, з спаданням time-у.
	pages := [][]int64{
		{1_700_008_000, 1_700_007_000},
		{1_700_006_000, 1_700_005_000},
		{1_700_004_000, 1_700_003_000},
	}
	var pageIdx atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		idx := int(pageIdx.Add(1)) - 1
		if idx >= len(pages) {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		var items []string
		for _, ts := range pages[idx] {
			items = append(items,
				fmt.Sprintf(`{"id":"op-%d","time":%d,"amount":-100,"currencyCode":"UAH"}`, ts, ts))
		}
		_, _ = w.Write([]byte("[" + strings.Join(items, ",") + "]"))
	})

	var got []string
	for op, err := range c.StatementAll(context.Background(), "acc-1", from, to, 0) {
		require.NoError(t, err)
		got = append(got, op.ID)
	}
	require.Len(t, got, 6)
	assert.Equal(t, "op-1700008000", got[0])
	assert.Equal(t, "op-1700003000", got[5])
}

// порожня сторінка зупиняє ітерацію.
func TestStatementAll_stopsOnEmptyPage(t *testing.T) {
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Hour)
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	var n int
	for _, err := range c.StatementAll(context.Background(), "x", from, to, 0) {
		require.NoError(t, err)
		n++
	}
	assert.Equal(t, 0, n)
}

// break зупиняє ітерацію без додаткових HTTP-викликів.
func TestStatementAll_breakStops(t *testing.T) {
	from := time.Unix(1_700_000_000, 0)
	to := time.Unix(1_700_010_000, 0)
	var calls atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(
			`[{"id":"a","time":1700009000,"amount":-1,"currencyCode":"UAH"},` +
				`{"id":"b","time":1700008000,"amount":-1,"currencyCode":"UAH"}]`))
	})
	var seen int
	for _, err := range c.StatementAll(context.Background(), "x", from, to, 0) {
		require.NoError(t, err)
		seen++
		if seen == 1 {
			break
		}
	}
	assert.Equal(t, 1, seen)
	assert.Equal(t, int32(1), calls.Load(), "break — без додаткових сторінок")
}

func TestStatementAll_propagatesError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Hour)
	var iters int
	for _, err := range c.StatementAll(context.Background(), "x", from, to, 0) {
		iters++
		require.Error(t, err)
	}
	assert.Equal(t, 1, iters)
}

func TestSearchContactsAll_passesQuery(t *testing.T) {
	var seenQuery string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenQuery = r.URL.Query().Get("query")
		_, _ = w.Write([]byte(`{"hasMore":false,"contacts":[{"id":"x"}]}`))
	})

	var got []Contact
	for c, err := range c.SearchContactsAll(context.Background(), "Петренко", 0) {
		require.NoError(t, err)
		got = append(got, c)
	}
	require.Len(t, got, 1)
	assert.Equal(t, "Петренко", seenQuery)
}
