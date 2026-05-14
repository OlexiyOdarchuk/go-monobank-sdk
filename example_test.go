package monobank_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/personal"
)

// Token-bucket: 1 запит на 60 секунд (типовий ліміт /personal/client-info).
func ExampleNewLimiter() {
	lim := monobank.NewLimiter(time.Minute, 1)

	cli := personal.New(os.Getenv("MONO_TOKEN"),
		monobank.WithRateLimiter(lim),
	)

	if _, err := cli.ClientInfo(context.Background()); err != nil {
		log.Fatal(err)
	}
}

// Per-account ліміт виписки: на кожен accountID — окрема корзина.
// idleTTL=10*time.Minute видаляє корзини, до яких не зверталися довше
// 10 хв, щоб мапа не росла на сервісах із багатьма accountID.
func ExampleNewKeyedLimiter() {
	klim := monobank.NewKeyedLimiter(time.Minute, 1, 10*time.Minute)
	defer klim.Stop()

	cli := personal.New(os.Getenv("MONO_TOKEN"),
		monobank.WithRateLimiter(klim),
	)

	to := time.Now()
	from := to.Add(-time.Hour)
	for _, acc := range []string{"acc-1", "acc-2"} {
		ctx := monobank.WithLimiterKey(context.Background(), acc)
		if _, err := cli.Transactions(ctx, acc, from, to); err != nil {
			log.Printf("%s: %v", acc, err)
		}
	}
}

// Розпізнавання конкретного типу помилки і доступ до errorDescription.
func ExampleAPIError() {
	cli := personal.New(os.Getenv("MONO_TOKEN"))

	_, err := cli.ClientInfo(context.Background())
	var apiErr *monobank.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusForbidden:
			fmt.Printf("token rejected: %s\n", apiErr.ErrorDescription)
		case http.StatusTooManyRequests:
			fmt.Println("rate limited — wait and retry")
		default:
			fmt.Printf("HTTP %d: %s\n", apiErr.StatusCode, apiErr.ErrorDescription)
		}
	}
}
