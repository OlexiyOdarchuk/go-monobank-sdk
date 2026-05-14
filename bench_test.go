package monobank

import (
	"context"
	"testing"
	"time"
)

func BenchmarkLimiterWait_Unlimited(b *testing.B) {
	lim := NewLimiter(0, 1) // every<=0 → fast path
	ctx := context.Background()
	b.ResetTimer()
	for range b.N {
		_ = lim.Wait(ctx)
	}
}

func BenchmarkLimiterWait_HighBurst(b *testing.B) {
	// burst large enough that Wait never blocks during the bench.
	lim := NewLimiter(time.Millisecond, b.N+1)
	ctx := context.Background()
	b.ResetTimer()
	for range b.N {
		_ = lim.Wait(ctx)
	}
}

func BenchmarkKeyedLimiterWait(b *testing.B) {
	klim := NewKeyedLimiter(0, 1, 0)
	ctx := WithLimiterKey(context.Background(), "acc-1")
	b.ResetTimer()
	for range b.N {
		_ = klim.Wait(ctx)
	}
}

var benchErrBody = []byte(`{"errorDescription":"Unknown 'X-Token'","traceId":"abc123"}`)

func BenchmarkParseErrorDescription(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = parseErrorDescription(benchErrBody)
	}
}
