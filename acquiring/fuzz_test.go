package acquiring

import "testing"

func FuzzParsePubKey(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("not-base64!!!"))
	f.Add([]byte("LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0K")) // "-----BEGIN PUBLIC KEY-----\n"
	f.Add([]byte("AAAA"))                                 // valid base64 of 3 zero bytes (not a key)

	f.Fuzz(func(t *testing.T, raw []byte) {
		// Property: ParsePubKey must never panic. Invalid base64,
		// invalid PEM, invalid SPKI, wrong curve — all return error.
		_, _ = ParsePubKey(raw)
	})
}

func FuzzParseWebhook(f *testing.F) {
	f.Add([]byte(`{"invoiceId":"i1","status":"success","amount":1000,"ccy":980,"createdDate":"2026-01-01T00:00:00Z","modifiedDate":"2026-01-01T00:00:01Z"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"status":"created","amount":"not-a-number"}`))

	f.Fuzz(func(t *testing.T, body []byte) {
		// Property: never panic. Invalid payloads return error.
		_, _ = ParseWebhook(body)
	})
}
