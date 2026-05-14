// Command webhook receives signed monobank personal-API webhooks and
// prints a one-line summary of each transaction. It uses webhook.Handler
// which takes care of signature verification, key rotation and parsing.
package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/webhook"
)

func sanitizeForLog(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

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
		OnError: func(err error) { log.Printf("webhook: %s", sanitizeForLog(err.Error())) },
	})
	if err != nil {
		log.Fatal(sanitizeForLog(err.Error()))
	}

	mux := http.NewServeMux()
	mux.Handle("/webhook", h)
	// ReadHeaderTimeout is required to defend against Slowloris-style
	// header-by-header attacks. http.ListenAndServe's default Server
	// leaves all timeouts at zero (unlimited) — fine for development,
	// dangerous for production.
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("listening on %s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}
