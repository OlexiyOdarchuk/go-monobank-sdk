package acquiring

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/currency"
)

func TestT2PPaymentStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/merchant/t2p/terminal/payment/external/status", r.URL.Path)
		assert.Equal(t, "ext-42", r.URL.Query().Get("externalPaymentId"))
		assert.Equal(t, "acq-token", r.Header.Get("X-Token"))
		_, _ = w.Write([]byte(`{
			"bank":"АТ Універсал Банк",
			"terminal":"MT000001",
			"paymentType":"purchase",
			"amount":4200,
			"ccy":"UAH",
			"status":"success",
			"internalPaymentId":"int-1",
			"externalPaymentId":"ext-42",
			"transactionId":"tr-1",
			"maskedPan":"414141******4141",
			"cardMask":"Visa",
			"approvalCode":"123456",
			"rrn":"123456789012",
			"dataTime":"2026-09-22T10:00:00Z",
			"responseCode":"1",
			"countryCard":"UA"
		}`))
	})

	out, err := c.T2PPaymentStatus(context.Background(), "ext-42")
	require.NoError(t, err)
	assert.Equal(t, T2PStatusSuccess, out.Status)
	assert.Equal(t, T2PPurchase, out.PaymentType)
	assert.Equal(t, "MT000001", out.Terminal)
	assert.Equal(t, "ext-42", out.ExternalPaymentID)
	assert.Equal(t, "2026-09-22T10:00:00Z", out.DataTime)
	assert.Equal(t, "1", out.ResponseCode)
	assert.Equal(t, int64(4200), out.Amount.Minor)
	assert.Equal(t, currency.UAH, out.Amount.Code, "alpha-3 ccy must resolve onto Amount.Code")
}

// Right after the payment succeeds the statement entry does not
// exist yet, so the card-side fields arrive empty — that is a valid
// response, not an error.
func TestT2PPaymentStatus_successWithoutStatementFields(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"bank":"АТ Універсал Банк",
			"terminal":"MT000001",
			"paymentType":"purchase",
			"amount":100,
			"ccy":"UAH",
			"status":"success",
			"internalPaymentId":"int-2",
			"externalPaymentId":"ext-2",
			"transactionId":"tr-2",
			"dataTime":"2026-09-22T10:00:00Z"
		}`))
	})

	out, err := c.T2PPaymentStatus(context.Background(), "ext-2")
	require.NoError(t, err)
	assert.Equal(t, T2PStatusSuccess, out.Status)
	assert.Empty(t, out.MaskedPan)
	assert.Empty(t, out.RRN)
	assert.Empty(t, out.ApprovalCode)
}

func TestT2PPaymentStatus_failed(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"status":"failed",
			"externalPaymentId":"ext-3",
			"responseCode":"59",
			"errorMessage":"insufficient funds"
		}`))
	})

	out, err := c.T2PPaymentStatus(context.Background(), "ext-3")
	require.NoError(t, err)
	assert.Equal(t, T2PStatusFailed, out.Status)
	assert.Equal(t, "59", out.ResponseCode)
	assert.Equal(t, "insufficient funds", out.ErrorMessage)
}

// The status page of the docs names states the callback table does
// not; they must survive the round-trip verbatim instead of being
// normalized away.
func TestT2PPaymentStatus_unknownStatusPreserved(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"payment_not_completed","externalPaymentId":"ext-4"}`))
	})

	out, err := c.T2PPaymentStatus(context.Background(), "ext-4")
	require.NoError(t, err)
	assert.Equal(t, T2PStatus("payment_not_completed"), out.Status)
}

func TestT2PPaymentStatus_emptyID(t *testing.T) {
	c := newTestClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected request for an empty externalPaymentId")
	})

	_, err := c.T2PPaymentStatus(context.Background(), "")
	assert.ErrorIs(t, err, ErrEmptyID)
}

func TestT2PPaymentStatus_notFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errCode":"NOT_FOUND","errText":"not found"}`))
	})

	_, err := c.T2PPaymentStatus(context.Background(), "ext-old")
	require.Error(t, err)
	assert.True(t, IsNotFound(err))
}

// An unrecognized alpha-3 code must not fail the whole response —
// the amount stays readable, only Code is left unset.
func TestT2PPaymentStatus_unknownCurrency(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","amount":7,"ccy":"XYZ"}`))
	})

	out, err := c.T2PPaymentStatus(context.Background(), "ext-5")
	require.NoError(t, err)
	assert.Equal(t, int64(7), out.Amount.Minor)
	assert.Zero(t, out.Amount.Code)
}
