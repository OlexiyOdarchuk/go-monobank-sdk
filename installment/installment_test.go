package installment_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/installment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testStoreID = "test_store"
	testSecret  = "secret_98765432--123-123"
)

func expectedSign(t *testing.T, body []byte) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func newClient(t *testing.T, handler http.HandlerFunc) (*installment.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := installment.New(testStoreID, testSecret, installment.WithBaseURL(srv.URL))
	require.NoError(t, err)
	return c, srv
}

func TestSign_isHMACSHA256Base64(t *testing.T) {
	c, errNew := installment.New(testStoreID, testSecret)
	require.NoError(t, errNew)
	got := c.Sign([]byte(`{"a":1}`))
	assert.Equal(t, expectedSign(t, []byte(`{"a":1}`)), got)
}

func TestVerifyCallback(t *testing.T) {
	c, errNew := installment.New(testStoreID, testSecret)
	require.NoError(t, errNew)
	body := []byte(`{"order_id":"o","state":"IN_PROCESS","order_sub_state":"WAITING_FOR_STORE_CONFIRM"}`)
	require.NoError(t, c.VerifyCallback(body, c.Sign(body)))
	require.Error(t, c.VerifyCallback(body, "tampered"))
}

// VerifyCallback з підписом некоректної довжини мусить
// бути відхиленим БЕЗ обчислення HMAC. До фіксу великий body + короткий/
// порожній signature змушував сервер витратити CPU на HMAC-SHA256 від N
// байтів, перш ніж відмовити. Тут перевіряємо лише результат (sentinel),
// бо хронометраж — крихкий у CI; саме виявлення короткого підпису
// раніше за HMAC задокументовано в коді.
func TestVerifyCallback_lengthFastPath(t *testing.T) {
	c, errNew := installment.New(testStoreID, testSecret)
	require.NoError(t, errNew)
	body := []byte(`{"order_id":"o","state":"IN_PROCESS"}`)

	// Wrong-length cases bail out before HMAC and surface the
	// ErrCallbackBadLength sentinel, so security telemetry can tell
	// "malformed header" apart from "forgery".
	badLen := map[string]string{
		"empty":    "",
		"short":    "AAAA",
		"43-chars": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",   // 43, want 44
		"45-chars": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", // 45
	}
	for name, sig := range badLen {
		t.Run(name, func(t *testing.T) {
			err := c.VerifyCallback(body, sig)
			assert.ErrorIs(t, err, installment.ErrCallbackBadLength)
		})
	}

	// Correct-length-but-wrong-MAC goes through HMAC and returns the
	// mismatch sentinel.
	t.Run("correct-len-bad", func(t *testing.T) {
		err := c.VerifyCallback(body, expectedSign(t, []byte("different body")))
		assert.ErrorIs(t, err, installment.ErrCallbackSignatureMismatch)
	})
}

func TestVerifyCallback_sentinelComparable(t *testing.T) {
	c, errNew := installment.New(testStoreID, testSecret)
	require.NoError(t, errNew)
	err := c.VerifyCallback([]byte(`{}`), "")
	assert.ErrorIs(t, err, installment.ErrCallbackBadLength,
		"errors.Is must work against ErrCallbackBadLength for wrong-length headers")
}

func TestCreateOrder_success(t *testing.T) {
	var capturedBody []byte
	var capturedSig string
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/order/create", r.URL.Path)
		assert.Equal(t, testStoreID, r.Header.Get(installment.HeaderStoreID))
		capturedSig = r.Header.Get(installment.HeaderSignature)
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"order_id":"fa4a8249-336e-4e6d-9b85-79bc8be62377"}`))
	})

	resp, err := cli.CreateOrder(context.Background(), &installment.CreateOrderRequest{
		StoreOrderID: "ORD-1",
		ClientPhone:  "+380501234561",
		TotalSum:     2499.99,
		Invoice: installment.CreateOrderInvoice{
			Number: "INV-1",
			Date:   "2026-05-13",
			Source: installment.SourceInternet,
		},
		AvailablePrograms: []installment.Program{{AvailablePartsCount: []int{3, 6}}},
		Products:          []installment.Product{{Name: "Cat food", Count: 1, Sum: 2499.99}},
	})
	require.NoError(t, err)
	assert.Equal(t, "fa4a8249-336e-4e6d-9b85-79bc8be62377", resp.OrderID)

	// Body has all the fields we passed.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(capturedBody, &raw))
	assert.Equal(t, "ORD-1", raw["store_order_id"])
	assert.Equal(t, "+380501234561", raw["client_phone"])
	assert.InDelta(t, 2499.99, raw["total_sum"], 0.001)

	// Signature matches.
	assert.Equal(t, expectedSign(t, capturedBody), capturedSig)
}

func TestCreateOrder_nilRequest(t *testing.T) {
	cli, errNew := installment.New(testStoreID, testSecret)
	require.NoError(t, errNew)
	_, err := cli.CreateOrder(context.Background(), nil)
	assert.ErrorIs(t, err, installment.ErrNilRequest)
}

func TestOrderState_decodesEnum(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"order_id":"o","state":"IN_PROCESS","order_sub_state":"WAITING_FOR_STORE_CONFIRM"}`))
	})
	st, err := cli.OrderState(context.Background(), "o")
	require.NoError(t, err)
	assert.Equal(t, installment.StateInProcess, st.State)
	assert.Equal(t, installment.SubWaitingForStoreConfirm, st.OrderSubState)
}

func TestConfirmAndReject(t *testing.T) {
	var calledPath string
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		calledPath = r.URL.Path
		_, _ = w.Write([]byte(`{"order_id":"o","state":"SUCCESS","order_sub_state":"ACTIVE"}`))
	})
	_, err := cli.ConfirmOrder(context.Background(), "o")
	require.NoError(t, err)
	assert.Equal(t, "/api/order/confirm", calledPath)

	_, err = cli.RejectOrder(context.Background(), "o")
	require.NoError(t, err)
	assert.Equal(t, "/api/order/reject", calledPath)
}

func TestReturnOrder(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/order/return", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var raw map[string]any
		_ = json.Unmarshal(body, &raw)
		assert.Equal(t, "RET-1", raw["store_return_id"])
		assert.InDelta(t, 500.0, raw["sum"], 0.001)
		assert.Equal(t, true, raw["return_money_to_card"])
		_, _ = w.Write([]byte(`{"status":"OK"}`))
	})
	resp, err := cli.ReturnOrder(context.Background(), &installment.ReturnRequest{
		OrderID:           "o",
		Sum:               500,
		StoreReturnID:     "RET-1",
		ReturnMoneyToCard: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "OK", resp.Status)
}

func TestCheckPaid(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"fully_paid":true,"bank_can_return_money_to_card":true}`))
	})
	resp, err := cli.CheckPaid(context.Background(), "o")
	require.NoError(t, err)
	assert.True(t, resp.FullyPaid)
	assert.True(t, resp.BankCanReturnMoneyToCard)
}

func TestValidateClient_v2(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/client/validate", r.URL.Path)
		_, _ = w.Write([]byte(`{"found":true}`))
	})
	found, err := cli.ValidateClient(context.Background(), "+380501234561")
	require.NoError(t, err)
	assert.True(t, found)
}

func TestValidateClient_legacy(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/client/validate", r.URL.Path)
		_, _ = w.Write([]byte(`{"found":true,"client":{"first_name":"Іван","last_name":"Петров","inn":"1234567890"}}`))
	})
	resp, err := cli.ValidateClientLegacy(context.Background(), "+380501234561")
	require.NoError(t, err)
	assert.True(t, resp.Found)
	require.NotNil(t, resp.Client)
	assert.Equal(t, "Іван", resp.Client.FirstName)
}

func TestDailyReport(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/store/report", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var raw map[string]any
		_ = json.Unmarshal(body, &raw)
		assert.Equal(t, "2026-05-13", raw["date"])
		_, _ = w.Write([]byte(`{"orders":[{"order_id":"o","total_sum":100.5,"pay_parts":3,"commission_percent":2.5,"transferred_sum":97.99,"commission":2.51}]}`))
	})
	orders, err := cli.DailyReport(context.Background(), "2026-05-13")
	require.NoError(t, err)
	require.Len(t, orders, 1)
	assert.Equal(t, "o", orders[0].OrderID)
	assert.InDelta(t, 100.5, orders[0].TotalSum, 0.001)
}

func TestGuaranteeLetterPDF(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/order/guarantee/letter", r.URL.Path)
		assert.Equal(t, "application/pdf", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 fake"))
	})
	pdf, err := cli.GuaranteeLetterPDF(context.Background(), &installment.OrderDataRequest{OrderID: "o"})
	require.NoError(t, err)
	assert.True(t, len(pdf) > 0)
	assert.Equal(t, "%PDF", string(pdf[:4]))
}

func TestGuaranteeLetterData_v1AndV2(t *testing.T) {
	var path string
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"header":{"request_id":"r","from_organization":"Mono","contract_number":"CN-1"},"expansion":{"bank":{"credit_amount":1234.56}}}`))
	})

	data, err := cli.GuaranteeLetterData(context.Background(), &installment.OrderDataRequest{OrderID: "o"})
	require.NoError(t, err)
	assert.Equal(t, "/api/order/data/for/guarantee/letter", path)
	assert.Equal(t, "Mono", data.Header.FromOrganization)
	require.NotNil(t, data.Expansion.Bank)
	assert.InDelta(t, 1234.56, data.Expansion.Bank.CreditAmount, 0.001)

	_, err = cli.GuaranteeLetterDataV2(context.Background(), &installment.OrderDataRequest{OrderID: "o"})
	require.NoError(t, err)
	assert.Equal(t, "/api/v2/order/data/for/guarantee/letter", path)
}

// Regression: every order-ID-taking endpoint must refuse an empty
// orderID locally instead of letting the HMAC-signed request go to
// Mono and getting back an opaque 400. Saves a round-trip AND avoids
// leaking the merchant secret in case the upstream URL is logged.
func TestOrderID_emptyRejectedLocally(t *testing.T) {
	cli, errNew := installment.New(testStoreID, testSecret)
	require.NoError(t, errNew)
	ctx := context.Background()

	type call struct {
		name string
		fn   func() error
	}
	calls := []call{
		{"OrderState", func() error { _, e := cli.OrderState(ctx, ""); return e }},
		{"ConfirmOrder", func() error { _, e := cli.ConfirmOrder(ctx, ""); return e }},
		{"RejectOrder", func() error { _, e := cli.RejectOrder(ctx, ""); return e }},
		{"OrderInfo", func() error { _, e := cli.OrderInfo(ctx, ""); return e }},
		{"OrderData", func() error { _, e := cli.OrderData(ctx, ""); return e }},
		{"CheckPaid", func() error { _, e := cli.CheckPaid(ctx, ""); return e }},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			assert.ErrorIs(t, c.fn(), installment.ErrEmptyOrderID)
		})
	}
}

func TestValidatePhone_rejectsMalformed(t *testing.T) {
	cli, errNew := installment.New(testStoreID, testSecret)
	require.NoError(t, errNew)
	ctx := context.Background()

	cases := map[string]error{
		"":              installment.ErrEmptyPhone,
		"380501234561":  installment.ErrInvalidPhone, // no leading +
		"+380abc1234":   installment.ErrInvalidPhone, // letters
		"+":             installment.ErrInvalidPhone, // just +
		"+380 50 12345": installment.ErrInvalidPhone, // spaces
	}
	for phone, want := range cases {
		t.Run(phone, func(t *testing.T) {
			_, err := cli.ValidateClient(ctx, phone)
			assert.ErrorIs(t, err, want)
		})
	}
}

func TestDailyReport_emptyDateRejected(t *testing.T) {
	cli, errNew := installment.New(testStoreID, testSecret)
	require.NoError(t, errNew)
	_, err := cli.DailyReport(context.Background(), "")
	assert.ErrorIs(t, err, installment.ErrEmptyDate)
}

func TestNew_rejectsEmptyCredentials(t *testing.T) {
	_, err := installment.New("", "secret")
	assert.ErrorIs(t, err, installment.ErrEmptyStoreID)

	_, err = installment.New("store", "")
	assert.ErrorIs(t, err, installment.ErrEmptySecret)
}

func TestNew_rejectsInsecureBaseURL(t *testing.T) {
	_, err := installment.New("store", "secret",
		installment.WithBaseURL("http://evil.example.com"))
	assert.ErrorIs(t, err, installment.ErrInsecureBaseURL)
}

func TestNew_allowsLoopbackHTTP(t *testing.T) {
	for _, u := range []string{
		"http://localhost:8080",
		"http://127.0.0.1:9000",
		"http://[::1]:8080",
	} {
		t.Run(u, func(t *testing.T) {
			_, err := installment.New("store", "secret", installment.WithBaseURL(u))
			assert.NoError(t, err, "loopback http must be accepted")
		})
	}
}

func TestNew_insecureOptOut(t *testing.T) {
	// Order does not matter — final validation runs after every option.
	for _, opts := range [][]installment.Option{
		{installment.WithInsecureBaseURL(true), installment.WithBaseURL("http://staging.internal")},
		{installment.WithBaseURL("http://staging.internal"), installment.WithInsecureBaseURL(true)},
	} {
		_, err := installment.New("store", "secret", opts...)
		assert.NoError(t, err, "WithInsecureBaseURL must work regardless of option order")
	}
}

func TestAPIError_decode(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Trace-Id", "abc-123")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad phone"}`))
	})
	// A well-formed but unknown number passes the client-side phone
	// validator and reaches the (mock) server, which then returns
	// the APIError shape we want to assert on.
	_, err := cli.ValidateClient(context.Background(), "+380000000000")
	var apiErr *installment.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Equal(t, "bad phone", apiErr.Message)
	assert.Equal(t, "abc-123", apiErr.TraceID)
	assert.Contains(t, apiErr.Error(), "trace=abc-123")
}

// Client.LogValue приховує store-secret у slog-виводі.
func TestClient_LogValueRedactsSecret(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	cli, errNew := installment.New("STORE-1", "my-very-secret-key")
	require.NoError(t, errNew)
	logger.Info("cli", "v", cli)
	out := buf.String()
	assert.NotContains(t, out, "my-very-secret-key", "store-secret НЕ повинен потрапити в логи")
	assert.Contains(t, out, "STORE-1", "storeID не секрет — має бути видимий")
	assert.Contains(t, out, "***")
}
