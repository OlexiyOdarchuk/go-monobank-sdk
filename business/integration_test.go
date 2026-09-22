//go:build integration

// Package business integration tests hit the real corp-api.monobank.ua.
// They are excluded from the default `go test ./...` run; enable with:
//
//	MONO_BUSINESS_TOKEN=xxx go test -tags=integration -run Integration ./business/...
//
// The tests skip when their inputs are absent, so the suite is a green
// no-op until someone supplies them. CI runs it weekly — see
// .github/workflows/integration.yaml.
//
// Read-only by default. The payslip deletes are destructive and need
// an explicit opt-in.
package business_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/business"
)

// deleteOptIn is the value MONO_ALLOW_PAYSLIP_DELETE must carry. The
// payslip deletes wipe a period's import for a whole company.
const deleteOptIn = "yes-delete-payslips"

func envOrSkip(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("%s is not set", key)
	}
	return v
}

func liveClient(t *testing.T) *business.Client {
	t.Helper()
	cli := business.New(envOrSkip(t, "MONO_BUSINESS_TOKEN"))
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// Accounts is the cheapest proof that the token works.
func TestIntegration_Accounts(t *testing.T) {
	accounts, err := liveClient(t).Accounts(liveContext(t))
	require.NoError(t, err, "live /ext/v1/accounts call must succeed")
	for _, a := range accounts {
		assert.NotEmpty(t, a.IBAN, "every account must carry an IBAN")
	}
}

// StatementItem.UltimateDebit was added from the spec without ever
// being seen on the wire. Budget payments are rare, so this test
// reports rather than asserts: point MONO_BUSINESS_IBAN at an account
// that receives them and read the log.
func TestIntegration_Statement_ultimateDebit(t *testing.T) {
	iban := envOrSkip(t, "MONO_BUSINESS_IBAN")
	to := time.Now()
	from := to.AddDate(0, 0, -30)

	items, err := liveClient(t).Statement(liveContext(t), iban, from, to,
		business.StatementDown, 100)
	require.NoError(t, err, "live statement call must succeed")

	seen := 0
	for _, it := range items {
		if it.UltimateDebit != nil {
			seen++
			t.Logf("ultimateDebit on %s: name=%q idCode=%q",
				it.ID, it.UltimateDebit.Name, it.UltimateDebit.IDCode)
		}
	}
	t.Logf("%d of %d statement entries carried ultimateDebit", seen, len(items))
}

// ImportStatus for a period with no import tells us how the bank
// answers the empty case, and it is read-only.
func TestIntegration_ImportStatus_unknownPeriod(t *testing.T) {
	_, err := liveClient(t).ImportStatus(liveContext(t), "1970-01")
	t.Logf("import status for an empty period returned: %v", err)
	if err != nil {
		assert.NotErrorIs(t, err, monobank.ErrUnauthorized,
			"a missing period must not read as an auth failure")
	}
}

// TestIntegration_DeleteImport is the test that settles the 204
// question. The published spec documents 204 for this endpoint and
// says nothing about 200; the SDK used to require 200 and therefore
// reported every successful delete as an error. Accepting both makes
// the bug impossible, but only a live call shows which one arrives.
//
// It DELETES the payslip import for MONO_BUSINESS_PAYSLIP_PERIOD, so
// it needs the opt-in and a period you are willing to lose.
func TestIntegration_DeleteImport(t *testing.T) {
	if os.Getenv("MONO_ALLOW_PAYSLIP_DELETE") != deleteOptIn {
		t.Skipf("this deletes a payslip import; set MONO_ALLOW_PAYSLIP_DELETE=%s to run", deleteOptIn)
	}
	period := envOrSkip(t, "MONO_BUSINESS_PAYSLIP_PERIOD")

	err := liveClient(t).DeleteImport(liveContext(t), period)
	require.NoError(t, err, "a successful delete must not come back as an error")
	t.Logf("payslip import for %s deleted", period)
}
