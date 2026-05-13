package monobank

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// retryPolicy controls automatic retries on transient HTTP failures.
//
// A zero-value retryPolicy disables retry (the request is attempted once).
// Configure it through [WithRetry] when constructing a Client.
type retryPolicy struct {
	maxAttempts int           // total attempts including the first; 0 or 1 means no retry
	baseDelay   time.Duration // initial backoff
	maxDelay    time.Duration // upper bound for any single sleep
}

// defaultRetry is the recipe applied by [WithRetry] when no overrides given.
var defaultRetry = retryPolicy{
	maxAttempts: 4,
	baseDelay:   500 * time.Millisecond,
	maxDelay:    30 * time.Second,
}

// run executes fn until it succeeds, the policy's attempt budget is
// exhausted, or fn returns a non-retryable error. Context cancellation
// short-circuits the loop immediately.
func (rp retryPolicy) run(ctx context.Context, fn func() error) error {
	attempts := rp.maxAttempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if i == attempts-1 {
			break
		}

		var ts *transientStatus
		if !errors.As(lastErr, &ts) {
			return lastErr
		}

		delay := ts.retryAfter
		if delay <= 0 {
			delay = backoff(rp.baseDelay, rp.maxDelay, i)
		}
		if delay > rp.maxDelay && rp.maxDelay > 0 {
			delay = rp.maxDelay
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

// backoff returns the delay for attempt n (0-indexed) using exponential
// growth with full jitter, clamped at max.
func backoff(base, max time.Duration, attempt int) time.Duration {
	if base <= 0 {
		return 0
	}
	d := base << attempt
	if d > max && max > 0 {
		d = max
	}
	jitter := time.Duration(rand.Int63n(int64(d) + 1))
	return jitter
}

// transientStatus is the error type wrapped around responses that look
// retryable. It carries a Retry-After hint (in nanoseconds) when the server
// supplied one.
type transientStatus struct {
	code       int
	retryAfter time.Duration
	cause      error
}

func (e *transientStatus) Error() string {
	if e.retryAfter > 0 {
		return fmt.Sprintf("transient HTTP %d (retry-after %s): %v", e.code, e.retryAfter, e.cause)
	}
	return fmt.Sprintf("transient HTTP %d: %v", e.code, e.cause)
}

func (e *transientStatus) Unwrap() error { return e.cause }

// parseRetryAfter parses a Retry-After header value per RFC 7231 §7.1.3.
// Returns 0 when the header is missing or unparseable.
func parseRetryAfter(h http.Header) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
