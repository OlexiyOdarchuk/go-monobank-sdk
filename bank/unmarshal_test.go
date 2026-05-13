package bank

import (
	"encoding/json"
	"testing"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Custom UnmarshalJSON на Account/Jar/Transaction має проставити Code у
// money.Money-поля з сусіднього CurrencyCode.

func TestAccount_UnmarshalJSON_propagatesCurrencyToMoney(t *testing.T) {
	in := []byte(`{
		"id":"acc-1",
		"balance":1234,
		"creditLimit":5000,
		"currencyCode":840,
		"type":"black",
		"iban":"UA1"
	}`)

	var a Account
	require.NoError(t, json.Unmarshal(in, &a))

	assert.Equal(t, int64(1234), a.Balance.Minor)
	assert.Equal(t, currency.USD, a.Balance.Code, "Balance.Code = currency.USD (840)")
	assert.Equal(t, int64(5000), a.CreditLimit.Minor)
	assert.Equal(t, currency.USD, a.CreditLimit.Code)
}

func TestAccount_UnmarshalJSON_preservesOtherFields(t *testing.T) {
	in := []byte(`{
		"id":"acc-1",
		"sendId":"snd-1",
		"balance":100,
		"creditLimit":0,
		"currencyCode":980,
		"cashbackType":"UAH",
		"maskedPan":["4141**1111","5555**2222"],
		"type":"white",
		"iban":"UA293220010000026"
	}`)

	var a Account
	require.NoError(t, json.Unmarshal(in, &a))

	assert.Equal(t, "acc-1", a.AccountID)
	assert.Equal(t, "snd-1", a.SendID)
	assert.Equal(t, "UAH", a.CashbackType)
	assert.Equal(t, []string{"4141**1111", "5555**2222"}, a.CardMasks)
	assert.Equal(t, White, a.Type)
	assert.Equal(t, "UA293220010000026", a.IBAN)
}

func TestAccount_UnmarshalJSON_malformed(t *testing.T) {
	var a Account
	require.Error(t, json.Unmarshal([]byte(`{not json}`), &a))
}

func TestJar_UnmarshalJSON_propagatesCurrencyToMoney(t *testing.T) {
	in := []byte(`{
		"id":"jar-1",
		"title":"Донати",
		"description":"опис",
		"currencyCode":978,
		"balance":12345,
		"goal":100000
	}`)

	var j Jar
	require.NoError(t, json.Unmarshal(in, &j))

	assert.Equal(t, int64(12345), j.Balance.Minor)
	assert.Equal(t, currency.EUR, j.Balance.Code, "Balance.Code = currency.EUR (978)")
	assert.Equal(t, int64(100000), j.Goal.Minor)
	assert.Equal(t, currency.EUR, j.Goal.Code)
	assert.Equal(t, "Донати", j.Title)
}

func TestJar_UnmarshalJSON_malformed(t *testing.T) {
	var j Jar
	require.Error(t, json.Unmarshal([]byte(`{not json}`), &j))
}

func TestTransaction_UnmarshalJSON_propagatesCurrencyToAllMoneyFields(t *testing.T) {
	in := []byte(`{
		"id":"tx-1",
		"time":1700000000,
		"description":"оплата",
		"mcc":5411,
		"originalMcc":5411,
		"hold":false,
		"amount":-20000,
		"operationAmount":-20000,
		"currencyCode":980,
		"commissionRate":0,
		"cashbackAmount":100,
		"balance":50000
	}`)

	var tx Transaction
	require.NoError(t, json.Unmarshal(in, &tx))

	assert.Equal(t, currency.UAH, tx.Amount.Code)
	assert.Equal(t, currency.UAH, tx.OperationAmount.Code)
	assert.Equal(t, currency.UAH, tx.CommissionRate.Code)
	assert.Equal(t, currency.UAH, tx.CashbackAmount.Code)
	assert.Equal(t, currency.UAH, tx.Balance.Code)

	assert.Equal(t, int64(-20000), tx.Amount.Minor)
	assert.Equal(t, int64(100), tx.CashbackAmount.Minor)
	assert.Equal(t, int64(50000), tx.Balance.Minor)
}

func TestTransaction_UnmarshalJSON_preservesFOPFields(t *testing.T) {
	in := []byte(`{
		"id":"tx-fop",
		"amount":10000,
		"operationAmount":10000,
		"currencyCode":980,
		"balance":0,
		"receiptId":"R-001",
		"invoiceId":"INV-42",
		"counterEdrpou":"12345678",
		"counterIban":"UA9999"
	}`)

	var tx Transaction
	require.NoError(t, json.Unmarshal(in, &tx))

	assert.Equal(t, "R-001", tx.ReceiptID)
	assert.Equal(t, "INV-42", tx.InvoiceID)
	assert.Equal(t, "12345678", tx.EDRPOU)
	assert.Equal(t, "UA9999", tx.IBAN)
}

func TestTransaction_UnmarshalJSON_malformed(t *testing.T) {
	var tx Transaction
	require.Error(t, json.Unmarshal([]byte(`{not json}`), &tx))
}

// Перевірка, що ClientInfo роботає на повному payload (Account + Jar
// разом, з різними валютами).
func TestClientInfo_endToEnd(t *testing.T) {
	in := []byte(`{
		"clientId":"c1",
		"name":"Test",
		"accounts":[
			{"id":"a1","currencyCode":980,"balance":100,"creditLimit":0,"type":"black","iban":"UA1"},
			{"id":"a2","currencyCode":840,"balance":200,"creditLimit":0,"type":"white","iban":"UA2"}
		],
		"jars":[
			{"id":"j1","title":"j","currencyCode":978,"balance":300,"goal":1000}
		]
	}`)

	var info ClientInfo
	require.NoError(t, json.Unmarshal(in, &info))

	require.Len(t, info.Accounts, 2)
	assert.Equal(t, currency.UAH, info.Accounts[0].Balance.Code)
	assert.Equal(t, currency.USD, info.Accounts[1].Balance.Code)
	require.Len(t, info.Jars, 1)
	assert.Equal(t, currency.EUR, info.Jars[0].Balance.Code)
}
