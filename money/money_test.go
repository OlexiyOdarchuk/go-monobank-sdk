package money

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
)

func TestNew(t *testing.T) {
	m := New(1234, currency.UAH)
	assert.Equal(t, int64(1234), m.Minor)
	assert.Equal(t, currency.UAH, m.Code)
}

func TestIsZero(t *testing.T) {
	assert.True(t, Money{}.IsZero())
	assert.True(t, New(0, currency.UAH).IsZero())
	assert.False(t, New(1, currency.UAH).IsZero())
}

func TestEqual(t *testing.T) {
	a := New(100, currency.UAH)
	assert.True(t, a.Equal(New(100, currency.UAH)))
	assert.False(t, a.Equal(New(100, currency.USD)), "різні валюти")
	assert.False(t, a.Equal(New(101, currency.UAH)), "різні суми")
}

func TestAdd_sameCurrency(t *testing.T) {
	sum, err := New(100, currency.UAH).Add(New(50, currency.UAH))
	require.NoError(t, err)
	assert.Equal(t, New(150, currency.UAH), sum)
}

func TestAdd_differentCurrencyErrors(t *testing.T) {
	_, err := New(100, currency.UAH).Add(New(50, currency.USD))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different currencies")
}

func TestSub(t *testing.T) {
	out, err := New(100, currency.UAH).Sub(New(30, currency.UAH))
	require.NoError(t, err)
	assert.Equal(t, int64(70), out.Minor)

	out, err = New(30, currency.UAH).Sub(New(100, currency.UAH))
	require.NoError(t, err)
	assert.Equal(t, int64(-70), out.Minor, "негативні результати дозволені")

	_, err = New(100, currency.UAH).Sub(New(10, currency.USD))
	require.Error(t, err)
}

func TestNeg(t *testing.T) {
	assert.Equal(t, New(-100, currency.UAH), New(100, currency.UAH).Neg())
	assert.Equal(t, New(50, currency.UAH), New(-50, currency.UAH).Neg())
}

func TestMul(t *testing.T) {
	cases := []struct {
		in   Money
		n    int64
		want Money
	}{
		{New(100, currency.UAH), 3, New(300, currency.UAH)},
		{New(100, currency.UAH), 0, New(0, currency.UAH)},
		{New(100, currency.UAH), -2, New(-200, currency.UAH)},
	}
	for _, c := range cases {
		got, err := c.in.Mul(c.n)
		require.NoError(t, err)
		assert.Equal(t, c.want, got)
	}
}

func TestMul_overflow(t *testing.T) {
	_, err := New(math.MaxInt64, currency.UAH).Mul(2)
	assert.ErrorIs(t, err, ErrOverflow)

	_, err = New(math.MinInt64, currency.UAH).Mul(-1)
	assert.ErrorIs(t, err, ErrOverflow)
}

func TestAdd_overflow(t *testing.T) {
	_, err := New(math.MaxInt64, currency.UAH).Add(New(1, currency.UAH))
	assert.ErrorIs(t, err, ErrOverflow)

	_, err = New(math.MinInt64, currency.UAH).Add(New(-1, currency.UAH))
	assert.ErrorIs(t, err, ErrOverflow)
}

func TestSub_overflow(t *testing.T) {
	_, err := New(math.MinInt64, currency.UAH).Sub(New(1, currency.UAH))
	assert.ErrorIs(t, err, ErrOverflow)

	_, err = New(math.MaxInt64, currency.UAH).Sub(New(-1, currency.UAH))
	assert.ErrorIs(t, err, ErrOverflow)
}

func TestScale(t *testing.T) {
	// 1000 копійок × 0.05 = 50 копійок (5%).
	assert.Equal(t, New(50, currency.UAH), New(1000, currency.UAH).Scale(0.05))

	// Округлення «half away from zero»:
	// 333 × 0.5 = 166.5 → 167
	assert.Equal(t, New(167, currency.UAH), New(333, currency.UAH).Scale(0.5))
	// -333 × 0.5 = -166.5 → -167
	assert.Equal(t, New(-167, currency.UAH), New(-333, currency.UAH).Scale(0.5))
}

func TestMajor(t *testing.T) {
	assert.InDelta(t, 42.50, New(4250, currency.UAH).Major(), 1e-9)
	assert.InDelta(t, -1.00, New(-100, currency.UAH).Major(), 1e-9)
	assert.InDelta(t, 0.0, Money{}.Major(), 1e-9)
}

// Major має поважати кількість знаків після коми за валютою.
// До фіксу UAH/USD ділилися на 100, але JPY/KRW (0 знаків) теж — отже
// 1250 єн відображалися як 12.50, що неправильно.
func TestMajor_currencyAwareDecimals(t *testing.T) {
	cases := []struct {
		name  string
		minor int64
		code  currency.Code
		want  float64
	}{
		{"UAH 2 decimals", 4250, currency.UAH, 42.50},
		{"USD 2 decimals", 100, currency.USD, 1.00},
		{"JPY 0 decimals", 1250, currency.JPY, 1250.0}, // не ділиться на 100
		{"unknown code defaults to 2 decimals", 4250, currency.Code(7777), 42.50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := New(tc.minor, tc.code).Major()
			assert.InDelta(t, tc.want, got, 1e-9)
		})
	}
}

func TestString(t *testing.T) {
	assert.Equal(t, "42.50 UAH", New(4250, currency.UAH).String())
	assert.Equal(t, "-10.00 USD", New(-1000, currency.USD).String())
	assert.Equal(t, "0.00 7777", New(0, currency.Code(7777)).String(), "невідома валюта — числовий код")
}

// String використовує currency-aware decimals — JPY без
// знаків після коми ("1250 JPY", не "12.50 JPY").
func TestString_currencyAwareDecimals(t *testing.T) {
	assert.Equal(t, "1250 JPY", New(1250, currency.JPY).String(),
		"JPY має 0 знаків після коми")
	assert.Equal(t, "42.50 UAH", New(4250, currency.UAH).String(),
		"UAH має 2 знаки після коми")
}

// Major для звичайних сум (UAH/USD/EUR) дає той самий результат, що
// й банальне ділення на 100 — math.Pow10(2) точно дорівнює 100.
// Це фіксація для випадку, якщо хтось колись захоче змінити реалізацію.
func TestMajor_precisionForCommonAmounts(t *testing.T) {
	cases := map[string]struct {
		minor int64
		code  currency.Code
		want  float64
	}{
		"1 копійка":     {1, currency.UAH, 0.01},
		"99 копійок":    {99, currency.UAH, 0.99},
		"100 копійок":   {100, currency.UAH, 1.00},
		"мільйон грн":   {100_000_000, currency.UAH, 1_000_000.00},
		"мільярд грн":   {100_000_000_000, currency.UAH, 1_000_000_000.00},
		"negative":      {-12345, currency.USD, -123.45},
		"JPY 1 одиниця": {1, currency.JPY, 1.0},
		"JPY мільйон":   {1_000_000, currency.JPY, 1_000_000.0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := New(tc.minor, tc.code).Major()
			assert.InDelta(t, tc.want, got, 1e-9)
		})
	}
}

// За дуже великих int64-сум (>2^53 ≈ 9e15) float64 втрачає
// точність — це задокументоване обмеження `Major`-перетворення.
// Тест перевіряє, що ми принаймні не панікуємо і повертаємо щось
// розумне (банк ніколи не повертає таких сум).
func TestMajor_extremeValues(t *testing.T) {
	m := New(1<<60, currency.UAH)
	got := m.Major()
	assert.False(t, got != got, "Major не повинна давати NaN") // NaN != NaN
	assert.Greater(t, got, 0.0)
}

func TestMarshalJSON(t *testing.T) {
	// Money серіалізується як гола int (Code зберігається у сусідньому полі).
	out, err := json.Marshal(New(12345, currency.UAH))
	require.NoError(t, err)
	assert.Equal(t, "12345", string(out))

	// Негативні значення.
	out, err = json.Marshal(New(-500, currency.USD))
	require.NoError(t, err)
	assert.Equal(t, "-500", string(out))
}

func TestUnmarshalJSON(t *testing.T) {
	var m Money
	require.NoError(t, json.Unmarshal([]byte("4200"), &m))
	assert.Equal(t, int64(4200), m.Minor)
	assert.Equal(t, currency.Code(0), m.Code, "Code лишається нульовим — батько проставить")
}

func TestJSON_roundtrip(t *testing.T) {
	original := New(4200, currency.UAH)
	out, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Money
	require.NoError(t, json.Unmarshal(out, &decoded))
	// Minor пройшов; Code треба проставити вручну (батьківська структура зробить це з CurrencyCode).
	decoded.Code = currency.UAH
	assert.Equal(t, original, decoded)
}
