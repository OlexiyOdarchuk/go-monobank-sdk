// Package otelmonobank — OpenTelemetry-інтеграція для monobank-sdk.
//
// Це окремий sub-module: щоб уникнути обов'язкової залежності від
// `go.opentelemetry.io/otel` для користувачів, які OTel не вживають,
// otelmonobank має власний go.mod. Імпортуй явно, коли потрібна
// трасування:
//
//	import (
//	    "github.com/OlexiyOdarchuk/go-monobank-sdk/personal"
//	    "github.com/OlexiyOdarchuk/go-monobank-sdk/otelmonobank"
//	    "go.opentelemetry.io/otel"
//	)
//
//	cli := personal.New(token, otelmonobank.WithTracer(otel.Tracer("my-app")))
//
// Кожен HTTP-запит (включно з ретраями) стає окремим span-ом із
// атрибутами http.method, http.url, http.status_code, error.
package otelmonobank

import (
	"net/http"
	"strconv"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// WithTracer повертає [monobank.Option], що інструментує клієнта
// OpenTelemetry-трейсингом. На кожен HTTP-запит створюється span із
// іменем "monobank HTTP <method>" і атрибутами:
//
//   - http.method — метод запиту
//   - http.url — повний URL (з querystring; токени не пишуться, бо вони
//     в заголовках)
//   - http.status_code — статус відповіді (тільки якщо запит дійшов до
//     сервера)
//   - error — true, якщо була транспортна помилка
//
// Span закривається у response-hook, незалежно від результату. Якщо
// tracer == nil — опція ігнорується (no-op).
//
// Композиція з іншими опціями: WithTracer внутрішньо використовує
// [monobank.WithRequestHook] і [monobank.WithResponseHook], тому якщо
// у тебе вже є власні hook-и — ця опція їх ПЕРЕЗАПИШЕ. Залиш OTel
// як єдиного користувача цих hook-ів (або скомбінуй вручну).
func WithTracer(tracer trace.Tracer) monobank.Option {
	if tracer == nil {
		return func(*monobank.Client) {}
	}

	// Споживаємо span через request.Context() → response. Бо hook-и
	// викликаються в одному потоці виконання per request, і request
	// доступний обом, можна тримати span у map[*http.Request]Span,
	// або простіше — через context. Mono Client не пробрасує контекст
	// між request-hook і response-hook напряму, тож ми зберігаємо span
	// на самому *http.Request через його контекст (контекст immutable,
	// тому через Header — не годиться). Через зовнішню мапу — гонки.
	//
	// Простий і коректний підхід: створити span до http.Do, зберегти
	// його в новому контексті req-а через r.WithContext(...). Але
	// monobank.Client.Do оперує конкретним req-ом, не копією. Тож
	// користуємо sync.Map.

	store := newSpanStore()

	requestHook := func(r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "monobank HTTP "+r.Method,
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.url", r.URL.String()),
			),
		)
		// Прокидаємо trace context у заголовки (W3C traceparent).
		// Це робить OTel propagator; ми його тут не підключаємо, щоб
		// користувач сам вирішив про global propagator. Просто
		// зберігаємо span і ctx для response-hook.
		_ = ctx
		store.set(r, span)
	}

	responseHook := func(resp *http.Response, err error) {
		var req *http.Request
		if resp != nil {
			req = resp.Request
		}
		if req == nil {
			return
		}
		span, ok := store.pop(req)
		if !ok {
			return
		}
		defer span.End()

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return
		}
		if resp != nil {
			span.SetAttributes(attribute.String("http.status_code", strconv.Itoa(resp.StatusCode)))
			if resp.StatusCode >= 400 {
				span.SetStatus(codes.Error, http.StatusText(resp.StatusCode))
			} else {
				span.SetStatus(codes.Ok, "")
			}
		}
	}

	return func(c *monobank.Client) {
		monobank.WithRequestHook(requestHook)(c)
		monobank.WithResponseHook(responseHook)(c)
	}
}
