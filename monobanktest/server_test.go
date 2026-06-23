package monobanktest_test

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/monobanktest"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/personal"
)

func TestServer_WithClientInfo_personalClient(t *testing.T) {
	srv := monobanktest.NewServer(t)
	srv.WithClientInfo(&bank.ClientInfo{
		ID:   "c-42",
		Name: "Тест",
		Accounts: bank.Accounts{
			{AccountID: "a1", Currency: 980, IBAN: "UA1"},
		},
	})

	cli := personal.New("token", srv.Option())
	info, err := cli.ClientInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "c-42", info.ID)
	assert.Equal(t, "Тест", info.Name)
	require.Len(t, info.Accounts, 1)
}

func TestServer_WithRates(t *testing.T) {
	srv := monobanktest.NewServer(t)
	srv.WithRates(bank.Rates{
		{CurrencyCodeA: 840, CurrencyCodeB: 980, RateBuy: 40, RateSell: 42},
	})

	cli := bank.New(srv.Option())
	rates, err := cli.Rates(context.Background())
	require.NoError(t, err)
	require.Len(t, rates, 1)
	assert.Equal(t, 840, rates[0].CurrencyCodeA)
	assert.InDelta(t, 42.0, rates[0].RateSell, 1e-9)
}

func TestServer_WithStatement_prefixMatch(t *testing.T) {
	srv := monobanktest.NewServer(t)
	srv.WithStatement("acc-1", bank.Transactions{
		{ID: "tx-1", Amount: bank.Transaction{}.Amount, Currency: 980},
	})

	cli := personal.New("token", srv.Option())
	from := time.Unix(1_700_000_000, 0)
	to := from.Add(time.Hour)
	txs, err := cli.Transactions(context.Background(), "acc-1", from, to)
	require.NoError(t, err)
	require.Len(t, txs, 1)
	assert.Equal(t, "tx-1", txs[0].ID)
	assert.Equal(t, currency.UAH, txs[0].Amount.Code)
}

func TestServer_Error(t *testing.T) {
	srv := monobanktest.NewServer(t)
	srv.Handle(http.MethodGet, "/personal/client-info",
		monobanktest.Error(http.StatusForbidden, "Unknown 'X-Token'"))

	cli := personal.New("bad-token", srv.Option())
	_, err := cli.ClientInfo(context.Background())
	require.Error(t, err)
	var apiErr *monobank.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	assert.Contains(t, string(apiErr.Body), "Unknown 'X-Token'")
}

func TestServer_Sequence(t *testing.T) {
	srv := monobanktest.NewServer(t)
	srv.Handle(http.MethodGet, "/personal/client-info", monobanktest.Sequence(
		monobanktest.Status(http.StatusServiceUnavailable),
		monobanktest.JSON(&bank.ClientInfo{ID: "c-1"}),
	))

	cli := personal.New("token", srv.Option(), monobank.WithRetry(3, time.Millisecond, 5*time.Millisecond))
	info, err := cli.ClientInfo(context.Background())
	require.NoError(t, err, "Sequence + retry: перший виклик 503, другий 200")
	assert.Equal(t, "c-1", info.ID)
}

func TestServer_Status_andRawJSONString(t *testing.T) {
	srv := monobanktest.NewServer(t)
	srv.Handle(http.MethodGet, "/x", monobanktest.JSON(`{"raw":"yes"}`))
	srv.Handle(http.MethodPost, "/x", monobanktest.Status(http.StatusNoContent))

	c := monobank.New(srv.Option())

	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	var body []byte
	require.NoError(t, c.Do(req, &body))
	assert.Equal(t, `{"raw":"yes"}`, string(body))

	req, _ = http.NewRequest(http.MethodPost, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil, http.StatusNoContent))
}

func TestServer_unexpectedPath_failsTest(t *testing.T) {
	// Sub-test із t.Run, щоб перехопити Errorf із fake server.
	captured := &captureT{TB: t}
	srv := monobanktest.NewServer(captured)
	// Реєструємо тільки /a. Викликаємо /b — має бути зафіксовано
	// помилку через captured.Errorf.
	srv.Handle(http.MethodGet, "/a", monobanktest.JSON(`{}`))

	c := monobank.New(srv.Option())
	req, _ := http.NewRequest(http.MethodGet, "/b", http.NoBody)
	_ = c.Do(req, nil)

	assert.NotEmpty(t, captured.errors, "має бути зафіксована помилка про unexpected /b")
	assert.Contains(t, captured.errors[0], "unexpected GET /b")
}

func TestServer_HandlePrefix(t *testing.T) {
	srv := monobanktest.NewServer(t)
	var hits atomic.Int32
	srv.HandlePrefix(http.MethodGet, "/api/foo/", monobanktest.ResponderFunc(
		func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			_, _ = w.Write([]byte(`{}`))
		}))

	c := monobank.New(srv.Option())
	for _, path := range []string{"/api/foo/1", "/api/foo/x/y", "/api/foo/bar"} {
		req, _ := http.NewRequest(http.MethodGet, path, http.NoBody)
		require.NoError(t, c.Do(req, nil))
	}
	assert.Equal(t, int32(3), hits.Load())
}

func TestServer_Option_composesBaseURL_and_HTTPClient(t *testing.T) {
	srv := monobanktest.NewServer(t)
	srv.WithRates(bank.Rates{})

	// Option має застосувати і baseURL, і HTTPClient (для self-signed TLS,
	// якщо колись httptest його ввімкне). Перевіряємо через коректну роботу.
	cli := bank.New(srv.Option())
	_, err := cli.Rates(context.Background())
	require.NoError(t, err)
}

// captureT — testing.TB, що зберігає Errorf-виклики для перевірки.
type captureT struct {
	testing.TB
	errors []string
}

func (c *captureT) Errorf(format string, args ...any) {
	// Не делегуємо в обгорнутий TB — інакше тест-обгортка теж зафейлить.
	c.errors = append(c.errors, fmt.Sprintf(format, args...))
}

func (c *captureT) Helper() {}
