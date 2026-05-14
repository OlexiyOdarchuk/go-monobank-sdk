package monobank

import (
	"net/http"
	"testing"
)

func FuzzParseErrorDescription(f *testing.F) {
	f.Add([]byte(`{"errorDescription":"Unknown 'X-Token'"}`))
	f.Add([]byte(`{"errorDescription":""}`))
	f.Add([]byte(`{"errorDescription":"oops","traceId":"abc"}`))
	f.Add([]byte(`{"foo":"bar"}`))
	f.Add([]byte(``))
	f.Add([]byte(`<html>error</html>`))
	f.Add([]byte(`{"errorDescription": null}`))
	f.Add([]byte(`{"errorDescription": 42}`))

	f.Fuzz(func(t *testing.T, body []byte) {
		// Property: never panics on arbitrary input. Result is always
		// a string (possibly empty) — implementation MUST NOT crash on
		// malformed JSON, weird types, huge inputs, etc.
		_ = parseErrorDescription(body)
	})
}

func FuzzParseRetryAfter(f *testing.F) {
	f.Add("0")
	f.Add("30")
	f.Add("Thu, 01 Jan 2026 00:00:00 GMT")
	f.Add("")
	f.Add("not a number")
	f.Add("-5")
	f.Add("99999999999999999999999")

	f.Fuzz(func(t *testing.T, raw string) {
		h := http.Header{}
		h.Set("Retry-After", raw)
		// Property: returns either noRetryAfter (-1) for unparseable
		// inputs, or a non-negative duration. Must not panic on any
		// input.
		got := parseRetryAfter(h)
		if got < 0 && got != noRetryAfter {
			t.Fatalf("parseRetryAfter returned unexpected negative duration %v for %q", got, raw)
		}
	})
}
