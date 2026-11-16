package installment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// SignedString returns the string the bank's guarantee-letter
// signature (Expansion.Sign) is laid over, built as the docs of
// /api/(v2/)order/data/for/guarantee/letter describe: the values
// below concatenated in this order, with no separators, an absent
// value contributing an empty string, the result encoded as UTF-8:
//
//	customer: last_name, first_name, middle_name,
//	  document series (passport / residence_permit /
//	  international_passport series; id_card registry_number),
//	  document number, issued (empty for an international passport),
//	  date_of_issue, inn
//	invoice: invoice_number, invoice_date, invoice_amount
//	payment_destination: dest_id, dest_name, dest_mfo,
//	  dest_bank_name, dest_acc_number
//	header: answer_datetime
//	bank: agreement, agreement_date, credit_amount,
//	  customer_pay_amount, bank_id, bank_name, bank_executive
//
// Values are taken exactly as they arrived in the response; for the
// three amounts that means [Invoice.InvoiceAmountRaw],
// [Bank.CreditAmountRaw] and [Bank.CustomerPayAmountRaw], so call
// this on an OrderData decoded from the response, not one built by
// hand. The signature itself is a detached CMS (DSTU 4145, hash
// DSTU 7564-256 "Kupyna"); verifying it is out of scope for this SDK.
func (d *OrderData) SignedString() string {
	if d == nil {
		return ""
	}
	var b strings.Builder
	e := d.Expansion
	if c := e.Customer; c != nil {
		b.WriteString(c.LastName)
		b.WriteString(c.FirstName)
		b.WriteString(c.MiddleName)
		series, number, issued, dateOfIssue := c.Document.signedFields()
		b.WriteString(series)
		b.WriteString(number)
		b.WriteString(issued)
		b.WriteString(dateOfIssue)
		b.WriteString(c.INN)
	}
	if inv := e.Invoice; inv != nil {
		b.WriteString(inv.InvoiceNumber)
		b.WriteString(inv.InvoiceDate)
		b.WriteString(inv.InvoiceAmountRaw)
	}
	if p := e.PaymentDestination; p != nil {
		b.WriteString(p.DestID)
		b.WriteString(p.DestName)
		b.WriteString(p.DestMFO)
		b.WriteString(p.DestBankName)
		b.WriteString(p.DestAccNo)
	}
	b.WriteString(d.Header.AnswerDatetime)
	if bk := e.Bank; bk != nil {
		b.WriteString(bk.Agreement)
		b.WriteString(bk.AgreementDate)
		b.WriteString(bk.CreditAmountRaw)
		b.WriteString(bk.CustomerPayAmountRaw)
		b.WriteString(bk.BankID)
		b.WriteString(bk.BankName)
		b.WriteString(bk.BankExecutive)
	}

	return b.String()
}

// signedFields picks the one populated document and returns its
// series, number, issued and date_of_issue as the signature
// concatenation uses them.
func (d *ClientDocuments) signedFields() (series, number, issued, dateOfIssue string) {
	switch {
	case d == nil:
		return "", "", "", ""
	case d.Passport != nil:
		p := d.Passport

		return p.Series, p.Number, p.Issued, p.DateOfIssue
	case d.ResidencePermit != nil:
		p := d.ResidencePermit

		return p.Series, p.Number, p.Issued, p.DateOfIssue
	case d.InternationalPassport != nil:
		p := d.InternationalPassport

		return p.Series, p.Number, "", p.DateOfIssue
	case d.IDCard != nil:
		p := d.IDCard

		return p.RegistryNumber, p.Number, p.Issued, p.DateOfIssue
	}

	return "", "", "", ""
}

// UnmarshalJSON decodes an Invoice and keeps the raw text of
// invoice_amount in InvoiceAmountRaw.
func (in *Invoice) UnmarshalJSON(data []byte) error {
	type alias Invoice
	aux := struct {
		*alias
		InvoiceAmount json.RawMessage `json:"invoice_amount"`
	}{alias: (*alias)(in)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.InvoiceAmount != nil {
		if err := in.InvoiceAmount.UnmarshalJSON(aux.InvoiceAmount); err != nil {
			return err
		}
		raw, err := rawScalar(aux.InvoiceAmount)
		if err != nil {
			return fmt.Errorf("installment: invoice_amount: %w", err)
		}
		in.InvoiceAmountRaw = raw
	}

	return nil
}

// UnmarshalJSON decodes a Bank, keeps the raw text of credit_amount
// and customer_pay_amount, and accepts available_parts_count both as
// a number and as a numeric string.
func (bk *Bank) UnmarshalJSON(data []byte) error {
	type alias Bank
	aux := struct {
		*alias
		CreditAmount        json.RawMessage `json:"credit_amount"`
		CustomerPayAmount   json.RawMessage `json:"customer_pay_amount"`
		AvailablePartsCount json.RawMessage `json:"available_parts_count"`
	}{alias: (*alias)(bk)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.CreditAmount != nil {
		if err := bk.CreditAmount.UnmarshalJSON(aux.CreditAmount); err != nil {
			return err
		}
		raw, err := rawScalar(aux.CreditAmount)
		if err != nil {
			return fmt.Errorf("installment: credit_amount: %w", err)
		}
		bk.CreditAmountRaw = raw
	}
	if aux.CustomerPayAmount != nil {
		if err := bk.CustomerPayAmount.UnmarshalJSON(aux.CustomerPayAmount); err != nil {
			return err
		}
		raw, err := rawScalar(aux.CustomerPayAmount)
		if err != nil {
			return fmt.Errorf("installment: customer_pay_amount: %w", err)
		}
		bk.CustomerPayAmountRaw = raw
	}
	if aux.AvailablePartsCount != nil {
		raw, err := rawScalar(aux.AvailablePartsCount)
		if err != nil {
			return fmt.Errorf("installment: available_parts_count: %w", err)
		}
		n := 0
		if s := strings.TrimSpace(raw); s != "" {
			if n, err = strconv.Atoi(s); err != nil {
				return fmt.Errorf("installment: available_parts_count: %w", err)
			}
		}
		bk.AvailablePartsCount = n
	}

	return nil
}

// rawScalar returns a JSON scalar as text: a string's contents
// (unescaped), null as "", anything else verbatim.
func rawScalar(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	switch {
	case len(raw) == 0, bytes.Equal(raw, []byte("null")):
		return "", nil
	case raw[0] == '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}

		return s, nil
	}

	return string(raw), nil
}
