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
		assert.Equal(t, DefaultPOSLanguage, r.Header.Get("Accept-Language"))
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

// The deprecated method discards the body; an empty one must still
// read as success.
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

func TestPOSTransactionCancelWithResponse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/merchant/pos-transaction-cancel", r.URL.Path)
		assert.Equal(t, "en", r.Header.Get("Accept-Language"))
		body, _ := io.ReadAll(r.Body)
		// Language is a header, never part of the body.
		assert.JSONEq(t, `{"rrn":"060189181768","amount":4200}`, string(body))
		_, _ = w.Write([]byte(`{"status":"ok","tranId":"acq123456789"}`))
	})

	out, err := c.POSTransactionCancelWithResponse(context.Background(), &POSTransactionCancelRequest{
		RRN: "060189181768", Amount: 4200, Language: "en",
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", out.Status)
	assert.Equal(t, "acq123456789", out.TranID)
}

func TestPOSTransactionCancelWithResponse_validation(t *testing.T) {
	c := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected request")
	})
	_, err := c.POSTransactionCancelWithResponse(context.Background(), nil)
	assert.ErrorIs(t, err, ErrNilRequest)
	_, err = c.POSTransactionCancelWithResponse(context.Background(), &POSTransactionCancelRequest{Amount: 1})
	assert.ErrorIs(t, err, ErrEmptyID)
}

func TestPOSTransactionCancelStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/merchant/pos-transaction-cancel/status", r.URL.Path)
		assert.Equal(t, "1234567890", r.URL.Query().Get("cancelRef"))
		_, _ = w.Write([]byte(`{"amount":4200,"cancelRef":"1234567890",` +
			`"createTime":"2026-10-01T10:30:00Z","modifyTime":"2026-10-01T11:30:00Z",` +
			`"originalTranId":"acq123456789","rrn":"060189181768","status":"new"}`))
	})

	out, err := c.POSTransactionCancelStatus(context.Background(), "1234567890")
	require.NoError(t, err)
	assert.Equal(t, int64(4200), out.Amount)
	assert.Equal(t, "1234567890", out.CancelRef)
	assert.Equal(t, "2026-10-01T10:30:00Z", out.CreateTime)
	assert.Equal(t, "2026-10-01T11:30:00Z", out.ModifyTime)
	assert.Equal(t, "acq123456789", out.OriginalTranID)
	assert.Equal(t, "060189181768", out.RRN)
	assert.Equal(t, POSRefundNew, out.Status)
}

func TestPOSTransactionCancelStatus_emptyRef(t *testing.T) {
	c := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected request for an empty cancelRef")
	})
	_, err := c.POSTransactionCancelStatus(context.Background(), "")
	assert.ErrorIs(t, err, ErrEmptyID)
}

func TestPOSTransactionCancelStatus_notFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errCode":"NOT_FOUND","errText":"not found"}`))
	})
	_, err := c.POSTransactionCancelStatus(context.Background(), "nope")
	assert.True(t, IsNotFound(err))
}
