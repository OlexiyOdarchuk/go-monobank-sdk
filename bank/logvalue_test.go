package bank_test

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/bank"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/money"
)

// PII redaction: ClientInfo.LogValue must hide full name and the
// per-tenant secret in webHookUrl. Account.LogValue must mask IBAN
// and card masks.
func TestClientInfo_LogValueRedactsPII(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	info := bank.ClientInfo{
		ID:         "cid-1",
		Name:       "Іван Петренко",
		WebHookURL: "https://merchant.example.com/private-webhook/very-secret-tenant-token",
		Accounts: bank.Accounts{
			{
				AccountID: "acc-1",
				Type:      bank.Black,
				Currency:  currency.Code(980),
				Balance:   money.New(12345, currency.Code(980)),
				IBAN:      "UA213996220000026007233566001",
				CardMasks: []string{"414141******4141"},
			},
		},
	}

	logger.Info("client", "info", info)
	out := buf.String()

	// The full name must NOT appear.
	assert.NotContains(t, out, "Іван Петренко",
		"raw name must not reach the log")
	// Per-tenant secret in URL must NOT appear.
	assert.NotContains(t, out, "very-secret-tenant-token",
		"the secret path segment must be redacted")
	// Full IBAN must NOT appear.
	assert.NotContains(t, out, "UA213996220000026007233566001",
		"raw IBAN must not reach the log")
	// Full card mask must NOT appear.
	assert.NotContains(t, out, "414141******4141",
		"raw card mask must not reach the log")

	// Useful, non-sensitive shape MUST appear (verifies LogValue
	// actually fired and produced something).
	assert.Contains(t, out, "cid-1", "clientId is not sensitive — must remain visible")
	assert.Contains(t, out, "accounts=1", "count of accounts must remain visible")
}

func TestAccount_LogValueShapes(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	a := bank.Account{
		AccountID: "acc-X",
		Type:      bank.Black,
		Currency:  currency.Code(980),
		Balance:   money.New(0, currency.Code(980)),
		IBAN:      "UA213996220000026007233566001",
		CardMasks: []string{"414141******4141", "555555******5555"},
	}
	logger.Info("a", "v", a)
	out := buf.String()

	require.NotContains(t, out, "UA213996220000026007233566001")
	require.NotContains(t, out, "414141******4141")
	require.NotContains(t, out, "555555******5555")
	// Last4 must still be present — it's the audit anchor.
	assert.Contains(t, out, "6001")
	assert.Contains(t, out, "4141")
	assert.Contains(t, out, "5555")
}

// A managed client is a third party's PII (name, РНОКПП, FOP IBANs).
// Logging the enclosing ClientInfo must reduce it to a count only.
func TestClientInfo_LogValueRedactsManagedClients(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	info := bank.ClientInfo{
		ID: "acct-1",
		ManagedClients: bank.ManagedClients{
			{
				ID:   "mc-1",
				TIN:  "1234567890",
				Name: "Іван Петренко",
				Accounts: bank.Accounts{
					{
						AccountID: "fop-1",
						Type:      bank.FOP,
						Currency:  currency.UAH,
						Balance:   money.New(100, currency.UAH),
						IBAN:      "UA213996220000026007233566001",
					},
				},
			},
		},
	}

	logger.Info("client", "info", info)
	out := buf.String()

	assert.NotContains(t, out, "Іван Петренко", "managed client name must not reach the log")
	assert.NotContains(t, out, "1234567890", "РНОКПП must not reach the log")
	assert.NotContains(t, out, "UA213996220000026007233566001", "managed FOP IBAN must not reach the log")
	assert.Contains(t, out, "managedClients=1", "count of managed clients must remain visible")
}

func TestManagedClient_LogValueShapes(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	mc := bank.ManagedClient{
		ID:   "mc-X",
		TIN:  "1234567890",
		Name: "Іван Петренко",
		Accounts: bank.Accounts{
			{AccountID: "fop-1", IBAN: "UA213996220000026007233566001"},
		},
	}
	logger.Info("mc", "v", mc)
	out := buf.String()

	require.NotContains(t, out, "Іван Петренко")
	require.NotContains(t, out, "1234567890")
	require.NotContains(t, out, "UA213996220000026007233566001")
	// A TIN keeps no digits at all — unlike an IBAN it has no
	// non-identifying prefix to anchor on.
	require.NotContains(t, out, "7890")
	// clientId is an opaque Mono id, not PII — it stays readable.
	assert.Contains(t, out, "mc-X")
	assert.Contains(t, out, "accounts=1")
}

// TestCollections_LogValue pins the slice-level redaction. slog does
// not reach through a slice to the element's LogValuer, so a type
// whose element redacts correctly still leaks when the slice itself
// is logged — which is exactly how ManagedClients and Accounts
// behaved before they grew their own LogValue.
func TestCollections_LogValue(t *testing.T) {
	const (
		tin  = "1234567890"
		iban = "UA213996220000026007233566001"
		name = "Іван Петренко"
	)
	accounts := bank.Accounts{{AccountID: "fop-1", IBAN: iban}}
	managed := bank.ManagedClients{{ID: "mc-X", TIN: tin, Name: name, Accounts: accounts}}

	tx := bank.Transaction{
		ID: "t-1", Description: "переказ від Іван Петренко", Comment: "за оренду",
		EDRPOU: tin, IBAN: iban, CounterName: name,
	}
	jar := bank.Jar{ID: "j-1", Title: "На лікування " + name, Description: "збір"}

	for _, tc := range []struct {
		name string
		v    any
	}{
		{"ManagedClients slice", managed},
		{"Accounts slice", accounts},
		// A statement entry is the densest PII the SDK handles.
		{"Transaction", tx},
		{"Transactions slice", bank.Transactions{tx}},
		{"Jar", jar},
		{"Jars slice", bank.Jars{jar}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			slog.New(slog.NewTextHandler(&buf, nil)).Info("x", "v", tc.v)
			out := buf.String()

			assert.NotContains(t, out, iban, "IBAN must not reach the log")
			assert.NotContains(t, out, tin, "РНОКПП must not reach the log")
			assert.NotContains(t, out, name, "name must not reach the log")
			assert.NotContains(t, out, "оренду", "a payment purpose must not reach the log")
			assert.NotContains(t, out, "лікування", "a jar title must not reach the log")
		})
	}
}
