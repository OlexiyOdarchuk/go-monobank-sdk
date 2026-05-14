package installment_test

import (
	"encoding/json"
	"testing"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/installment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoney_ExactRoundTrip(t *testing.T) {
	// The motivation for the type: 2499.99 is NOT exactly
	// representable as float64. We need byte-for-byte fidelity.
	cases := []struct {
		raw  string
		want int64 // kopecks
	}{
		{`2499.99`, 249999},
		{`0`, 0},
		{`0.05`, 5},
		{`0.5`, 50},
		{`100`, 10000},
		{`-100.50`, -10050},
		{`1000000.01`, 100000001},
		// String forms also work (some endpoints quote money).
		{`"2499.99"`, 249999},
		{`"0"`, 0},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			var m installment.Money
			require.NoError(t, m.UnmarshalJSON([]byte(c.raw)))
			assert.Equal(t, c.want, m.Kopecks)

			// And back out exactly (without quotes — JSON number).
			out, err := m.MarshalJSON()
			require.NoError(t, err)
			// Unmarshalling the marshalled output yields the same int.
			var roundTrip installment.Money
			require.NoError(t, roundTrip.UnmarshalJSON(out))
			assert.Equal(t, m.Kopecks, roundTrip.Kopecks,
				"round-trip must preserve every kopeck; got %q", string(out))
		})
	}
}

func TestMoney_NewMoneyHelpers(t *testing.T) {
	assert.Equal(t, int64(249999), installment.NewMoney(2499, 99).Kopecks)
	assert.Equal(t, int64(50), installment.MoneyFromKopecks(50).Kopecks)
	assert.Equal(t, int64(50), installment.MoneyFromMajor(0.5).Kopecks)
	// Rounding half away from zero.
	assert.Equal(t, int64(1), installment.MoneyFromMajor(0.005).Kopecks)
	assert.Equal(t, int64(-1), installment.MoneyFromMajor(-0.005).Kopecks)
}

func TestMoney_String(t *testing.T) {
	assert.Equal(t, "2499.99", installment.NewMoney(2499, 99).String())
	assert.Equal(t, "0.05", installment.MoneyFromKopecks(5).String())
	assert.Equal(t, "0.00", installment.MoneyFromKopecks(0).String())
	assert.Equal(t, "-100.50", installment.MoneyFromKopecks(-10050).String())
}

// Regression: marshalling a struct that embeds Money must keep the
// JSON number shape (no quotes), so Mono's server-side decoder is
// happy.
func TestMoney_WireFormat(t *testing.T) {
	req := installment.CreateOrderRequest{
		StoreOrderID: "ORD-1",
		ClientPhone:  "+380501234561",
		TotalSum:     installment.NewMoney(2499, 99),
		Products:     []installment.Product{{Name: "X", Count: 1, Sum: installment.NewMoney(2499, 99)}},
	}
	body, err := json.Marshal(req)
	require.NoError(t, err)

	got := string(body)
	assert.Contains(t, got, `"total_sum":2499.99`,
		"Money must serialise as bare JSON number, not a string")
	assert.Contains(t, got, `"sum":2499.99`)
}

// Fuzz: UnmarshalJSON must never panic on adversarial input. It can
// legitimately return ErrInvalidMoney; what it cannot do is corrupt
// memory or hang.
func FuzzMoney_UnmarshalJSON(f *testing.F) {
	seeds := []string{
		"0", "1", "1.5", "-1", "2499.99", `"2499.99"`,
		"", "null", ".5", "00.05", "1.500", "1e10",
		"99999999999999999999", "-0.00",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(_ *testing.T, b []byte) {
		var m installment.Money
		_ = m.UnmarshalJSON(b)
		// Whether parse succeeded or not, calling MarshalJSON
		// afterwards must always succeed and be valid JSON.
		out, err := m.MarshalJSON()
		if err != nil {
			panic(err)
		}
		if len(out) == 0 {
			panic("MarshalJSON returned empty bytes")
		}
		var roundTrip installment.Money
		if err := roundTrip.UnmarshalJSON(out); err != nil {
			panic("MarshalJSON output failed to UnmarshalJSON: " + err.Error())
		}
	})
}

// Regression: the wire format keeps full precision even for values
// that float64 cannot represent exactly. 0.10 is one such value.
func TestMoney_NoFloatPrecisionLoss(t *testing.T) {
	m := installment.NewMoney(0, 10) // 0.10 UAH
	body, err := json.Marshal(m)
	require.NoError(t, err)
	assert.Equal(t, "0.10", string(body))

	var back installment.Money
	require.NoError(t, json.Unmarshal(body, &back))
	assert.Equal(t, int64(10), back.Kopecks)
}
