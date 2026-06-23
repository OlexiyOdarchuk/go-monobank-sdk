package acquiring_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
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
		Amount:   4200, // 42.00 UAH in kopecks
		Currency: 980,
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
		Currency:    980,
		PaymentType: acquiring.PaymentHold,
	})

	// 2. Customer pays; the invoice goes to status "hold".

	// 3. Capture (full amount, or any amount ≤ original).
	out, err := cli.FinalizeInvoice(context.Background(), &acquiring.FinalizeRequest{
		InvoiceID: inv.InvoiceID,
		Amount:    8_000, // capture less than authorized
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

// NewBasket валідовано збирає кошик: обовʼязковий code, total=qty*sum,
// а Build звіряє суму кошика з amount інвойсу.
func ExampleNewBasket() {
	items, err := acquiring.NewBasket().
		AddItem("Кава", "SKU-1", 2, money.UAH(45).Minor, acquiring.WithUnit("шт.")).
		AddItem("Чай", "SKU-2", 1, money.UAH(30).Minor).
		Build(money.UAH(120).Minor) // 2×45 + 1×30 = 120.00
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(len(items), items[0].Total)
	// Output: 2 9000
}

// ReconcileStatement порівнює виписку банку з локальними записами і
// показує неузгодженості (суми, статуси, відсутні з одного боку).
func ExampleReconcileStatement() {
	stmt := []acquiring.StatementInvoice{
		{InvoiceID: "inv-1", Amount: money.New(14999, currency.UAH), Status: acquiring.StatusSuccess},
	}
	local := map[string]acquiring.LocalPayment{
		"inv-1": {Amount: money.New(9999, currency.UAH)}, // локально інша сума
	}

	rec := acquiring.ReconcileStatement(stmt, local)
	fmt.Println(rec.Clean(), rec.Mismatches[0].Reason)
	// Output: false amount
}

// ClassifyCharge драйвить grace-логіку підписок: невдале списання при
// живому токені — тимчасове (тримати доступ), мертвий токен — ні.
func ExampleClassifyCharge() {
	h := acquiring.ClassifyCharge(acquiring.SubscriptionPaymentFailed, acquiring.WalletCreated)
	fmt.Println(h, h.GraceEligible())
	// Output: charge_failed true
}

func ExampleClient_PollInvoice() {
	cli := acquiring.New(os.Getenv("MONO_ACQUIRING_TOKEN"))

	// Реконсіляційний fallback на випадок втраченого webhook (і єдиний
	// спосіб побачити "expired", по якому webhook не приходить).
	inv, err := cli.PollInvoice(context.Background(), "p2_9ZgpZVsl3", acquiring.PollOptions{
		Interval: time.Second,
		Timeout:  2 * time.Minute,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(inv.Status)
}

func ExampleNewWebhookHandler() {
	cli := acquiring.New(os.Getenv("MONO_ACQUIRING_TOKEN"))

	h, err := acquiring.NewWebhookHandler(context.Background(), acquiring.WebhookHandlerOptions{
		Keys:   cli,
		Dedup:  acquiring.NewMemoryDeduper(4096), // у проді — persistent
		MaxAge: 15 * time.Minute,                 // anti-replay
		OnEvent: func(_ context.Context, inv *acquiring.InvoiceStatusResponse) error {
			fmt.Println("invoice", inv.InvoiceID, "→", inv.Status)
			return nil // помилка → 500 → Mono ретраїть
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/mono/webhook", h)
}

// IsNotFound / IsTooManyRequests класифікують типізовану помилку
// еквайрингу ({errCode, errText}) без розбору рядків.
func ExampleIsNotFound() {
	cli := acquiring.New(os.Getenv("MONO_ACQUIRING_TOKEN"))

	_, err := cli.InvoiceStatus(context.Background(), "does-not-exist")
	switch {
	case acquiring.IsNotFound(err):
		fmt.Println("невідомий invoiceId")
	case acquiring.IsTooManyRequests(err):
		fmt.Println("rate limit — back off")
	case err != nil:
		log.Fatal(err)
	}
}
