package bank_test

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
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
