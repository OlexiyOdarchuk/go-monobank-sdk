package monobank

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// noRetryAfter is the sentinel value of parseRetryAfter meaning
// "header missing or unparseable". It distinguishes this case from a
// real "Retry-After: 0" (retry immediately).
const noRetryAfter = time.Duration(-1)

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

		var delay time.Duration
		switch {
		case ts.retryAfter > 0:
			delay = ts.retryAfter
		case ts.retryAfter == 0:
			// Server explicitly said "retry now" (Retry-After: 0).
			delay = 0
		default: // noRetryAfter — header absent
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

// minBackoffFloor is the smallest delay backoff ever returns when
// the policy is enabled (base > 0). Without a floor, full or equal
// jitter at very small bases can produce 0ns delays — a 5xx storm
// would translate into an instant-retry storm against the bank.
const minBackoffFloor = 50 * time.Millisecond

// backoff returns the delay for attempt n (0-indexed) using
// exponential growth with EQUAL jitter (d/2 + rand(d/2)), clamped
// at max. The d/2 floor avoids the full-jitter degenerate case
// where rand.Int64N(d+1) can produce 0 and turn the retry loop
// into a tight spin. Guarded against int64 overflow for large
// attempt values: as soon as the shift would overflow the base,
// returns max.
func backoff(base, max time.Duration, attempt int) time.Duration {
	if base <= 0 {
		return 0
	}
	// Check overflow BEFORE the shift: if base << attempt would exceed
	// MaxInt64, do not shift at all — return max (or base itself when
	// max is unset).
	var d time.Duration
	const maxShift = 62 // 1 << 62 still fits in int64
	if attempt >= maxShift || base > time.Duration(math.MaxInt64>>attempt) {
		d = max
		if d <= 0 {
			d = base
		}
	} else {
		d = base << attempt
	}
	if max > 0 && d > max {
		d = max
	}
	if d <= 0 {
		return 0
	}
	// Equal jitter — half deterministic, half random. math/rand/v2
	// has no global mutex (PCG per goroutine), safe for high RPS.
	half := int64(d) / 2
	if half <= 0 {
		half = 1
	}
	jitter := time.Duration(half + rand.Int64N(half))
	if jitter < minBackoffFloor {
		jitter = minBackoffFloor
	}
	if max > 0 && jitter > max {
		jitter = max
	}
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
// Returns the duration the client should wait. Special cases:
//   - header missing or unparseable → [noRetryAfter] (-1)
//   - explicit "0" or HTTP-date in the past → 0 (retry immediately)
//   - any positive value → that value
//
// Distinguishing "absent" from "0" lets the retry logic apply
// exponential backoff only when the server gave no hint, rather than
// when it said "retry now".
func parseRetryAfter(h http.Header) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return noRetryAfter
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
		return 0 // past date → retry immediately
	}
	return noRetryAfter
}
