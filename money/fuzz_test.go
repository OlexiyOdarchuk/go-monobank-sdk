package money

import (
	"encoding/json"
	"testing"
)

func FuzzMoneyUnmarshalJSON(f *testing.F) {
	f.Add([]byte(`12550`))
	f.Add([]byte(`0`))
	f.Add([]byte(`-1`))
	f.Add([]byte(`9223372036854775807`))  // max int64
	f.Add([]byte(`-9223372036854775808`)) // min int64
	f.Add([]byte(`null`))
	f.Add([]byte(`""`))
	f.Add([]byte(`"123"`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`[1,2,3]`))
	f.Add([]byte(`9999999999999999999999999`)) // overflows int64

	f.Fuzz(func(t *testing.T, raw []byte) {
		// Property: UnmarshalJSON never panics. It either succeeds and
		// produces a Money whose JSON round-trips, or returns an error.
		var m Money
		if err := json.Unmarshal(raw, &m); err != nil {
			return
		}
		out, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("Marshal after successful Unmarshal failed: %v", err)
		}
		var m2 Money
		if err := json.Unmarshal(out, &m2); err != nil {
			t.Fatalf("round-trip Unmarshal failed: %v (out=%s)", err, out)
		}
		if m.Minor != m2.Minor {
			t.Fatalf("round-trip mismatch: %d != %d", m.Minor, m2.Minor)
		}
	})
}
