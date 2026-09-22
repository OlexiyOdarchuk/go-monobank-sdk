package installment_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/installment"
)

func TestCreateQRCart_success(t *testing.T) {
	var capturedBody []byte
	var capturedSig string
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/qr/cart", r.URL.Path)
		assert.Equal(t, testStoreID, r.Header.Get(installment.HeaderStoreID))
		capturedSig = r.Header.Get(installment.HeaderSignature)
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	})

	err := cli.CreateQRCart(context.Background(), &installment.CreateQRCartRequest{
		QRID:         "qr-1",
		StoreOrderID: "fa4a8249-336e-4e6d-9b85-79bc8be62377",
		Products: []installment.Product{
			{Name: "Cat food", Count: 2, Sum: installment.NewMoney(2499, 99)},
		},
		ResultCallback: "https://example.com/cb",
	})
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(capturedBody, &raw))
	assert.Equal(t, "qr-1", raw["qr_id"])
	assert.Equal(t, "fa4a8249-336e-4e6d-9b85-79bc8be62377", raw["store_order_id"])
	assert.Equal(t, "https://example.com/cb", raw["result_callback"])
	products, ok := raw["products"].([]any)
	require.True(t, ok)
	require.Len(t, products, 1)
	assert.InDelta(t, 2499.99, products[0].(map[string]any)["sum"], 0.001)

	assert.Equal(t, expectedSign(t, capturedBody), capturedSig)
}

// 200 instead of 201 means the bank did not create the cart, so it
// must surface as an error rather than a silent success.
func TestCreateQRCart_wrongSuccessStatus(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	err := cli.CreateQRCart(context.Background(), &installment.CreateQRCartRequest{QRID: "qr-1"})
	require.Error(t, err)
	var apiErr *installment.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusOK, apiErr.StatusCode)
}

// Cancel expects 200; a 201 (the create-side success code) must not
// be mistaken for success here.
func TestCancelQRCart_wrongSuccessStatus(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	err := cli.CancelQRCart(context.Background(), "qr-1")
	require.Error(t, err)
	var apiErr *installment.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusCreated, apiErr.StatusCode)
}

func TestCreateQRCart_storeDisabled(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"store is disabled"}`))
	})

	err := cli.CreateQRCart(context.Background(), &installment.CreateQRCartRequest{QRID: "qr-1"})
	require.Error(t, err)
	var apiErr *installment.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	assert.Equal(t, "store is disabled", apiErr.Message)
}

func TestCreateQRCart_nilRequest(t *testing.T) {
	cli, errNew := installment.New(testStoreID, testSecret)
	require.NoError(t, errNew)
	err := cli.CreateQRCart(context.Background(), nil)
	assert.ErrorIs(t, err, installment.ErrNilRequest)
}

func TestCreateQRCart_emptyQRID(t *testing.T) {
	cli, errNew := installment.New(testStoreID, testSecret)
	require.NoError(t, errNew)
	err := cli.CreateQRCart(context.Background(), &installment.CreateQRCartRequest{
		StoreOrderID: "s-1",
	})
	assert.ErrorIs(t, err, installment.ErrEmptyQRID)
}

func TestCancelQRCart_success(t *testing.T) {
	var capturedBody []byte
	var capturedSig string
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/qr/cart/cancel", r.URL.Path)
		assert.Equal(t, testStoreID, r.Header.Get(installment.HeaderStoreID))
		capturedSig = r.Header.Get(installment.HeaderSignature)
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})

	require.NoError(t, cli.CancelQRCart(context.Background(), "qr-2"))
	assert.JSONEq(t, `{"qr_id":"qr-2"}`, string(capturedBody))
	assert.Equal(t, expectedSign(t, capturedBody), capturedSig)
}

func TestCancelQRCart_foreignQR(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"qr_id does not belong to the store"}`))
	})

	err := cli.CancelQRCart(context.Background(), "qr-foreign")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "qr_id does not belong to the store")
}

// Empty qr_id must be rejected locally — the handler stays untouched.
func TestCancelQRCart_emptyQRID(t *testing.T) {
	called := false
	cli, _ := newClient(t, func(http.ResponseWriter, *http.Request) { called = true })

	err := cli.CancelQRCart(context.Background(), "")
	assert.ErrorIs(t, err, installment.ErrEmptyQRID)
	assert.False(t, called)
}
