package monobank

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRetryAfter(t *testing.T) {
	tests := map[string]struct {
		header string
		want   time.Duration
	}{
		"missing":       {"", noRetryAfter},
		"seconds":       {"5", 5 * time.Second},
		"zero seconds":  {"0", 0}, // explicit immediate retry
		"negative":      {"-3", noRetryAfter},
		"garbage":       {"soon", noRetryAfter},
		"http-date now": {"Sun, 06 Nov 1994 08:49:37 GMT", 0}, // past → immediate
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			h := http.Header{}
			if tc.header != "" {
				h.Set("Retry-After", tc.header)
			}
			assert.Equal(t, tc.want, parseRetryAfter(h))
		})
	}

	t.Run("http-date future", func(t *testing.T) {
		future := time.Now().Add(10 * time.Second).UTC().Format(http.TimeFormat)
		h := http.Header{"Retry-After": []string{future}}
		got := parseRetryAfter(h)
		assert.Greater(t, got, 5*time.Second)
		assert.LessOrEqual(t, got, 10*time.Second)
	})
}

// Regression: equal jitter must never return less than ~half of the
// computed backoff. Full jitter (the previous policy) could return 0
// after a 5xx, turning the retry loop into a tight spin that
// hammered the bank instead of giving it slack.
func TestBackoff_equalJitterHasFloor(t *testing.T) {
	const base = 500 * time.Millisecond
	const maxD = 30 * time.Second
	// Run many samples — random distribution must always land in
	// [d/2, d] for an attempt, clamped at max.
	for i := 0; i < 200; i++ {
		got := backoff(base, maxD, 2) // d = base << 2 = 2s
		assert.GreaterOrEqual(t, got, time.Second,
			"equal jitter must give at least d/2 (1s), got %s", got)
		assert.LessOrEqual(t, got, 2*time.Second,
			"equal jitter must not exceed d (2s), got %s", got)
	}

	// Smallest delay must respect the absolute floor even when the
	// base is tiny (1ns).
	tiny := backoff(time.Nanosecond, time.Second, 0)
	assert.GreaterOrEqual(t, tiny, minBackoffFloor,
		"backoff must respect the 50ms floor")
}

// дуже великий attempt не повинен переповнювати int64
// і панікувати в jitter.
func TestBackoff_NoOverflow(t *testing.T) {
	// attempt=63 → base<<63 переповнить int64.
	got := backoff(500*time.Millisecond, 30*time.Second, 63)
	assert.LessOrEqual(t, got, 30*time.Second, "must clamp at max")
	assert.GreaterOrEqual(t, got, time.Duration(0))

	// Без max → повертає base замість overflow.
	got2 := backoff(time.Second, 0, 70)
	assert.GreaterOrEqual(t, got2, time.Duration(0))
}

func TestRetryPolicy_disabledByDefault(t *testing.T) {
	var calls atomic.Int32
	err := retryPolicy{}.run(context.Background(), func() error {
		calls.Add(1)
		return errors.New("nope")
	})
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load(), "zero-value policy must not retry")
}

func TestRetryPolicy_retriesTransient(t *testing.T) {
	rp := retryPolicy{maxAttempts: 3, baseDelay: time.Millisecond, maxDelay: 5 * time.Millisecond}

	var attempts atomic.Int32
	err := rp.run(context.Background(), func() error {
		n := attempts.Add(1)
		if n < 3 {
			return &transientStatus{code: 503, cause: errors.New("upstream")}
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int32(3), attempts.Load())
}

func TestRetryPolicy_giveUpAfterMaxAttempts(t *testing.T) {
	rp := retryPolicy{maxAttempts: 2, baseDelay: time.Millisecond, maxDelay: 2 * time.Millisecond}

	var attempts atomic.Int32
	err := rp.run(context.Background(), func() error {
		attempts.Add(1)
		return &transientStatus{code: 503, cause: errors.New("still down")}
	})
	require.Error(t, err)
	assert.Equal(t, int32(2), attempts.Load())
	var ts *transientStatus
	assert.ErrorAs(t, err, &ts)
}

func TestRetryPolicy_doesNotRetryPermanent(t *testing.T) {
	rp := retryPolicy{maxAttempts: 5, baseDelay: time.Millisecond, maxDelay: 2 * time.Millisecond}

	var attempts atomic.Int32
	permanent := errors.New("4xx-ish")
	err := rp.run(context.Background(), func() error {
		attempts.Add(1)
		return permanent
	})
	assert.ErrorIs(t, err, permanent)
	assert.Equal(t, int32(1), attempts.Load(), "non-transient errors must not retry")
}

func TestRetryPolicy_respectsContext(t *testing.T) {
	rp := retryPolicy{maxAttempts: 10, baseDelay: 50 * time.Millisecond, maxDelay: time.Second}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := rp.run(ctx, func() error {
		// noRetryAfter → backoff sleep gives ctx a chance to fire.
		return &transientStatus{code: 503, retryAfter: noRetryAfter, cause: errors.New("down")}
	})
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRetryPolicy_endToEndViaClient_with429AndRetryAfter(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(
		WithHTTPClient(srv.Client()),
		WithBaseURL(srv.URL),
		WithRetry(3, time.Millisecond, 10*time.Millisecond),
	)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))
	assert.Equal(t, int32(2), hits.Load(), "expected one retry after 429")
}

func TestTransientStatusError(t *testing.T) {
	e := &transientStatus{code: 503, retryAfter: 2 * time.Second, cause: errors.New("upstream")}
	got := e.Error()
	require.Contains(t, got, "503")
	require.Contains(t, got, "2s")

	e2 := &transientStatus{code: 502, cause: fmt.Errorf("bad gateway")}
	require.Contains(t, e2.Error(), "502")
}
