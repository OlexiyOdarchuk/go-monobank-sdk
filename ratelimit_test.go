package monobank

import (
	"context"
	"net/http"
	"net/http/httptest"
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
