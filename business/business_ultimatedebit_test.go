package business

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UltimateDebit is a pointer so that an ordinary (non-budget) payment
// does not ship an empty object the bank would have to ignore.
func TestPreparePayment_ultimateDebitOmittedWhenUnset(t *testing.T) {
	var raw map[string]any
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &raw))
		_, _ = w.Write([]byte(`{"id":"pmt-1"}`))
	})

	_, err := c.PreparePayment(context.Background(), "k", &PaymentRequest{
		SenderIBAN:  "UA-from",
		Receiver:    PaymentReceiver{IBAN: "UA-to", EDRPOU: "123", Name: "X"},
		Destination: "test",
		Amount:      100,
		Currency:    "UAH",
	})
	require.NoError(t, err)
	assert.NotContains(t, raw, "ultimateDebit")
}

func TestPreparePayment_sendsUltimateDebit(t *testing.T) {
	var got PaymentRequest
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &got))
		_, _ = w.Write([]byte(`{"id":"pmt-1"}`))
	})

	_, err := c.PreparePayment(context.Background(), "k", &PaymentRequest{
		SenderIBAN:    "UA-from",
		Receiver:      PaymentReceiver{IBAN: "UA-to", EDRPOU: "123", Name: "X"},
		Destination:   "budget",
		Amount:        100,
		Currency:      "UAH",
		UltimateDebit: &UltimateDebit{ID: "3096889974", Name: "ТОВ ПРИКЛАД"},
	})
	require.NoError(t, err)
	require.NotNil(t, got.UltimateDebit)
	assert.Equal(t, "3096889974", got.UltimateDebit.ID)
	assert.Equal(t, "ТОВ ПРИКЛАД", got.UltimateDebit.Name)
}

// The statement spells the code idCode, not id — decoding it into the
// request-side shape would silently drop it.
func TestStatementItem_decodesUltimateDebit(t *testing.T) {
	var item StatementItem
	require.NoError(t, json.Unmarshal([]byte(
		`{"id":"op-1","time":1,"description":"d","amount":100,"currencyCode":"980",
		  "status":"PENDING",
		  "ultimateDebit":{"name":"ТОВ ПРИКЛАД","idCode":"3096889974"}}`), &item))

	require.NotNil(t, item.UltimateDebit)
	assert.Equal(t, "ТОВ ПРИКЛАД", item.UltimateDebit.Name)
	assert.Equal(t, "3096889974", item.UltimateDebit.IDCode)
}

func TestStatementItem_ultimateDebitAbsent(t *testing.T) {
	var item StatementItem
	require.NoError(t, json.Unmarshal([]byte(
		`{"id":"op-1","time":1,"description":"d","amount":100,"currencyCode":"980","status":"PENDING"}`), &item))
	assert.Nil(t, item.UltimateDebit)
}
