package installment

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Money is an exact UAH amount used in installment payloads.
// Internally a count of kopecks (1 UAH = 100 kopecks). The wire
// format is a JSON number with up to 2 decimal places — Mono's
// installment API does not accept other currencies, so a single
// fixed scale of 100 is correct.
//
// Two motivations for the type:
//
//  1. The float64 representation that the package historically used
//     silently loses cents on values near 2^53/100 and on every
//     repeated addition. Banking arithmetic must round-trip
//     exactly.
//  2. JSON numbers are parsed through float64 by Go's encoding/json
//     by default; using a custom UnmarshalJSON ensures the decimal
//     representation is preserved bit-for-bit on the way in too.
//
// Construction:
//
//	installment.NewMoney(2499, 99)          // 2499.99 UAH
//	installment.MoneyFromKopecks(249999)    // 2499.99 UAH
//	installment.MoneyFromMajor(2499.99)     // convenience — rounds half away from zero
//
// Arithmetic is done on the integer Kopecks field directly.
type Money struct {
	// Kopecks holds the value in 1/100 of UAH. May be negative for
	// refunds; constructors clamp fractional inputs by rounding
	// half away from zero.
	Kopecks int64
}

// ErrInvalidMoney is returned by [Money.UnmarshalJSON] when the wire
// value cannot be parsed as a non-negative decimal number.
var ErrInvalidMoney = errors.New("installment: invalid money value")

// NewMoney builds a [Money] from integer (hryvnia, kopecks) parts.
// Both parts must be non-negative; the sign of the resulting amount
// is positive. For a negative value (refund), negate the result
// with -.Kopecks.
func NewMoney(hryvnia, kopecks int64) Money {
	return Money{Kopecks: hryvnia*100 + kopecks}
}

// MoneyFromKopecks wraps a raw kopecks count.
func MoneyFromKopecks(kopecks int64) Money { return Money{Kopecks: kopecks} }

// MoneyFromMajor converts a hryvnia (major-unit) float to Money,
// rounding to the nearest kopeck (round half away from zero).
//
// Convenience only — for accounting-grade values prefer [NewMoney]
// or [MoneyFromKopecks]. Inputs whose absolute value exceeds
// MaxInt64/100 saturate at the int64 boundary.
func MoneyFromMajor(major float64) Money {
	if math.IsNaN(major) || math.IsInf(major, 0) {
		return Money{}
	}
	scaled := major * 100
	if scaled >= float64(math.MaxInt64) {
		return Money{Kopecks: math.MaxInt64}
	}
	if scaled <= float64(math.MinInt64) {
		return Money{Kopecks: math.MinInt64}
	}
	if scaled >= 0 {
		return Money{Kopecks: int64(scaled + 0.5)}
	}
	return Money{Kopecks: int64(scaled - 0.5)}
}

// Major returns the value as hryvnia (a float64). Precision is exact
// for |Kopecks| < 2^53; beyond that float64 starts losing low bits.
// Use [Money.String] when an exact decimal representation matters.
func (m Money) Major() float64 { return float64(m.Kopecks) / 100 }

// String formats Money as a fixed-point decimal with exactly 2
// decimals: "2499.99", "0.05", "-100.00". The output matches what
// MarshalJSON emits.
func (m Money) String() string {
	neg := m.Kopecks < 0
	abs := m.Kopecks
	if neg {
		abs = -abs
	}
	hr := abs / 100
	kop := abs % 100
	if neg {
		return fmt.Sprintf("-%d.%02d", hr, kop)
	}
	return fmt.Sprintf("%d.%02d", hr, kop)
}

// IsZero reports whether the amount is exactly zero.
func (m Money) IsZero() bool { return m.Kopecks == 0 }

// MarshalJSON emits Money as a JSON number with exactly 2 decimals.
// For example: 2499.99 or 0.05. Trailing zeros are kept so the wire
// representation is stable and round-trips through `cat | jq`.
func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(m.String()), nil
}

// UnmarshalJSON accepts both JSON numbers (2499.99, 100, 1.5) and
// JSON strings ("2499.99") so the decoder is forward-compatible
// with whatever wire shape Mono ships. Decoding is exact — the
// kopecks count is derived from string digits, not from float64.
//
// Returns [ErrInvalidMoney] when the value is not a valid decimal.
func (m *Money) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "null" || s == "" {
		m.Kopecks = 0
		return nil
	}
	// Strip optional quotes (some endpoints ship money as a string).
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if s == "" {
		m.Kopecks = 0
		return nil
	}
	k, err := parseDecimalKopecks(s)
	if err != nil {
		return fmt.Errorf("%w: %q: %v", ErrInvalidMoney, s, err)
	}
	m.Kopecks = k
	return nil
}

// parseDecimalKopecks parses "-2499.99", "2499", "2499.9", ".05" etc.
// into kopecks WITHOUT going through float64.
func parseDecimalKopecks(s string) (int64, error) {
	neg := false
	switch {
	case s[0] == '+':
		s = s[1:]
	case s[0] == '-':
		neg = true
		s = s[1:]
	}
	if s == "" {
		return 0, errors.New("empty number")
	}
	dot := strings.IndexByte(s, '.')
	var hrPart, kopPart string
	if dot < 0 {
		hrPart = s
	} else {
		hrPart = s[:dot]
		kopPart = s[dot+1:]
	}
	if hrPart == "" {
		hrPart = "0"
	}
	var hr int64
	if hrPart != "" {
		v, err := strconv.ParseInt(hrPart, 10, 64)
		if err != nil {
			return 0, err
		}
		hr = v
	}
	var kop int64
	if kopPart != "" {
		// Pad / truncate to exactly 2 digits.
		switch {
		case len(kopPart) == 1:
			kopPart += "0"
		case len(kopPart) > 2:
			// Round half away from zero.
			rest := kopPart[2:]
			kopPart = kopPart[:2]
			if rest[0] >= '5' {
				k, err := strconv.ParseInt(kopPart, 10, 64)
				if err != nil {
					return 0, err
				}
				kop = k + 1
				kopPart = ""
			}
		}
		if kopPart != "" {
			k, err := strconv.ParseInt(kopPart, 10, 64)
			if err != nil {
				return 0, err
			}
			kop = k
		}
	}
	if hr < 0 {
		return 0, errors.New("hryvnia part must not have its own sign")
	}
	total := hr*100 + kop
	if neg {
		total = -total
	}
	return total, nil
}
