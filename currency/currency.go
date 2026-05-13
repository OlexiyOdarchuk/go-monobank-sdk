// Package currency — типізовані ISO 4217 числові коди валют, що
// приходять у payload-ах Mono. Банк віддає валюту як звичайний int
// (Account.CurrencyCode, Transaction.CurrencyCode тощо) — обгорни його
// в [Code] для типізованих порівнянь і отримання alpha-3 імені через
// [Code.String].
package currency

import "strconv"

// Code — ISO 4217 числовий код валюти.
type Code int

// Коди валют, що зустрічаються у payload-ах Mono. Список не вичерпний —
// додавай за потребою.
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

// alpha3 — мапа з числових кодів вище у їх alpha-3 еквіваленти.
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

// fromAlpha3 — зворотна мапа до [alpha3]: "UAH" → 980. Заповнюється у
// init() з alpha3, щоб тримати єдине джерело правди.
var fromAlpha3 map[string]Code

func init() {
	fromAlpha3 = make(map[string]Code, len(alpha3))
	for code, name := range alpha3 {
		fromAlpha3[name] = code
	}
}

// FromAlpha3 повертає числовий код за alpha-3 ім'ям (наприклад "UAH" → 980).
// ok=false, якщо валюта не відома SDK (доповни [alpha3], якщо треба).
//
// Потрібно тим API, що шлють валюту рядком (наприклад, corp-api
// /ext/v1/statement: `"currencyCode": "UAH"`).
func FromAlpha3(s string) (Code, bool) {
	c, ok := fromAlpha3[s]
	return c, ok
}

// String повертає alpha-3 код (наприклад "UAH"), якщо валюта відома, або
// десяткове представлення числового коду — як fallback для невідомих
// валют. Це робить тип сумісним з fmt-друком напряму:
//
//	fmt.Println(currency.Code(t.CurrencyCode)) // "UAH" або "7777"
func (c Code) String() string {
	if s, ok := alpha3[c]; ok {
		return s
	}
	return strconv.Itoa(int(c))
}
