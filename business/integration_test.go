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

// StatementAll pages backwards from `to` and relies on DOWN returning
// the newest operation first. The published spec (1.0.2) describes
// `direction` the other way round — "UP: new operations first, DOWN:
// old operations first" — and marks it deprecated. This pins down what the server actually does; the
// window needs at least two operations at different seconds.
func TestIntegration_Statement_directionOrder(t *testing.T) {
	iban := envOrSkip(t, "MONO_BUSINESS_IBAN")
	cli := liveClient(t)
	to := time.Now()
	from := to.AddDate(0, 0, -30)

	order := func(dir business.StatementDirection) string {
		items, err := cli.Statement(liveContext(t), iban, from, to, dir, 50)
		require.NoError(t, err, "live statement call must succeed")
		if len(items) < 2 || items[0].Time.Equal(items[len(items)-1].Time.Time) {
			t.Skipf("%s: %d operations in the window, not enough to see an order", dir, len(items))
		}
		first, last := items[0].Time.Time, items[len(items)-1].Time.Time
		t.Logf("%s: %d items, first %s, last %s", dir, len(items),
			first.Format(time.RFC3339), last.Format(time.RFC3339))
		if first.After(last) {
			return "newest first"
		}
		return "oldest first"
	}

	down, up := order(business.StatementDown), order(business.StatementUp)
	t.Logf("DOWN → %s, UP → %s", down, up)
	assert.Equal(t, "newest first", down,
		"StatementAll assumes DOWN is newest-first; if this fails, it skips operations")
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
