package monobank

import (
	"context"
	"sync"
	"time"
)

// RateLimiter throttles outbound requests. Implementations must be safe
// for concurrent use. [Wait] blocks until a token is available or ctx
// is canceled.
//
// The signature matches *golang.org/x/time/rate.Limiter.Wait — any
// existing limiter can be dropped in without a wrapper.
type RateLimiter interface {
	Wait(ctx context.Context) error
}

// Limiter is a simple token bucket. The bucket refills at one token
// every every; up to burst tokens are stored at once. Safe for
// concurrent use.
//
// Mono's default limits:
//
//   - /personal/client-info — 1 call per 60 s (every=time.Minute, burst=1)
//   - /personal/statement/{account}/… — 1 call per account per 60 s
//
// For per-account limits, create a separate [Limiter] (and a separate
// client) for each account, or implement a custom [RateLimiter].
type Limiter struct {
	mu     sync.Mutex
	every  time.Duration
	burst  int
	tokens float64
	last   time.Time
}

// NewLimiter returns a limiter that allows one request every every
// with short bursts up to burst. every <= 0 means "no limit" — Wait
// always returns immediately. burst < 1 is normalized to 1.
//
//	// 1 request per 60 seconds (as in /personal/client-info)
//	lim := monobank.NewLimiter(time.Minute, 1)
//	cli := personal.New(token, monobank.WithRateLimiter(lim))
func NewLimiter(every time.Duration, burst int) *Limiter {
	if burst < 1 {
		burst = 1
	}
	return &Limiter{
		every:  every,
		burst:  burst,
		tokens: float64(burst),
		last:   time.Now(),
	}
}

// Wait blocks until at least one token is available or the context
// is canceled.
func (l *Limiter) Wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if l.every <= 0 {
		return nil
	}
	for {
		l.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(l.last)
		l.last = now
		l.tokens += float64(elapsed) / float64(l.every)
		if l.tokens > float64(l.burst) {
			l.tokens = float64(l.burst)
		}
		if l.tokens >= 1 {
			l.tokens--
			l.mu.Unlock()
			return nil
		}
		need := 1 - l.tokens
		wait := time.Duration(need * float64(l.every))
		l.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// limiterKeyType is the private context-key type, used to avoid
// collisions with keys from other packages.
type limiterKeyType struct{}

// WithLimiterKey returns a copy of ctx carrying key, which
// [KeyedLimiter] uses to pick the matching per-key bucket. If the
// key is absent from the context, KeyedLimiter treats the request as
// "" (the shared default bucket).
//
//	ctx = monobank.WithLimiterKey(ctx, accountID)
//	cli.Transactions(ctx, accountID, from, to)
func WithLimiterKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, limiterKeyType{}, key)
}

// limiterKeyFrom returns the key threaded through by [WithLimiterKey]
// or "".
func limiterKeyFrom(ctx context.Context) string {
	v, _ := ctx.Value(limiterKeyType{}).(string)
	return v
}

// KeyedLimiter lazily creates a separate [Limiter] per key — typically
// the accountID, because Mono limits /personal/statement/{account}/…
// independently per account. Implements [RateLimiter]: the key is
// taken from the context via [WithLimiterKey].
//
//	// idleTTL=10*time.Minute — buckets that have not been used for
//	// 10 min are removed by a background sweeper so the map does
//	// not grow without bound in long-running processes.
//	klim := monobank.NewKeyedLimiter(time.Minute, 1, 10*time.Minute)
//	defer klim.Stop()
//
//	cli := personal.New(token, monobank.WithRateLimiter(klim))
//
//	for _, acc := range info.Accounts {
//	    ctx := monobank.WithLimiterKey(ctx, acc.ID)
//	    txs, err := cli.Transactions(ctx, acc.ID, from, to)
//	    // …
//	}
//
// Safe for concurrent use.
type KeyedLimiter struct {
	every   time.Duration
	burst   int
	idleTTL time.Duration

	mu       sync.Mutex
	limiters map[string]*keyedBucket

	stopCh   chan struct{}
	stopOnce sync.Once
}

type keyedBucket struct {
	lim        *Limiter
	lastAccess time.Time
}

// NewKeyedLimiter returns a limiter that, for each unique key, creates
// its own bucket with the every / burst parameters (as in [NewLimiter]).
//
// idleTTL > 0 starts a background sweeper that removes buckets not
// touched for longer than idleTTL (guards against memory leaks with a
// large number of unique keys). idleTTL <= 0 disables eviction; fine
// for short-lived CLI utilities, but long-running processes should
// always pass a reasonable value (for example, 10× every).
//
// Always call [KeyedLimiter.Stop] on shutdown (via defer right after
// construction) to stop the sweeper goroutine.
func NewKeyedLimiter(every time.Duration, burst int, idleTTL time.Duration) *KeyedLimiter {
	if burst < 1 {
		burst = 1
	}
	k := &KeyedLimiter{
		every:    every,
		burst:    burst,
		idleTTL:  idleTTL,
		limiters: make(map[string]*keyedBucket),
		stopCh:   make(chan struct{}),
	}
	if idleTTL > 0 {
		go k.sweep()
	}
	return k
}

// Stop stops the background sweeper. Safe to call multiple times,
// and on a limiter without a sweeper (idleTTL <= 0) — in that case
// it is a no-op. After Stop the limiter still serves Wait/WaitKey
// calls correctly; the buckets are simply no longer evicted
// automatically.
func (k *KeyedLimiter) Stop() {
	k.stopOnce.Do(func() { close(k.stopCh) })
}

func (k *KeyedLimiter) sweep() {
	interval := k.idleTTL / 2
	if interval < time.Second {
		interval = time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-k.stopCh:
			return
		case now := <-t.C:
			k.evictIdle(now)
		}
	}
}

func (k *KeyedLimiter) evictIdle(now time.Time) {
	k.mu.Lock()
	defer k.mu.Unlock()
	for key, b := range k.limiters {
		if now.Sub(b.lastAccess) > k.idleTTL {
			delete(k.limiters, key)
		}
	}
}

// WaitKey blocks until a token is available in the bucket for key.
// Handy when the caller does not want to thread the key through the
// context.
func (k *KeyedLimiter) WaitKey(ctx context.Context, key string) error {
	return k.bucket(key).Wait(ctx)
}

// Wait implements [RateLimiter]. It extracts the key from the
// context via [WithLimiterKey]; if no key is present, it uses the
// shared bucket with the "" key.
func (k *KeyedLimiter) Wait(ctx context.Context) error {
	return k.WaitKey(ctx, limiterKeyFrom(ctx))
}

func (k *KeyedLimiter) bucket(key string) *Limiter {
	k.mu.Lock()
	defer k.mu.Unlock()
	b, ok := k.limiters[key]
	if !ok {
		b = &keyedBucket{lim: NewLimiter(k.every, k.burst)}
		k.limiters[key] = b
	}
	b.lastAccess = time.Now()
	return b.lim
}
