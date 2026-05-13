package acquiring_test

import (
	"encoding/json"
	"testing"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Усі типи відповідей із Ccy-полем повинні прив'язувати Code до Money.

func TestInvoiceStatusResponse_propagatesCcy(t *testing.T) {
	in := []byte(`{
		"invoiceId":"i-1",
		"status":"success",
		"amount":4250,
		"ccy":980,
		"finalAmount":4000,
		"paymentInfo":{
			"maskedPan":"4141**4141",
			"terminal":"T1",
			"paymentSystem":"visa",
			"paymentMethod":"pan",
			"fee":50,
			"agentFee":10
		},
		"cancelList":[{"status":"success","amount":250,"ccy":980}]
	}`)

	var resp acquiring.InvoiceStatusResponse
	require.NoError(t, json.Unmarshal(in, &resp))

	assert.Equal(t, currency.UAH, resp.Amount.Code)
	assert.Equal(t, currency.UAH, resp.FinalAmount.Code)
	require.NotNil(t, resp.PaymentInfo)
	assert.Equal(t, currency.UAH, resp.PaymentInfo.Fee.Code)
	assert.Equal(t, currency.UAH, resp.PaymentInfo.AgentFee.Code)
	require.Len(t, resp.CancelList, 1)
	assert.Equal(t, currency.UAH, resp.CancelList[0].Amount.Code)
}

func TestStatementInvoice_propagatesCcy(t *testing.T) {
	in := []byte(`{
		"invoiceId":"s-1",
		"status":"success",
		"maskedPan":"4141**4141",
		"date":"2026-01-15T10:00:00Z",
		"paymentScheme":"full",
		"amount":10000,
		"profitAmount":9800,
		"ccy":840,
		"cancelList":[{"amount":500,"ccy":840,"date":"x","maskedPan":"4141**4141"}]
	}`)

	var s acquiring.StatementInvoice
	require.NoError(t, json.Unmarshal(in, &s))

	assert.Equal(t, currency.USD, s.Amount.Code)
	assert.Equal(t, currency.USD, s.ProfitAmount.Code)
	require.Len(t, s.CancelList, 1)
	assert.Equal(t, currency.USD, s.CancelList[0].Amount.Code)
}

func TestPaymentDirectResponse_propagatesCcy(t *testing.T) {
	in := []byte(`{
		"invoiceId":"pd-1",
		"status":"success",
		"amount":1000,
		"ccy":978,
		"createdDate":"a","modifiedDate":"b"
	}`)

	var p acquiring.PaymentDirectResponse
	require.NoError(t, json.Unmarshal(in, &p))

	assert.Equal(t, currency.EUR, p.Amount.Code)
	assert.Equal(t, int64(1000), p.Amount.Minor)
}

func TestQRDetails_propagatesCcy(t *testing.T) {
	in := []byte(`{"shortQrId":"s","invoiceId":"i","amount":100,"ccy":980}`)

	var q acquiring.QRDetails
	require.NoError(t, json.Unmarshal(in, &q))
	assert.Equal(t, currency.UAH, q.Amount.Code)
}
