package currency_test

import (
	"fmt"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
)

func ExampleCode_String() {
	// Account.CurrencyCode arrives as int; wrap to get the alpha-3 name.
	codes := []int{980, 840, 978, 7777}
	for _, raw := range codes {
		fmt.Printf("%d → %s\n", raw, currency.Code(raw))
	}
	// Output:
	// 980 → UAH
	// 840 → USD
	// 978 → EUR
	// 7777 → 7777
}
