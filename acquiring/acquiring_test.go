package acquiring

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New("acq-token", monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
}

func TestTokenAuth_setsXTokenHeader(t *testing.T) {
	var seen string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Token")
		_, _ = w.Write([]byte(`{"merchantId":"m","merchantName":"M","edrpou":"1"}`))
	})

	_, err := c.MerchantDetails(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "acq-token", seen)
}

func TestMerchantDetails(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/merchant/details", r.URL.Path)
		_, _ = w.Write([]byte(`{"merchantId":"m1","merchantName":"Acme","edrpou":"12345678"}`))
	})

	out, err := c.MerchantDetails(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "m1", out.MerchantID)
	assert.Equal(t, "Acme", out.MerchantName)
}

func TestEmployees_unwrapsList(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"list":[{"id":"e1","name":"A","extRef":"x"}]}`))
	})

	out, err := c.Employees(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "A", out[0].Name)
}

func TestPubKey(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"key":"-----BEGIN PUBLIC KEY-----\nMFY...\n-----END PUBLIC KEY-----\n"}`))
	})

	out, err := c.PubKey(context.Background())
	require.NoError(t, err)
	assert.Contains(t, out.Key, "BEGIN PUBLIC KEY")
}

func TestCreateInvoice(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/merchant/invoice/create", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var got CreateInvoiceRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, int64(1000), got.Amount)
		assert.Equal(t, PaymentHold, got.PaymentType)
		_, _ = w.Write([]byte(`{"invoiceId":"i1","pageUrl":"https://pay/x"}`))
	})

	out, err := c.CreateInvoice(context.Background(), &CreateInvoiceRequest{
		Amount:      1000,
		Ccy:         980,
		PaymentType: PaymentHold,
	})
	require.NoError(t, err)
	assert.Equal(t, "i1", out.InvoiceID)
}

func TestCreateInvoice_nil(t *testing.T) {
	c := New("x")
	_, err := c.CreateInvoice(context.Background(), nil)
	assert.ErrorIs(t, err, ErrNilRequest)
}

func TestInvoiceStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "i42", r.URL.Query().Get("invoiceId"))
		_, _ = w.Write([]byte(`{"invoiceId":"i42","status":"success","amount":1000,"ccy":980,"paymentInfo":{"maskedPan":"414141**4141","terminal":"T1","paymentSystem":"visa","paymentMethod":"pan"}}`))
	})

	out, err := c.InvoiceStatus(context.Background(), "i42")
	require.NoError(t, err)
	assert.Equal(t, InvoiceSuccess, out.Status)
	require.NotNil(t, out.PaymentInfo)
	assert.Equal(t, Visa, out.PaymentInfo.PaymentSystem)
}

func TestCancelInvoice(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"processing","createdDate":"2026-01-15T00:00:00Z","modifiedDate":"2026-01-15T00:00:00Z"}`))
	})

	out, err := c.CancelInvoice(context.Background(), &CancelRequest{
		InvoiceID: "i1",
		Amount:    500,
	})
	require.NoError(t, err)
	assert.Equal(t, "processing", out.Status)
}

func TestFinalizeInvoice(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success"}`))
	})

	out, err := c.FinalizeInvoice(context.Background(), &FinalizeRequest{
		InvoiceID: "i1",
		Amount:    1000,
	})
	require.NoError(t, err)
	assert.Equal(t, "success", out.Status)
}

func TestRemoveInvoice(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var got RemoveRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "i9", got.InvoiceID)
		_, _ = w.Write([]byte(`{}`))
	})

	require.NoError(t, c.RemoveInvoice(context.Background(), "i9"))
}

func TestFiscalChecks(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"checks":[{"id":"c1","type":"sale","status":"done","fiscalizationSource":"checkbox"}]}`))
	})

	out, err := c.FiscalChecks(context.Background(), "i1")
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, FiscalSale, out[0].Type)
	assert.Equal(t, FiscalDone, out[0].Status)
}

func TestQRList(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"list":[{"shortQrId":"S","qrId":"Q","amountType":"merchant","pageUrl":"https://pay"}]}`))
	})

	out, err := c.QRList(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "Q", out[0].QrID)
}

func TestQRResetAmount(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var got ResetAmountRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "Q1", got.QrID)
		_, _ = w.Write([]byte(`{}`))
	})

	require.NoError(t, c.QRResetAmount(context.Background(), "Q1"))
}

func TestWallet(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "w1", r.URL.Query().Get("walletId"))
		_, _ = w.Write([]byte(`{"wallet":[{"cardToken":"tok","maskedPan":"414141**4141","country":"UA"}]}`))
	})

	out, err := c.Wallet(context.Background(), "w1")
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "tok", out[0].CardToken)
}

func TestDeleteCard(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "tok-123", r.URL.Query().Get("cardToken"))
		_, _ = w.Write([]byte(`{}`))
	})

	require.NoError(t, c.DeleteCard(context.Background(), "tok-123"))
}

func TestWalletPayment(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"invoiceId":"i2","status":"success","amount":500,"ccy":980,"createdDate":"x","modifiedDate":"y"}`))
	})

	out, err := c.WalletPayment(context.Background(), &WalletPaymentRequest{
		CardToken:      "tok",
		Amount:         500,
		Ccy:            980,
		InitiationKind: InitMerchant,
	})
	require.NoError(t, err)
	assert.Equal(t, "i2", out.InvoiceID)
}

func TestStatement_query(t *testing.T) {
	from := time.Unix(1_700_000_000, 0)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, strconv.FormatInt(from.Unix(), 10), r.URL.Query().Get("from"))
		assert.Empty(t, r.URL.Query().Get("to"), "to omitted when zero")
		_, _ = w.Write([]byte(`{"list":[]}`))
	})

	_, err := c.Statement(context.Background(), from, time.Time{}, "")
	require.NoError(t, err)
}

func TestSubmerchants(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"list":[{"code":"s1","iban":"UA1"}]}`))
	})

	out, err := c.Submerchants(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "UA1", out[0].IBAN)
}

func TestQRDetails(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Q1", r.URL.Query().Get("qrId"))
		_, _ = w.Write([]byte(`{"shortQrId":"S","invoiceId":"i","amount":100,"ccy":980}`))
	})

	out, err := c.QRDetails(context.Background(), "Q1")
	require.NoError(t, err)
	assert.Equal(t, int64(100), out.Amount.Minor)
}
