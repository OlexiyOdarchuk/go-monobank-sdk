package money_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
)

func TestUAH(t *testing.T) {
	// 42.00 грн = 4200 копійок, валюта проставлена.
	m := money.UAH(42.00)
	assert.Equal(t, int64(4200), m.Minor)
	assert.Equal(t, currency.UAH, m.Code)

	// Округлення half-away-from-zero на копійці.
	assert.Equal(t, int64(15000), money.UAH(149.999).Minor)
	assert.Equal(t, int64(14999), money.UAH(149.994).Minor)
}

func TestUAH_roundTrip(t *testing.T) {
	// UAH -> Minor -> Major повертає вихідне значення.
	assert.InDelta(t, 42.00, money.UAH(42.00).Major(), 1e-9)
	assert.InDelta(t, 149.99, money.UAH(149.99).Major(), 1e-9)
}

func TestFromMajor_decimals(t *testing.T) {
	// JPY має 0 знаків після коми: 1250 -> 1250 (а не *100).
	assert.Equal(t, int64(1250), money.FromMajor(1250, currency.JPY).Minor)
	// Динари — 3 знаки.
	assert.Equal(t, int64(1500), money.FromMajor(1.5, currency.KWD).Minor)
	// Від'ємні.
	assert.Equal(t, int64(-4200), money.FromMajor(-42.00, currency.UAH).Minor)
}

func TestParseMajor(t *testing.T) {
	cases := []struct {
		in   string
		code currency.Code
		want int64
	}{
		{"42.00", currency.UAH, 4200},
		{"149.99", currency.UAH, 14999},
		{"0.10", currency.UAH, 10}, // ціломатематичний шлях без float-похибки
		{"1.5", currency.UAH, 150}, // правий паддінг дробу
		{"-3.5", currency.UAH, -350},
		{"+7", currency.UAH, 700},
		{"100", currency.UAH, 10000},
		{"1250", currency.JPY, 1250}, // 0 знаків
		{".5", currency.UAH, 50},
		{"5.", currency.UAH, 500},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			m, err := money.ParseMajor(c.in, c.code)
			require.NoError(t, err)
			assert.Equal(t, c.want, m.Minor)
			assert.Equal(t, c.code, m.Code)
		})
	}
}

func TestParseMajor_errors(t *testing.T) {
	// Забагато знаків після коми для UAH (2).
	_, err := money.ParseMajor("1.234", currency.UAH)
	require.Error(t, err)

	// JPY не приймає дробову частину.
	_, err = money.ParseMajor("12.5", currency.JPY)
	require.Error(t, err)

	// Сміття.
	_, err = money.ParseMajor("abc", currency.UAH)
	require.Error(t, err)

	_, err = money.ParseMajor("", currency.UAH)
	require.Error(t, err)
}

func TestParseMajor_overflow(t *testing.T) {
	_, err := money.ParseMajor("99999999999999999999", currency.UAH)
	require.ErrorIs(t, err, money.ErrOverflow)
}

func TestParseMajor_matchesFromMajor(t *testing.T) {
	// Для "круглих" значень ParseMajor і FromMajor збігаються.
	for _, s := range []string{"42.00", "1.25", "1000.00"} {
		p, err := money.ParseMajor(s, currency.UAH)
		require.NoError(t, err)
		f := money.UAH(p.Major())
		assert.Equal(t, p.Minor, f.Minor, "value %s", s)
	}
}

func TestParseMajor_notErrOverflowOnGarbage(t *testing.T) {
	_, err := money.ParseMajor("1.2.3", currency.UAH)
	require.Error(t, err)
	assert.False(t, errors.Is(err, money.ErrOverflow))
}
