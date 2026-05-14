package monobank

import (
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
)

const defaultBaseURL = "https://api.monobank.ua"

// Option конфігурує [Client]. Передається у [New] (і так само у фабрики
// підпакетів — [personal.New], [bank.New] тощо, які пробрасують опції
// в New). Аддитивний дизайн: нові опції додаються без ламання існуючих
// викликів.
type Option func(*Client)

// WithHTTPClient ставить кастомний *http.Client (зручно для таймаутів,
// власних транспортів, проксі). nil або відсутність опції — використає
// стандартний &http.Client{} без таймауту.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient == nil {
			return
		}
		c.http = httpClient
	}
}

// WithHTTPDoer приймає будь-який [HTTPDoer]. Зручно для підставляння
// middleware-ів (circuit breakers, кастомні транспорти, тестові фейки).
// nil — ігнорується.
func WithHTTPDoer(d HTTPDoer) Option {
	return func(c *Client) {
		if d == nil {
			return
		}
		c.http = d
	}
}

// WithAuth прив'язує [auth.Authorizer] до клієнта. Підпакети (personal,
// corporate, business, acquiring) застосовують її, щоб підставити свою
// схему авторизації (X-Token, ECDSA-підпис тощо). nil ігнорується —
// дефолт залишається [auth.Public] (без авторизації).
func WithAuth(a auth.Authorizer) Option {
	return func(c *Client) {
		if a == nil {
			return
		}
		c.auth = a
	}
}

// WithLogger прикріплює *slog.Logger до клієнта. Якщо встановлений, SDK
// логує:
//
//   - Debug "monobank: sending request" — перед кожним HTTP-викликом
//     (method, url).
//   - Debug "monobank: http response" — успішну відповідь (method, url,
//     status, duration).
//   - Warn "monobank: http error" — транспортний збій (method, url,
//     duration, err).
//
// Логер викликається на кожну спробу окремо — ретраї дадуть кілька
// записів. nil ігнорується. Дефолт: не логувати.
//
//	cli := personal.New(token, monobank.WithLogger(slog.Default()))
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) {
		if l == nil {
			return
		}
		c.logger = l
	}
}

// WithRequestHook ставить callback, який викликається перед кожним
// надсиланням HTTP-запиту (включно з кожним ретраєм). Запит уже
// зрезолвлений до повного URL і має виставлені заголовки авторизації —
// hook може домутитити власні (наприклад, OpenTelemetry trace context,
// X-Correlation-Id). nil ігнорується.
//
//	cli := personal.New(token, monobank.WithRequestHook(func(r *http.Request) {
//	    r.Header.Set("X-Correlation-Id", uuid.NewString())
//	}))
func WithRequestHook(fn func(*http.Request)) Option {
	return func(c *Client) {
		if fn == nil {
			return
		}
		c.onReq = fn
	}
}

// WithResponseHook ставить callback, який викликається після кожної
// HTTP-відповіді (успіх і збій). Викликається ОДРАЗУ після
// http.Doer.Do, до парсингу й до перевірки expected-status. resp може
// бути nil (якщо err != nil); err може бути nil (якщо все добре). Корисно
// для метрик (latency per attempt, error counts). nil ігнорується.
//
//	cli := personal.New(token, monobank.WithResponseHook(func(r *http.Response, err error) {
//	    if r != nil {
//	        metrics.Counter("mono.responses", "status", strconv.Itoa(r.StatusCode)).Inc()
//	    }
//	}))
func WithResponseHook(fn func(*http.Response, error)) Option {
	return func(c *Client) {
		if fn == nil {
			return
		}
		c.onResp = fn
	}
}

// WithBaseURL перевизначає дефолтний base URL (https://api.monobank.ua).
// Корисно для тестування проти httptest.Server, записаного proxy або
// окремих хостів (corp-api.monobank.ua — використовується автоматично
// у [business.New], тут не потрібно).
//
// Якщо uri використовує не-https схему (крім localhost / 127.0.0.1) і
// клієнт сконфігурований із [WithLogger], логується Warn — токен у
// X-Token піде відкритим текстом, і це майже завжди помилка
// конфігурації.
func WithBaseURL(uri string) Option {
	return func(c *Client) {
		c.SetBaseURL(uri)
		if c.logger != nil && isInsecureBaseURL(uri) {
			c.logger.Warn("monobank: base URL uses insecure scheme — credentials will be sent in cleartext",
				slog.String("url", uri))
		}
	}
}

// isInsecureBaseURL повертає true, якщо схема не https і хост не
// є loopback. Виділено окремо, щоб не залежати від порядку опцій
// (warn спрацює тільки якщо WithLogger йде ДО WithBaseURL).
func isInsecureBaseURL(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil || u == nil {
		return false
	}
	if u.Scheme == "https" || u.Scheme == "" {
		return false
	}
	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return false
	}
	return true
}

// WithRetry вмикає автоматичний ретрай для transient-помилок (5xx і
// 429). Бекоф — експонентний із full jitter; Retry-After із відповіді
// поважається, якщо є.
//
// attempts == 0 — успадковує дефолти (4 спроби, base 500ms, max 30s).
// attempts <= 1 — явно вимикає ретрай. Непозитивні baseDelay / maxDelay
// успадковують дефолти.
//
// Ретраяться тільки transient-помилки (5xx, 429); чотирьохсотки повертаються
// як [APIError] без повтору, бо це справжні помилки клієнтського коду.
func WithRetry(attempts int, baseDelay, maxDelay time.Duration) Option {
	return func(c *Client) {
		if attempts == 0 {
			c.retry = defaultRetry
			return
		}
		c.retry.maxAttempts = attempts
		if baseDelay > 0 {
			c.retry.baseDelay = baseDelay
		} else if c.retry.baseDelay == 0 {
			c.retry.baseDelay = defaultRetry.baseDelay
		}
		if maxDelay > 0 {
			c.retry.maxDelay = maxDelay
		} else if c.retry.maxDelay == 0 {
			c.retry.maxDelay = defaultRetry.maxDelay
		}
	}
}

// WithUnsafeRetries вмикає автоматичні ретраї POST/PATCH без заголовка
// Idempotency-Key. За замовчуванням такі методи не ретраяться, бо
// 502/504 від балансера може прийти ПІСЛЯ того, як upstream уже
// обробив запит — повтор створить дублікат операції (наприклад, два
// інвойси через [acquiring.Client.CreateInvoice]).
//
// Mono приймає Idempotency-Key для всіх мутаційних endpoint-ів, де він
// має сенс (див. [business.NewIdempotencyKey], який автоматично
// проставляється у [business.Client.PreparePayment] /
// [business.Client.CreateSalaryRegistry]). Для решти POST-методів,
// якщо ти впевнений, що endpoint ідемпотентний на стороні Mono або
// готовий мирно жити з дублями — постав WithUnsafeRetries(true).
func WithUnsafeRetries(enabled bool) Option {
	return func(c *Client) {
		c.unsafeRetries = enabled
	}
}

// WithRateLimiter ставить клієнтський throttle. [RateLimiter.Wait]
// викликається ОДИН раз на логічний запит (до retry-циклу) — токен не
// витрачається повторно при ретраях. nil ігнорується.
//
// Mono має жорсткі ліміти (наприклад, /personal/client-info — 1 виклик
// на 60 с); без лімітера SDK покладається лише на серверні 429
// + [WithRetry] backoff. Локальний лімітер допомагає не вистрілити в
// 429 з самого початку.
//
//	lim := monobank.NewLimiter(time.Minute, 1)
//	cli := personal.New(token, monobank.WithRateLimiter(lim))
//
// Можна підставити будь-який *golang.org/x/time/rate.Limiter — його
// сигнатура Wait(ctx) збігається з [RateLimiter].
func WithRateLimiter(l RateLimiter) Option {
	return func(c *Client) {
		if l == nil {
			return
		}
		c.limiter = l
	}
}

// New повертає базовий [Client], зібраний із переданих опцій. Без
// [WithAuth] клієнт використовує [auth.Public] (no-op) — підходить для
// публічних bank-endpoint-ів через підпакет bank, але не для personal,
// corporate, business чи acquiring (їм потрібна реальна авторизація;
// зазвичай вони викликають New самі, додаючи свій [auth.Authorizer]).
//
//	c := monobank.New(
//	    monobank.WithHTTPClient(myHTTP),
//	    monobank.WithRetry(5, 0, 0),
//	)
func New(opts ...Option) Client {
	base, _ := url.Parse(defaultBaseURL)
	c := Client{
		http:    &http.Client{},
		auth:    auth.Public{},
		baseURL: base,
	}
	for _, opt := range opts {
		opt(&c)
	}
	return c
}
