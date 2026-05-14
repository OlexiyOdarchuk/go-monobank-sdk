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

Потрібен Go 1.23+ (через `iter.Seq2` для пагінаторів). Для `otelmonobank` —
Go 1.25+ (вимога самого OpenTelemetry).

## Покриття API

Покрито всі п'ять публічних API monobank плюс два community-документовані
ендпоінти банок (jars). Внутрішній mobile-API публічної специфікації не
має — поза скоупом.

| API | Специфікація | Підпакет | Endpoints |
|---|---|---|---|
| Open API — personal (X-Token) | [api.monobank.ua/docs](https://api.monobank.ua/docs/) | `personal` | 4 |
| Open API — corporate (ECDSA) + monoКЕП | [corporate.html](https://api.monobank.ua/docs/corporate.html) | `corporate` | 11 |
| corp-api для юр. осіб | [corp-api.monobank.ua](https://corp-api.monobank.ua/) | `business` | 23 |
| Acquiring (`/api/merchant/*`) | [acquiring.html](https://api.monobank.ua/docs/acquiring.html) | `acquiring` | 31 |
| Покупка частинами (HMAC-SHA256) | [chast docs](https://u2-demo-ext.mono.st4g3.com/docs/index.html) | `installment` | 14 |
| JAR / банки (community) | community docs | `jar` | 2 |

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

key := business.NewIdempotencyKey() // UUID v4
out, _ := cli.PreparePayment(ctx, key, &business.PaymentRequest{...})
```

Усі мутаційні endpoint-и приймають `Idempotency-Key` — генеруйте через
`business.NewIdempotencyKey()` на кожну логічну спробу. Повтор із тим самим
ключем гарантовано безпечний — банк віддасть ту саму відповідь, не дублюючи
операцію.

## Швидкий старт: Acquiring (еквайринг)

```go
import "github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring"

cli := acquiring.New(os.Getenv("MONO_ACQUIRING_TOKEN"))

inv, _ := cli.CreateInvoice(ctx, &acquiring.CreateInvoiceRequest{
    Amount: 10000, // копійки → 100.00 грн
    Ccy:    980,   // UAH
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

cli := installment.New(
    os.Getenv("CHAST_STORE_ID"),
    os.Getenv("CHAST_SECRET"),
    installment.WithBaseURL(installment.BaseURLSandbox),
)

order, _ := cli.CreateOrder(ctx, &installment.CreateOrderRequest{
    StoreOrderID: "ORD-001",
    ClientPhone:  "+380501234561", // ...1 = sandbox-успіх
    TotalSum:     2499.99,         // гривні з копійками, НЕ мінорні одиниці
    Invoice: installment.CreateOrderInvoice{
        Number: "INV-001", Date: "2026-05-13",
        Source: installment.SourceInternet,
    },
    AvailablePrograms: []installment.Program{{AvailablePartsCount: []int{3, 6}}},
    Products:          []installment.Product{{Name: "Кит-набір", Count: 1, Sum: 2499.99}},
    ResultCallback:    "https://shop/api/chast/cb",
})
// → poll cli.OrderState(ctx, order.OrderID) до WAITING_FOR_STORE_CONFIRM
// → видали товар → cli.ConfirmOrder(ctx, order.OrderID)
```

Підпис тіла рахується автоматично (HMAC-SHA256 з вашим store-secret).
Для вхідного callback від банку — `cli.VerifyCallback(body, signatureHeader)`.

## Швидкий старт: Jar (банки)

```go
import "github.com/OlexiyOdarchuk/go-monobank-sdk/jar"

cli := jar.New()

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
- `APIError` — типізована помилка з методом, URL, отриманим/очікуваним
  статусом, розпарсеним `ErrorDescription` і сирим body.
- `WithRequestHook` / `WithResponseHook` — підключіть власні метрики чи
  трейсинг без замін транспорту.

### Rate limits

Mono обмежує більшість endpoint-ів — наприклад, `/personal/client-info`
і `/personal/statement/{account}/…` дозволяють лише 1 виклик на 60 секунд
(на токен/акаунт відповідно). Поза лімітом сервер повертає `429` з
`Retry-After`, який `WithRetry` поважає автоматично.

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
тримайте окремий `*Limiter` на акаунт, або реалізуйте власний
`RateLimiter` із dispatch-логікою.

### Структуровані помилки

Усі personal/corporate/business/acquiring API повертають помилки у форматі
`{"errorDescription": "..."}`. SDK розпарсює їх у поле `APIError.ErrorDescription`:

```go
_, err := cli.ClientInfo(ctx)
var apiErr *monobank.APIError
if errors.As(err, &apiErr) {
    if apiErr.StatusCode == http.StatusForbidden {
        log.Printf("token rejected: %s", apiErr.ErrorDescription)
    }
    // apiErr.Body — оригінальні байти, якщо потрібен власний парсинг.
}
```

`installment` має власний формат і власний `installment.APIError` з полями
`Message`, `TraceID`.

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

### `money` — типізовані суми

```go
m := money.New(12550, currency.UAH) // 125.50 грн
m = m.Mul(3)                         // 376.50 грн
sum, err := m.Add(other)             // помилка, якщо валюти різні
fmt.Println(m)                       // "125.50 UAH"
```

`Money` серіалізується в JSON як ціле число мінорних одиниць — wire-сумісно
з форматом Mono.

### `currency`, `mcc` — enum-и

- `currency.Code(980).String()` → `"UAH"`; `currency.FromAlpha3("USD")` → `840`.
- `mcc.Code(5411).Category()` → `mcc.CategoryGroceries`.

### `monobanktest` — фейковий monobank для тестів

```go
srv := monobanktest.NewServer(t)
srv.WithClientInfo(&bank.ClientInfo{ID: "c1", Name: "Тест"})
cli := personal.New("token", srv.Option())

info, _ := cli.ClientInfo(ctx) // повертає те, що зашили вище
```

Білдери покривають типові сценарії: клієнт-інфо, виписки за період,
курси, webhook-події. Конкурентно-безпечні.

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

Повна godoc-документація на кожен пакет, тип, функцію — українською:
<https://pkg.go.dev/github.com/OlexiyOdarchuk/go-monobank-sdk>.

Wire-ідентифікатори (HTTP-заголовки, JSON-ключі) лишаються англійською —
як це бачить власне API.

- [`CHANGELOG.md`](CHANGELOG.md) — історія релізів (Keep a Changelog).
- [`SECURITY.md`](SECURITY.md) — як приватно повідомити про вразливість.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — як долучитися PR-ом.

## Сумісність

- Go: 1.23+ (через `iter.Seq2`). `otelmonobank` — 1.25+.
- Перевірено на 1.23.x – 1.26.x у CI.
- Race-clean — `go test -race ./...` проходить.

## Зв'язок з vtopc/go-monobank

Upstream свідомо мінімалістичний: тонкі обгортки лише для Personal та
Corporate Open API, без серверних helper-ів, без enum-ів, без corp-api
чи еквайрингу. Цей SDK залишає кожен endpoint upstream-у і додає решту.
Хочете «батерейки в комплекті» — беріть це; хочете мінімум — беріть upstream.

MIT-ліцензія несе авторські права upstream-у далі (див. [LICENSE](LICENSE)).

## Ліцензія

MIT — див. [LICENSE](LICENSE).
