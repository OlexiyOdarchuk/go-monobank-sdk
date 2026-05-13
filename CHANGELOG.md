# Changelog

Усі помітні зміни в `go-monobank-sdk` фіксуються в цьому файлі.

Формат — [Keep a Changelog](https://keepachangelog.com/uk/1.1.0/);
версіонування — [SemVer](https://semver.org/lang/uk/).

## [Unreleased]

### Added

- `monobank.KeyedLimiter` — per-key token bucket для endpoint-ів із
  per-account/per-resource лімітами (наприклад,
  `/personal/statement/{account}/…`). Реалізує `RateLimiter`; ключ
  береться з контексту через `monobank.WithLimiterKey`.
- `monobank.WithLimiterKey(ctx, key)` — context helper для прокидання
  ключа в `KeyedLimiter`.
- `bank/integration_test.go` (`//go:build integration`) — smoke-тести
  `Rates` і `ServerKey` проти живого `api.monobank.ua`.
- `.github/workflows/integration.yaml` — щотижневий cron + ручний
  `workflow_dispatch` для інтеграційних тестів (поза основним PR-pipeline).
- Godoc `Example`-функції для root (`NewLimiter`, `NewKeyedLimiter`,
  `APIError`), `bank` (`Rates`, `Rates.Convert`, `ServerKey`), `jar`
  (`ByLongID`, `ByShortID`), `installment` (`New`, `VerifyCallback`),
  `money` (`New`, `Add`, `MarshalJSON`) — рендеряться інлайн на
  pkg.go.dev поруч із сигнатурами.
- Fuzz-тести для парсерів і верифікаторів підпису:
  `parseErrorDescription`, `parseRetryAfter`, `webhook.Parse`,
  `webhook.Verify`, `money.Money.UnmarshalJSON`, `acquiring.ParsePubKey`,
  `acquiring.ParseWebhook`. Запуск — `go test -fuzz=Fuzz... -fuzztime=30s`.
- Бенчмарки гарячих шляхів: `Limiter.Wait`/`KeyedLimiter.Wait`,
  `parseErrorDescription`, `bank.Transaction.UnmarshalJSON`,
  `money.Money.{Add,Scale,String}`, `webhook.{Verify,Parse}`. Запуск —
  `go test -bench=. -benchmem ./...`.
- `CONTRIBUTING.md` — гайд для зовнішніх контриб'юторів.
- `.github/CODEOWNERS` — авто-reviewer на всі PR.
- `.github/dependabot.yml` — щотижневі апдейти Go-модулів і GitHub Actions.
- `.github/ISSUE_TEMPLATE/` — шаблони для bug report / feature request
  + конфіг із посиланням на приватний звіт безпеки.
- `.github/PULL_REQUEST_TEMPLATE.md` — чек-лист для PR.

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
