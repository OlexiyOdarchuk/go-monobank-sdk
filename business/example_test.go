package business_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/business"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
)

func ExampleNew() {
	cli := business.New(os.Getenv("MONO_BUSINESS_TOKEN"))

	accs, err := cli.Accounts(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	for _, a := range accs {
		fmt.Printf("%s · %.2f %s\n", a.IBAN, a.Balance, currency.Code(a.Currency))
	}
}

func ExampleClient_PreparePayment() {
	cli := business.New(os.Getenv("MONO_BUSINESS_TOKEN"))

	// Idempotency-Key must be a fresh UUID v4 for each new payment
	// attempt — retrying with the same key is safe.
	key, err := business.NewIdempotencyKey()
	if err != nil {
		log.Fatal(err)
	}
	out, err := cli.PreparePayment(context.Background(), key,
		&business.PaymentRequest{
			SenderIBAN: "UA293220010000026000000000001",
			Receiver: business.PaymentReceiver{
				IBAN:   "UA293220010000026002700000001",
				EDRPOU: "12345678",
				Name:   `ТОВ "Контрагент"`,
			},
			Destination:       "оплата за послуги згідно рах. №42",
			Amount:            100_000, // 1000.00 UAH in kopecks
			Currency:          "UAH",
			ExternalReference: "invoice-2026-001",
		})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("payment id:", out.ID)
}

func ExampleClient_Statement() {
	cli := business.New(os.Getenv("MONO_BUSINESS_TOKEN"))

	from := time.Now().Add(-7 * 24 * time.Hour)
	items, err := cli.Statement(context.Background(),
		"UA293220010000026000000000001",
		from, time.Time{}, // empty `to` → "until now"
		business.StatementDown, 100)
	if err != nil {
		log.Fatal(err)
	}
	for _, op := range items {
		fmt.Printf("%s · %+d %s · %s\n",
			time.Unix(op.Time.Unix(), 0).Format(time.RFC3339),
			op.Amount, op.CurrencyAlpha3, op.Description)
	}
}
