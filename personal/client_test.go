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

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New("test-token", monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
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

func TestTransactionsRange_zeroOrNegative(t *testing.T) {
	c := New("x")
	now := time.Now()
	out, err := c.TransactionsRange(context.Background(), "x", now, time.Time{})
	require.NoError(t, err)
	assert.Nil(t, out)
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
