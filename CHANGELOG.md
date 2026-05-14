# Changelog

Усі помітні зміни в `go-monobank-sdk` фіксуються в цьому файлі.

Формат — [Keep a Changelog](https://keepachangelog.com/uk/1.1.0/);
версіонування — [SemVer](https://semver.org/lang/uk/).

## [Unreleased]

## [1.2.0] — 2026-05-14

DX і security-quality life. Перші додатки, які не закривають баг із
v1.1.x-аудиту, а навмисне піднімають планку SDK ближче до того, що
очікують від professional libraries: ідентифікація через User-Agent,
автоматична редакція токенів у логах, явний Close для звільнення
фонових ресурсів, заборона insecure base URL за замовчуванням.

Source-incompatible: `WithBaseURL` із http://-URL на не-loopback-хост
тепер повертає `ErrInsecureBaseURL` замість тихого warn. Старе
поводження повертається опцією `WithInsecureBaseURL(true)`.

### Added

- **`User-Agent`**: SDK шле `go-monobank-sdk/vX.Y.Z (linux; go1.26.2)`
  на кожному запиті. Версія дістається з `runtime/debug.ReadBuildInfo`,
  тож автоматично відповідає реально лінкованому модулю. Mono матиме
  змогу розрізняти твій сервіс у support-кейсах і fraud-моніторингу.
- **`WithUserAgent(string)`**: перевизначи дефолтний UA, наприклад
  щоб додати ім'я свого сервісу попереду. Експортовано `UserAgent()`
  на випадок, якщо хочеш зберегти SDK-частину.
- **`Client.Close() error` + у всіх підпакетах** (`personal`,
  `corporate`, `business`, `acquiring`, `bank`): зупиняє sweeper
  `KeyedLimiter`-а та інші майбутні фонові ресурси. Реалізує
  `io.Closer`. Безпечно викликати на клієнті без лімітера.
- **`WithInsecureBaseURL(bool)`**: opt-in bypass для нової
  захисної перевірки. Корисно для MITM-проксі дебагу (mitmproxy,
  burp) або staging-середовищ за VPN-ом.
- **`ErrInsecureBaseURL` sentinel**: для `errors.Is`-перевірок.
- **Token redaction через `slog.LogValuer`**: `auth.Personal`,
  `business.TokenAuth`, `acquiring.TokenAuth`, `auth.CorpAuthMaker`,
  `installment.Client` тепер рендеряться як `***`, коли ти
  передаєш їх у slog-виклик. До цього сирий токен/секрет
  потрапляв у логи в людиночитаному вигляді.

### Changed (breaking)

- `WithBaseURL("http://...")` для не-loopback-хоста повертає
  `ErrInsecureBaseURL` з першого `Client.Do`. До v1.2 була лише
  warn-логування — деплой із staging-конфігом у прод проходив тихо.
  Якщо ти свідомо хочеш http (MITM-debug, internal staging) —
  оберни у `WithInsecureBaseURL(true)`.

### Migration з v1.1.x

```go
// Якщо ти раніше робив http://-staging для тестів — додай явну опцію:
cli := personal.New(token,
    monobank.WithInsecureBaseURL(true), // має бути ДО WithBaseURL
    monobank.WithBaseURL("http://staging.example.com"),
)

// Рекомендований патерн (новий):
cli := personal.New(token, monobank.WithRateLimiter(klim))
defer cli.Close() // звільнить sweeper KeyedLimiter-а
```

## [1.1.3] — 2026-05-14

Production-readiness polish. Жодних змін поведінки коду — лише
посилення тестів, CI та документації. v1.1.2 продовжує працювати; цей
реліз обовʼязковий лише якщо ти хочеш повну Migration-секцію у
CHANGELOG-у або стабільніше CI-покриття race-prone-шляхів.

### Added

- CI: новий job `race-stress (10x)` — `go test -count=10 -race`
  на одній Go-версії. Спіймав би singleflight-рейс, який раніше
  виявився тільки під `-count=20` локально.
- CHANGELOG: повний розділ «Migration з v1.0.0» під v1.1.0 із
  before/after-прикладами для Currency / Status / Permission /
  KeyedLimiter / POST-retry змін.
- `RELEASING.md`: посилений чек-лист — README-sync, retract-семантика,
  обовʼязкове очікування CI перед тегом.
- Root `doc.go`: явно перераховує `WithUnsafeRetries`,
  `RateLimiter` / `KeyedLimiter`, `WithLimiterKey`, sentinel-помилки.
- Doc-warning у `installment.WithBaseURL` / `jar.WithAPIBaseURL` /
  `WithSendBaseURL`: тільки https у production (ці пакети не мають
  slog-логера, тож runtime-warn неможливий).
- Тести: `TestMajor_precisionForCommonAmounts` /
  `TestMajor_extremeValues` (money) і
  `TestKeyedLimiter_EvictionUnderConcurrentLoad` — фіксують поведінку,
  яку дотепер ніщо явно не перевіряло.

## [1.1.2] — 2026-05-14

Documentation hot-fix v1.1.1: README мав застарілі приклади
(`Ccy: 980`, 2-аргументний `NewKeyedLimiter`), які буквально не
компілювалися на v1.1.x, а нові фічі (`WithUnsafeRetries`, sentinel-
помилки, `MaxBodyBytes`, currency-aware Money, iterator-и) у
документацію не потрапили. v1.1.1 retract-нутий разом із v1.1.0.

### Fixed

- README.md / README.en.md: оновлено acquiring-приклад (`Currency:`
  замість видаленого `Ccy:`), `KeyedLimiter`-приклад (3-аргументна
  сигнатура + `defer Stop()`), business endpoint count (23, не 17).
- Усі `// Регресія X:` префікси в тестах прибрані — це були посилання
  на сесійні tracking-ID, які стариіють із часом.

### Added

- README-секції: `WithUnsafeRetries`, sentinel-помилки
  (`monobank.ErrUnauthorized` etc), `Options.MaxBodyBytes`,
  `singleflight`-throttle на refresh ключа, currency-aware
  `Money.Major` / `String`, `currency.Code.Decimals()`, повний
  перелік `iter.Seq2`-пагінаторів.
- Нові godoc Example-функції: `ExampleErrUnauthorized`,
  `ExampleWithUnsafeRetries`.

### Retracted

- v1.1.1 — документація розходилася з кодом.
- v1.1.0 — не збиралася на Go 1.23/1.24 (вже retract-нутий у v1.1.1).

## [1.1.1] — 2026-05-14

Hot-fix v1.1.0: модуль не збирався на Go 1.23/1.24 (через x/sync@v0.20
що вимагає Go 1.25). v1.1.0 retract-нутий у go.mod.

### Fixed

- Downgrade `golang.org/x/sync` v0.20.0 → v0.10.0 — підтримка Go 1.23
  у відповідності з директивою `go` модуля.
- Race у `webhook.Handler.refreshCoalesced` (виявлений через
  `go test -count=20 -race`): між моментом, коли singleflight
  завершував першу refresh-функцію, і моментом оновлення
  `lastRefreshAt`, могла прослизнути друга хвиля горутин і викликати
  ще один `/bank/sync`. Тепер double-check `lastRefreshAt` всередині
  singleflight-callback гарантує ≤2 виклики ServerKey() при
  N конкурентних webhook-ах з невідомим X-Key-Id.
- `gofmt` після `sed`-renames у v1.1.0 (CI блокувався).

### Added

- Регресійні тести для всіх v1.1.0 fixes: H1 (MaxBytesReader → 413),
  H3 (singleflight + 50-goroutine stress), H5 (VerifyCallback
  fast-path), M2 (off-curve point + wrong length/prefix), L4 (P-384
  rejection), M7 (PathEscape з spec-символами), M8 (JPY 0 decimals),
  M9 (204 No Content + Content-Length: 0), L7 (errors.Is sentinels +
  errors.As chain), L2 (insecure baseURL helper), C2 (shouldRetry
  matrix), L8 (StatementAll / SubscriptionListAll / Payments
  iterators), L6b (typed Permission).

### Retracted

- v1.1.0 — не збиралася на Go 1.23/1.24.

## [1.1.0] — 2026-05-14

Реліз із виправленнями знайденими у повному code review v1.0.0:
runtime-баги retry/limiter-стеку, DoS-векторів у webhook/installment/jar
+ публічна type-cleanup. Часом source-incompatible — оновлення з v1.0.0
вимагає реімпорту, але v1.0.0 ще ніхто не використовував.

### Migration з v1.0.0

Рекомендую переходити одразу на v1.1.2 (v1.1.0/v1.1.1 retracted).
Source-incompatible зміни:

**Currency**: усі int-поля `Ccy` (acquiring) і `CurrencyCode` (bank)
перейменовані на `Currency` з типом `currency.Code`:

```go
// До v1.0.0:
req := acquiring.CreateInvoiceRequest{Amount: 10000, Ccy: 980}
acc.CurrencyCode == 980

// Починаючи з v1.1.0:
req := acquiring.CreateInvoiceRequest{Amount: 10000, Currency: 980}
// або типобезпечно: Currency: currency.UAH
acc.Currency == currency.UAH
```

В `business.StatementItem` поле `CurrencyCode string` (alpha-3
рядок) перейменоване на `CurrencyAlpha3 string` — щоб різниця між
int-Currency у решті SDK і string-Currency у business кидалася в
очі.

**Status enums**: сирі `string`-статуси в acquiring перейменовані
на типізовані `acquiring.ProcessingStatus`:

```go
// До:
if resp.Status == "success" { ... }
// Після:
if resp.Status == acquiring.StatusSuccess { ... }
```

Aналогічно `WalletData.Status` → `acquiring.WalletStatus` із
константами `WalletNew/Created/Failed`.

**Permission**: `auth.PermSt/PermPI/PermFOP` тепер типу
`auth.Permission`. Сигнатура `corporate.Client.Auth` і
`CorpAuthMaker.NewPermissions` приймає `...auth.Permission`. З
константами все працює без змін; зламається лише там, де ти
передавав сирий рядок:

```go
cli.Auth(ctx, url, "s", "p")        // зламається
cli.Auth(ctx, url, auth.PermSt, auth.PermPI) // ок
```

**KeyedLimiter**: тепер 3-аргументний — додано `idleTTL` для
eviction. Завжди викликай `Stop()` для long-running сервісів:

```go
// До:
klim := monobank.NewKeyedLimiter(time.Minute, 1)

// Після:
klim := monobank.NewKeyedLimiter(time.Minute, 1, 10*time.Minute)
defer klim.Stop()
// Або без eviction (як раніше): NewKeyedLimiter(every, burst, 0)
```

**Retry POST**: до v1.1.0 будь-який POST/PATCH ретраївся на 5xx —
це могло створити дублікати на 502/504 від балансера. Тепер вони
ретраяться ТІЛЬКИ за наявності заголовка `Idempotency-Key`. Якщо
твій код покладався на старе поводження, увімкни
`monobank.WithUnsafeRetries(true)`. `business.PreparePayment` і
`CreateSalaryRegistry` уже додають `Idempotency-Key` автоматично.

### Fixed (critical)

- **Retry POST з body тепер працює.** До цього `client.Do` не
  скидав `req.Body` між спробами; будь-який `acquiring.CreateInvoice`,
  `personal.SetWebHook` чи `business.PreparePayment`, що тригернув
  ретрай на 5xx/429, надсилав другий запит із порожнім тілом і
  отримував "400 Bad Request". Тепер тіло один раз буферується через
  `req.GetBody`. Регресійний тест: `TestClient_Do_retriesPreserveBody`.
- **POST без `Idempotency-Key` більше не ретраїться автоматично.**
  Раніше 502/504 від балансера після того, як upstream уже обробив
  запит, створював дублікат операції. Тепер POST/PATCH ретраяться
  тільки за наявності `Idempotency-Key`. Поведінку відновлюй через
  нову опцію `monobank.WithUnsafeRetries(true)`.
- **`KeyedLimiter` більше не тече памʼяттю.** Дефолтний eviction
  через TTL+sweeper. Сигнатура `NewKeyedLimiter` тепер приймає
  третій параметр `idleTTL`; передавай 0 для коротких CLI, розумний
  TTL (~10× за `every`) для long-running сервісів. Виклик
  `Stop()` зупиняє sweeper-горутину.
- **`backoff` більше не панікує при високих attempts.** До цього
  `WithRetry(40, 500ms, 30s)` падав із `rand.Int63n: invalid argument`
  через int64-overflow на 35-й спробі.

### Fixed (high — DoS hardening)

- **Webhook handler обмежує body 1 MiB за замовчуванням** (
  `webhook.Options.MaxBodyBytes`). Без цього атакуючий міг
  OOM-нути сервіс, надіславши гігабайтне тіло.
- **Webhook ServerKey refresh** тепер через
  `golang.org/x/sync/singleflight` + 30 с дросель. Без цього
  атакуючий, що знає публічний URL вебхуку, амплифікував DoS на
  `/bank/sync` 1:1, вичерпуючи ліміт Mono і провалюючи реальні
  ротації ключа.
- **`installment` і `jar` body обмежений** через `io.LimitReader`
  (50 MiB для PDF, 1 MiB для JSON).
- **`installment.VerifyCallback` має fast-path** на довжину підпису
  до обчислення HMAC — без цього атакуючий з гігабайтним body +
  порожнім signature зʼїдав CPU.
- **Webhook handler nil-key захист.** `Handler{}` без `NewHandler`
  тепер віддає 503 замість NPE.

### Fixed (medium / low)

- `bank.serverkey`: валідація точки на кривій через
  `secp256k1.ParsePubKey` (захист від MITM-injected off-curve key).
- `acquiring.ParsePubKey`: явне відхилення не-P-256 кривих.
- `auth`: deprecated `elliptic.Marshal` замінено на нативний
  `secp256k1.SerializeUncompressed`.
- `personal.Transactions`/`corporate.Transactions`: `url.PathEscape`
  для accountID — як уже є в `business`.
- `parseRetryAfter` повертає `-1` для відсутнього header-а — щоб
  відрізняти від явного `0` (миттєвий повтор).
- `client.Do` коректно обробляє 204 No Content і `Content-Length: 0`
  — без `io.EOF` як помилки декоду.
- `math/rand` → `math/rand/v2` у backoff (без глобального мутекса).
- `WithBaseURL` логує warn при не-https + не-localhost (token у
  cleartext — майже завжди помилка конфігурації).
- README виправлено: dedup-ключ — `event.Data.Transaction.ID` (а не
  неіснуюче `Response.UID`); business має 23 endpoint-и (не 17).

### Added

- `monobank.RateLimiter` sentinel-помилки: `ErrUnauthorized`,
  `ErrForbidden`, `ErrNotFound`, `ErrTooManyRequests`. `APIError.Is`
  робить `errors.Is(err, monobank.ErrUnauthorized)` валідним.
- `monobank.WithUnsafeRetries(bool)` — opt-in повернення старої
  поведінки ретраю POST без `Idempotency-Key`.
- `currency.Code.Decimals()` метод; `Money.Major`/`Money.String`
  тепер currency-aware (JPY=0, інші 2 знаки).
- `business.StatementAll`, `acquiring.SubscriptionListAll`,
  `acquiring.SubscriptionPaymentsAll` — `iter.Seq2`-пагінатори.
- `golang.org/x/sync` — нова залежність (singleflight у webhook).

### Changed (BREAKING — source-incompatible)

- `bank.Account.CurrencyCode int` → `Currency currency.Code`. Те саме
  для `bank.Jar`, `bank.Transaction`. Wire-формат (`json:"currencyCode"`)
  не змінюється.
- `acquiring.*` усі поля `Ccy int` → `Currency currency.Code` (
  `CreateInvoiceRequest`, `InvoiceStatusResponse`, `PaymentDirectResponse`,
  `WalletPaymentRequest`, `CancelOp`, `QRDetails`, `StatementInvoice`,
  `StatementRefund`, `SubscriptionCreateRequest`,
  `SubscriptionStatusResponse`, `SubscriptionPayment`, `SyncPaymentRequest`).
- `business.StatementItem.CurrencyCode string` → `CurrencyAlpha3 string`
  (експліцитно, що це alpha-3, на відміну від інших Currency-полів).
- `acquiring`: `WalletData.Status string` → `WalletStatus`;
  `CancelOp.Status` / `CancelResponse.Status` /
  `FinalizeResponse.Status` / `PaymentDirectResponse.Status` /
  `StatementInvoice.Status` → `ProcessingStatus` (типізовані enum-и).
- `auth.Permission` тепер typed `string` замість сирого; `PermSt`,
  `PermPI`, `PermFOP` — `Permission`-консти. Сигнатура
  `corporate.Client.Auth(...permissions ...auth.Permission)`,
  `auth.CorpAuthMakerAPI.NewPermissions(...auth.Permission)`.
- `monobank.NewKeyedLimiter(every, burst)` →
  `NewKeyedLimiter(every, burst, idleTTL)`. Передавай `0` для
  колишньої поведінки без eviction.



Перший стабільний реліз. Публічний API зафіксовано: подальші мінорні
версії додають функціональність без ламань; breaking-зміни вимагатимуть
`v2.0.0` (з `/v2` у шляху імпорту).

Об'єднує всі зміни з невипущеного `v0.1.1` і подальшої роботи поверх
`v0.1.0`.

### Added

#### Throttling та обробка помилок
- `monobank.RateLimiter` — інтерфейс клієнтського throttle із сигнатурою
  `Wait(ctx) error`, сумісною з `*golang.org/x/time/rate.Limiter`.
- `monobank.NewLimiter(every, burst)` — вбудований token-bucket без
  додаткових залежностей. Один токен витрачається на логічний `Do`
  (а не на кожну спробу retry).
- `monobank.WithRateLimiter(RateLimiter)` — опція клієнта.
- `monobank.KeyedLimiter` — per-key token bucket для endpoint-ів із
  per-account/per-resource лімітами (наприклад,
  `/personal/statement/{account}/…`). Реалізує `RateLimiter`; ключ
  береться з контексту через `monobank.WithLimiterKey`.
- `monobank.WithLimiterKey(ctx, key)` — context helper для прокидання
  ключа в `KeyedLimiter`.
- `APIError.ErrorDescription` — розпарсене значення поля `errorDescription`
  з JSON-тіла відповідей Mono (personal/corporate/business/acquiring).
  Сирі байти лишаються в `APIError.Body`.

#### Тестування і якість
- Fuzz-тести для парсерів і верифікаторів підпису:
  `parseErrorDescription`, `parseRetryAfter`, `webhook.Parse`,
  `webhook.Verify`, `money.Money.UnmarshalJSON`, `acquiring.ParsePubKey`,
  `acquiring.ParseWebhook`. Запуск — `go test -fuzz=Fuzz... -fuzztime=30s`.
- Бенчмарки гарячих шляхів: `Limiter.Wait`/`KeyedLimiter.Wait`,
  `parseErrorDescription`, `bank.Transaction.UnmarshalJSON`,
  `money.Money.{Add,Scale,String}`, `webhook.{Verify,Parse}`. Запуск —
  `go test -bench=. -benchmem ./...`.
- `bank/integration_test.go` (`//go:build integration`) — smoke-тести
  `Rates` і `ServerKey` проти живого `api.monobank.ua`.
- Godoc `Example`-функції для root (`NewLimiter`, `NewKeyedLimiter`,
  `APIError`), `bank` (`Rates`, `Rates.Convert`, `ServerKey`), `jar`
  (`ByLongID`, `ByShortID`), `installment` (`New`, `VerifyCallback`),
  `money` (`New`, `Add`, `MarshalJSON`) — рендеряться інлайн на
  pkg.go.dev поруч із сигнатурами.

#### CI / security / dev-tooling
- `.github/workflows/codeql.yaml` — per-push і weekly CodeQL
  (security-and-quality query suite); знахідки в Security tab.
- govulncheck-job у `ci.yaml` — сканує root і `otelmonobank` на CVE
  у stdlib і залежностях.
- `.github/workflows/integration.yaml` — щотижневий cron + ручний
  `workflow_dispatch` для інтеграційних тестів (поза основним PR-pipeline).
- `.github/workflows/release.yaml` — на push тегу `v*` створює GitHub
  Release з body, витягнутим із відповідної секції `CHANGELOG.md`.
- `.github/CODEOWNERS`, `.github/dependabot.yml`,
  `.github/ISSUE_TEMPLATE/` (bug + feature + config),
  `.github/PULL_REQUEST_TEMPLATE.md`.
- `Makefile` із загальними dev-таргетами: `test`/`test-race`/`test-all`/
  `cover`/`cover-html`/`bench`/`fuzz`/`fuzz-all`/`lint`/`fmt`/`vet`/`tidy`/
  `integration`/`ci`. `make help` (default) — список усіх.
- `go.work` — workspace для одночасної розробки кореня + `otelmonobank`.
- `CONTRIBUTING.md`, `RELEASING.md`, `SECURITY.md`.
- Англомовний `README.en.md` із language switcher на верху обох README.
- `.editorconfig` і `.gitattributes` (eol=lf, Linguist-виключення).

### Changed

- `APIError.Error()` тепер показує чисте `ErrorDescription` замість сирого
  JSON, коли воно доступне. Статус-код, метод, URL — без змін.
- Усі GitHub Actions запіновано на SHA (а не теги): захист від
  компрометації переписаного тегу. Версія коментується поряд (`# v6.0.2`),
  тож dependabot автоматично оновлює і SHA, і коментар.
- CI-workflow-и тепер мають `concurrency` блок із
  `cancel-in-progress: true` — на нових пушах в PR старіші прогони
  кенселяться, економить CI-хвилини.
- `.codecov.yml` — patch threshold піднятий з 75% до 80%; додано
  виключення `**/*_test.go` і `monobanktest/**` з обчислення покриття.
- `flake.nix` — прибрано `export GOFLAGS="-mod=mod"`, що конфліктував
  із workspace-режимом (`go.work`).

### Fixed

- Деflake `auth.TestCorpSuite/Test_sign`: тест чекав фіксовану довжину
  base64 (96 символів), але ASN.1 DER підпис ECDSA — змінної довжини
  (8-72 байт), тож ~1% запусків падало. Тепер декодуємо і перевіряємо
  структуру через `asn1.Unmarshal`.

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

[Unreleased]: https://github.com/OlexiyOdarchuk/go-monobank-sdk/compare/v1.2.0...HEAD
[1.2.0]: https://github.com/OlexiyOdarchuk/go-monobank-sdk/compare/v1.1.3...v1.2.0
[1.1.3]: https://github.com/OlexiyOdarchuk/go-monobank-sdk/compare/v1.1.2...v1.1.3
[1.1.2]: https://github.com/OlexiyOdarchuk/go-monobank-sdk/compare/v1.1.1...v1.1.2
[1.1.1]: https://github.com/OlexiyOdarchuk/go-monobank-sdk/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/OlexiyOdarchuk/go-monobank-sdk/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/OlexiyOdarchuk/go-monobank-sdk/compare/v0.1.0...v1.0.0
[0.1.0]: https://github.com/OlexiyOdarchuk/go-monobank-sdk/releases/tag/v0.1.0
