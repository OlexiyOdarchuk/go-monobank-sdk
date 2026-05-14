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
	JPY Code = 392 // Japanese yen (0 decimals)
	CZK Code = 203 // Czech koruna
	CAD Code = 124 // Canadian dollar
	AUD Code = 36  // Australian dollar
	CNY Code = 156 // Chinese yuan
	// 3-decimal currencies — uncommon in Mono payloads but well-defined
	// by ISO 4217. Kept here for [Code.Decimals] correctness on
	// cross-currency conversions.
	BHD Code = 48  // Bahraini dinar (3 decimals)
	JOD Code = 400 // Jordanian dinar (3 decimals)
	KWD Code = 414 // Kuwaiti dinar (3 decimals)
	OMR Code = 512 // Omani rial (3 decimals)
	TND Code = 788 // Tunisian dinar (3 decimals)
	// 0-decimal currencies.
	KRW Code = 410 // South Korean won (0 decimals)
)

// alpha3 maps the numeric codes above to their alpha-3 equivalents.
// Kept package-private; access through [Code.String]. The map is
// effectively read-only at runtime — do not mutate.
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
	BHD: "BHD",
	JOD: "JOD",
	KWD: "KWD",
	OMR: "OMR",
	TND: "TND",
	KRW: "KRW",
}

// fromAlpha3 is the reverse map of [alpha3]: "UAH" → 980. It is
// populated in init() from alpha3 to keep a single source of truth.
// Kept package-private; access through [FromAlpha3].
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
// are not in the map. Kept package-private; access through
// [Code.Decimals].
var decimals = map[Code]int{
	// 0 decimals.
	JPY: 0,
	KRW: 0,
	// 3 decimals.
	BHD: 3,
	JOD: 3,
	KWD: 3,
	OMR: 3,
	TND: 3,
}

// Decimals returns the number of decimal places for the currency's
// major unit per ISO 4217. The default is 2 (as for UAH/USD/EUR/...).
// JPY/KRW use 0, BHD/JOD/KWD/OMR/TND use 3.
//
// Used by [money.Money.Major] and currency-aware money builders for
// the correct minor ↔ major conversion (1250 kopecks = 12.50 UAH;
// 1250 yen = 1250 yen; 1250 BHD-fils = 1.250 BHD).
func (c Code) Decimals() int {
	if d, ok := decimals[c]; ok {
		return d
	}
	return 2
}

// MinorPerMajor returns 10^Decimals — the factor that converts
// between a currency's major and minor units (100 for UAH/USD,
// 1 for JPY/KRW, 1000 for BHD/JOD/KWD/OMR/TND). Useful for
// currency-aware float-to-int conversion in API payloads that
// ship balance as a decimal float.
func (c Code) MinorPerMajor() int64 {
	switch c.Decimals() {
	case 0:
		return 1
	case 1:
		return 10
	case 2:
		return 100
	case 3:
		return 1000
	default:
		// Defensive fallback; not reachable for any ISO 4217 code.
		p := int64(1)
		for range c.Decimals() {
			p *= 10
		}
		return p
	}
}
