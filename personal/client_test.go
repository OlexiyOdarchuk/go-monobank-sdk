package personal

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New("test-token", monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
	// Keep the default pacing logic but never sleep for real.
	c.sleep = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }

	return c
}

func TestClientInfo_sendsToken(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-token", r.Header.Get("X-Token"))
		assert.Equal(t, "/personal/client-info", r.URL.Path)
		_, _ = w.Write([]byte(`{"clientId":"c1","name":"Test","accounts":[]}`))
	})

	out, err := c.ClientInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "c1", out.ID)
	assert.Equal(t, "Test", out.Name)
}

func TestTransactions(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		segs := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		require.Len(t, segs, 5)
		assert.Equal(t, "personal", segs[0])
		assert.Equal(t, "statement", segs[1])
		assert.Equal(t, "acc-1", segs[2])
		_, _ = w.Write([]byte(`[{"id":"t1","time":1700000000,"amount":-100}]`))
	})

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Hour)
	out, err := c.Transactions(context.Background(), "acc-1", from, to)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, int64(-100), out[0].Amount.Minor)
}

func TestTransactionsRange_paginates31DayWindows(t *testing.T) {
	var hits atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		segs := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		from, _ := strconv.ParseInt(segs[3], 10, 64)
		_, _ = w.Write([]byte(`[{"id":"` + segs[3] + `","time":` + strconv.FormatInt(from, 10) + `,"amount":0}]`))
	})

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(70 * 24 * time.Hour) // 70 days → ⌈70/31⌉ = 3 windows
	got, err := c.TransactionsRange(context.Background(), "a", from, to)
	require.NoError(t, err)
	assert.Equal(t, int32(3), hits.Load())
	assert.Len(t, got, 3)
}

// Regression: consecutive windows must not overlap on the boundary
// second. Mono's /personal/statement is inclusive on both ends —
// before the +1s shift, a transaction landing exactly at the window
// boundary was returned in BOTH the current and the next chunk.
func TestTransactionsRange_noBoundaryDuplicate(t *testing.T) {
	var calls atomic.Int32
	// Server records which from/to it was asked for; checks must
	// not see two requests sharing the same boundary second.
	seenFrom := map[int64]bool{}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		segs := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		from, _ := strconv.ParseInt(segs[3], 10, 64)
		to, _ := strconv.ParseInt(segs[4], 10, 64)
		// from of window N must NOT equal to of window N-1.
		assert.False(t, seenFrom[from],
			"window boundary %d appears in two requests — caller will see duplicates", from)
		seenFrom[from] = true
		assert.True(t, to >= from, "to must be >= from")
		_, _ = w.Write([]byte(`[]`))
	})

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(70 * 24 * time.Hour) // 3 windows
	_, err := c.TransactionsRange(context.Background(), "a", from, to)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, calls.Load(), int32(2))
}

func TestTransactionsRange_toNotAfterFrom(t *testing.T) {
	c := New("x") // no request may be made
	now := time.Now()
	out, err := c.TransactionsRange(context.Background(), "x", now, now.Add(-time.Hour))
	require.NoError(t, err)
	assert.Nil(t, out)
	out, err = c.TransactionsRange(context.Background(), "x", now, now)
	require.NoError(t, err)
	assert.Nil(t, out)
}

// The spec makes {to} optional: "якщо відсутній, буде використовуватись
// поточний час". A zero to must drop the segment, not send year 1.
func TestTransactions_zeroToOmitsSegment(t *testing.T) {
	var seenPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		_, _ = w.Write([]byte(`[]`))
	})
	_, err := c.Transactions(context.Background(), "0", time.Unix(1_700_000_000, 0), time.Time{})
	require.NoError(t, err)
	assert.Equal(t, "/personal/statement/0/1700000000", seenPath)
}

func TestTransactionsRange_zeroToMeansNow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	var paths []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(`[]`))
	})
	c.now = func() time.Time { return now }

	_, err := c.TransactionsRange(context.Background(), "acc", now.Add(-40*24*time.Hour), time.Time{})
	require.NoError(t, err)
	require.Len(t, paths, 2, "40 days = two 31-day windows")
	assert.True(t, strings.HasSuffix(paths[1], "/"+strconv.FormatInt(now.Unix(), 10)),
		"the last window must end at now, got %s", paths[1])
}

// fakeClock drives the pacing seams without real sleeps.
type fakeClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (f *fakeClock) install(c *Client) {
	c.now = func() time.Time { return f.now }
	c.sleep = func(ctx context.Context, d time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		f.sleeps = append(f.sleeps, d)
		f.now = f.now.Add(d)

		return nil
	}
}

func TestTransactionsRange_pacesRequests(t *testing.T) {
	var hits atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`[]`))
	})
	clk := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	clk.install(c)

	from := time.Unix(1_700_000_000, 0)
	_, err := c.TransactionsRange(context.Background(), "acc", from, from.Add(70*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int32(3), hits.Load())
	assert.Equal(t, []time.Duration{monobank.DefaultStatementPacing, monobank.DefaultStatementPacing}, clk.sleeps,
		"no wait before the first request, 60 s before each of the next two")
}

func TestTransactionsRange_pacingDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	c := New("tok", monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()),
		monobank.WithStatementPacing(0))
	clk := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	clk.install(c)

	from := time.Unix(1_700_000_000, 0)
	_, err := c.TransactionsRange(context.Background(), "acc", from, from.Add(70*24*time.Hour))
	require.NoError(t, err)
	assert.Empty(t, clk.sleeps)
}

func TestTransactionsRange_pacingHonoursContext(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"a","time":1700000000,"amount":1}]`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	c.sleep = func(context.Context, time.Duration) error {
		cancel()

		return context.Canceled
	}

	from := time.Unix(1_700_000_000, 0)
	out, err := c.TransactionsRange(ctx, "acc", from, from.Add(70*24*time.Hour))
	require.ErrorIs(t, err, context.Canceled)
	assert.Len(t, out, 1, "the first window is kept")
}

func TestTransactionsRange_returnsPartialOnError(t *testing.T) {
	var hits atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			_, _ = w.Write([]byte(`[{"id":"a","time":1700000000,"amount":1}]`))

			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"errorDescription":"Too many requests"}`))
	})

	from := time.Unix(1_700_000_000, 0)
	out, err := c.TransactionsRange(context.Background(), "acc", from, from.Add(40*24*time.Hour))
	require.ErrorIs(t, err, monobank.ErrTooManyRequests)
	require.Len(t, out, 1)
	assert.Equal(t, "a", out[0].ID)
}

// accountID мусить бути url.PathEscape-нутим у path-сегменті
// /personal/statement/{accountID}/{from}/{to}. Без цього зловмисний
// accountID типу "../foo" зміг би пробити SDK на інший endpoint, а
// account-IDs зі слешем / спецсимволом ламали би 31-day pagination.
func TestTransactions_accountIDIsPathEscaped(t *testing.T) {
	var seenPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`[]`))
	})

	weird := "acc/../foo bar"
	from := time.Unix(1_700_000_000, 0)
	_, err := c.Transactions(context.Background(), weird, from, from.Add(time.Hour))
	require.NoError(t, err)
	// url.PathEscape: "acc/../foo bar" → "acc%2F..%2Ffoo%20bar"
	assert.Contains(t, seenPath, "acc%2F..%2Ffoo%20bar")
	assert.NotContains(t, seenPath, "acc/../foo")
}

func TestSetWebHook(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/personal/webhook", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var got webhookRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "https://example.com/hook", got.WebHookURL)
		w.WriteHeader(http.StatusOK)
	})

	require.NoError(t, c.SetWebHook(context.Background(), "https://example.com/hook"))
}
