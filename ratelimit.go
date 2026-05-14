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

// limiterKeyType — приватний тип ключа контексту, щоб уникнути колізій
// з ключами інших пакетів.
type limiterKeyType struct{}

// WithLimiterKey повертає копію ctx із key, який [KeyedLimiter] використає
// для вибору відповідної per-key корзини. Якщо ключа в контексті нема,
// KeyedLimiter трактує запит як "" (спільна дефолтна корзина).
//
//	ctx = monobank.WithLimiterKey(ctx, accountID)
//	cli.Transactions(ctx, accountID, from, to)
func WithLimiterKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, limiterKeyType{}, key)
}

// limiterKeyFrom повертає ключ, прокладений [WithLimiterKey], або "".
func limiterKeyFrom(ctx context.Context) string {
	v, _ := ctx.Value(limiterKeyType{}).(string)
	return v
}

// KeyedLimiter ліниво створює окремий [Limiter] на кожен ключ — типово
// це accountID, бо Mono обмежує /personal/statement/{account}/… незалежно
// для кожного рахунку. Реалізує [RateLimiter]: ключ береться з контексту
// через [WithLimiterKey].
//
//	// idleTTL=10*time.Minute — корзини, до яких не зверталися
//	// 10 хв, видаляються фоновим sweeper-ом, щоб мапа не росла
//	// безкінечно у long-running процесах.
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
// Безпечний для конкурентного використання.
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

// NewKeyedLimiter повертає лімітер, що для кожного унікального ключа
// створює власну корзину з параметрами every / burst (як [NewLimiter]).
//
// idleTTL > 0 запускає фоновий sweeper, який видаляє корзини, до яких
// не зверталися довше за idleTTL (захист від витоку пам'яті при
// великій кількості унікальних ключів). idleTTL <= 0 — eviction
// вимкнений; підходить для коротких CLI-утиліт, але long-running
// процесам обов'язково передавай розумне значення (наприклад, у 10×
// більше за every).
//
// Завжди викликай [KeyedLimiter.Stop] на завершенні роботи (через
// defer одразу після створення), щоб зупинити sweeper-горутину.
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

// Stop зупиняє фоновий sweeper. Безпечно викликати кілька разів і на
// лімітері без sweeper-а (idleTTL <= 0) — у такому випадку no-op.
// Після Stop лімітер усе ще обслуговує Wait/WaitKey-виклики коректно;
// просто корзини більше не евіктяться автоматично.
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

// WaitKey блокує до моменту, коли в корзині для key буде доступний токен.
// Зручно, коли користувач не хоче пробрасувати ключ через контекст.
func (k *KeyedLimiter) WaitKey(ctx context.Context, key string) error {
	return k.bucket(key).Wait(ctx)
}

// Wait — реалізація [RateLimiter]. Витягує ключ із контексту через
// [WithLimiterKey]; якщо ключа нема — використовує спільну корзину з
// ключем "".
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
