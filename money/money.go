// Package money provides a typed representation of monetary amounts
// together with their currency — so "10 kopecks" and "10 cents" do
// not get conflated inside the same int64.
//
// The representation is a pair (Minor, Code), where Minor is the
// currency's minor unit (kopecks for UAH, cents for USD/EUR) and
// Code is the ISO 4217 numeric code. In JSON, [Money] serializes as
// a bare count of minor units (for wire compatibility with Mono's
// format, where amount and currency are separate fields) — parent
// structures set Code from the adjacent CurrencyCode field in their
// own [json.Unmarshaler].
package money

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
)

// Money is an amount in minor units (kopecks, cents) with its
// currency attached. The zero value is 0 in an unknown currency.
type Money struct {
	Minor int64
	Code  currency.Code
}

// New builds a Money from a minor-unit count and a currency.
func New(minor int64, code currency.Code) Money {
	return Money{Minor: minor, Code: code}
}

// IsZero reports whether the amount is 0 (the currency is ignored).
func (m Money) IsZero() bool { return m.Minor == 0 }

// Equal reports whether two Money values are equal in both amount
// and currency.
func (m Money) Equal(other Money) bool {
	return m.Minor == other.Minor && m.Code == other.Code
}

// Add adds another Money of the same currency. Returns an error when
// the currencies differ (for cross-currency arithmetic use
// [bank.Rates.Convert]).
func (m Money) Add(other Money) (Money, error) {
	if m.Code != other.Code {
		return Money{}, fmt.Errorf("money: cannot add %s + %s — different currencies", m.Code, other.Code)
	}
	return Money{Minor: m.Minor + other.Minor, Code: m.Code}, nil
}

// Sub subtracts another Money of the same currency. The error is
// the same as for [Money.Add].
func (m Money) Sub(other Money) (Money, error) {
	if m.Code != other.Code {
		return Money{}, fmt.Errorf("money: cannot sub %s - %s — different currencies", m.Code, other.Code)
	}
	return Money{Minor: m.Minor - other.Minor, Code: m.Code}, nil
}

// Neg returns Money with the sign flipped (the currency does not
// change).
func (m Money) Neg() Money { return Money{Minor: -m.Minor, Code: m.Code} }

// Mul multiplies the amount by an integer scalar. Handy for
// aggregates like "10 units at this price". For fractional
// multipliers (fees, percentages) use [Money.Scale].
func (m Money) Mul(n int64) Money { return Money{Minor: m.Minor * n, Code: m.Code} }

// Scale multiplies the amount by a fractional factor (for example
// 0.05 for a 5% fee), rounding to the nearest minor unit (banker's
// rounding is not used — plain "round half away from zero").
func (m Money) Scale(factor float64) Money {
	r := float64(m.Minor) * factor
	if r >= 0 {
		return Money{Minor: int64(r + 0.5), Code: m.Code}
	}
	return Money{Minor: int64(r - 0.5), Code: m.Code}
}

// Major returns the amount in major units (UAH/USD/EUR/...). The
// number of decimals comes from [currency.Code.Decimals]: 2 for most
// currencies, 0 for JPY (1250 yen = 1250 units, not 12.50). For
// currencies the SDK does not know, 2 decimals are assumed.
func (m Money) Major() float64 {
	d := m.Code.Decimals()
	if d == 0 {
		return float64(m.Minor)
	}
	return float64(m.Minor) / math.Pow10(d)
}

// String formats the amount as "42.50 UAH" (the number of decimals
// matches the currency — 2 for UAH, 0 for JPY etc.). Handy for logs
// and debugging.
func (m Money) String() string {
	return fmt.Sprintf("%.*f %s", m.Code.Decimals(), m.Major(), m.Code)
}

// MarshalJSON serializes Money as a bare count of minor units —
// compatible with Mono's wire format (where amount and currency are
// separate fields).
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.Minor)
}

// UnmarshalJSON reads a bare int64 into Minor. Code stays zero —
// the parent structure sets it in its own UnmarshalJSON by copying
// from the adjacent CurrencyCode field.
func (m *Money) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &m.Minor)
}
