// Command corporate demonstrates the full Corporate Open API access
// flow: bind the company's ECDSA key, request access on a client's
// behalf, wait for the client to approve, then fetch their ClientInfo
// using the granted request id.
//
// Usage:
//
//	MONO_CORP_KEY=/path/to/priv.pem MONO_CALLBACK=https://yourapp/cb \
//	  go run ./examples/corporate
//
// The flow:
//
//  1. NewCorpAuthMaker loads the PEM key and derives X-Key-Id.
//  2. corporate.Client.Auth(callbackURL, perms…) starts a request and
//     returns AcceptURL — show this to the client (QR code or redirect).
//  3. The client opens AcceptURL in their mono app and approves.
//  4. Mono POSTs to your callback (or you poll CheckAuth).
//  5. With CheckAuth returning nil, call ClientInfo / Transactions /
//     SetWebHook with the request id.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/corporate"
)

func main() {
	keyPath := os.Getenv("MONO_CORP_KEY")
	callback := os.Getenv("MONO_CALLBACK")
	if keyPath == "" || callback == "" {
		log.Fatal("MONO_CORP_KEY and MONO_CALLBACK env vars are required")
	}

	pemBytes, err := os.ReadFile(keyPath)
	if err != nil {
		log.Fatalf("read key: %v", err)
	}

	maker, err := auth.NewCorpAuthMaker(pemBytes)
	if err != nil {
		log.Fatalf("decode key: %v", err)
	}
	fmt.Printf("X-Key-Id: %s\n", maker.KeyID)

	cli, err := corporate.New(maker)
	if err != nil {
		log.Fatalf("client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 1. Start an access request.
	tok, err := cli.Auth(ctx, callback, auth.PermSt, auth.PermPI)
	if err != nil {
		safeErr := strings.ReplaceAll(err.Error(), "\n", "")
		safeErr = strings.ReplaceAll(safeErr, "\r", "")
		log.Fatalf("Auth: %s", safeErr)
	}
	fmt.Printf("Request ID: %s\n", tok.RequestID)
	fmt.Printf("Show this to the client: %s\n", tok.AcceptURL)

	// 2. Poll until the client approves (every 5 s, up to ctx deadline).
	fmt.Println("Polling /personal/auth/request until approval…")
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Fatalf("waiting: %v", ctx.Err())
		case <-tick.C:
		}

		err := cli.CheckAuth(ctx, tok.RequestID)
		if err == nil {
			fmt.Println("Approved!")
			break
		}
		var apiErr *monobank.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 403 {
			// 403 == not yet approved; keep polling.
			fmt.Print(".")
			continue
		}
		safeErr := strings.ReplaceAll(err.Error(), "\n", "")
		safeErr = strings.ReplaceAll(safeErr, "\r", "")
		log.Fatalf("CheckAuth: %s", safeErr)
	}

	// 3. Fetch client data with the granted request id.
	info, err := cli.ClientInfo(ctx, tok.RequestID)
	if err != nil {
		safeErr := strings.ReplaceAll(err.Error(), "\n", "")
		safeErr = strings.ReplaceAll(safeErr, "\r", "")
		log.Fatalf("ClientInfo: %s", safeErr)
	}
	fmt.Printf("\nClient: %s\n", info.Name)
	for _, a := range info.Accounts {
		fmt.Printf("  %s %s · balance %s\n", a.Type, a.IBAN, a.Balance.String())
	}
}
