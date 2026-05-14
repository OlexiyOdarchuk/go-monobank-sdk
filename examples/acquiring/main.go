// Command acquiring demonstrates the acquiring (merchant) API:
// create an invoice for 1.00 UAH, print its checkout URL, then poll
// until the invoice is paid, cancelled or expires.
//
// Usage:
//
//	MONO_ACQUIRING_TOKEN=xxx go run ./examples/acquiring
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring"
)

func sanitizeLogValue(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

func main() {
	token := os.Getenv("MONO_ACQUIRING_TOKEN")
	if token == "" {
		log.Fatal("MONO_ACQUIRING_TOKEN env var is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cli := acquiring.New(token)

	// Sanity check: merchant we're authed as.
	mer, err := cli.MerchantDetails(ctx)
	if err != nil {
		log.Fatalf("MerchantDetails: %s", sanitizeLogValue(err.Error()))
	}
	fmt.Printf("Merchant: %s (EDRPOU %s)\n", mer.MerchantName, mer.EDRPOU)

	// Create a 1.00 UAH invoice (100 kopecks).
	inv, err := cli.CreateInvoice(ctx, &acquiring.CreateInvoiceRequest{
		Amount:   100,
		Currency: 980,
		MerchantPaymInfo: &acquiring.MerchantPaymInfo{
			Reference:   fmt.Sprintf("demo-%d", time.Now().Unix()),
			Destination: "monobank-sdk example invoice",
		},
		Validity:    600, // 10 minutes
		PaymentType: acquiring.PaymentDebit,
	})
	if err != nil {
		log.Fatalf("CreateInvoice: %s", sanitizeLogValue(err.Error()))
	}
	fmt.Printf("Invoice %s\n  Checkout: %s\n", inv.InvoiceID, inv.PageURL)

	// Poll status until terminal.
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Fatalf("waiting: %s", sanitizeLogValue(ctx.Err().Error()))
		case <-tick.C:
		}

		st, err := cli.InvoiceStatus(ctx, inv.InvoiceID)
		if err != nil {
			log.Fatalf("InvoiceStatus: %s", sanitizeLogValue(err.Error()))
		}
		fmt.Printf("  status=%s amount=%d finalAmount=%d\n",
			st.Status, st.Amount, st.FinalAmount)

		switch st.Status {
		case acquiring.InvoiceSuccess, acquiring.InvoiceFailure,
			acquiring.InvoiceReversed, acquiring.InvoiceExpired:
			return
		}
	}
}
