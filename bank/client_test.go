package bank

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

func TestRates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/bank/currency", r.URL.Path)
		_, _ = w.Write([]byte(`[{"currencyCodeA":840,"currencyCodeB":980,"date":1700000000,"rateBuy":40,"rateSell":41}]`))
	}))
	defer srv.Close()

	c := New(monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
	got, err := c.Rates(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 840, got[0].CurrencyCodeA)
	assert.InDelta(t, 40.0, got[0].RateBuy, 1e-9)
}

func TestServerKey_invalidPoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// 5 bytes — not a valid uncompressed secp256k1 point.
		_, _ = w.Write([]byte(`{"serverKeyId":"k","serverPubKey":"AAAAAQ==","serverTimeMsec":0}`))
	}))
	defer srv.Close()

	c := New(monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
	_, err := c.ServerKey(context.Background())
	assert.ErrorIs(t, err, ErrInvalidPubKey)
}

func TestTransaction_MCC(t *testing.T) {
	tx := Transaction{MCC: 5411, OriginalMCC: 4829}
	assert.Equal(t, "Groceries", string(tx.MCCCode().Category()))
	assert.Equal(t, "MoneyTransfer", string(tx.OriginalMCCCode().Category()))
}
