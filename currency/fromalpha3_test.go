package currency_test

import (
	"testing"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/stretchr/testify/assert"
)

func TestFromAlpha3(t *testing.T) {
	tests := map[string]struct {
		want Code
		ok   bool
	}{
		"UAH":     {currency.UAH, true},
		"USD":     {currency.USD, true},
		"EUR":     {currency.EUR, true},
		"GBP":     {currency.GBP, true},
		"XYZ":     {0, false}, // невідома
		"":        {0, false}, // порожня
		"uah":     {0, false}, // case-sensitive (alpha-3 — uppercase)
		"UAH UAH": {0, false}, // garbage
	}
	for s, tc := range tests {
		got, ok := currency.FromAlpha3(s)
		assert.Equalf(t, tc.ok, ok, "%q ok-flag", s)
		assert.Equalf(t, tc.want, got, "%q code", s)
	}
}

func TestFromAlpha3_isReverseOfString(t *testing.T) {
	// Кожен відомий Code → String → FromAlpha3 → той самий Code.
	for _, c := range []Code{currency.UAH, currency.USD, currency.EUR, currency.GBP,
		currency.PLN, currency.CHF, currency.JPY, currency.CZK,
		currency.CAD, currency.AUD, currency.CNY} {
		back, ok := currency.FromAlpha3(c.String())
		assert.True(t, ok, "round-trip ok for %s", c)
		assert.Equal(t, c, back)
	}
}

// Code тип реекспортовано для зручності в тесті.
type Code = currency.Code
