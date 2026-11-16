package installment_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/installment"
)

// The docs' guarantee-letter example, as JSON. Amounts deliberately
// use three different wire shapes to prove they are kept verbatim.
const guaranteeLetterJSON = `{
  "header": {"request_id": "180702CM023453", "answer_datetime": "2018-07-02T12:05:10",
             "contract_number": "HFIOEWDD", "contract_date": "2018-07-02"},
  "expansion": {
    "customer": {"first_name": "Василій", "last_name": "Іванов", "middle_name": "Ігорович",
                 "inn": "1234567890",
                 "document": {"passport": {"number": "548967", "issued": "Дніпровським РВ",
                                           "date_of_issue": "2000-05-12", "series": "АК"}}},
    "invoice": {"invoice_number": "СФGIT-0000000134", "invoice_date": "2018-07-02",
                "invoice_amount": 3500},
    "payment_destination": {"dest_id": "36962487", "dest_name": "ТОВ \"Рога\"",
                            "dest_mfo": "305299", "dest_bank_name": "IBOX",
                            "dest_acc_number": "26000000000"},
    "bank": {"agreement": "КД-0001", "agreement_date": "2018-07-20",
             "credit_amount": "3500.00", "customer_pay_amount": 0.0,
             "product_types": "БР", "available_parts_count": "7",
             "bank_id": "21133352", "bank_name": "ПАТ \"УНІВЕРСАЛ БАНК\"",
             "bank_executive": "Петренко І.В."},
    "sign": "12E0F76458C6D1E4"
  }
}`

func TestOrderData_SignedString(t *testing.T) {
	cli, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(guaranteeLetterJSON))
	})
	data, err := cli.GuaranteeLetterDataV2(context.Background(), &installment.OrderDataRequest{OrderID: "o"})
	require.NoError(t, err)

	// Money still decodes exactly; the raw text is kept alongside.
	assert.Equal(t, int64(350000), data.Expansion.Invoice.InvoiceAmount.Kopecks)
	assert.Equal(t, "3500", data.Expansion.Invoice.InvoiceAmountRaw)
	assert.Equal(t, "3500.00", data.Expansion.Bank.CreditAmountRaw)
	assert.Equal(t, "0.0", data.Expansion.Bank.CustomerPayAmountRaw)
	assert.Equal(t, 7, data.Expansion.Bank.AvailablePartsCount)

	want := "Іванов" + "Василій" + "Ігорович" +
		"АК" + "548967" + "Дніпровським РВ" + "2000-05-12" + "1234567890" +
		"СФGIT-0000000134" + "2018-07-02" + "3500" +
		"36962487" + `ТОВ "Рога"` + "305299" + "IBOX" + "26000000000" +
		"2018-07-02T12:05:10" +
		"КД-0001" + "2018-07-20" + "3500.00" + "0.0" +
		"21133352" + `ПАТ "УНІВЕРСАЛ БАНК"` + "Петренко І.В."
	assert.Equal(t, want, data.SignedString())
}

func TestOrderData_SignedString_documentVariants(t *testing.T) {
	idCard := installment.OrderData{Expansion: installment.Expansion{
		Customer: &installment.Customer{Document: &installment.ClientDocuments{
			IDCard: &installment.IDCardDocument{RegistryNumber: "R1", Number: "N1", Issued: "I1", DateOfIssue: "D1"},
		}},
	}}
	assert.Equal(t, "R1N1I1D1", idCard.SignedString())

	// An international passport has no issuing authority: empty string.
	intl := installment.OrderData{Expansion: installment.Expansion{
		Customer: &installment.Customer{Document: &installment.ClientDocuments{
			InternationalPassport: &installment.InternationalPassport{Series: "FA", Number: "123456", DateOfIssue: "D2"},
		}},
	}}
	assert.Equal(t, "FA123456D2", intl.SignedString())

	var nilData *installment.OrderData
	assert.Empty(t, nilData.SignedString())
}

func TestBank_availablePartsCount_number(t *testing.T) {
	var b installment.Bank
	require.NoError(t, json.Unmarshal([]byte(`{"available_parts_count": 9, "credit_amount": 12.5}`), &b))
	assert.Equal(t, 9, b.AvailablePartsCount)
	assert.Equal(t, int64(1250), b.CreditAmount.Kopecks)
	assert.Equal(t, "12.5", b.CreditAmountRaw)
}

func TestBank_availablePartsCount_invalid(t *testing.T) {
	var b installment.Bank
	require.Error(t, json.Unmarshal([]byte(`{"available_parts_count": "seven"}`), &b))
}

// Raw fields are decode-only: they must not leak into a re-encoded
// payload.
func TestInvoice_rawNotMarshalled(t *testing.T) {
	inv := installment.Invoice{InvoiceAmount: installment.NewMoney(1, 0), InvoiceAmountRaw: "1"}
	out, err := json.Marshal(inv)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "Raw")
}
