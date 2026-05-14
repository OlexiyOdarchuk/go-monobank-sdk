package monobank

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimiter_AllowsBurst(t *testing.T) {
	lim := NewLimiter(time.Second, 3)
	for range 3 {
		require.NoError(t, lim.Wait(context.Background()))
	}
}

func TestLimiter_BlocksBeyondBurst(t *testing.T) {
	lim := NewLimiter(50*time.Millisecond, 1)
	// First Wait drains the bucket immediately.
	require.NoError(t, lim.Wait(context.Background()))

	start := time.Now()
	require.NoError(t, lim.Wait(context.Background()))
	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 40*time.Millisecond,
		"second Wait must block until refill (~50ms)")
}

func TestLimiter_RespectsContext(t *testing.T) {
	lim := NewLimiter(time.Hour, 1)
	require.NoError(t, lim.Wait(context.Background())) // drain

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	err := lim.Wait(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestLimiter_ZeroEveryDisablesLimit(t *testing.T) {
	lim := NewLimiter(0, 1)
	for range 100 {
		require.NoError(t, lim.Wait(context.Background()))
	}
}

func TestLimiter_BurstNormalizedToOne(t *testing.T) {
	lim := NewLimiter(time.Second, 0)
	require.NoError(t, lim.Wait(context.Background()))
	// burst of 0 was normalized to 1; second call should now block
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	assert.Error(t, lim.Wait(ctx))
}

func TestLimiter_PreCanceledContext(t *testing.T) {
	lim := NewLimiter(time.Second, 5)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := lim.Wait(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

// recordingLimiter counts Wait invocations.
type recordingLimiter struct {
	calls atomic.Int32
	err   error
}

func (r *recordingLimiter) Wait(ctx context.Context) error {
	r.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.err
}

func TestWithRateLimiter_WaitCalledOncePerDo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	lim := &recordingLimiter{}
	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithRateLimiter(lim))

	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))
	assert.Equal(t, int32(1), lim.calls.Load())
}

// On retry, the limiter must NOT be re-consumed: 1 logical request = 1 token.
func TestWithRateLimiter_NotCalledPerRetryAttempt(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	lim := &recordingLimiter{}
	c := New(
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithRateLimiter(lim),
		WithRetry(3, time.Millisecond, 10*time.Millisecond),
	)
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))
	assert.Equal(t, int32(2), hits.Load(), "request should retry once after 429")
	assert.Equal(t, int32(1), lim.calls.Load(), "limiter should be hit only once per Do call")
}

func TestWithRateLimiter_PropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("server must not be hit when limiter denies")
	}))
	defer srv.Close()

	// Pre-canceled context — limiter's Wait must return ctx.Err and Do
	// must propagate it without dialing the server.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithRateLimiter(NewLimiter(time.Hour, 5)))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/x", http.NoBody)
	assert.ErrorIs(t, c.Do(req, nil), context.Canceled)
}

func TestWithRateLimiter_NilIgnored(t *testing.T) {
	c := New(WithRateLimiter(nil))
	assert.Nil(t, c.limiter)
}

// --- KeyedLimiter ---

func TestKeyedLimiter_PerKeyIndependent(t *testing.T) {
	klim := NewKeyedLimiter(time.Hour, 1, 0)
	require.NoError(t, klim.WaitKey(context.Background(), "a"))
	// Different key → different bucket, must not block.
	require.NoError(t, klim.WaitKey(context.Background(), "b"))

	// Same key again → would block. Use short ctx to confirm.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, klim.WaitKey(ctx, "a"), context.DeadlineExceeded)
}

func TestKeyedLimiter_ReadsKeyFromContext(t *testing.T) {
	klim := NewKeyedLimiter(time.Hour, 1, 0)
	ctxA := WithLimiterKey(context.Background(), "acc-A")
	ctxB := WithLimiterKey(context.Background(), "acc-B")

	require.NoError(t, klim.Wait(ctxA))
	require.NoError(t, klim.Wait(ctxB)) // separate bucket
}

func TestKeyedLimiter_NoKeyUsesDefault(t *testing.T) {
	klim := NewKeyedLimiter(time.Hour, 2, 0)
	// Both calls share the "" bucket; burst=2 covers them.
	require.NoError(t, klim.Wait(context.Background()))
	require.NoError(t, klim.Wait(context.Background()))
}

func TestKeyedLimiter_LazyCreation(t *testing.T) {
	klim := NewKeyedLimiter(time.Second, 1, 0)
	assert.Empty(t, klim.limiters, "no buckets created until first Wait")
	_ = klim.WaitKey(context.Background(), "x")
	assert.Len(t, klim.limiters, 1)
	_ = klim.WaitKey(context.Background(), "y")
	assert.Len(t, klim.limiters, 2)
}

func TestKeyedLimiter_EvictsIdle(t *testing.T) {
	klim := NewKeyedLimiter(time.Hour, 1, 50*time.Millisecond)
	defer klim.Stop()

	require.NoError(t, klim.WaitKey(context.Background(), "x"))
	require.NoError(t, klim.WaitKey(context.Background(), "y"))
	require.Len(t, klim.limiters, 2)

	// sweeper runs every idleTTL/2 (capped at 1s); wait long enough for
	// at least one tick AFTER all entries cross the TTL threshold.
	require.Eventually(t, func() bool {
		klim.mu.Lock()
		defer klim.mu.Unlock()
		return len(klim.limiters) == 0
	}, 3*time.Second, 50*time.Millisecond, "idle buckets should be evicted")
}

func TestKeyedLimiter_StopIdempotent(t *testing.T) {
	klim := NewKeyedLimiter(time.Hour, 1, time.Minute)
	klim.Stop()
	klim.Stop() // must not panic
}

func TestKeyedLimiter_ImplementsRateLimiter(t *testing.T) {
	var _ RateLimiter = (*KeyedLimiter)(nil)
}

func TestKeyedLimiter_ConcurrentSafe(t *testing.T) {
	klim := NewKeyedLimiter(0, 1, 0) // unlimited, just stress map access
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "k" + strconv.Itoa(n%5)
			for range 10 {
				_ = klim.WaitKey(context.Background(), key)
			}
		}(i)
	}
	wg.Wait()
	assert.LessOrEqual(t, len(klim.limiters), 5)
}

func TestWithLimiterKey_RoundTrip(t *testing.T) {
	ctx := WithLimiterKey(context.Background(), "abc")
	assert.Equal(t, "abc", limiterKeyFrom(ctx))
	assert.Equal(t, "", limiterKeyFrom(context.Background()))
}
