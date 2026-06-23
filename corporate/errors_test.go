package corporate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

func TestClientInfo_apiError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errorDescription":"client did not approve access"}`))
	})

	_, err := c.ClientInfo(context.Background(), "rq-1")
	require.Error(t, err)
	var apiErr *monobank.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

// CheckAuth returns nil only on 200. While waiting for approval the bank
// returns 403, which surfaces as a non-nil error caller can inspect.
func TestCheckAuth_403WhileWaiting(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errorDescription":"request is pending"}`))
	})

	err := c.CheckAuth(context.Background(), "rq-1")
	require.Error(t, err)
	var apiErr *monobank.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestTransactions_retriesOn5xx(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c, err := New(stubMaker{},
		monobank.WithBaseURL(srv.URL),
		monobank.WithHTTPClient(srv.Client()),
		monobank.WithRetry(3, time.Millisecond, 10*time.Millisecond),
	)
	require.NoError(t, err)

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Hour)
	_, err = c.Transactions(context.Background(), "rq-1", "acc", from, to)
	require.NoError(t, err)
	assert.Equal(t, int32(2), hits.Load(), "expected one retry after 502")
}
