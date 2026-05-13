# Changelog

Усі помітні зміни в `go-monobank-sdk` фіксуються в цьому файлі.

Формат — [Keep a Changelog](https://keepachangelog.com/uk/1.1.0/);
версіонування — [SemVer](https://semver.org/lang/uk/).

## [Unreleased]

## [0.1.1] — 2026-05-14

### Added

- `monobank.RateLimiter` — інтерфейс клієнтського throttle із сигнатурою
  `Wait(ctx) error`, сумісною з `*golang.org/x/time/rate.Limiter`.
- `monobank.NewLimiter(every, burst)` — вбудований token-bucket без
  додаткових залежностей. Один токен витрачається на логічний `Do`
  (а не на кожну спробу retry).
- `monobank.WithRateLimiter(RateLimiter)` — опція клієнта.
- `APIError.ErrorDescription` — розпарсене значення поля `errorDescription`
  з JSON-тіла відповідей Mono (personal/corporate/business/acquiring).
  Сирі байти лишаються в `APIError.Body`.

### Changed

- `APIError.Error()` тепер показує чисте `ErrorDescription` замість сирого
  JSON, коли воно доступне. Статус-код, метод, URL — без змін.

## [0.1.0] — 2026-05-13

Перший публічний реліз.

### Added

- Кореневий `monobank.Client` із підтримкою `context.Context`, кастомного
  `*http.Client` / `HTTPDoer`, retry з експонентним backoff і повагою до
  `Retry-After`, slog-логування, request/response hooks.
- Підпакети: `personal`, `corporate` (з monoКЕП), `business` (corp-api),
  `acquiring` (інвойси, QR, wallet, subscriptions, monopay-keys, split,
  T2P), `installment` (Покупка частинами), `jar` (публічні банки),
  `bank` (публічні endpoint-и), `auth` (Personal/Corp/Public + ECDSA),
  `currency` (ISO 4217), `mcc` (ISO 18245), `money` (типізовані суми).
- `webhook` — ECDSA-верифікація, парсинг, готовий `http.Handler` з
  дедуплікацією і автооновленням `ServerKey`.
- `otelmonobank` — OpenTelemetry-інструментація як окремий submodule.
- `monobanktest` — мок-сервер на `httptest.Server` із fluent-builder-ами.
- Пагінатори через `iter.Seq2` (Go 1.23+).

[Unreleased]: https://github.com/OlexiyOdarchuk/go-monobank-sdk/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/OlexiyOdarchuk/go-monobank-sdk/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/OlexiyOdarchuk/go-monobank-sdk/releases/tag/v0.1.0
