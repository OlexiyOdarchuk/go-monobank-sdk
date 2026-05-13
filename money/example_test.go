package money_test

import (
	"encoding/json"
	"fmt"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
)

func ExampleNew() {
	price := money.New(12550, currency.UAH) // 125.50 грн (мінорні одиниці)
	fmt.Println(price)
	// Output: 125.50 UAH
}

// Add повертає помилку при різних валютах — інших комбінацій банк не приймає.
func ExampleMoney_Add() {
	a := money.New(10000, currency.UAH)
	b := money.New(5000, currency.UAH)

	sum, err := a.Add(b)
	if err != nil {
		panic(err)
	}
	fmt.Println(sum)
	// Output: 150.00 UAH
}

// Money серіалізується JSON-сумісно з форматом Mono — як ціле число
// мінорних одиниць у полі amount.
func ExampleMoney_MarshalJSON() {
	type Invoice struct {
		Amount money.Money `json:"amount"`
	}
	inv := Invoice{Amount: money.New(12550, currency.UAH)}

	out, _ := json.Marshal(inv)
	fmt.Println(string(out))
	// Output: {"amount":12550}
}
