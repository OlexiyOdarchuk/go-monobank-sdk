package acquiring_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/acquiring"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/currency"
)

// Поля, доданого для вирівнювання з invoice/create: окремі
// successUrl/failUrl, displayType, withAppUrl, metadata.
func TestCreateInvoiceRequest_newFields(t *testing.T) {
	b, err := json.Marshal(acquiring.CreateInvoiceRequest{
		Amount:      100,
		Currency:    currency.UAH,
		SuccessURL:  "https://ok",
		FailURL:     "https://fail",
		DisplayType: acquiring.DisplayIframe,
		WithAppURL:  true,
		MerchantPaymInfo: &acquiring.MerchantPaymInfo{
			Reference: "r-1",
			Metadata:  map[string]string{"orderId": "42"},
		},
	})
	require.NoError(t, err)
	s := string(b)
	assert.Contains(t, s, `"successUrl":"https://ok"`)
	assert.Contains(t, s, `"failUrl":"https://fail"`)
	assert.Contains(t, s, `"displayType":"iframe"`)
	assert.Contains(t, s, `"withAppUrl":true`)
	assert.Contains(t, s, `"metadata":{"orderId":"42"}`)
}

// Порожній запит не повинен слати нових optional-полів.
func TestCreateInvoiceRequest_omitsEmptyNewFields(t *testing.T) {
	b, err := json.Marshal(acquiring.CreateInvoiceRequest{Amount: 100})
	require.NoError(t, err)
	s := string(b)
	assert.NotContains(t, s, "successUrl")
	assert.NotContains(t, s, "failUrl")
	assert.NotContains(t, s, "displayType")
	assert.NotContains(t, s, "withAppUrl")
}

// appUrl приходить у відповіді invoice/create при withAppUrl=true
// (підтверджено на живому API: тег appUrl).
func TestCreateInvoiceResponse_appUrl(t *testing.T) {
	var r acquiring.CreateInvoiceResponse
	err := json.Unmarshal([]byte(`{
		"invoiceId":"260702AjZE77HYwGRmtv",
		"pageUrl":"https://pay.monobank.ua/frame/260702AjZE77HYwGRmtv",
		"appUrl":"https://mbnk.app/or/260702CAHwARtn8BL91A"
	}`), &r)
	require.NoError(t, err)
	assert.Equal(t, "https://mbnk.app/or/260702CAHwARtn8BL91A", r.AppURL)
}
