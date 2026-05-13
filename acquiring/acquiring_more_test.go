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

func TestReceipt(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/merchant/invoice/receipt", r.URL.Path)
		assert.Equal(t, "i-42", r.URL.Query().Get("invoiceId"))
		assert.Equal(t, "buyer@example.com", r.URL.Query().Get("email"))
		_, _ = w.Write([]byte(`{"file":"JVBERi0xLjQK..."}`)) // base64 PDF
	})

	out, err := c.Receipt(context.Background(), "i-42", "buyer@example.com")
	require.NoError(t, err)
	assert.Contains(t, out.File, "JVBERi0xLjQ")
}

func TestReceipt_emailOmittedWhenBlank(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.Query().Get("email"))
		_, _ = w.Write([]byte(`{"file":"AA=="}`))
	})

	_, err := c.Receipt(context.Background(), "i-1", "")
	require.NoError(t, err)
}

func TestPaymentDirect(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/merchant/invoice/payment-direct", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var got PaymentDirectRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, int64(1000), got.Amount)
		assert.Equal(t, "4444333322221111", got.CardData.PAN)
		assert.Equal(t, InitClient, got.InitiationKind)
		_, _ = w.Write([]byte(`{"invoiceId":"i-99","status":"processing","amount":1000,"ccy":980,"createdDate":"x","modifiedDate":"y","tdsUrl":"https://3ds/x"}`))
	})

	out, err := c.PaymentDirect(context.Background(), &PaymentDirectRequest{
		Amount: 1000,
		Ccy:    980,
		CardData: CardData{
			PAN: "4444333322221111", Exp: "1228", CVV: "123",
		},
		PaymentType:    PaymentDebit,
		InitiationKind: InitClient,
	})
	require.NoError(t, err)
	assert.Equal(t, "i-99", out.InvoiceID)
	assert.Equal(t, "processing", out.Status)
	assert.Equal(t, "https://3ds/x", out.TDSUrl)
}

func TestPaymentDirect_nil(t *testing.T) {
	c := New("x")
	_, err := c.PaymentDirect(context.Background(), nil)
	assert.ErrorIs(t, err, ErrNilRequest)
}

func TestSyncPayment_appleFlow(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/merchant/invoice/sync-payment", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var got SyncPaymentRequest
		require.NoError(t, json.Unmarshal(body, &got))
		require.NotNil(t, got.ApplePay)
		assert.Equal(t, "tok-apple", got.ApplePay.Token)
		assert.Nil(t, got.GooglePay)
		assert.Nil(t, got.CardData)
		_, _ = w.Write([]byte(`{"invoiceId":"i-apple","status":"success","amount":500,"ccy":980,"paymentInfo":{"maskedPan":"414141**4141","terminal":"T","paymentSystem":"visa","paymentMethod":"apple"}}`))
	})

	out, err := c.SyncPayment(context.Background(), &SyncPaymentRequest{
		Amount: 500,
		Ccy:    980,
		ApplePay: &ApplePayPayload{
			Token:        "tok-apple",
			Exp:          "1228",
			EciIndicator: "05",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "i-apple", out.InvoiceID)
	assert.Equal(t, InvoiceSuccess, out.Status)
	require.NotNil(t, out.PaymentInfo)
	assert.Equal(t, MethodApple, out.PaymentInfo.PaymentMethod)
}

func TestSyncPayment_nil(t *testing.T) {
	c := New("x")
	_, err := c.SyncPayment(context.Background(), nil)
	assert.ErrorIs(t, err, ErrNilRequest)
}
