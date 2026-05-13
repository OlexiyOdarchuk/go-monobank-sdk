package personal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientInfo_apiError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errorDescription":"Unknown 'X-Token'"}`))
	})

	_, err := c.ClientInfo(context.Background())
	require.Error(t, err)
	var apiErr *monobank.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	assert.Contains(t, string(apiErr.Body), "Unknown 'X-Token'")
}

// Per-account /personal/statement returns 429 with Retry-After when over
// the quota. The WithRetry policy must retry transparently.
func TestTransactions_retriesOn429(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"errorDescription":"rate limited"}`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := New("tok",
		monobank.WithBaseURL(srv.URL),
		monobank.WithHTTPClient(srv.Client()),
		monobank.WithRetry(3, time.Millisecond, 10*time.Millisecond),
	)

	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Hour)
	_, err := c.Transactions(context.Background(), "acc", from, to)
	require.NoError(t, err)
	assert.Equal(t, int32(2), hits.Load(), "expected one retry after 429")
}
