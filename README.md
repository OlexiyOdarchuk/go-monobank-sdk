# monobank-sdk

🇺🇦 Українська · [🇬🇧 English](README.en.md)

[![Go Reference](https://pkg.go.dev/badge/github.com/OlexiyOdarchuk/go-monobank-sdk.svg)](https://pkg.go.dev/github.com/OlexiyOdarchuk/go-monobank-sdk)
[![CI](https://github.com/OlexiyOdarchuk/go-monobank-sdk/actions/workflows/ci.yaml/badge.svg)](https://github.com/OlexiyOdarchuk/go-monobank-sdk/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/OlexiyOdarchuk/go-monobank-sdk/branch/main/graph/badge.svg)](https://codecov.io/gh/OlexiyOdarchuk/go-monobank-sdk)
[![Go Report Card](https://goreportcard.com/badge/github.com/OlexiyOdarchuk/go-monobank-sdk)](https://goreportcard.com/report/github.com/OlexiyOdarchuk/go-monobank-sdk)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Повноцінний Go SDK для всіх публічних API monobank: **personal**, **corporate**
(з monoКЕП), **business** (corp-api для юр. осіб), **acquiring** (еквайринг із
subscriptions / monopay-keys / split-receivers / T2P), **installment** (Покупка
частинами) та **jar** (публічні банки). Поза самими клієнтами SDK містить
серверні helper-и, без яких безпечно приймати webhook-и доводилось би писати
самотужки — верифікація ECDSA-підпису, готовий `http.Handler`, дедуплікація
ретраїв, HMAC-SHA256 для ПЧ-callback, типізовані enum-и MCC та валют, утиліти
для тестування.

> Стартував як форк [vtopc/go-monobank](https://github.com/vtopc/go-monobank);
> розрісся, бо upstream свідомо лишається тонкою обгорткою лише над Open API.
> Тут ви знайдете все, чого там нема.

## Встановлення

```bash
go get github.com/OlexiyOdarchuk/go-monobank-sdk
```

Потрібен Go 1.25+ (пагінатори використовують `iter.Seq2`; весь воркспейс
пінить 1.25 через залежність `otelmonobank` від OpenTelemetry).

## Покриття API

Покрито всі п'ять публічних API monobank плюс два community-документовані
ендпоінти банок (jars). Внутрішній mobile-API публічної специфікації не
має — поза скоупом.

| API | Специфікація | Підпакет | Endpoints |
|---|---|---|---|
| Open API — personal (X-Token) | [api.monobank.ua/docs](https://api.monobank.ua/docs/) | `personal` | 4 |
| Open API — corporate (ECDSA) + monoКЕП | [api-docs/providers](https://monobank.ua/api-docs/providers) | `corporate` | 11 |
| corp-api для юр. осіб | [corp-api.monobank.ua](https://corp-api.monobank.ua/) | `business` | 23 |
| Acquiring (`/api/merchant/*`) | [api-docs/acquiring](https://monobank.ua/api-docs/acquiring) | `acquiring` | 31 |
| Покупка частинами (HMAC-SHA256) | [api-docs/chast](https://monobank.ua/api-docs/chast) | `installment` | 14 |
| JAR / банки (community) | [community docs](https://github.com/andrew-demb/monobank-api-community-docs) | `jar` | 2 |

## Структура пакетів

```
monobank-sdk/
├── (root)          базовий HTTP-транспорт: Client, Option, retry, hooks
├── auth/           Authorizer + Personal/Corporate/Public + monoКЕП хелпери
├── bank/           публічні endpoint-и (Rates, ServerKey) + спільні типи
│                   (ClientInfo, Account, Jar, Transaction) + Rates.Convert
├── personal/       Personal Open API (X-Token)
├── corporate/      Corporate Open API (ECDSA) + monoКЕП
├── business/       corp-api.monobank.ua — 23 endpoint-и
├── acquiring/      /api/merchant/* — 31 endpoint (інвойси, QR, wallet,
│                   subscriptions, monopay-keys, split, T2P) +
│                   ECDSA-верифікація webhook
├── installment/    «Покупка частинами» (u2.monobank.com.ua) — 14 endpoint-ів,
│                   HMAC-SHA256 підпис тіла, VerifyCallback
├── jar/            публічні банки: /bank/jar/{id} + send.monobank.ua/api/handler
├── webhook/        Verify, Parse, http.Handler, Deduper для personal webhook
├── monobanktest/   фейковий monobank-сервер + білдери для тестування
├── money/          типізована грошова сума з валютою + арифметика
├── currency/       ISO 4217 числовий код + alpha-3 ім'я
├── mcc/            ISO 18245 enum + Category (Groceries, Fuel, Restaurants…)
└── otelmonobank/   (окремий sub-module) OpenTelemetry-інтеграція
```

Кожен підпакет імпортується незалежно — беріть рівно те, що потрібно.

## Швидкий старт: Personal API

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "time"

    "github.com/OlexiyOdarchuk/go-monobank-sdk/personal"
)

func main() {
    cli := personal.New(os.Getenv("MONO_TOKEN"))
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    info, err := cli.ClientInfo(ctx)
    if err != nil {
        log.Fatal(err)
    }
    for _, a := range info.Accounts {
        fmt.Println(a.IBAN, a.Balance)
    }

    // Виписка за період > 31 дня автоматично розбивається на сторінки.
    to := time.Now()
    from := to.Add(-90 * 24 * time.Hour)
    for tx, err := range cli.TransactionsRangeIter(ctx, info.Accounts[0].AccountID, from, to) {
        if err != nil {
            log.Fatal(err)
        }
        fmt.Println(tx.Time, tx.Description, tx.Amount, tx.MCCCode().Category())
    }
}
```

Токен — на <https://api.monobank.ua/>.

## Швидкий старт: Webhook (приймати події)

```go
package main

import (
    "context"
    "log"
    "net/http"

    "github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
    "github.com/OlexiyOdarchuk/go-monobank-sdk/webhook"
)

func main() {
    keys := bank.New() // KeyProvider — тягне публічний ключ /bank/sync і кешує

    h, err := webhook.NewHandler(context.Background(), webhook.Options{
        Keys:  keys,
        Dedup: webhook.NewMemoryDeduper(1024),
        OnEvent: func(_ context.Context, e *webhook.Response) error {
            t := e.Data.Transaction
            log.Printf("%s · %s · %s",
                e.Data.AccountID, t.Amount, t.MCCCode().Category())
            return nil
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    http.Handle("/webhook", h)
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

Підписатись на URL — один раз: `personal.New(token).SetWebHook(ctx, "https://you/webhook")`.

`webhook.Handler` сам:
- завантажує ключ через `Keys.ServerKey` на старті й автоматично рефрешить
  при ротації (`X-Key-Id` у заголовку перестав збігатися),
- перевіряє ECDSA-підпис у `X-Sign`,
- парсить payload у типізований `Response`,
- консультується з `Deduper`, щоб Mono-ретраї не виконали ваш callback двічі,
- повертає 200/400/500 відповідно до результату.

## Швидкий старт: Corporate API (ECDSA)

```go
package main

import (
    "context"
    "os"

    "github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
    "github.com/OlexiyOdarchuk/go-monobank-sdk/corporate"
)

func main() {
    privPEM, _ := os.ReadFile("mono-corp.key")
    maker, _ := auth.NewCorpAuthMaker(privPEM)
    cli, _ := corporate.New(maker)

    ctx := context.Background()
    tok, _ := cli.Auth(ctx, "https://yourapp/cb", auth.PermSt, auth.PermPI)
    // → редіректнути користувача на tok.AcceptURL,
    //    потім поллити cli.CheckAuth(ctx, tok.RequestID).
}
```

Підпакет `corporate` також покриває monoКЕП — електронні підписи документів
у мобільному додатку: `cli.SignatureCreate / SignatureStatus / SignatureCancel`.

## Швидкий старт: Business API (юр. особи)

```go
import "github.com/OlexiyOdarchuk/go-monobank-sdk/business"

cli := business.New(os.Getenv("MONO_BUSINESS_TOKEN"))
accs, _ := cli.Accounts(ctx)

key, err := business.NewIdempotencyKey() // UUID v4
if err != nil { return err }
out, _ := cli.PreparePayment(ctx, key, &business.PaymentRequest{...})
```

Усі мутаційні endpoint-и приймають `Idempotency-Key` — генеруйте його через
`business.NewIdempotencyKey()` на кожну логічну спробу. Повтор із тим самим
ключем гарантовано безпечний — банк віддасть ту саму відповідь, не дублюючи
операцію. Порожній ключ відхиляється локально (`ErrIdempotencyKeyRequired`),
бо банк потрактує відсутність ключа як «дедуп не треба» і створить дубль.

## Швидкий старт: Acquiring (еквайринг)

```go
import "github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring"

cli := acquiring.New(os.Getenv("MONO_ACQUIRING_TOKEN"))

inv, _ := cli.CreateInvoice(ctx, &acquiring.CreateInvoiceRequest{
    Amount:   10000,           // копійки → 100.00 грн
    Currency: currency.UAH,    // або просто 980
    MerchantPaymInfo: &acquiring.MerchantPaymInfo{
        Reference:   "order-42",
        Destination: "Замовлення №42",
    },
    Validity:    600,
    PaymentType: acquiring.PaymentDebit,
})
// → клієнту віддаєте inv.PageURL; статус — через webhook або InvoiceStatus.

// Верифікація acquiring webhook (ECDSA-SHA256, ASN.1 DER).
// Поле `key` із /api/merchant/pubkey — це base64(PEM(SPKI)).
keyResp, _ := cli.PubKey(ctx)
pub, _ := keyResp.Public() // або acquiring.ParsePubKey([]byte(keyResp.Key))
if err := acquiring.VerifyWebhook(pub, body, r.Header.Get("X-Sign")); err != nil {
    http.Error(w, "bad signature", 400)
    return
}

// Опційно: захист від replay'а — відхилити webhook-и, modifiedDate яких
// старший за 5 хвилин. Це поверх dedup-у за (invoiceId, modifiedDate),
// який ви ОБОВ'ЯЗКОВО мусите тримати persistent (Mono ретраїть на 60s/600s).
if err := acquiring.VerifyWebhookFresh(pub, body, sig, 5*time.Minute); err != nil {
    // ErrWebhookStale / ErrWebhookNoTimestamp / ErrBadSignature
}
```

Покриті сценарії: одношагова оплата (debit), auth-then-capture (hold +
finalize), рекурент через токенізовані картки, регулярні платежі
(`SubscriptionCreate/Edit/Remove/Status/List/Payments`), QR-каси,
повернення, фіскальні чеки, період-виписка для звірки, прямі PAN-потоки
(для мерчантів з PCI DSS scope), monopay-кнопка (`MonoPayKeyImport/Delete/List`),
розщеплення платежів (`SplitReceivers`), T2P-термінали (`Terminals`).

## Швидкий старт: Installment («Покупка частинами»)

```go
import "github.com/OlexiyOdarchuk/go-monobank-sdk/installment"

cli, err := installment.New(
    os.Getenv("CHAST_STORE_ID"),
    os.Getenv("CHAST_SECRET"),
    installment.WithBaseURL(installment.BaseURLSandbox),
)
if err != nil { return err } // ErrEmptyStoreID / ErrEmptySecret / ErrInsecureBaseURL

order, _ := cli.CreateOrder(ctx, &installment.CreateOrderRequest{
    StoreOrderID: "ORD-001",
    ClientPhone:  "+380501234561",                // ...1 = sandbox-успіх
    TotalSum:     installment.NewMoney(2499, 99), // 2499.99 UAH (типована Money)
    Invoice: installment.CreateOrderInvoice{
        Number: "INV-001", Date: "2026-05-13",
        Source: installment.SourceInternet,
    },
    AvailablePrograms: []installment.Program{{AvailablePartsCount: []int{3, 6}}},
    Products: []installment.Product{
        {Name: "Кит-набір", Count: 1, Sum: installment.NewMoney(2499, 99)},
    },
    ResultCallback: "https://shop/api/chast/cb",
})
// → poll cli.OrderState(ctx, order.OrderID) до WAITING_FOR_STORE_CONFIRM
// → видали товар → cli.ConfirmOrder(ctx, order.OrderID)
```

Підпис тіла рахується автоматично (HMAC-SHA256 з вашим store-secret).
Для вхідного callback від банку — `cli.VerifyCallback(body, signatureHeader)`.
Невалідна довжина підпису → `ErrCallbackBadLength`; невалідний MAC →
`ErrCallbackSignatureMismatch` — окремі sentinel-и для security-телеметрії.

Усі money-поля installment (`TotalSum`, `Sum`, `Reverse.Sum`, `Commission`,
`CreditAmount` тощо) — це `installment.Money` із exact-decimal JSON
serializer (int64 копійок всередині, MarshalJSON через рядкову арифметику).
float64 на парсингу/серіалізації не з'являється — `0.10`/`0.20`/`0.30`
round-trip точно.

## Швидкий старт: Jar (банки)

```go
import "github.com/OlexiyOdarchuk/go-monobank-sdk/jar"

cli, err := jar.New()
if err != nil { return err }

// 1) Якщо знаєш longJarId (з URL віджета) — одразу /bank/jar/{id}:
info, _ := cli.ByLongID(ctx, "2zQL6sqnKgTYi7e69271YYWKTXTfMK8g")
fmt.Println(info.Title, info.Amount, "/", info.Goal)

// 2) Якщо є тільки коротке share-посилання send.monobank.ua/<id>:
short, _ := cli.ByShortID(ctx, "clientIdFromShareLink")
// → short.LongJarID можна закешувати й далі ходити в ByLongID
```

Обидва ендпоінти без авторизації, read-only. Ліміти на `send.monobank.ua`
суворіші — кешуй `LongJarID` і ходи лише в `ByLongID` для регулярних оновлень.

## Можливості

### Базовий клієнт ([root])

- `monobank.New(opts...)` — `*http.Client`, `HTTPDoer`, base URL, retry,
  auth, slog-логер, request/response hooks, клієнтський rate limiter.
- `WithRetry(attempts, base, max)` — експонентний бекоф з full jitter,
  повага до `Retry-After`, ретраїться лише на 5xx/429.
- `WithRateLimiter(RateLimiter)` — клієнтський throttle (див. розділ
  «Rate limits» нижче).
- `WithUnsafeRetries(bool)` — за замовчуванням POST/PATCH ретраяться
  ЛИШЕ за наявності заголовка `Idempotency-Key` (інакше 502 від
  балансера може створити дублікат операції). Вмикайте, якщо точно
  знаєте, що ваш endpoint ідемпотентний і дублі прийнятні.
- `APIError` — типізована помилка з методом, URL, отриманим/очікуваним
  статусом, розпарсеним `ErrorDescription` і сирим body. Реалізує
  `errors.Is` проти sentinel-помилок (див. нижче).
- `WithRequestHook` / `WithResponseHook` — підключіть власні метрики чи
  трейсинг без замін транспорту. Перезаписують один одного — щоб
  ДОДАТИ hook поверх існуючого (наприклад, application hook +
  OpenTelemetry): `Client.ChainRequestHook(fn)` / `ChainResponseHook(fn)`.
  Для повноцінного middleware-chain — `WithRoundTripper(http.RoundTripper)`:
  чистий слот під OpenTelemetry / Datadog / circuit-breaker, що зберігає
  таймаут і Cookie-jar вбудованого `*http.Client`.
- `WithUserAgent(string)` — перевизначає дефолтний User-Agent
  (`go-monobank-sdk/vX.Y.Z (linux; goX.Y.Z)`). Mono використовує його
  у support-кейсах для розрізнення твого сервісу серед інших юзерів
  SDK. Експортовано `monobank.UserAgent()` — якщо хочеш зберегти
  SDK-частину і додати свій префікс.
- `Client.Close() error` — реалізує `io.Closer`; зупиняє sweeper
  фоновим `KeyedLimiter`. Безпечно викликати на клієнті без лімітера.

### Insecure base URL

За замовчуванням `WithBaseURL("http://...")` для не-loopback-хоста
повертає `monobank.ErrInsecureBaseURL` із першого `Do` — щоб
конфігураційна помилка не призвела до відправлення X-Token у
відкритому вигляді. Loopback дозволений завжди — і `localhost`, і
вся `127.0.0.0/8`, і `::1` (через `net.IP.IsLoopback`, не лише
дослівні `localhost`/`127.0.0.1`). Свідомо обійти для MITM-проксі
чи staging-у за VPN-ом:

```go
cli := personal.New(token,
    monobank.WithInsecureBaseURL(true),
    monobank.WithBaseURL("http://staging.example.com"),
)
// Порядок опцій не важить — New робить two-pass apply.
```

Та сама гарда у `installment.New`, `jar.New`, `corporate.Client.Auth`
(для `X-Callback URL`). Кожен з пакетів має власну
`WithInsecureBaseURL` / `AllowInsecureCallback` для opt-out.

### Token redaction у логах

`auth.Personal`, `business.TokenAuth`, `acquiring.TokenAuth`,
`auth.CorpAuthMaker`, `installment.Client` реалізують
`slog.LogValuer` і рендеряться як `***` у slog-виводі:

```go
slog.Info("auth", "creds", auth.NewPersonal(token))
// → ... creds=auth.Personal{token:***}
```

Захист від випадкового потрапляння токену/секрету в DEBUG-логи через
`slog.Info("token", token)`. Якщо ти логуєш токен явно через
`slog.String("token", token)` — це твоя відповідальність.

SDK також маскує PII у logs: `bank.ClientInfo` (Name → перша
літера+`***`, WebHookURL шлях → `***`), `bank.Account` (IBAN →
country+last4, card masks → `****`+last4), а також
`installment.ClientInfo` (ПІБ/ІНН). Усе через `LogValue` — raw
структура поведена нормально, але `slog.Info("info", info)` не
розкриє ПІБ клієнта в log-аґрегаторі.

### Rate limits

Mono обмежує більшість endpoint-ів — наприклад, `/personal/client-info`
і `/personal/statement/{account}/…` дозволяють лише 1 виклик на 60 секунд
(на токен/акаунт відповідно). Поза лімітом сервер повертає `429` з
`Retry-After`, який `WithRetry` поважає автоматично.

> При систематичному перевищенні лімітів edge Mono (AWS) може віддати
> `403` з HTML-тілом (а не JSON `errorDescription`) і заблокувати IP
> приблизно на добу — це не те саме, що `403` про права токена.
> Sentinel `ErrForbidden` все одно зматчиться за статусом, але
> `APIError.ErrorDescription` буде порожнім, а `Body` міститиме HTML.
> Не ретрайте такий `403` агресивно. (Поведінка недокументована офіційно,
> описана [community-докою](https://github.com/andrew-demb/monobank-api-community-docs).)

Щоб не вистрілити в `429` з самого початку, додайте `WithRateLimiter`:

```go
import (
    "time"

    monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
    "github.com/OlexiyOdarchuk/go-monobank-sdk/personal"
)

lim := monobank.NewLimiter(time.Minute, 1) // 1 запит / 60 с
cli := personal.New(token,
    monobank.WithRateLimiter(lim),
    monobank.WithRetry(3, 0, 0), // на випадок, якщо все одно прилетить 429
)
```

`Wait` викликається один раз на логічний `Do` (а не на кожен retry),
тож токен не витрачається повторно. Сигнатура `RateLimiter.Wait(ctx)`
сумісна з `*golang.org/x/time/rate.Limiter` — підставляйте, якщо вам
потрібні дрібніші налаштування.

Для per-account обмежень (виписка по кожному рахунку — окремий ліміт)
використовуйте `KeyedLimiter`:

```go
// idleTTL=10*time.Minute — корзини, до яких не зверталися >10 хв,
// видаляються фоновим sweeper-ом, щоб мапа не росла безкінечно.
klim := monobank.NewKeyedLimiter(time.Minute, 1, 10*time.Minute)
defer klim.Stop()

cli := personal.New(token, monobank.WithRateLimiter(klim))

for _, acc := range info.Accounts {
    ctx := monobank.WithLimiterKey(ctx, acc.AccountID)
    txs, _ := cli.Transactions(ctx, acc.AccountID, from, to)
    // …
}
```

`idleTTL <= 0` вимикає eviction — годиться для коротких CLI-утиліт, але
у long-running сервісах задавайте розумне значення (~10× за `every`).

### Структуровані помилки

Усі personal/corporate/business/acquiring API повертають помилки у форматі
`{"errorDescription": "..."}`. SDK розпарсює їх у поле `APIError.ErrorDescription`:

```go
_, err := cli.ClientInfo(ctx)

// Sentinel-помилки за статусом — для типового control-flow:
switch {
case errors.Is(err, monobank.ErrUnauthorized):    // 401
    log.Println("токен невалідний")
case errors.Is(err, monobank.ErrForbidden):       // 403
    log.Println("токен не має прав на endpoint")
case errors.Is(err, monobank.ErrNotFound):        // 404
    log.Println("сутність не існує")
case errors.Is(err, monobank.ErrTooManyRequests): // 429
    log.Println("rate-limit перевищено")
}

// Повний APIError — коли треба ErrorDescription, raw body тощо:
var apiErr *monobank.APIError
if errors.As(err, &apiErr) {
    log.Printf("HTTP %d: %s", apiErr.StatusCode, apiErr.ErrorDescription)
    // apiErr.Body — оригінальні байти, якщо потрібен власний парсинг.
}
```

`installment` має власний формат і власний `installment.APIError` з полями
`Message`, `TraceID`. Підпис callback порівнюється або з
`installment.ErrCallbackBadLength` (для wrong-length header), або з
`installment.ErrCallbackSignatureMismatch` (для wrong MAC) — окремі
sentinels дозволяють security-телеметрії розрізняти "malformed request"
і "forgery attempt".

Інші локальні sentinel-помилки, які варто знати:

- `monobank.{ErrEmptyRequest, ErrInvalidURL, ErrInsecureBaseURL}`
- `auth.ErrEmptyToken` — порожній X-Token відхиляється з першого `Do`.
- `business.{ErrIdempotencyKeyRequired, ErrInvalidTimeRange, ErrNilRequest}`
- `installment.{ErrEmptyOrderID, ErrEmptyDate, ErrEmptyPhone, ErrInvalidPhone, ErrEmptyStoreID, ErrEmptySecret, ErrInsecureBaseURL}`
- `acquiring.{ErrEmptyID, ErrBadSignature, ErrWebhookStale, ErrWebhookNoTimestamp}`
- `corporate.{ErrInsecureCallback, ErrInvalidCallback, ErrLogoTooLarge, ErrEmptyAuthMaker, ErrNilRequest}`
- `jar.{ErrNotFound, ErrEmptyJarID, ErrEmptyClientID, ErrInsecureBaseURL}`
- `money.ErrOverflow` — int64 overflow на `Add/Sub/Mul`.

### `bank` — публічні endpoint-и

- `Rates(ctx)` — поточні курси банку.
- `Rates.Convert(amount Money, to Code)` — конвертує суму через курс
  банку (через UAH-крос, якщо потрібно).
- `ServerKey(ctx)` — публічний ECDSA-ключ для верифікації webhook;
  кешується, рефрешиться автоматично у `webhook.Handler`.

### `webhook` — серверна сторона

- `Verify(pub, body, signatureB64)` — приймає і raw `r||s`, і ASN.1 DER.
- `Parse(body)` → `*Response` з типізованими полями (transaction, mcc,
  amount, balance, opAmount, currencyCode, hold тощо).
- `NewHandler(ctx, Options)` — готовий `http.Handler`: верифікація,
  парсинг, дедуп, авторефреш ключа.
- `NewMemoryDeduper(n)` — LRU-set на `Data.Transaction.ID` (id транзакції
  з payload-у); підставте власну реалізацію `Deduper` для Redis/SQL —
  in-memory deduper втрачає стан при рестарті, що дозволяє replay
  валідно підписаного webhook-у.
- `Options.MaxBodyBytes` — стеля на розмір тіла запиту (за дефолтом
  1 MiB через `DefaultMaxBodyBytes`). Захист від OOM на публічно
  доступному webhook-URL.
- Refresh серверного ключа дроссельований до раз на 30 с і об'єднаний
  через `singleflight` — атакуючий, що шле POST з випадковим
  `X-Key-Id`, не амплифікує DoS на `/bank/sync`.

### `acquiring` — серверні helper-и та звірка

Поверх самих ендпоінтів еквайринг має батарейки для типових production-задач:

```go
// 1. Готовий http.Handler для acquiring-webhook-ів (аналог webhook.NewHandler,
//    але для еквайрингу): верифікація → freshness → парсинг → dedup → колбек.
//    Dedup за (invoiceId, modifiedDate); ключ авторефрешиться при поганому підписі.
h, _ := acquiring.NewWebhookHandler(ctx, acquiring.WebhookHandlerOptions{
    Keys:   cli,                              // *acquiring.Client (PubKeyProvider)
    Dedup:  acquiring.NewMemoryDeduper(4096), // у проді — persistent (Redis/SQL)
    MaxAge: 15 * time.Minute,                 // anti-replay поверх dedup-у
    OnEvent: func(ctx context.Context, inv *acquiring.InvoiceStatusResponse) error {
        return fulfil(ctx, inv) // помилка → 500 → Mono ретраїть; nil → 200
    },
})
mux.Handle("/mono/webhook", h)

// 2. Reconciliation: добити статус, якщо webhook загубився (єдиний спосіб
//    побачити "expired", по якому webhook не приходить) + diff виписки.
inv, err := cli.PollInvoice(ctx, invoiceID, acquiring.PollOptions{
    Interval: time.Second, Timeout: 2 * time.Minute,
}) // → термінальний стан або acquiring.ErrPollTimeout
stmt, _ := cli.Statement(ctx, from, time.Time{}, "")
rec := acquiring.ReconcileStatement(stmt, local) // local: map[invoiceID]LocalPayment
if !rec.Clean() { /* rec.OnlyRemote / OnlyLocal / Mismatches */ }

// 3. Basket-білдер із валідацією (найчастіша помилка — відсутній code або
//    total != qty*sum). Build звіряє суму кошика з amount інвойсу.
items, err := acquiring.NewBasket().
    AddItem("Кава", "SKU-1", 2, money.UAH(45).Minor, acquiring.WithUnit("шт.")).
    AddItem("Чай", "SKU-2", 1, money.UAH(30).Minor).
    Build(money.UAH(120).Minor)

// 4. Типізовані помилки API ({errCode, errText}) з предикатами.
if _, err := cli.InvoiceStatus(ctx, id); err != nil {
    switch {
    case acquiring.IsNotFound(err):        // невідомий invoiceId
    case acquiring.IsTooManyRequests(err): // back off
    }
    if e, ok := acquiring.AsAPIError(err); ok { log.Println(e.Code, e.Text) }
}

// 5. Subscription lifecycle: класифікатор, що драйвить grace-логіку.
switch acquiring.ClassifyCharge(payment.Status, sub.WalletData.Status) {
case acquiring.HealthChargeFailed: // тимчасово — тримати доступ у grace-вікні
case acquiring.HealthWalletDead:   // токен мертвий — попросити перепривʼязку картки
case acquiring.HealthActive:       // ок
}
```

- `NewWebhookHandler` дзеркалить `webhook.NewHandler`: `singleflight` + throttle
  на рефреш ключа, `MaxBodyBytes` (дефолт 1 MiB), `OnError`-хук, ack-200 для
  GET-пінгу підписки.
- `Deduper` / `NewMemoryDeduper(n)` — LRU за `DedupKey(inv)` = `invoiceId|modifiedDate`.
- `InvoiceStatus.IsTerminal()` і `PollOptions.TreatHoldAsTerminal` — для
  auth-then-capture, де цільовий стан саме `hold`.
- `AsAPIError` / `Code` / `Is{BadRequest,Forbidden,NotFound,TooManyRequests,InternalError}`
  — падають на HTTP-статус, якщо тіло без `errCode`; зберігають monobank-sentinel-и
  через `Unwrap`.
- `SubscriptionHealth`: `GraceEligible()`, `Terminal()`, `RetainAccess()`.

### `money` — типізовані суми

```go
m := money.New(12550, currency.UAH)        // 125.50 грн
k := money.UAH(125.50)                      // 12550 копійок — без плутанини *100
p, _ := money.ParseMajor("0.10", currency.UAH) // 10 копійок, ціломатематично (без float-похибки)
fmt.Println(money.UAH(125.50).Major())     // 125.5 — назад у гривні
trip, err := m.Mul(3)                      // → ErrOverflow при MaxInt64*N
if err != nil { return err }
sum, err := m.Add(other)                   // різні валюти → error
                                           // переповнення → ErrOverflow
fmt.Println(trip)                          // "376.50 UAH"
fmt.Println(money.New(1250, currency.JPY)) // "1250 JPY" — 0 знаків після коми
```

`Money` серіалізується в JSON як ціле число мінорних одиниць — wire-сумісно
з форматом Mono. `Money.Major` / `String` поважають
`currency.Code.Decimals()` — JPY/KRW (0 знаків), BHD/JOD/KWD/OMR/TND
(3 знаки), UAH/USD/EUR/... (2 знаки) форматуються коректно без жорстко
зашитого ділення на 100. Helper `currency.Code.MinorPerMajor()` повертає
`10^Decimals` для конвертації decimal-float ↔ int64 minor.

`Add/Sub/Mul` повертають `ErrOverflow` при перетині межі int64 — silent
wrap на сумах рівня квартальної виписки виключений.

### `installment.Money` — UAH з exact-decimal wire-форматом

`installment` API унікальна тим, що шле гроші як `"total_sum": 2499.99`
(decimal-float), а не як int64 minor units. Щоб уникнути float64 precision
loss, у пакеті є окремий тип:

```go
amt := installment.NewMoney(2499, 99)     // 2499.99 грн
amt = installment.MoneyFromKopecks(249999)
amt = installment.MoneyFromMajor(2499.99) // round half away from zero

// MarshalJSON емітить `2499.99` (без лапок); UnmarshalJSON парсить через
// рядок, без float64. 0.10, 0.20, 0.30 round-trip точно.
```

### `currency`, `mcc` — enum-и

- `currency.Code(980).String()` → `"UAH"`; `currency.FromAlpha3("USD")` → `840`.
- `currency.UAH.Decimals()` → `2`; `currency.JPY.Decimals()` → `0`;
  `currency.BHD.Decimals()` → `3`.
- `mcc.Code(5411).Category()` → `mcc.CategoryGroceries`.
- `mcc.Code(3500).Category()` → `mcc.CategoryHotels` (3000-3299 авіакомпанії,
  3300-3499 оренда авто, 3500-3999 готелі).
- `mcc.Code(5912).Category()` → `mcc.CategoryHealth` (аптека-override).

### Пагінатори

Усі ендпоінти, де є природний курсор, мають `iter.Seq2`-обгортки:

- `personal.Client.TransactionsRangeIter` — виписка за довільний період,
  ліниво по 31-денних вікнах. Вікна, де більше за ліміт у 500 транзакцій
  на відповідь, добираються прозоро (Mono віддає максимум 500 рядків і не
  має offset-курсора — SDK переанкорує `to` на найстарішу секунду й
  дедупить за id, як `business.StatementAll`).
- `corporate.Client.TransactionsRangeIter` — те саме для corp-API.
- `business.Client.ContactsAll` / `SearchContactsAll` — зарплатні
  контакти.
- `business.Client.StatementAll` — виписка через DOWN-курсор по часу.
- `acquiring.Client.SubscriptionListAll` / `SubscriptionPaymentsAll` —
  списки підписок та їх платежів.

Усі поважають `break` (зупиняють подальші HTTP-виклики) і `ctx.Done()`.

### `monobanktest` — фейковий monobank для тестів

```go
srv := monobanktest.NewServer(t)
srv.WithClientInfo(&bank.ClientInfo{ID: "c1", Name: "Тест"})
cli := personal.New("token", srv.Option())

info, _ := cli.ClientInfo(ctx) // повертає те, що зашили вище
```

Білдери покривають типові сценарії: клієнт-інфо, виписки за період,
курси, webhook-події. Конкурентно-безпечні.

Для інтеграційних тестів верифікації acquiring-webhook-ів є генератор
підписаного тіла — не треба руками піднімати ECDSA-ключі:

```go
signer := monobanktest.NewAcquiringWebhookSigner(t)        // P-256 keypair
srv := monobanktest.NewServer(t).WithAcquiringPubKey(signer) // віддає /api/merchant/pubkey
cli := acquiring.New("tok", srv.Option())
h, _ := acquiring.NewWebhookHandler(ctx, acquiring.WebhookHandlerOptions{
    Keys: cli, OnEvent: onEvent,
})

body := signer.MarshalBody(t, &acquiring.InvoiceStatusResponse{
    InvoiceID: "p2_x", Status: acquiring.InvoiceSuccess,
})
req := signer.Request(t, "POST", "/webhook", body) // X-Sign проставлено
h.ServeHTTP(rec, req)                              // підпис верифікується справжнім ключем
```

### `otelmonobank` — OpenTelemetry

```go
import (
    "go.opentelemetry.io/otel"
    "github.com/OlexiyOdarchuk/go-monobank-sdk/otelmonobank"
    "github.com/OlexiyOdarchuk/go-monobank-sdk/personal"
)

cli := personal.New(token, otelmonobank.WithTracer(otel.Tracer("my-app")))
```

Кожен HTTP-виклик (включно з ретраями) стає окремим span-ом із атрибутами
`http.method`, `http.url`, `http.status_code`. Винесено в окремий
sub-module, щоб OTel не тягнувся у проєкти, які його не використовують.

## Приклади

Робочі програми в [`examples/`](examples/):

| Каталог | Що показує |
|---|---|
| `examples/personal` | ClientInfo, виписка за 7 днів, MCC-категорії |
| `examples/corporate` | ECDSA-авторизація, очікування підтвердження клієнта |
| `examples/business` | Список рахунків + виписка з пагінацією |
| `examples/acquiring` | Створення інвойсу, поллінг до фінального стану |
| `examples/installment` | Sandbox-флоу ПЧ: create → state → confirm |
| `examples/jar` | Lookup банки за longJarId та коротким clientId |
| `examples/webhook` | HTTP-сервер з ECDSA-верифікацією і дедупом |

Кожен — `go run ./examples/<name>` з відповідною env-змінною (`MONO_TOKEN`,
`MONO_ACQUIRING_TOKEN`, `CHAST_STORE_ID`+`CHAST_SECRET`, `LONG_JAR_ID` тощо).

## Документація

Повна godoc-документація на кожен пакет, тип, функцію:
<https://pkg.go.dev/github.com/OlexiyOdarchuk/go-monobank-sdk>.

Wire-ідентифікатори (HTTP-заголовки, JSON-ключі) лишаються англійською —
як це бачить власне API.

- [`CHANGELOG.md`](CHANGELOG.md) — історія релізів (Keep a Changelog).
- [`SECURITY.md`](SECURITY.md) — як приватно повідомити про вразливість.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — як долучитися PR-ом.

## Сумісність

- Go: 1.25+ (`iter.Seq2`-пагінатори; `otelmonobank` пінить 1.25 через OpenTelemetry).
- Перевірено на 1.25.x – 1.26.x у CI.
- Race-clean — `go test -race ./...` проходить.

## Зв'язок з vtopc/go-monobank

Upstream свідомо мінімалістичний: тонкі обгортки лише для Personal та
Corporate Open API, без серверних helper-ів, без enum-ів, без corp-api
чи еквайрингу. Цей SDK залишає кожен endpoint upstream-у і додає решту.
Хочете «батерейки в комплекті» — беріть це; хочете мінімум — беріть upstream.

MIT-ліцензія несе авторські права upstream-у далі (див. [LICENSE](LICENSE)).

## Ліцензія

MIT — див. [LICENSE](LICENSE).
