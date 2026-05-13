package mcc_test

import (
	"fmt"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/mcc"
)

func ExampleCode_Category() {
	// MCC codes come straight off a Transaction:
	codes := []mcc.Code{5411, 5541, 5812, 4829, 8398, 9999, 1234}
	for _, c := range codes {
		fmt.Printf("%d → %s\n", c, c.Category())
	}
	// Output:
	// 5411 → Groceries
	// 5541 → Fuel
	// 5812 → Restaurants
	// 4829 → MoneyTransfer
	// 8398 → Charity
	// 9999 → Government
	// 1234 → Agriculture
}
