//go:build integration

// Package bank integration tests hit the real api.monobank.ua. They are
// excluded from the default `go test ./...` run; enable with:
//
//	go test -tags=integration -run Integration ./bank/...
//
// CI runs them on a weekly schedule + manual workflow_dispatch — see
// .github/workflows/integration.yaml. They must stay read-only and
// rate-limited (mono caps /bank/currency at 1 req per ~5 min per IP).
package bank_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
)

func TestIntegration_Rates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cli := bank.New(monobank.WithRetry(3, time.Second, 10*time.Second))
	rates, err := cli.Rates(ctx)
	require.NoError(t, err, "live /bank/currency call must succeed")
	require.NotEmpty(t, rates, "rates list must not be empty")

	hasUSDtoUAH := false
	for _, r := range rates {
		if currency.Code(r.CurrencyCodeA) == currency.USD &&
			currency.Code(r.CurrencyCodeB) == currency.UAH {
			hasUSDtoUAH = true
			assert.Greater(t, r.RateBuy+r.RateSell+r.RateCross, float64(0),
				"USD/UAH rate must carry a non-zero price")
			break
		}
	}
	assert.True(t, hasUSDtoUAH, "USD/UAH must appear in the live response")
}

func TestIntegration_ServerKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cli := bank.New()
	key, err := cli.ServerKey(ctx)
	require.NoError(t, err, "live /personal/auth/key call must succeed")
	require.NotNil(t, key)
	assert.NotEmpty(t, key.ID)
	assert.NotNil(t, key.PubKey, "public key must be parsed")
}
