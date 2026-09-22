package business_test

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/business"
)

// corp-api is the payroll surface, so these are the strings that must
// never appear in a log — whichever DTO carries them, and whether it
// is logged on its own, inside a slice, or inside a wrapper.
const (
	fullName = "Петренко Іван Миколайович"
	inn      = "3096889974"
	docNum   = "АБ123456"
	iban     = "UA213996220000026007233566001"
	pan      = "4441114455556666"
	edrpou   = "12345678"
	purpose  = "зарплата за вересень"
)

func logged(t *testing.T, v any) string {
	t.Helper()
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("x", "v", v)
	return buf.String()
}

func TestBusinessDTOs_LogValueRedacts(t *testing.T) {
	contact := business.Contact{
		ID: "c-1", FullName: fullName, INN: inn,
		DocumentNumber: docNum, DocumentSeries: "АБ", IBAN: iban, PAN: pan,
	}
	recipient := business.SalaryRecipient{
		FullName: fullName, INN: inn, DocumentNumber: docNum, IBAN: iban, Amount: 1234500,
	}
	item := business.StatementItem{
		ID: "op-1", Description: purpose, CounterName: fullName,
		CounterEdrpou: edrpou, CounterIBAN: iban,
		UltimateDebit: &business.StatementUltimateDebit{Name: fullName, IDCode: inn},
	}
	employee := business.BatchEmployee{
		Identification: inn,
		Attributes:     business.BatchAttributes{{AttributeName: "Нараховано", Value: "12345.00"}},
	}
	all := []string{fullName, inn, docNum, iban, pan, edrpou, purpose}

	for _, tc := range []struct {
		name    string
		v       any
		secrets []string
	}{
		{"Contact", contact, all},
		{"Contacts slice", business.Contacts{contact}, all},
		{"ContactsPage", business.ContactsPage{Contacts: business.Contacts{contact}}, all},
		{"CreateContactRequest", business.CreateContactRequest{
			FirstName: "Іван", LastName: "Петренко", INN: inn,
			DocumentNumber: docNum, IBAN: iban, PAN: pan,
		}, []string{inn, docNum, iban, pan, "Петренко"}},
		{"SalaryRecipient", recipient, all},
		{"SalaryRecipients slice", business.SalaryRecipients{recipient}, all},
		{"CreateSalaryRegistryRequest", business.CreateSalaryRegistryRequest{
			SenderIBAN: iban, Recipients: business.SalaryRecipients{recipient},
		}, all},
		{"Account", business.Account{IBAN: iban}, []string{iban}},
		{"Accounts slice", business.Accounts{{IBAN: iban}}, []string{iban}},
		{"StatementItem", item, all},
		{"StatementItems slice", business.StatementItems{item}, all},
		{"StatementUltimateDebit", business.StatementUltimateDebit{Name: fullName, IDCode: inn}, all},
		{"UltimateDebit", business.UltimateDebit{Name: fullName, ID: inn}, all},
		{"PaymentReceiver", business.PaymentReceiver{IBAN: iban, EDRPOU: edrpou, Name: fullName}, all},
		{"PaymentRequest", business.PaymentRequest{
			SenderIBAN: iban, Destination: purpose,
			Receiver: business.PaymentReceiver{IBAN: iban, EDRPOU: edrpou, Name: fullName},
		}, all},
		{"BatchAttribute", business.BatchAttribute{AttributeName: "Нараховано", Value: "12345.00"}, []string{"12345.00"}},
		{"BatchAttributes slice", business.BatchAttributes{{Value: "12345.00"}}, []string{"12345.00"}},
		{"BatchEmployee", employee, []string{inn, "12345.00"}},
		{"BatchEmployees slice", business.BatchEmployees{employee}, []string{inn, "12345.00"}},
		{"BatchPayslipRequest", business.BatchPayslipRequest{
			Period: "2026-01", Employees: business.BatchEmployees{employee},
		}, []string{inn, "12345.00"}},
		{"DeletePayslipsRequest", business.DeletePayslipsRequest{
			Period: "2026-01", Identifications: []string{inn},
		}, []string{inn}},
		{"FailedEmployee", business.FailedEmployee{Identification: inn}, []string{inn}},
		{"FailedEmployees slice", business.FailedEmployees{{Identification: inn}}, []string{inn}},
		{"BatchPayslipResponse", business.BatchPayslipResponse{
			FailedEmployees: business.FailedEmployees{{Identification: inn}},
		}, []string{inn}},
		{"ImportStatusResponse", business.ImportStatusResponse{
			FailedEmployees: business.FailedEmployees{{Identification: inn}},
		}, []string{inn}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := logged(t, tc.v)
			for _, secret := range tc.secrets {
				assert.NotContains(t, out, secret, "%s must not reach the log", secret)
			}
		})
	}
}

// Redaction is worthless if it hides what makes an entry actionable.
func TestBusinessDTOs_LogValueKeepsHandles(t *testing.T) {
	out := logged(t, business.Contact{ID: "c-1", IBAN: iban})
	assert.Contains(t, out, "c-1", "the opaque contact id is not PII")
	assert.Contains(t, out, "UA***6001", "an IBAN keeps its prefix and last four")

	out = logged(t, business.StatementItem{ID: "op-1", ExternalReference: "ext-7"})
	assert.Contains(t, out, "op-1")
	assert.Contains(t, out, "ext-7")
}
