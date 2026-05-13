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
