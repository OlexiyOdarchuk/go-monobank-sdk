package business

import (
	"log/slog"
	"strings"
)

// corp-api is the payroll surface of the SDK, which makes it the one
// carrying the densest personal data: employee names, ІНН, passport
// and ID-card numbers, IBANs, card numbers and per-person salary
// figures. The DTOs below therefore implement [slog.LogValuer] so a
// debug-level log of a request or response cannot turn into a payroll
// export.
//
// The masking rules match the bank and openbanking packages, so the
// same entity reads the same way wherever it surfaces in the SDK.
//
// Two traps are handled explicitly. slog does not reach through a
// slice to the element's LogValuer, so every slice of a redacting type
// has a named type of its own; and it does not reach through a struct
// field either, so every wrapper reduces its payload to counts and
// already-redacted groups rather than nesting raw values.

// maskName keeps the first character and stars out the rest, leaving
// the length visible without naming the person.
func maskName(name string) string {
	if name == "" {
		return ""
	}
	r := []rune(name)
	if len(r) <= 1 {
		return "*"
	}
	return string(r[0]) + strings.Repeat("*", len(r)-1)
}

// redactIBAN keeps the country prefix and the last four digits.
func redactIBAN(iban string) string {
	if iban == "" {
		return ""
	}
	if len(iban) <= 6 {
		return "***"
	}
	return iban[:2] + "***" + iban[len(iban)-4:]
}

// redactID drops an ІНН / РНОКПП / ЄДРПОУ or a document number
// entirely. These are bare identifiers with no non-identifying prefix
// to anchor on, so keeping any digits would narrow the search space
// enough to re-identify the holder.
func redactID(id string) string {
	if id == "" {
		return ""
	}
	return "***"
}

// LogValue implements [slog.LogValuer], hiding the IBAN. The balance
// stays readable — it is the reason to log an account at all.
func (a Account) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("iban", redactIBAN(a.IBAN)),
		slog.Int("currency", a.Currency),
		slog.String("balance", a.BalanceMoney().String()),
	)
}

// Accounts is the result of [Client.Accounts].
type Accounts []Account

// LogValue implements [slog.LogValuer] for the slice.
func (as Accounts) LogValue() slog.Value {
	vals := make([]slog.Value, len(as))
	for i := range as {
		vals[i] = as[i].LogValue()
	}
	return slog.AnyValue(vals)
}

// LogValue implements [slog.LogValuer]. A payroll contact is a named
// person with their tax number, identity document, account and card —
// only the opaque ids survive.
func (c Contact) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", c.ID),
		slog.Int64("leClientID", c.LEClientID),
		slog.String("fullName", maskName(c.FullName)),
		slog.String("inn", redactID(c.INN)),
		slog.String("documentType", string(c.DocumentType)),
		slog.String("documentNumber", redactID(c.DocumentNumber)),
		slog.String("documentSeries", redactID(c.DocumentSeries)),
		slog.String("iban", redactIBAN(c.IBAN)),
		slog.Bool("panSet", c.PAN != ""),
	)
}

// Contacts is a list of payroll contacts.
type Contacts []Contact

// LogValue implements [slog.LogValuer] for the slice.
func (cs Contacts) LogValue() slog.Value {
	vals := make([]slog.Value, len(cs))
	for i := range cs {
		vals[i] = cs[i].LogValue()
	}
	return slog.AnyValue(vals)
}

// LogValue implements [slog.LogValuer].
func (p ContactsPage) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Bool("hasMore", p.HasMore),
		slog.Int("contacts", len(p.Contacts)),
	)
}

// LogValue implements [slog.LogValuer]. This is a request body, but it
// is the same personal data as [Contact] on the way in.
func (r CreateContactRequest) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("firstName", maskName(r.FirstName)),
		slog.String("lastName", maskName(r.LastName)),
		slog.String("middleName", maskName(r.MiddleName)),
		slog.String("inn", redactID(r.INN)),
		slog.String("documentType", string(r.DocumentType)),
		slog.String("documentNumber", redactID(r.DocumentNumber)),
		slog.String("documentSeries", redactID(r.DocumentSeries)),
		slog.String("iban", redactIBAN(r.IBAN)),
		slog.Bool("panSet", r.PAN != ""),
	)
}

// LogValue implements [slog.LogValuer]. Amount stays readable: on its
// own, with the person masked, it is a figure rather than a salary
// attributable to anyone.
func (r SalaryRecipient) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("fullName", maskName(r.FullName)),
		slog.String("inn", redactID(r.INN)),
		slog.String("documentType", string(r.DocumentType)),
		slog.String("documentNumber", redactID(r.DocumentNumber)),
		slog.String("documentSeries", redactID(r.DocumentSeries)),
		slog.String("iban", redactIBAN(r.IBAN)),
		slog.Int64("amount", r.Amount),
	)
}

// SalaryRecipients is the roster of a salary registry.
type SalaryRecipients []SalaryRecipient

// LogValue implements [slog.LogValuer] for the slice.
func (rs SalaryRecipients) LogValue() slog.Value {
	vals := make([]slog.Value, len(rs))
	for i := range rs {
		vals[i] = rs[i].LogValue()
	}
	return slog.AnyValue(vals)
}

// LogValue implements [slog.LogValuer]. A registry is a whole payroll
// run, so only its shape is logged.
func (r CreateSalaryRegistryRequest) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("senderIban", redactIBAN(r.SenderIBAN)),
		slog.Int("recipients", len(r.Recipients)),
	)
}

// LogValue implements [slog.LogValuer], hiding the counterparty and
// the free-text description.
func (s StatementItem) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", s.ID),
		slog.String("externalReference", s.ExternalReference),
		slog.Time("time", s.Time.Time),
		slog.String("amount", s.Amount.String()),
		slog.String("currencyCode", s.CurrencyCode),
		slog.String("status", string(s.Status)),
		slog.Bool("reverse", s.Reverse),
		slog.Bool("descriptionSet", s.Description != ""),
		slog.String("counterIban", redactIBAN(s.CounterIBAN)),
		slog.String("counterEdrpou", redactID(s.CounterEdrpou)),
		slog.String("counterName", maskName(s.CounterName)),
		slog.Bool("ultimateDebitSet", s.UltimateDebit != nil),
	)
}

// StatementItems is a page of statement entries.
type StatementItems []StatementItem

// LogValue implements [slog.LogValuer] for the slice.
func (ss StatementItems) LogValue() slog.Value {
	vals := make([]slog.Value, len(ss))
	for i := range ss {
		vals[i] = ss[i].LogValue()
	}
	return slog.AnyValue(vals)
}

// LogValue implements [slog.LogValuer].
func (u UltimateDebit) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", redactID(u.ID)),
		slog.String("name", maskName(u.Name)),
	)
}

// LogValue implements [slog.LogValuer].
func (u StatementUltimateDebit) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("idCode", redactID(u.IDCode)),
		slog.String("name", maskName(u.Name)),
	)
}

// LogValue implements [slog.LogValuer].
func (r PaymentReceiver) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("iban", redactIBAN(r.IBAN)),
		slog.String("edrpou", redactID(r.EDRPOU)),
		slog.String("name", maskName(r.Name)),
	)
}

// LogValue implements [slog.LogValuer]. Destination is the payment
// purpose — free text that routinely names a person or a contract.
func (r PaymentRequest) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("senderIban", redactIBAN(r.SenderIBAN)),
		slog.Attr{Key: "receiver", Value: r.Receiver.LogValue()},
		slog.Bool("destinationSet", r.Destination != ""),
		slog.Int64("amount", r.Amount),
		slog.String("currency", r.Currency),
		slog.String("externalReference", r.ExternalReference),
		slog.Bool("ultimateDebitSet", r.UltimateDebit != nil),
	)
}

// LogValue implements [slog.LogValuer]. A payslip attribute is a line
// of someone's pay slip — the name of the line is harmless, its value
// is not.
func (a BatchAttribute) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("attributeName", a.AttributeName),
		slog.Bool("valueSet", a.Value != ""),
		slog.Int("sortOrder", a.SortOrder),
	)
}

// BatchAttributes is the set of lines on one payslip.
type BatchAttributes []BatchAttribute

// LogValue implements [slog.LogValuer] for the slice.
func (as BatchAttributes) LogValue() slog.Value {
	vals := make([]slog.Value, len(as))
	for i := range as {
		vals[i] = as[i].LogValue()
	}
	return slog.AnyValue(vals)
}

// LogValue implements [slog.LogValuer]. Identification is the
// employee's tax number.
func (e BatchEmployee) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("identification", redactID(e.Identification)),
		slog.Int("attributes", len(e.Attributes)),
	)
}

// BatchEmployees is the roster of a payslip batch.
type BatchEmployees []BatchEmployee

// LogValue implements [slog.LogValuer] for the slice.
func (es BatchEmployees) LogValue() slog.Value {
	vals := make([]slog.Value, len(es))
	for i := range es {
		vals[i] = es[i].LogValue()
	}
	return slog.AnyValue(vals)
}

// LogValue implements [slog.LogValuer].
func (r BatchPayslipRequest) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("period", r.Period),
		slog.Int("employees", len(r.Employees)),
	)
}

// LogValue implements [slog.LogValuer]. Identifications is a list of
// tax numbers, so only its length is logged.
func (r DeletePayslipsRequest) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("period", r.Period),
		slog.Int("identifications", len(r.Identifications)),
	)
}

// LogValue implements [slog.LogValuer].
func (e FailedEmployee) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("identification", redactID(e.Identification)),
		slog.String("reason", string(e.Reason)),
	)
}

// FailedEmployees lists the rows that did not make it into a batch.
type FailedEmployees []FailedEmployee

// LogValue implements [slog.LogValuer] for the slice.
func (es FailedEmployees) LogValue() slog.Value {
	vals := make([]slog.Value, len(es))
	for i := range es {
		vals[i] = es[i].LogValue()
	}
	return slog.AnyValue(vals)
}

// LogValue implements [slog.LogValuer]. The per-batch and overall
// counters are the useful part; the failed rows carry tax numbers, so
// only how many there are is logged.
func (r BatchPayslipResponse) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("period", r.Period),
		slog.String("status", r.Status),
		slog.Int("employeesInBatch", r.BatchStats.EmployeesInBatch),
		slog.Int("successInBatch", r.BatchStats.SuccessInBatch),
		slog.Int("failedInBatch", r.BatchStats.FailedInBatch),
		slog.Int("failedEmployees", len(r.FailedEmployees)),
	)
}

// LogValue implements [slog.LogValuer], for the same reason as
// [BatchPayslipResponse.LogValue].
func (r ImportStatusResponse) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("period", r.Period),
		slog.String("status", string(r.Status)),
		slog.Int("totalEmployees", r.TotalEmployees),
		slog.Int("totalSuccessEmployees", r.TotalSuccessEmployees),
		slog.Int("totalFailedEmployees", r.TotalFailedEmployees),
		slog.Int("failedEmployees", len(r.FailedEmployees)),
	)
}
