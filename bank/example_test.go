package bank_test

import (
	"context"
	"fmt"
	"log"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
)

func ExampleClient_Rates() {
	cli := bank.New()

	rates, err := cli.Rates(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range rates {
		a := currency.Code(r.CurrencyCodeA)
		b := currency.Code(r.CurrencyCodeB)
		if a == currency.USD && b == currency.UAH {
			fmt.Printf("USD/UAH buy=%.2f sell=%.2f\n", r.RateBuy, r.RateSell)
		}
	}
}

// Конвертація через крос-курс банку. Якщо прямої пари немає — Rates.Convert
// автоматично йде через UAH.
func ExampleRates_Convert() {
	cli := bank.New()

	rates, err := cli.Rates(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	hundredUSD := money.New(10000, currency.USD) // 100.00 USD
	uah, err := rates.Convert(hundredUSD, currency.UAH)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("100 USD ≈ %s\n", uah)
}

// ServerKey кешуйте локально й рефрешите лише за невідповідністю
// X-Key-Id у вебхуку — webhook.NewHandler робить це автоматично.
func ExampleClient_ServerKey() {
	cli := bank.New()

	key, err := cli.ServerKey(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("server key id: %s (curve %s)\n", key.ID, key.PubKey.Curve.Params().Name)
}
