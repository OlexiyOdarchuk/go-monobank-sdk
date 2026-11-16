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

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
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
// returns 401 "Request is unauthorized yet"; IsAuthPending recognizes
// it, and does not mistake 403 / 404 for pending.
func TestCheckAuth_pendingIs401(t *testing.T) {
	for _, tc := range []struct {
		status  int
		pending bool
	}{
		{http.StatusUnauthorized, true},
		{http.StatusForbidden, false},
		{http.StatusNotFound, false},
	} {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"errorDescription":"x"}`))
		})

		err := c.CheckAuth(context.Background(), "rq-1")
		require.Error(t, err)
		var apiErr *monobank.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, tc.status, apiErr.StatusCode)
		assert.Equal(t, tc.pending, IsAuthPending(err), tc.status)
	}
	assert.False(t, IsAuthPending(nil))
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
