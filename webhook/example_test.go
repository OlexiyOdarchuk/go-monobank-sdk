package webhook_test

import (
	"context"
	"log"
	"net/http"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/webhook"
)

func ExampleNewHandler() {
	// bank.Client implements webhook.KeyProvider via /bank/sync.
	keys := bank.New()

	h, err := webhook.NewHandler(context.Background(), webhook.Options{
		Keys:  keys,
		Dedup: webhook.NewMemoryDeduper(1024),
		OnEvent: func(_ context.Context, e *webhook.Response) error {
			t := e.Data.Transaction
			log.Printf("%s · %d %d", e.Data.AccountID, t.Amount, t.CurrencyCode)
			return nil
		},
		OnError: func(err error) { log.Printf("webhook: %v", err) },
	})
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/webhook", h)
	_ = http.ListenAndServe(":8080", nil)
}

func ExampleVerify() {
	// Webhook callbacks come signed in the X-Sign header. Use the
	// PubKey from bank.Client.ServerKey for verification; cache the
	// key and re-fetch when X-Key-Id stops matching.
	body := []byte(`{"type":"StatementItem","data":{}}`)
	xSign := "..." // r.Header.Get("X-Sign")

	keys := bank.New()
	sk, err := keys.ServerKey(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	if err := webhook.Verify(sk.PubKey, body, xSign); err != nil {
		log.Printf("invalid signature: %v", err)
		return
	}
	// safe to trust body now
}

func ExampleParse() {
	body := []byte(`{"type":"StatementItem","data":{"account":"acc","statementItem":{"id":"tx-1","amount":-1234}}}`)
	r, err := webhook.Parse(body)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%s · amount %d", r.Data.AccountID, r.Data.Transaction.Amount)
}
