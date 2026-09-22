package acquiring

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPOSTransactionCancel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/merchant/pos-transaction-cancel", r.URL.Path)
		assert.Equal(t, "acq-token", r.Header.Get("X-Token"))
		body, _ := io.ReadAll(r.Body)
		var got POSTransactionCancelRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "123456789012", got.RRN)
		assert.Equal(t, int64(4200), got.Amount)
		w.WriteHeader(http.StatusOK)
	})

	err := c.POSTransactionCancel(context.Background(), &POSTransactionCancelRequest{
		RRN:    "123456789012",
		Amount: 4200,
	})
	require.NoError(t, err)
}

// The docs publish no 200 body for this endpoint, so an empty
// response must still read as success.
func TestPOSTransactionCancel_emptyBodyIsSuccess(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	err := c.POSTransactionCancel(context.Background(), &POSTransactionCancelRequest{RRN: "rrn-1", Amount: 1})
	require.NoError(t, err)
}

func TestPOSTransactionCancel_nil(t *testing.T) {
	c := New("x")
	assert.ErrorIs(t, c.POSTransactionCancel(context.Background(), nil), ErrNilRequest)
}

// An empty RRN is rejected locally — no request should reach the
// bank, which would only answer 400 anyway.
func TestPOSTransactionCancel_emptyRRN(t *testing.T) {
	c := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected request for an empty rrn")
	})

	err := c.POSTransactionCancel(context.Background(), &POSTransactionCancelRequest{Amount: 100})
	assert.ErrorIs(t, err, ErrEmptyID)
}

func TestPOSTransactionCancel_notFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errCode":"NOT_FOUND","errText":"transaction not found"}`))
	})

	err := c.POSTransactionCancel(context.Background(), &POSTransactionCancelRequest{RRN: "nope", Amount: 1})
	require.Error(t, err)
	assert.True(t, IsNotFound(err))
}
