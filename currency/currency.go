// Package currency provides typed ISO 4217 numeric currency codes
// that arrive in Mono payloads. The bank ships the currency as a
// plain int (Account.CurrencyCode, Transaction.CurrencyCode etc.) —
// wrap it in [Code] for typed comparisons and to obtain the alpha-3
// name via [Code.String].
package currency

import "strconv"

// Code is an ISO 4217 numeric currency code.
type Code int

// Currency codes seen in Mono payloads. The list is non-exhaustive —
// extend as needed.
const (
	UAH Code = 980 // Ukrainian hryvnia (account default)
	USD Code = 840 // US dollar
	EUR Code = 978 // Euro
	GBP Code = 826 // Pound sterling
	PLN Code = 985 // Polish złoty
	CHF Code = 756 // Swiss franc
	JPY Code = 392 // Japanese yen
	CZK Code = 203 // Czech koruna
	CAD Code = 124 // Canadian dollar
	AUD Code = 36  // Australian dollar
	CNY Code = 156 // Chinese yuan
)

// alpha3 maps the numeric codes above to their alpha-3 equivalents.
var alpha3 = map[Code]string{
	UAH: "UAH",
	USD: "USD",
	EUR: "EUR",
	GBP: "GBP",
	PLN: "PLN",
	CHF: "CHF",
	JPY: "JPY",
	CZK: "CZK",
	CAD: "CAD",
	AUD: "AUD",
	CNY: "CNY",
}

// fromAlpha3 is the reverse map of [alpha3]: "UAH" → 980. It is
// populated in init() from alpha3 to keep a single source of truth.
var fromAlpha3 map[string]Code

func init() {
	fromAlpha3 = make(map[string]Code, len(alpha3))
	for code, name := range alpha3 {
		fromAlpha3[name] = code
	}
}

// FromAlpha3 returns the numeric code for an alpha-3 name (for
// example "UAH" → 980). ok=false when the currency is unknown to
// the SDK (extend [alpha3] if needed).
//
// Used by APIs that ship the currency as a string (for example
// corp-api /ext/v1/statement: `"currencyCode": "UAH"`).
func FromAlpha3(s string) (Code, bool) {
	c, ok := fromAlpha3[s]
	return c, ok
}

// String returns the alpha-3 code (for example "UAH") when the
// currency is known, or the decimal form of the numeric code as a
// fallback for unknown currencies. That makes the type fmt-printable
// directly:
//
//	fmt.Println(currency.Code(t.CurrencyCode)) // "UAH" or "7777"
func (c Code) String() string {
	if s, ok := alpha3[c]; ok {
		return s
	}
	return strconv.Itoa(int(c))
}

// decimals overrides the default of 2 for currencies for which ISO
// 4217 fixes a different number of decimal places. Most currencies
// (UAH/USD/EUR/GBP/PLN/CHF/CZK/CAD/AUD/CNY) have 2 decimals, so they
// are not in the map.
var decimals = map[Code]int{
	JPY: 0, // Japanese yen — the minor unit is not used
}

// Decimals returns the number of decimal places for the currency's
// major unit per ISO 4217. The default is 2 (as for UAH/USD/EUR/...).
// Some currencies (JPY=0, KRW=0, BHD/JOD/KWD/OMR/TND=3) have other
// values — add them to [decimals] if you need them.
//
// Used by [money.Money.Major] for the correct minor → major
// conversion (1250 kopecks = 12.50 UAH; 1250 yen = 1250 yen).
func (c Code) Decimals() int {
	if d, ok := decimals[c]; ok {
		return d
	}
	return 2
}
