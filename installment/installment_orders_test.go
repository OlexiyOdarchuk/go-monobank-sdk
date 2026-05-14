package installment_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/installment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderInfo(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/order/info", r.URL.Path)
		_, _ = w.Write([]byte(`{"total_sum":1234.5,"store_order_id":"s-1","maskedCard":"414141**4141"}`))
	})

	out, err := cli.OrderInfo(context.Background(), "o-1")
	require.NoError(t, err)
	assert.Equal(t, "s-1", out.StoreOrderID)
	assert.Equal(t, "414141**4141", out.MaskedCard)
}

func TestOrderData(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/order/data", r.URL.Path)
		_, _ = w.Write([]byte(`{"total_sum":2500,"store_order_id":"s-2","iban":"UA12"}`))
	})

	out, err := cli.OrderData(context.Background(), "o-2")
	require.NoError(t, err)
	assert.Equal(t, "s-2", out.StoreOrderID)
	assert.Equal(t, "UA12", out.IBAN)
	assert.Equal(t, int64(250000), out.TotalSum.Kopecks) // 2500.00 UAH
}

func TestWithHTTPClient_overridesDefault(t *testing.T) {
	custom := &http.Client{Timeout: 1 * time.Second}
	cli, err := installment.New(testStoreID, testSecret, installment.WithHTTPClient(custom))
	require.NoError(t, err)
	require.NotNil(t, cli)
	assert.NotEmpty(t, cli.Sign([]byte(`{}`)))
}

func TestReturnOrder_serverError(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Trace-Id", "trace-xyz")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"return denied"}`))
	})

	_, err := cli.ReturnOrder(context.Background(), &installment.ReturnRequest{OrderID: "o-1"})
	require.Error(t, err)
	var apiErr *installment.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Equal(t, "return denied", apiErr.Message)
	assert.Equal(t, "trace-xyz", apiErr.TraceID)
}

func TestReturnOrder_nilRequest(t *testing.T) {
	cli, errNew := installment.New(testStoreID, testSecret)
	require.NoError(t, errNew)
	_, err := cli.ReturnOrder(context.Background(), nil)
	assert.ErrorIs(t, err, installment.ErrNilRequest)
}

func TestGuaranteeLetterPDF_serverError(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`internal err`))
	})

	_, err := cli.GuaranteeLetterPDF(context.Background(), &installment.OrderDataRequest{OrderID: "o-1"})
	require.Error(t, err)
	var apiErr *installment.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
	assert.Contains(t, apiErr.Error(), "500")
}

func TestGuaranteeLetterPDF_nil(t *testing.T) {
	cli, errNew := installment.New(testStoreID, testSecret)
	require.NoError(t, errNew)
	_, err := cli.GuaranteeLetterPDF(context.Background(), nil)
	assert.ErrorIs(t, err, installment.ErrNilRequest)
}

func TestGuaranteeLetterData_nil(t *testing.T) {
	cli, errNew := installment.New(testStoreID, testSecret)
	require.NoError(t, errNew)
	_, err := cli.GuaranteeLetterData(context.Background(), nil)
	assert.ErrorIs(t, err, installment.ErrNilRequest)
}

func TestGuaranteeLetterDataV2_nil(t *testing.T) {
	cli, errNew := installment.New(testStoreID, testSecret)
	require.NoError(t, errNew)
	_, err := cli.GuaranteeLetterDataV2(context.Background(), nil)
	assert.ErrorIs(t, err, installment.ErrNilRequest)
}

func TestGuaranteeLetterData_serverError(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"order not found"}`))
	})

	_, err := cli.GuaranteeLetterData(context.Background(), &installment.OrderDataRequest{OrderID: "missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "order not found")
}

func TestGuaranteeLetterDataV2_serverError(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"bad signature"}`))
	})

	_, err := cli.GuaranteeLetterDataV2(context.Background(), &installment.OrderDataRequest{OrderID: "o-x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad signature")
}
