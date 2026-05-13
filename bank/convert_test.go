package bank

import (
	"testing"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Стандартний набір курсів — USD/UAH (buy/sell) і EUR/UAH (тільки cross).
var testRates = Rates{
	{CurrencyCodeA: 840, CurrencyCodeB: 980, RateBuy: 40.0, RateSell: 42.0},
	{CurrencyCodeA: 978, CurrencyCodeB: 980, RateCross: 45.0},
}

func TestConvert_sameCurrency(t *testing.T) {
	m := money.New(1000, currency.UAH)
	out, err := testRates.Convert(m, currency.UAH)
	require.NoError(t, err)
	assert.Equal(t, m, out)
}

func TestConvert_UAHToUSD_usesSell(t *testing.T) {
	// 4200 копійок (42 грн) / 42 = 100 центів (1 USD).
	out, err := testRates.Convert(money.New(4200, currency.UAH), currency.USD)
	require.NoError(t, err)
	assert.Equal(t, currency.USD, out.Code)
	assert.Equal(t, int64(100), out.Minor)
}

func TestConvert_USDToUAH_usesBuy(t *testing.T) {
	// 100 центів (1 USD) × 40 = 4000 копійок (40 грн).
	out, err := testRates.Convert(money.New(100, currency.USD), currency.UAH)
	require.NoError(t, err)
	assert.Equal(t, currency.UAH, out.Code)
	assert.Equal(t, int64(4000), out.Minor)
}

func TestConvert_EURToUAH_usesCross(t *testing.T) {
	// 100 центів (1 EUR) × 45 = 4500 копійок (RateCross fallback).
	out, err := testRates.Convert(money.New(100, currency.EUR), currency.UAH)
	require.NoError(t, err)
	assert.Equal(t, int64(4500), out.Minor)
}

func TestConvert_UAHToEUR_usesCross(t *testing.T) {
	// 4500 копійок / 45 = 100 центів EUR.
	out, err := testRates.Convert(money.New(4500, currency.UAH), currency.EUR)
	require.NoError(t, err)
	assert.Equal(t, int64(100), out.Minor)
}

func TestConvert_unknownPair(t *testing.T) {
	_, err := testRates.Convert(money.New(100, currency.GBP), currency.PLN)
	assert.ErrorIs(t, err, ErrNoRate)
}

func TestConvert_skipsZeroRateRow(t *testing.T) {
	// Перший рядок із нульовими курсами має бути пропущений, другий — використаний.
	rates := Rates{
		{CurrencyCodeA: 840, CurrencyCodeB: 980, RateBuy: 0, RateSell: 0, RateCross: 0},
		{CurrencyCodeA: 840, CurrencyCodeB: 980, RateBuy: 40, RateSell: 42},
	}
	out, err := rates.Convert(money.New(4200, currency.UAH), currency.USD)
	require.NoError(t, err)
	assert.Equal(t, int64(100), out.Minor)
}

func TestConvert_rounding(t *testing.T) {
	// 1 USD = 42.5 UAH. 100 центів → 4250 копійок (точно).
	rates := Rates{
		{CurrencyCodeA: 840, CurrencyCodeB: 980, RateBuy: 42.5, RateSell: 42.5},
	}
	out, err := rates.Convert(money.New(100, currency.USD), currency.UAH)
	require.NoError(t, err)
	assert.Equal(t, int64(4250), out.Minor)

	// Зворотне: 100 копійок UAH / 42.5 ≈ 2.353 цента → 2 цента (округлення half away from zero).
	out, err = rates.Convert(money.New(100, currency.UAH), currency.USD)
	require.NoError(t, err)
	assert.Equal(t, int64(2), out.Minor)
}
