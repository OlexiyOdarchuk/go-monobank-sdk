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
	"errors"
	"fmt"
	"math"
	"math/bits"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
)

// ErrOverflow is returned by [Money.Add], [Money.Sub] and
// [Money.Mul] when the int64 result would overflow. Callers that
// know their inputs are bounded (single transaction amounts, for
// example) can ignore this safely — but aggregate sums over a
// statement range should always check.
var ErrOverflow = errors.New("money: arithmetic overflow")

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

// FromMajor builds a Money from an amount in major units (hryvnias,
// dollars, ...) and a currency, rounding to the nearest minor unit
// (round half away from zero). The number of decimals comes from
// [currency.Code.Decimals]: 2 for most currencies, 0 for JPY/KRW, 3
// for dinars. It is the inverse of [Money.Major].
//
//	money.FromMajor(42.00, currency.UAH) // {Minor: 4200, Code: 980}
//	money.FromMajor(12.50, currency.JPY) // {Minor: 13,   Code: 392} (0 decimals)
//
// Precision: float64 is exact for integers up to 2^53, which covers
// every realistic single amount. To build Money from a
// human-entered decimal string, prefer [ParseMajor] — it uses
// integer math and avoids the binary-float error of literals like
// 0.1.
func FromMajor(major float64, code currency.Code) Money {
	factor := math.Pow10(code.Decimals())
	r := major * factor
	if r >= 0 {
		return Money{Minor: int64(r + 0.5), Code: code}
	}
	return Money{Minor: int64(r - 0.5), Code: code}
}

// UAH is the hryvnia-specific shorthand for
// FromMajor(major, currency.UAH): it turns 42.00 into 4200 kopecks.
// The vast majority of Mono integrations are UAH-only, and confusing
// hryvnias with kopecks (a 100× error) is the single most common
// money bug — this constructor makes the unit explicit at the call
// site.
//
//	inv := &acquiring.CreateInvoiceRequest{Amount: money.UAH(149.99).Minor}
func UAH(major float64) Money {
	return FromMajor(major, currency.UAH)
}

// ParseMajor parses a decimal string in major units ("42.00",
// "149.99", "-3.5") into Money using integer math, so values like
// "0.10" do not pick up the float64 representation error of the
// literal 0.10. The number of fractional digits accepted is capped
// by the currency's decimals; extra digits are an error rather than
// being silently truncated, so a typo cannot quietly drop kopecks.
//
//	m, err := money.ParseMajor("149.99", currency.UAH) // {14999, 980}
func ParseMajor(s string, code currency.Code) (Money, error) {
	if s == "" {
		return Money{}, fmt.Errorf("money: empty major-unit string")
	}
	neg := false
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		neg = true
		s = s[1:]
	}

	intPart, fracPart, hasFrac := s, "", false
	if i := indexByte(s, '.'); i >= 0 {
		intPart, fracPart, hasFrac = s[:i], s[i+1:], true
	}
	if intPart == "" && fracPart == "" {
		return Money{}, fmt.Errorf("money: %q is not a valid amount", s)
	}

	decimals := code.Decimals()
	if len(fracPart) > decimals {
		return Money{}, fmt.Errorf("money: %q has more than %d fractional digits for %s", s, decimals, code)
	}

	var minor int64
	for _, r := range intPart {
		d, err := digit(r)
		if err != nil {
			return Money{}, fmt.Errorf("money: %q is not a valid amount: %w", s, err)
		}
		next := minor*10 + int64(d)
		if next < minor { // int64 overflow
			return Money{}, fmt.Errorf("%w: %q", ErrOverflow, s)
		}
		minor = next
	}
	// Shift the integer part into minor units.
	for range decimals {
		minor *= 10
	}
	if hasFrac {
		var frac int64
		for _, r := range fracPart {
			d, err := digit(r)
			if err != nil {
				return Money{}, fmt.Errorf("money: %q is not a valid amount: %w", s, err)
			}
			frac = frac*10 + int64(d)
		}
		// Right-pad the fraction to the currency's decimals (so "1.5"
		// in a 2-decimal currency becomes 50, not 5 minor units).
		for i := len(fracPart); i < decimals; i++ {
			frac *= 10
		}
		minor += frac
	}
	if neg {
		minor = -minor
	}
	return Money{Minor: minor, Code: code}, nil
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func digit(r rune) (int, error) {
	if r < '0' || r > '9' {
		return 0, fmt.Errorf("unexpected character %q", r)
	}
	return int(r - '0'), nil
}

// IsZero reports whether the amount is 0 (the currency is ignored).
func (m Money) IsZero() bool { return m.Minor == 0 }

// Equal reports whether two Money values are equal in both amount
// and currency.
func (m Money) Equal(other Money) bool {
	return m.Minor == other.Minor && m.Code == other.Code
}

// Add adds another Money of the same currency. Returns an error
// when the currencies differ (for cross-currency arithmetic use
// [bank.Rates.Convert]) or when the int64 result overflows
// ([ErrOverflow]). At the wire scale Mono uses (single-transaction
// amounts well under MaxInt64) overflow is practically impossible,
// but aggregating a multi-year statement can in theory reach the
// limit — the explicit error makes that visible.
func (m Money) Add(other Money) (Money, error) {
	if m.Code != other.Code {
		return Money{}, fmt.Errorf("money: cannot add %s + %s — different currencies", m.Code, other.Code)
	}
	sum, carry := bits.Add64(uint64(m.Minor), uint64(other.Minor), 0)
	// Signed overflow check: the carry out of the high bit and the
	// sign bit of the result must agree iff both inputs had the same
	// sign as the result. The simple form: overflow when the signs
	// of m and other agree but disagree with the result.
	if (m.Minor < 0) == (other.Minor < 0) && (m.Minor < 0) != (int64(sum) < 0) {
		_ = carry // silence linter; signed-overflow is what we care about
		return Money{}, fmt.Errorf("%w: %d + %d", ErrOverflow, m.Minor, other.Minor)
	}
	return Money{Minor: int64(sum), Code: m.Code}, nil
}

// Sub subtracts another Money of the same currency. The currency
// mismatch error is the same as for [Money.Add]; the overflow error
// is [ErrOverflow].
func (m Money) Sub(other Money) (Money, error) {
	if m.Code != other.Code {
		return Money{}, fmt.Errorf("money: cannot sub %s - %s — different currencies", m.Code, other.Code)
	}
	diff, _ := bits.Sub64(uint64(m.Minor), uint64(other.Minor), 0)
	// Signed-subtraction overflow: when the signs of m and other
	// differ AND the result disagrees with the sign of m.
	if (m.Minor < 0) != (other.Minor < 0) && (m.Minor < 0) != (int64(diff) < 0) {
		return Money{}, fmt.Errorf("%w: %d - %d", ErrOverflow, m.Minor, other.Minor)
	}
	return Money{Minor: int64(diff), Code: m.Code}, nil
}

// Neg returns Money with the sign flipped (the currency does not
// change). Note: math.MinInt64 has no positive counterpart in int64,
// so Neg on that exact value returns the same MinInt64 (a known
// two's-complement edge case). In practice no monetary value comes
// close to that boundary.
func (m Money) Neg() Money { return Money{Minor: -m.Minor, Code: m.Code} }

// Mul multiplies the amount by an integer scalar. Handy for
// aggregates like "10 units at this price". Returns [ErrOverflow]
// when m.Minor*n would overflow int64. For fractional multipliers
// (fees, percentages) use [Money.Scale].
func (m Money) Mul(n int64) (Money, error) {
	if m.Minor == 0 || n == 0 {
		return Money{Minor: 0, Code: m.Code}, nil
	}
	prod := m.Minor * n
	// Cheap overflow check that avoids 128-bit math: divide back and
	// compare. Works for every input except the
	// MinInt64 * -1 corner, which we detect explicitly.
	if (m.Minor == math.MinInt64 && n == -1) || (n == math.MinInt64 && m.Minor == -1) {
		return Money{}, fmt.Errorf("%w: %d * %d", ErrOverflow, m.Minor, n)
	}
	if prod/n != m.Minor {
		return Money{}, fmt.Errorf("%w: %d * %d", ErrOverflow, m.Minor, n)
	}
	return Money{Minor: prod, Code: m.Code}, nil
}

// Scale multiplies the amount by a fractional factor (for example
// 0.05 for a 5% fee), rounding to the nearest minor unit (banker's
// rounding is not used — plain "round half away from zero").
//
// Precision: float64 is exact for integers up to 2^53. Mono's
// per-transaction values comfortably fit, but aggregating across
// many transactions can in theory push m.Minor past 2^53 — at that
// point Scale silently loses precision in the low bits. For
// accounting-grade arithmetic on aggregates, prefer
// integer-only paths: [Money.Mul] for an integer multiplier, or
// hand-rolled (numerator/denominator) scaling.
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
