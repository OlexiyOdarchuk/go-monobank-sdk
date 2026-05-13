package money

import (
	"testing"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
)

func BenchmarkAdd(b *testing.B) {
	x := New(12550, currency.UAH)
	y := New(7777, currency.UAH)
	b.ResetTimer()
	for range b.N {
		_, _ = x.Add(y)
	}
}

func BenchmarkScale(b *testing.B) {
	x := New(12550, currency.UAH)
	b.ResetTimer()
	for range b.N {
		_ = x.Scale(1.07)
	}
}

func BenchmarkString(b *testing.B) {
	x := New(12550, currency.UAH)
	b.ReportAllocs()
	for range b.N {
		_ = x.String()
	}
}
