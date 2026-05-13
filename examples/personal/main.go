// Command personal demonstrates the Personal Open API:
// fetch ClientInfo, list accounts/jars with typed currency, and print
// the last 7 days of transactions on the first account with a bucketed
// MCC category.
//
// Usage:
//
//	MONO_TOKEN=xxx go run ./examples/personal
//
// Get a token at https://api.monobank.ua/. The personal API is
// rate-limited to one /personal/client-info call per 60 seconds and one
// /personal/statement call per 60 seconds per account, so don't run this
// in a tight loop.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/personal"
)

func main() {
	token := os.Getenv("MONO_TOKEN")
	if token == "" {
		log.Fatal("MONO_TOKEN env var is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cli := personal.New(token)

	info, err := cli.ClientInfo(ctx)
	if err != nil {
		log.Fatalf("ClientInfo: %v", err)
	}

	fmt.Printf("# %s (%s)\n", info.Name, info.ID)
	fmt.Println()
	fmt.Println("## Accounts")
	for _, a := range info.Accounts {
		fmt.Printf("  %-12s %s · balance %s · %d cards\n",
			a.Type, a.IBAN, a.Balance, len(a.CardMasks))
	}
	if len(info.Jars) > 0 {
		fmt.Println()
		fmt.Println("## Jars")
		for _, j := range info.Jars {
			fmt.Printf("  %s · %s / %s\n", j.Title, j.Balance, j.Goal)
		}
	}

	if len(info.Accounts) == 0 {
		return
	}

	fmt.Println()
	fmt.Println("## Last 7 days (first account)")
	to := time.Now()
	from := to.Add(-7 * 24 * time.Hour)
	txs, err := cli.Transactions(ctx, info.Accounts[0].AccountID, from, to)
	if err != nil {
		log.Fatalf("Transactions: %v", err)
	}
	for _, t := range txs {
		fmt.Printf("  %s · %+8s · %-16s · %s\n",
			t.Time.Format(time.RFC3339),
			t.Amount, t.MCCCode().Category(),
			t.Description)
	}
	if len(txs) == 0 {
		fmt.Println("  (no transactions in window)")
	}
}
