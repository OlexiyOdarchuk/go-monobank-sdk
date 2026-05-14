// Command business demonstrates the corp-api.monobank.ua client
// (юридичні особи / legal-entity API): list company accounts, fetch
// recent statement entries, and prepare a sample outgoing payment.
//
// Usage:
//
//	MONO_BUSINESS_TOKEN=xxx go run ./examples/business
//
// Get a token at https://web.monobank.ua/?modal=tokens.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/business"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
)

func main() {
	token := os.Getenv("MONO_BUSINESS_TOKEN")
	if token == "" {
		log.Fatal("MONO_BUSINESS_TOKEN env var is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cli := business.New(token)

	accs, err := cli.Accounts(ctx)
	if err != nil {
		log.Fatalf("Accounts: %v", err)
	}
	fmt.Println("# Accounts")
	for _, a := range accs {
		// BalanceMoney() rounds to the correct minor unit per
		// currency (1 for JPY, 100 for UAH/USD/EUR, 1000 for BHD/JOD)
		// — safer than %.2f, which silently truncates for JPY and
		// underflows for 3-decimal currencies.
		_ = currency.Code(a.Currency) // keep currency import in case of future use
		fmt.Printf("  %s · balance %s\n", a.IBAN, a.BalanceMoney().String())
	}

	if len(accs) == 0 {
		return
	}

	// Statement for the first account, last 7 days.
	from := time.Now().Add(-7 * 24 * time.Hour)
	to := time.Now()
	items, err := cli.Statement(ctx, accs[0].IBAN, from, to, business.StatementDown, 50)
	if err != nil {
		safeErr := strings.ReplaceAll(err.Error(), "\n", "")
		safeErr = strings.ReplaceAll(safeErr, "\r", "")
		log.Fatalf("Statement: %s", safeErr)
	}
	fmt.Println()
	fmt.Println("# Recent operations on", accs[0].IBAN)
	for _, op := range items {
		fmt.Printf("  %s · %+8s · %-9s · %s\n",
			time.Unix(op.Time.Unix(), 0).Format(time.RFC3339),
			op.Amount, op.Status, op.Description)
	}
}
