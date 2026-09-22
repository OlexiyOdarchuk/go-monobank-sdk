package openbanking_test

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/mcc"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/openbanking"
)

// The strings below must never reach a log, whichever DTO carries
// them and whether it is logged on its own or inside a slice.
const (
	iban    = "UA213996220000026007233566001"
	name    = "Іван Петренко"
	taxID   = "1234567890"
	purpose = "оплата за договором №17 від 03.04"
	psuIP   = "192.0.2.44"
)

func logged(t *testing.T, v any) string {
	t.Helper()
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("x", "v", v)
	return buf.String()
}

func TestDTOs_LogValueRedacts(t *testing.T) {
	tx := openbanking.Transaction{
		TransactionID:                     "tx-1",
		BookingDate:                       openbanking.NewDate(time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)),
		TransactionAmount:                 openbanking.Amount{Currency: "UAH", Value: "100.00"},
		Creditor:                          &openbanking.Party{Name: name},
		RemittanceInformationUnstructured: []string{purpose},
	}
	account := openbanking.AccountDetails{ResourceID: "acc-1", IBAN: iban, Currency: "UAH"}
	access := openbanking.AccountAccess{Payments: []openbanking.AccountAccessRights{{
		Account: openbanking.Account{IBAN: iban},
		Rights:  []openbanking.AccessRight{openbanking.AccessBalances},
	}}}

	for _, tc := range []struct {
		name string
		v    any
		// secrets that must be absent from the rendered log line
		secrets []string
	}{
		{"PSU", openbanking.PSU{ID: iban, CorporateID: taxID, IPAddress: psuIP}, []string{iban, taxID, psuIP}},
		{"AccountDetails", account, []string{iban}},
		{"AccountReference", openbanking.AccountReference{IBAN: iban}, []string{iban}},
		{"PaymentAccountReference", openbanking.PaymentAccountReference{IBAN: iban}, []string{iban}},
		{"Party", openbanking.Party{Name: name}, []string{name}},
		{"Creditor", openbanking.Creditor{Name: name, CreditorID: taxID}, []string{name, taxID}},
		{"Transaction", tx, []string{name, purpose}},
		// Slices are the trap: slog does not consult the element's
		// LogValuer through a plain slice.
		{"AccountList", openbanking.AccountList{account}, []string{iban}},
		{"TransactionList", openbanking.TransactionList{tx}, []string{name, purpose}},
		{"AccountsResponse", openbanking.AccountsResponse{Accounts: openbanking.AccountList{account}}, []string{iban}},
		{"AccountReport", openbanking.AccountReport{Booked: openbanking.TransactionList{tx}}, []string{name, purpose}},
		// Wrappers leak the same way a slice does unless they redact
		// too, so every struct that embeds one of the types above is
		// pinned here as well.
		{"Account", openbanking.Account{IBAN: iban}, []string{iban}},
		{"AccountAccessRights", openbanking.AccountAccessRights{
			Account: openbanking.Account{IBAN: iban},
		}, []string{iban}},
		{"AccountAccess", access, []string{iban}},
		{"CreateConsentRequest", openbanking.CreateConsentRequest{Access: access}, []string{iban}},
		{"Consent", openbanking.Consent{Access: access}, []string{iban}},
		{"InitiationOptions", openbanking.InitiationOptions{
			PSU: openbanking.PSU{ID: iban, IPAddress: psuIP},
		}, []string{iban, psuIP}},
		{"BalancesResponse", openbanking.BalancesResponse{
			Account: openbanking.AccountReference{IBAN: iban},
		}, []string{iban}},
		{"TransactionsResponse", openbanking.TransactionsResponse{
			Account:      openbanking.AccountReference{IBAN: iban},
			Transactions: &openbanking.AccountReport{Booked: openbanking.TransactionList{tx}},
		}, []string{iban, name, purpose}},
		{"CreatePaymentRequest", openbanking.CreatePaymentRequest{
			Creditor:                          openbanking.Creditor{Name: name, CreditorID: taxID},
			CreditorAccount:                   openbanking.PaymentAccountReference{IBAN: iban},
			DebtorAccount:                     openbanking.PaymentAccountReference{IBAN: iban},
			RemittanceInformationUnstructured: []string{purpose},
		}, []string{iban, name, taxID, purpose}},
		{"AccountOptions", openbanking.AccountOptions{ConsentID: "c", PSUIPAddress: psuIP}, []string{psuIP}},
		{"AccountsOptions", openbanking.AccountsOptions{
			AccountOptions: openbanking.AccountOptions{PSUIPAddress: psuIP},
		}, []string{psuIP}},
		{"TransactionsOptions", openbanking.TransactionsOptions{
			AccountOptions: openbanking.AccountOptions{PSUIPAddress: psuIP},
		}, []string{psuIP}},
		{"CertificateRequest", openbanking.CertificateRequest{
			ContactPerson: name, Phone: "+380501234567",
			Email: "ivan@example.com", EDRPOU: taxID,
		}, []string{name, taxID, "+380501234567", "ivan@example.com"}},
		{"StartAuthorisationResponse", openbanking.StartAuthorisationResponse{
			AuthorisationID: "auth-1",
			Links: openbanking.AuthorisationLinks{
				SCARedirect: &openbanking.Href{Href: "https://bank.example/sca?token=SECRETSCA"},
			},
		}, []string{"SECRETSCA"}},
		{"Payment", openbanking.Payment{
			Creditor:        openbanking.Creditor{Name: name, CreditorID: taxID},
			CreditorAccount: openbanking.PaymentAccountReference{IBAN: iban},
			DebtorAccount:   openbanking.PaymentAccountReference{IBAN: iban},
		}, []string{iban, name, taxID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := logged(t, tc.v)
			for _, secret := range tc.secrets {
				assert.NotContains(t, out, secret, "%s must not reach the log", secret)
			}
		})
	}
}

// Redaction is worthless if it also hides what makes a log entry
// actionable, so pin the fields that must stay readable.
func TestDTOs_LogValueKeepsHandles(t *testing.T) {
	out := logged(t, openbanking.AccountDetails{ResourceID: "acc-1", IBAN: iban, Currency: "UAH"})
	assert.Contains(t, out, "acc-1", "the opaque resourceId is not PII and must stay")
	assert.Contains(t, out, "UA***6001", "an IBAN keeps its prefix and last four")

	out = logged(t, openbanking.Transaction{
		TransactionID:     "tx-1",
		TransactionAmount: openbanking.Amount{Currency: "UAH", Value: "100.00"},
	})
	assert.Contains(t, out, "tx-1")
	assert.Contains(t, out, "100.00")
}

// An empty page carries a nil *AccountReport; LogValue must not
// panic on it.
func TestTransactionsResponse_LogValueNilPage(t *testing.T) {
	assert.NotPanics(t, func() {
		out := logged(t, openbanking.TransactionsResponse{
			Account: openbanking.AccountReference{IBAN: iban},
		})
		assert.Contains(t, out, "booked=0")
		assert.NotContains(t, out, iban)
	})
}

// MTLSConfig holds the QWAC private key; logging it must not print
// the key material.
func TestMTLSConfig_LogValue(t *testing.T) {
	out := logged(t, openbanking.MTLSConfig{
		CertPEM: []byte("-----BEGIN CERTIFICATE-----"),
		KeyPEM:  []byte("-----BEGIN PRIVATE KEY-----\nsupersecret\n"),
	})
	assert.NotContains(t, out, "supersecret")
	assert.NotContains(t, out, "BEGIN PRIVATE KEY")
	assert.Contains(t, out, "keyPEM=***")
}

// The bridges to the SDK's typed enums: Open Banking spells both
// fields differently from the rest of the bank's APIs, so the
// conversion has to be pinned.
func TestTypedBridges(t *testing.T) {
	code, ok := openbanking.Amount{Currency: "UAH"}.CurrencyCode()
	assert.True(t, ok)
	assert.Equal(t, currency.UAH, code)

	_, ok = openbanking.Amount{Currency: "ZZZ"}.CurrencyCode()
	assert.False(t, ok, "an unknown alpha-3 must not silently map to something")

	assert.Equal(t, mcc.Code(5411),
		openbanking.CardTransactionDetails{MerchantCategoryCode: "5411"}.MCCCode())
	for _, bad := range []string{"", "abcd", "0", "12345", "-1"} {
		assert.Equal(t, mcc.Code(0),
			openbanking.CardTransactionDetails{MerchantCategoryCode: bad}.MCCCode(),
			"%q must fold to the unknown code", bad)
	}
}
