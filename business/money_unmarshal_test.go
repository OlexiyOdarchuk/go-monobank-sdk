package business_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/business"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
)

func TestStatementItem_propagatesCurrencyString(t *testing.T) {
	in := []byte(`{
		"id":"op-1",
		"amount":-12345,
		"currencyCode":"UAH",
		"status":"DONE"
	}`)

	var s business.StatementItem
	require.NoError(t, json.Unmarshal(in, &s))

	assert.Equal(t, currency.UAH, s.Amount.Code, "alpha-3 'UAH' → currency.UAH")
	assert.Equal(t, int64(-12345), s.Amount.Minor)
}

func TestStatementItem_unknownCurrency_zeroCode(t *testing.T) {
	in := []byte(`{"id":"op-2","amount":100,"currencyCode":"XYZ"}`)
	var s business.StatementItem
	require.NoError(t, json.Unmarshal(in, &s))
	// Minor усе одно валідний; Code лишається 0 для невідомої валюти.
	assert.Equal(t, int64(100), s.Amount.Minor)
	assert.Equal(t, currency.Code(0), s.Amount.Code)
}

func TestAccount_BalanceMoney(t *testing.T) {
	a := business.Account{IBAN: "UA1", Currency: 980, Balance: 1234.56}

	m := a.BalanceMoney()
	assert.Equal(t, int64(123456), m.Minor, "1234.56 → 123456 копійок")
	assert.Equal(t, currency.UAH, m.Code)
}

func TestAccount_BalanceMoney_negativeAndZero(t *testing.T) {
	assert.Equal(t, int64(-1000),
		business.Account{Currency: 980, Balance: -10.00}.BalanceMoney().Minor)
	assert.Equal(t, int64(0),
		business.Account{Currency: 980, Balance: 0.0}.BalanceMoney().Minor)
}

func TestBalancePoint_Money(t *testing.T) {
	bp := business.BalancePoint{Date: "2026-01-15", Balance: 42.50, IsFinal: true}
	m := bp.Money(currency.USD)
	assert.Equal(t, int64(4250), m.Minor)
	assert.Equal(t, currency.USD, m.Code)
}
