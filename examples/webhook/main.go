// Command webhook receives signed monobank personal-API webhooks and
// prints a one-line summary of each transaction. It uses webhook.Handler
// which takes care of signature verification, key rotation and parsing.
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/webhook"
)

func main() {
	keys := bank.New()

	h, err := webhook.NewHandler(context.Background(), webhook.Options{
		Keys: keys,
		OnEvent: func(_ context.Context, event *webhook.Response) error {
			t := event.Data.Transaction
			log.Printf("account=%s amount=%d %q hold=%v",
				event.Data.AccountID, t.Amount, t.Description, t.Hold)
			return nil
		},
		OnError: func(err error) { log.Printf("webhook: %v", err) },
	})
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/webhook", h)
	log.Printf("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
