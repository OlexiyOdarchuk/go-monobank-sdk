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

// TestTransactionsRange_paginates31DayWindows mirrors the personal test:
// a 70-day window must be split into 3 calls (⌈70/31⌉) and the results
// concatenated in chronological order.
func TestTransactionsRange_paginates31DayWindows(t *testing.T) {
	var hits atomic.Int32
	var seenRequestIDs []string

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		seenRequestIDs = append(seenRequestIDs, r.Header.Get("X-Request-Id"))

		// path: /personal/statement/{accountID}/{from}/{to}
		segs := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		require.Len(t, segs, 5)
		from, _ := strconv.ParseInt(segs[3], 10, 64)

		body := `[{"id":"` + segs[3] + `","time":` + strconv.FormatInt(from, 10) + `,"amount":0}]`
		_, _ = w.Write([]byte(body))
	})

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(70 * 24 * time.Hour) // 70 days

	got, err := c.TransactionsRange(context.Background(), "rq-1", "acc", from, to)
	require.NoError(t, err)
	assert.Equal(t, int32(3), hits.Load(), "expected 3 windows for 70-day range")
	assert.Len(t, got, 3)

	// Chronological order: each window's from < next window's from.
	for i := 1; i < len(got); i++ {
		assert.Less(t, got[i-1].Time.Unix(), got[i].Time.Unix())
	}
}

// A zero to means "until now": the last window ends at the client's
// clock, and every request carries an explicit {to}.
func TestTransactionsRange_zeroToMeansNow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	var lastTo int64
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		segs := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		require.Len(t, segs, 5)
		lastTo, _ = strconv.ParseInt(segs[4], 10, 64)
		_, _ = w.Write([]byte(`[]`))
	})
	c.now = func() time.Time { return now }

	_, err := c.TransactionsRange(context.Background(), "rq", "acc", now.Add(-40*24*time.Hour), time.Time{})
	require.NoError(t, err)
	assert.Equal(t, now.Unix(), lastTo)

	lastTo = 0
	for _, err := range c.TransactionsRangeIter(context.Background(), "rq", "acc", now.Add(-time.Hour), time.Time{}) {
		require.NoError(t, err)
	}
	assert.Equal(t, now.Unix(), lastTo)
}

// A failure in a later window keeps what earlier windows returned.
func TestTransactionsRange_partialOnError(t *testing.T) {
	var hits atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errorDescription":"boom"}`))

			return
		}
		_, _ = w.Write([]byte(`[{"id":"a","time":1700000000,"amount":0}]`))
	})
	from := time.Unix(1_700_000_000, 0)
	got, err := c.TransactionsRange(context.Background(), "rq", "acc", from, from.Add(40*24*time.Hour))
	require.Error(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].ID)
}

func TestTransactionsRange_toBeforeFrom(t *testing.T) {
	c, _ := New(stubMaker{})
	now := time.Now()
	out, err := c.TransactionsRange(context.Background(), "rq", "acc", now, now.Add(-time.Hour))
	require.NoError(t, err)
	assert.Nil(t, out)
}

// Auth must forward callbackURL via the X-Callback header.
func TestAuth_setsXCallback(t *testing.T) {
	var seen string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Callback")
		_, _ = w.Write([]byte(`{"tokenRequestId":"r","acceptUrl":"u"}`))
	})

	_, err := c.Auth(context.Background(), "https://yourapp/cb")
	require.NoError(t, err)
	assert.Equal(t, "https://yourapp/cb", seen)
}

// Regression: Auth must reject an http:// callback on a non-loopback
// host before signing any request, because Mono will send a usable
// requestID to it.
func TestAuth_rejectsInsecureCallback(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("Auth must not reach the server with a bad callback")
	})
	_, err := c.Auth(context.Background(), "http://yourapp/cb")
	assert.ErrorIs(t, err, ErrInsecureCallback)
}

// Loopback http:// callbacks are accepted (httptest, local dev).
func TestAuth_loopbackHTTPCallbackAllowed(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tokenRequestId":"r","acceptUrl":"u"}`))
	})
	for _, cb := range []string{
		"http://localhost:8080/cb",
		"http://127.0.0.1:9000/cb",
		"http://[::1]:8080/cb",
	} {
		_, err := c.Auth(context.Background(), cb)
		assert.NoError(t, err, cb)
	}
}

// AllowInsecureCallback opt-out lets http://non-loopback through.
func TestAuth_insecureCallbackOptOut(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tokenRequestId":"r","acceptUrl":"u"}`))
	})
	c.AllowInsecureCallback(true)
	_, err := c.Auth(context.Background(), "http://staging.internal/cb")
	assert.NoError(t, err)
}

// An empty callback is optional per the spec: no X-Callback header is
// sent and the caller polls CheckAuth instead.
func TestAuth_emptyCallbackOmitsHeader(t *testing.T) {
	var present bool
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, present = r.Header["X-Callback"]
		_, _ = w.Write([]byte(`{"tokenRequestId":"r","acceptUrl":"u"}`))
	})
	tok, err := c.Auth(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "r", tok.RequestID)
	assert.False(t, present, "X-Callback must be omitted when empty")
}

// Unparseable callbacks are rejected before signing.
func TestAuth_invalidCallback(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("Auth must not reach the server with invalid callback")
	})
	for _, bad := range []string{"not-a-url", "no-scheme/foo"} {
		_, err := c.Auth(context.Background(), bad)
		assert.ErrorIs(t, err, ErrInvalidCallback, bad)
	}
}
