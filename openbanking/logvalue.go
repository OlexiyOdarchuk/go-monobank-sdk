package openbanking

import (
	"log/slog"
	"strings"
)

// Open Banking payloads are made almost entirely of other people's
// data: IBANs, names, taxpayer ids, payment purposes. The DTOs below
// therefore implement [slog.LogValuer] so that a debug-level
// slog.Any() of a response cannot dump a customer's statement into a
// log aggregator.
//
// The masking rules deliberately match the bank package (an IBAN
// keeps its country prefix and last four, a name keeps its initial),
// so the same entity reads the same way wherever it surfaces in the
// SDK.
//
// Note the collection types further down: slog does not reach through
// a plain slice to the element's LogValuer, so a []Transaction would
// print in full despite Transaction having a LogValue. Every slice of
// a redacting type therefore has a named type of its own.

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

// redactIBAN keeps the country prefix and the last four digits —
// enough to tell two accounts apart in a log, not enough to pay into
// one.
func redactIBAN(iban string) string {
	if iban == "" {
		return ""
	}
	if len(iban) <= 6 {
		return "***"
	}
	return iban[:2] + "***" + iban[len(iban)-4:]
}

// redactID drops a taxpayer id (РНОКПП) or company code (ЄДРПОУ)
// entirely. Unlike an IBAN these are bare identifiers with no
// non-identifying prefix to anchor on, so keeping any digits would
// narrow the search space enough to re-identify the holder.
func redactID(id string) string {
	if id == "" {
		return ""
	}
	return "***"
}

// LogValue implements [slog.LogValuer]. The identifiers name the
// customer, and IPAddress is their device address, so only their
// presence and type survive.
func (p PSU) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", redactID(p.ID)),
		slog.String("idType", string(p.IDType)),
		slog.String("corporateId", redactID(p.CorporateID)),
		slog.String("corporateIdType", string(p.CorporateIDType)),
		slog.Bool("ipAddressSet", p.IPAddress != ""),
	)
}

// LogValue implements [slog.LogValuer], hiding the IBAN.
func (a AccountReference) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("iban", redactIBAN(a.IBAN)),
		slog.String("currency", a.Currency),
		slog.String("product", a.Product),
	)
}

// LogValue implements [slog.LogValuer], hiding the IBAN.
func (a PaymentAccountReference) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("iban", redactIBAN(a.IBAN)),
		slog.String("currency", a.Currency),
	)
}

// LogValue implements [slog.LogValuer], hiding the IBAN. ResourceID
// is an opaque bank-side handle, so it stays readable — it is what
// makes a log entry actionable.
func (a AccountDetails) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("resourceId", a.ResourceID),
		slog.String("iban", redactIBAN(a.IBAN)),
		slog.String("currency", a.Currency),
		slog.String("product", a.Product),
		slog.Int("balances", len(a.Balances)),
	)
}

// LogValue implements [slog.LogValuer], masking the counterparty.
func (p Party) LogValue() slog.Value {
	return slog.GroupValue(slog.String("name", maskName(p.Name)))
}

// LogValue implements [slog.LogValuer]. Both the beneficiary's name
// and their taxpayer/company id identify a real party.
func (c Creditor) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("name", maskName(c.Name)),
		slog.String("creditorId", redactID(c.CreditorID)),
		slog.String("creditorIdType", string(c.CreditorIDType)),
	)
}

// LogValue implements [slog.LogValuer]. The counterparties go through
// their own redaction and the payment purpose is reduced to its
// presence — free text is where card numbers and phone numbers end up
// in practice.
func (t Transaction) LogValue() slog.Value {
	attrs := []slog.Attr{
		slog.String("transactionId", t.TransactionID),
		slog.String("amount", t.TransactionAmount.Value),
		slog.String("currency", t.TransactionAmount.Currency),
		slog.Bool("remittanceInfoSet", len(t.RemittanceInformationUnstructured) > 0),
	}
	if !t.BookingDate.IsZero() {
		attrs = append(attrs, slog.String("bookingDate", t.BookingDate.String()))
	}
	// Resolved eagerly rather than via slog.Any: inside
	// [TransactionList.LogValue] the group is carried as a plain
	// value, and an unresolved LogValuer there would print in full.
	if t.Creditor != nil {
		attrs = append(attrs, slog.Attr{Key: "creditor", Value: t.Creditor.LogValue()})
	}
	if t.Debtor != nil {
		attrs = append(attrs, slog.Attr{Key: "debtor", Value: t.Debtor.LogValue()})
	}
	return slog.GroupValue(attrs...)
}

// AccountList is the result of [Client.Accounts].
type AccountList []AccountDetails

// ByResourceID returns the account with the given resourceId.
// ok=false when none matches. The pointer aliases the slice element.
func (as AccountList) ByResourceID(id string) (*AccountDetails, bool) {
	for i := range as {
		if as[i].ResourceID == id {
			return &as[i], true
		}
	}
	return nil, false
}

// LogValue implements [slog.LogValuer] for the slice — see the note
// at the top of this file on why the element's LogValue is not
// enough.
func (as AccountList) LogValue() slog.Value {
	vals := make([]slog.Value, len(as))
	for i := range as {
		vals[i] = as[i].LogValue()
	}
	return slog.AnyValue(vals)
}

// TransactionList is a page of statement entries.
type TransactionList []Transaction

// LogValue implements [slog.LogValuer] for the slice, for the same
// reason as [AccountList.LogValue].
func (ts TransactionList) LogValue() slog.Value {
	vals := make([]slog.Value, len(ts))
	for i := range ts {
		vals[i] = ts[i].LogValue()
	}
	return slog.AnyValue(vals)
}

// A struct is as leaky as a slice: slog renders a field holding a
// redacting type with %v unless the enclosing struct redacts too. The
// wrappers below therefore reduce their payload to counts and already
// redacted groups, the way bank.ClientInfo does.

// LogValue implements [slog.LogValuer].
func (a Account) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("iban", redactIBAN(a.IBAN)),
		slog.String("currency", a.Currency),
	)
}

// LogValue implements [slog.LogValuer].
func (a AccountAccessRights) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Attr{Key: "account", Value: a.Account.LogValue()},
		slog.Any("rights", a.Rights),
	)
}

// LogValue implements [slog.LogValuer]. Only how many accounts were
// asked for in each family — the IBANs themselves add nothing to a
// log line.
func (a AccountAccess) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("cards", len(a.Cards)),
		slog.Int("payments", len(a.Payments)),
		slog.Int("savings", len(a.Savings)),
		slog.Int("loans", len(a.Loans)),
	)
}

// LogValue implements [slog.LogValuer].
func (r CreateConsentRequest) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Attr{Key: "access", Value: r.Access.LogValue()},
		slog.String("consentType", string(r.ConsentType)),
		slog.Bool("recurringIndicator", r.RecurringIndicator),
		slog.String("validTo", r.ValidTo.String()),
		slog.Int("frequencyPerDay", r.FrequencyPerDay),
	)
}

// LogValue implements [slog.LogValuer].
func (c Consent) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Attr{Key: "access", Value: c.Access.LogValue()},
		slog.String("consentType", string(c.ConsentType)),
		slog.String("consentStatus", string(c.ConsentStatus)),
		slog.Bool("recurringIndicator", c.RecurringIndicator),
		slog.String("validTo", c.ValidTo.String()),
	)
}

// LogValue implements [slog.LogValuer].
func (o InitiationOptions) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Attr{Key: "psu", Value: o.PSU.LogValue()},
		slog.String("tppRedirectURI", o.TPPRedirectURI),
	)
}

// LogValue implements [slog.LogValuer].
func (r AccountsResponse) LogValue() slog.Value {
	return slog.GroupValue(slog.Int("accounts", len(r.Accounts)))
}

// LogValue implements [slog.LogValuer].
func (r BalancesResponse) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Attr{Key: "account", Value: r.Account.LogValue()},
		slog.Int("balances", len(r.Balances)),
	)
}

// LogValue implements [slog.LogValuer]. Transactions is nil on a page
// with no entries, so the count is read defensively.
func (r TransactionsResponse) LogValue() slog.Value {
	booked := 0
	if r.Transactions != nil {
		booked = len(r.Transactions.Booked)
	}
	return slog.GroupValue(
		slog.Attr{Key: "account", Value: r.Account.LogValue()},
		slog.Int("booked", booked),
	)
}

// LogValue implements [slog.LogValuer].
func (r AccountReport) LogValue() slog.Value {
	return slog.GroupValue(slog.Int("booked", len(r.Booked)))
}

// LogValue implements [slog.LogValuer].
func (r CreatePaymentRequest) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Attr{Key: "creditor", Value: r.Creditor.LogValue()},
		slog.Attr{Key: "creditorAccount", Value: r.CreditorAccount.LogValue()},
		slog.Attr{Key: "debtorAccount", Value: r.DebtorAccount.LogValue()},
		slog.String("amount", r.InstructedAmount.Value),
		slog.String("currency", r.InstructedAmount.Currency),
	)
}

// LogValue implements [slog.LogValuer].
func (p Payment) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Attr{Key: "creditor", Value: p.Creditor.LogValue()},
		slog.Attr{Key: "creditorAccount", Value: p.CreditorAccount.LogValue()},
		slog.Attr{Key: "debtorAccount", Value: p.DebtorAccount.LogValue()},
		slog.String("amount", p.InstructedAmount.Value),
		slog.String("currency", p.InstructedAmount.Currency),
		slog.String("transactionStatus", string(p.TransactionStatus)),
	)
}

// LogValue implements [slog.LogValuer]. PSUIPAddress is the
// customer's own IP — personal data in its own right — and it sits
// here as a plain field rather than inside [PSU], so it needs its own
// redaction.
func (o AccountOptions) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("consentId", o.ConsentID),
		slog.Bool("psuIPAddressSet", o.PSUIPAddress != ""),
	)
}

// LogValue implements [slog.LogValuer].
func (o AccountsOptions) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Attr{Key: "account", Value: o.AccountOptions.LogValue()},
		slog.Bool("withBalance", o.WithBalance),
	)
}

// LogValue implements [slog.LogValuer].
func (o TransactionsOptions) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Attr{Key: "account", Value: o.AccountOptions.LogValue()},
		slog.String("bookingStatus", string(o.BookingStatus)),
		slog.String("dateFrom", o.DateFrom.String()),
		slog.String("dateTo", o.DateTo.String()),
		slog.Bool("pageIDSet", o.PageID != ""),
	)
}

// LogValue implements [slog.LogValuer]. The request carries a named
// contact person with their phone and email, plus the company code —
// none of which belongs in a log.
func (r CertificateRequest) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("organization", maskName(r.Organization)),
		slog.String("contactPerson", maskName(r.ContactPerson)),
		slog.Bool("phoneSet", r.Phone != ""),
		slog.Bool("emailSet", r.Email != ""),
		slog.String("edrpou", redactID(r.EDRPOU)),
		slog.Int("csrPEMBytes", len(r.CSRPEM)),
	)
}

// LogValue implements [slog.LogValuer]. An SCA redirect URL may carry
// a single-use authorisation parameter; whoever reads it from a log
// could reach the confirmation screen before the customer does. The
// spec does not say whether it does, so the link is not logged.
func (h Href) LogValue() slog.Value {
	return slog.GroupValue(slog.Bool("hrefSet", h.Href != ""))
}

// LogValue implements [slog.LogValuer], keeping the SCA links out of
// the log for the reason given on [Href.LogValue].
func (r StartAuthorisationResponse) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("authorisationId", r.AuthorisationID),
		slog.String("scaStatus", string(r.SCAStatus)),
		slog.Bool("scaRedirectSet", r.Links.SCARedirect != nil),
		slog.Bool("scaStatusLinkSet", r.Links.SCAStatus != nil),
	)
}
