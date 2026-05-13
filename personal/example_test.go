package personal_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/personal"
)

func ExampleNew() {
	cli := personal.New(os.Getenv("MONO_TOKEN"))

	info, err := cli.ClientInfo(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	for _, a := range info.Accounts {
		fmt.Printf("%s · %d %s\n",
			a.IBAN, a.Balance, currency.Code(a.CurrencyCode))
	}
}

func ExampleClient_TransactionsRange() {
	cli := personal.New(os.Getenv("MONO_TOKEN"))

	// Statement window: 90 days. Mono caps a single call at 31 days, so
	// TransactionsRange transparently pages.
	to := time.Now()
	from := to.Add(-90 * 24 * time.Hour)

	txs, err := cli.TransactionsRange(context.Background(), "acc-id", from, to)
	if err != nil {
		log.Fatal(err)
	}
	for _, t := range txs {
		fmt.Printf("%s · %d · %s\n",
			t.Time.Format(time.RFC3339),
			t.Amount, t.MCCCode().Category())
	}
}

func ExampleClient_SetWebHook() {
	cli := personal.New(os.Getenv("MONO_TOKEN"))

	if err := cli.SetWebHook(context.Background(), "https://yourapp.example.com/webhook"); err != nil {
		log.Fatal(err)
	}
	// From now on mono POSTs StatementItem events to that URL.
	// See the webhook package for a drop-in http.Handler that verifies
	// signatures and dedupes mono's 60s/600s retries.
}
