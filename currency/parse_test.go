package currency_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/currency"
)

func TestParse(t *testing.T) {
	tests := map[string]struct {
		want currency.Code
		ok   bool
	}{
		"UAH":  {currency.UAH, true}, // alpha-3
		"USD":  {currency.USD, true},
		"980":  {currency.UAH, true}, // числовий код рядком
		"840":  {currency.USD, true},
		"7777": {currency.Code(7777), true}, // невідомий, але валідний числовий
		"XYZ":  {0, false},                  // невідома alpha-3
		"":     {0, false},                  // порожня
		"uah":  {0, false},                  // case-sensitive
		"0":    {0, false},                  // нуль не є валідним кодом
		"-980": {0, false},                  // відʼємний
		"98a":  {0, false},                  // не число і не alpha-3
	}
	for s, tc := range tests {
		got, ok := currency.Parse(s)
		assert.Equalf(t, tc.ok, ok, "%q ok-flag", s)
		assert.Equalf(t, tc.want, got, "%q code", s)
	}
}
