// Command openbanking demonstrates the Open Banking (PSD2) API:
// register an account-access consent, start its authorisation, wait
// for the customer to approve it, then read accounts and balances.
//
// Usage:
//
//	OB_CERT=qwac.pem OB_KEY=qwac.key OB_IBAN=UA... \
//	    go run ./examples/openbanking
//
// Unlike every other API in this SDK there is no token: the bank
// authenticates the TPP by the QWAC client certificate presented
// during the TLS handshake. Without a valid one the connection is
// dropped before any HTTP status exists, so failures surface as TLS
// errors rather than 401. Request a sandbox certificate through
// [openbanking.Client.RequestTestCertificate].
//
// The consent only becomes usable after a human approves it in the
// bank's UI, so this program pauses at that point and polls.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/openbanking"
)

// psuIP is the customer's IP address. The bank requires it on every
// call the customer is present for; a hard-coded value is fine for a
// demo but a real TPP must forward the address it actually saw.
const psuIP = "127.0.0.1"

func main() {
	certFile, keyFile, iban := os.Getenv("OB_CERT"), os.Getenv("OB_KEY"), os.Getenv("OB_IBAN")
	if certFile == "" || keyFile == "" || iban == "" {
		log.Fatal("OB_CERT, OB_KEY and OB_IBAN env vars are required")
	}

	// OB_CA is optional: leave it unset to trust the system roots.
	httpClient, err := openbanking.NewMTLSHTTPClientFromFiles(certFile, keyFile, os.Getenv("OB_CA"))
	if err != nil {
		log.Fatalf("build mTLS client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cli := openbanking.New(
		monobank.WithBaseURL(openbanking.BaseURLSandbox),
		monobank.WithHTTPClient(httpClient),
	)
	defer cli.Close()

	consentID, err := createConsent(ctx, cli, iban)
	if err != nil {
		log.Fatalf("create consent: %v", err)
	}
	fmt.Println("consent:", consentID)

	if err := authorise(ctx, cli, consentID); err != nil {
		log.Fatalf("authorise consent: %v", err)
	}

	if err := readAccounts(ctx, cli, consentID); err != nil {
		log.Fatalf("read accounts: %v", err)
	}
}

// createConsent asks for balances and transactions on a single IBAN.
// A detailed consent names the accounts up front — the customer sees
// exactly what is being requested.
func createConsent(ctx context.Context, cli *openbanking.Client, iban string) (string, error) {
	req := &openbanking.CreateConsentRequest{
		Access: openbanking.AccountAccess{
			Payments: []openbanking.AccountAccessRights{{
				Account: openbanking.Account{IBAN: iban},
				Rights: []openbanking.AccessRight{
					openbanking.AccessBalances,
					openbanking.AccessTransactions,
				},
			}},
		},
		ConsentType:        openbanking.ConsentDetailed,
		RecurringIndicator: true,
		ValidTo:            openbanking.NewDate(time.Now().AddDate(0, 3, 0)),
		FrequencyPerDay:    4,
	}
	opts := openbanking.InitiationOptions{
		PSU:            openbanking.PSU{IPAddress: psuIP},
		TPPRedirectURI: "https://example.com/psd2/return",
	}

	out, err := cli.CreateConsent(ctx, req, opts)
	if err != nil {
		return "", err
	}
	return out.ConsentID, nil
}

// authorise starts the SCA process and waits for the customer. Under
// the REDIRECT approach the bank hands back a link to send them to;
// a legal entity approves in its own app and gets no link, so the
// only thing left is to poll.
func authorise(ctx context.Context, cli *openbanking.Client, consentID string) error {
	auth, err := cli.StartConsentAuthorisation(ctx, consentID)
	if err != nil {
		return err
	}
	if auth.Links.SCARedirect != nil {
		fmt.Println("send the customer to:", auth.Links.SCARedirect.Href)
	} else {
		fmt.Println("the customer approves in their monobank app")
	}

	// The consent flow exposes no per-authorisation status endpoint,
	// so the consent's own status is what we poll. Back off rather
	// than hammering: a human is doing the slow part.
	delay := 2 * time.Second
	for {
		status, err := cli.ConsentStatus(ctx, consentID)
		if err != nil {
			return err
		}
		fmt.Println("consent status:", status.ConsentStatus)
		if status.ConsentStatus == openbanking.ConsentValid {
			return nil
		}
		if status.ConsentStatus != openbanking.ConsentReceived {
			return fmt.Errorf("consent did not become valid: %s", status.ConsentStatus)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		if delay < 30*time.Second {
			delay *= 2
		}
	}
}

// readAccounts lists what the consent unlocked. WithBalance embeds
// the balances into the very same response, saving a call per
// account — it works only where the consent granted AccessBalances.
func readAccounts(ctx context.Context, cli *openbanking.Client, consentID string) error {
	base := openbanking.AccountOptions{ConsentID: consentID, PSUIPAddress: psuIP}

	accounts, err := cli.Accounts(ctx, openbanking.AccountsOptions{
		AccountOptions: base,
		WithBalance:    true,
	})
	if err != nil {
		return err
	}
	if len(accounts) == 0 {
		fmt.Println("no accounts behind this consent")
		return nil
	}

	for _, acc := range accounts {
		fmt.Printf("\n%s  %s  %s\n", acc.ResourceID, acc.Currency, acc.IBAN)

		balances := acc.Balances
		if len(balances) == 0 {
			// The bank may omit embedded balances even when asked;
			// fall back to the dedicated call before giving up.
			out, err := cli.Balances(ctx, acc.ResourceID, base)
			if err != nil {
				if errors.Is(err, monobank.ErrForbidden) {
					fmt.Println("  balances not covered by the consent")
					continue
				}
				return err
			}
			balances = out.Balances
		}
		for _, b := range balances {
			fmt.Printf("  %-12s %s %s\n", b.BalanceType, b.BalanceAmount.Value, b.BalanceAmount.Currency)
		}
	}
	return nil
}
