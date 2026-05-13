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
// Сигнатура збігається з *golang.org/x/time/rate.Limiter.Wait — туди
// можна підставити будь-який вже існуючий лімітер без обгортки.
type RateLimiter interface {
	Wait(ctx context.Context) error
}

// Limiter — простий token-bucket. Корзина наповнюється 1 токеном кожні
// every; максимум — burst токенів одночасно. Безпечний для конкурентного
// використання.
//
// Дефолтні ліміти Mono:
//
//   - /personal/client-info — 1 виклик на 60 с (every=time.Minute, burst=1)
//   - /personal/statement/{account}/… — 1 виклик на акаунт на 60 с
//
// Для per-account обмежень створюй окремий [Limiter] (і відповідно
// окремий клієнт) на кожен акаунт або реалізуй власний [RateLimiter].
type Limiter struct {
	mu     sync.Mutex
	every  time.Duration
	burst  int
	tokens float64
	last   time.Time
}

// NewLimiter повертає лімітер, що допускає 1 запит кожні every із
// можливістю короткочасних сплесків до burst. every <= 0 означає «без
// обмежень» — Wait завжди повертається миттєво. burst < 1 нормалізується
// до 1.
//
//	// 1 запит на 60 секунд (як у /personal/client-info)
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

// Wait блокує до моменту, коли доступний хоча б один токен, або до
// скасування контексту.
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
