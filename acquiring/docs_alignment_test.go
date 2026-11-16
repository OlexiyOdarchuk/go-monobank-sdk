package acquiring_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/acquiring"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/currency"
)

// Every code the docs list under a status must satisfy that status's
// predicate, not only the first one.
func TestPredicates_documentedCodes(t *testing.T) {
	for _, code := range []string{"BAD_REQUEST", "1001", "INVALID_MERCHANT_PAYM_INFO",
		"ORDER_IN_PROGRESS", "HOLD_INVOICE_NOT_FINALIZED", "WRONG_CANCEL_AMOUNT", "TOKEN_NOT_FOUND"} {
		assert.True(t, acquiring.IsBadRequest(apiErr(400, `{"errCode":"`+code+`"}`)), code)
	}
	assert.True(t, acquiring.IsNotFound(apiErr(404, `{"errCode":"1004"}`)))
	assert.True(t, acquiring.IsInternalError(apiErr(500, `{"errCode":"CANCEL_NOT_AVAILABLE"}`)))

	// The status alone decides, even for a code the SDK does not know.
	assert.True(t, acquiring.IsNotFound(apiErr(404, `{"errCode":"SOMETHING_NEW"}`)))
	// A documented code still matches on an unexpected status.
	assert.True(t, acquiring.IsNotFound(apiErr(400, `{"errCode":"NOT_FOUND"}`)))
	// And nothing crosses classes.
	assert.False(t, acquiring.IsBadRequest(apiErr(404, `{"errCode":"1004"}`)))
	assert.False(t, acquiring.IsInternalError(apiErr(400, `{"errCode":"WRONG_CANCEL_AMOUNT"}`)))
}

func TestCancelItem_splitReceiverID(t *testing.T) {
	b, err := json.Marshal(acquiring.CancelRequest{
		InvoiceID: "p2_1",
		Amount:    3000,
		Items: []acquiring.CancelItem{{
			Name: "Товар", Qty: 1, Sum: 3000, Code: "product_1", SplitReceiverID: "sr_aB3dC5eF7g",
		}},
	})
	require.NoError(t, err)
	assert.Contains(t, string(b), `"splitReceiverId":"sr_aB3dC5eF7g"`)

	// Outside the split scheme the field stays off the wire.
	b, err = json.Marshal(acquiring.CancelItem{Name: "X", Qty: 1, Sum: 1, Code: "c"})
	require.NoError(t, err)
	assert.NotContains(t, string(b), "splitReceiverId")
}

func TestStatementInvoice_agentFeeAndSplitReceivers(t *testing.T) {
	var s acquiring.StatementInvoice
	require.NoError(t, json.Unmarshal([]byte(`{
		"invoiceId":"p2_1","status":"success","maskedPan":"4441**1111",
		"date":"2024-06-01T14:15:22Z","paymentScheme":"full",
		"amount":10000,"profitAmount":9800,"agentFee":100,"ccy":980,
		"splitReceiverList":[
			{"splitReceiverId":"sr_1","amount":6000,"profit":5900,"agentFee":60},
			{"splitReceiverId":"sr_2","amount":4000,"profit":3900}
		]
	}`), &s))

	assert.Equal(t, int64(100), s.AgentFee.Minor)
	assert.Equal(t, currency.UAH, s.AgentFee.Code)
	require.Len(t, s.SplitReceiverList, 2)
	first := s.SplitReceiverList[0]
	assert.Equal(t, "sr_1", first.SplitReceiverID)
	assert.Equal(t, int64(6000), first.Amount.Minor)
	assert.Equal(t, int64(5900), first.Profit.Minor)
	assert.Equal(t, int64(60), first.AgentFee.Minor)
	assert.Equal(t, currency.UAH, first.Amount.Code)
	assert.Equal(t, currency.UAH, first.Profit.Code)
	assert.Equal(t, int64(0), s.SplitReceiverList[1].AgentFee.Minor)
}

func TestWalletPaymentRequest_walletTokens(t *testing.T) {
	b, err := json.Marshal(acquiring.WalletPaymentRequest{
		GToken:         `{"signature":"MEUCI"}`,
		Amount:         10000,
		Currency:       currency.UAH,
		InitiationKind: acquiring.InitClient,
	})
	require.NoError(t, err)
	s := string(b)
	assert.Contains(t, s, `"gToken":"{\"signature\":\"MEUCI\"}"`)
	assert.NotContains(t, s, "cardToken", "an empty card token must not be sent next to gToken")
	assert.NotContains(t, s, "aToken")

	b, err = json.Marshal(acquiring.WalletPaymentRequest{AToken: `{"data":""}`, Amount: 1, Currency: currency.UAH})
	require.NoError(t, err)
	assert.Contains(t, string(b), `"aToken":"{\"data\":\"\"}"`)

	// The saved-card flow is unchanged.
	b, err = json.Marshal(acquiring.WalletPaymentRequest{CardToken: "tok", Amount: 1, Currency: currency.UAH})
	require.NoError(t, err)
	assert.Contains(t, string(b), `"cardToken":"tok"`)
	assert.Contains(t, string(b), `"initiationKind":""`)
}

func TestWithCMS_onlyOnDocumentedEndpoints(t *testing.T) {
	seen := map[string][2]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = [2]string{r.Header.Get("X-Cms"), r.Header.Get("X-Cms-Version")}
		switch r.URL.Path {
		case "/api/merchant/invoice/create":
			_, _ = w.Write([]byte(`{"invoiceId":"p2_1","pageUrl":"https://pay"}`))
		default:
			_, _ = w.Write([]byte(`{"invoiceId":"p2_1","status":"created","amount":1,"ccy":980}`))
		}
	}))
	t.Cleanup(srv.Close)
	cli := acquiring.New("tok",
		monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()),
		acquiring.WithCMS("WooCommerce", "8.9"))

	ctx := context.Background()
	_, err := cli.CreateInvoice(ctx, &acquiring.CreateInvoiceRequest{Amount: 1})
	require.NoError(t, err)
	_, err = cli.InvoiceStatus(ctx, "p2_1")
	require.NoError(t, err)

	assert.Equal(t, [2]string{"WooCommerce", "8.9"}, seen["/api/merchant/invoice/create"])
	assert.Equal(t, [2]string{"", ""}, seen["/api/merchant/invoice/status"])
}

func TestWithCMS_emptyIsNoop(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Cms")
		_, _ = w.Write([]byte(`{"invoiceId":"p2_1","pageUrl":"https://pay"}`))
	}))
	t.Cleanup(srv.Close)
	cli := acquiring.New("tok", monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()),
		acquiring.WithCMS("", ""))
	_, err := cli.CreateInvoice(context.Background(), &acquiring.CreateInvoiceRequest{Amount: 1})
	require.NoError(t, err)
	assert.Empty(t, got)
}
