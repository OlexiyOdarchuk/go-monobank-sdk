package bank

import (
	"encoding/json"
	"testing"
)

var benchTxBody = []byte(`{
	"id": "ZuHWzqkKGVo=",
	"time": 1700000000,
	"description": "Покупка у Сільпо",
	"mcc": 5411,
	"originalMcc": 5411,
	"hold": false,
	"amount": -25099,
	"operationAmount": -25099,
	"currencyCode": 980,
	"commissionRate": 0,
	"cashbackAmount": 502,
	"balance": 1234567,
	"comment": "обід",
	"receiptId": "",
	"invoiceId": "",
	"counterEdrpou": "",
	"counterIban": ""
}`)

func BenchmarkTransactionUnmarshal(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchTxBody)))
	for range b.N {
		var t Transaction
		if err := json.Unmarshal(benchTxBody, &t); err != nil {
			b.Fatal(err)
		}
	}
}
