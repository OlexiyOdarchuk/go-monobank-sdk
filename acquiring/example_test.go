package acquiring_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring"
)

func ExampleNew() {
	cli := acquiring.New(os.Getenv("MONO_ACQUIRING_TOKEN"))

	mer, err := cli.MerchantDetails(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(mer.MerchantID, mer.MerchantName)
}

func ExampleClient_CreateInvoice() {
	cli := acquiring.New(os.Getenv("MONO_ACQUIRING_TOKEN"))

	inv, err := cli.CreateInvoice(context.Background(), &acquiring.CreateInvoiceRequest{
		Amount: 4200, // 42.00 UAH in kopecks
		Currency:    980,
		MerchantPaymInfo: &acquiring.MerchantPaymInfo{
			Reference:   "order-2026-42",
			Destination: "Замовлення №42",
		},
		RedirectURL: "https://yourapp.example.com/done",
		WebHookURL:  "https://yourapp.example.com/invoice-cb",
		PaymentType: acquiring.PaymentDebit,
		Validity:    600, // 10 minutes
	})
	if err != nil {
		log.Fatal(err)
	}
	// Show inv.PageURL to the customer (or 302-redirect).
	fmt.Println(inv.InvoiceID, inv.PageURL)
}

func ExampleClient_FinalizeInvoice() {
	// Hold + finalize is the "auth-then-capture" pattern.
	cli := acquiring.New(os.Getenv("MONO_ACQUIRING_TOKEN"))

	// 1. Create with PaymentType: PaymentHold (authorisation only).
	inv, _ := cli.CreateInvoice(context.Background(), &acquiring.CreateInvoiceRequest{
		Amount:      10_000,
		Currency:         980,
		PaymentType: acquiring.PaymentHold,
	})

	// 2. Customer pays; the invoice goes to status "hold".

	// 3. Capture (full amount, or any amount ≤ original).
	out, err := cli.FinalizeInvoice(context.Background(), &acquiring.FinalizeRequest{
		InvoiceID: inv.InvoiceID,
		Amount:    8_000, // capture less than authorised
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(out.Status)
}

func ExampleClient_Statement() {
	cli := acquiring.New(os.Getenv("MONO_ACQUIRING_TOKEN"))

	// Last 24 hours of merchant statement.
	from := time.Now().Add(-24 * time.Hour)
	items, err := cli.Statement(context.Background(), from, time.Time{}, "")
	if err != nil {
		log.Fatal(err)
	}
	for _, op := range items {
		fmt.Printf("%s · %d · %s\n", op.InvoiceID, op.Amount, op.Status)
	}
}
