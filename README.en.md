# monobank-sdk

[🇺🇦 Українська](README.md) · 🇬🇧 English

[![Go Reference](https://pkg.go.dev/badge/github.com/OlexiyOdarchuk/go-monobank-sdk.svg)](https://pkg.go.dev/github.com/OlexiyOdarchuk/go-monobank-sdk)
[![CI](https://github.com/OlexiyOdarchuk/go-monobank-sdk/actions/workflows/ci.yaml/badge.svg)](https://github.com/OlexiyOdarchuk/go-monobank-sdk/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/OlexiyOdarchuk/go-monobank-sdk/branch/main/graph/badge.svg)](https://codecov.io/gh/OlexiyOdarchuk/go-monobank-sdk)
[![Go Report Card](https://goreportcard.com/badge/github.com/OlexiyOdarchuk/go-monobank-sdk)](https://goreportcard.com/report/github.com/OlexiyOdarchuk/go-monobank-sdk)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Full-featured Go SDK for every public monobank API: **personal**, **corporate**
(with monoКЕП digital signatures), **business** (corp-api for legal entities),
**acquiring** (with subscriptions / monopay-keys / split-receivers / T2P),
**installment** ("Покупка частинами") and **jar** (public jars). On top of
the API clients the SDK ships server-side helpers you would otherwise have
to write yourself — ECDSA signature verification, a drop-in `http.Handler`,
retry deduplication, HMAC-SHA256 for installment callbacks, typed enums for
MCC and currency codes, and a fake server for tests.

> Started as a fork of [vtopc/go-monobank](https://github.com/vtopc/go-monobank);
> grew because the upstream is intentionally a thin Open-API wrapper. Everything
> beyond that lives here.

## Installation

```bash
go get github.com/OlexiyOdarchuk/go-monobank-sdk
```

Requires Go 1.23+ (`iter.Seq2` for paginators). `otelmonobank` needs Go 1.25+
(an OpenTelemetry requirement).

## API coverage

All five public monobank APIs plus two community-documented jar endpoints.
The internal mobile API has no public spec — out of scope.

| API | Spec | Subpackage | Endpoints |
|---|---|---|---|
| Open API — personal (X-Token) | [api.monobank.ua/docs](https://api.monobank.ua/docs/) | `personal` | 4 |
| Open API — corporate (ECDSA) + monoКЕП | [corporate.html](https://api.monobank.ua/docs/corporate.html) | `corporate` | 11 |
| corp-api for legal entities | [corp-api.monobank.ua](https://corp-api.monobank.ua/) | `business` | 17 |
| Acquiring (`/api/merchant/*`) | [acquiring.html](https://api.monobank.ua/docs/acquiring.html) | `acquiring` | 31 |
| Installment (HMAC-SHA256) | [chast docs](https://u2-demo-ext.mono.st4g3.com/docs/index.html) | `installment` | 14 |
| JAR / public jars (community) | community docs | `jar` | 2 |

## Package layout

```
monobank-sdk/
├── (root)          base HTTP transport: Client, Option, retry, hooks
├── auth/           Authorizer + Personal/Corporate/Public + monoКЕП helpers
├── bank/           public endpoints (Rates, ServerKey) + shared types
│                   (ClientInfo, Account, Jar, Transaction) + Rates.Convert
├── personal/       Personal Open API (X-Token)
├── corporate/      Corporate Open API (ECDSA) + monoКЕП
├── business/       corp-api.monobank.ua — 17 endpoints
├── acquiring/      /api/merchant/* — 31 endpoints (invoices, QR, wallet,
│                   subscriptions, monopay-keys, split, T2P) +
│                   ECDSA webhook verification
├── installment/    "Покупка частинами" (u2.monobank.com.ua) — 14 endpoints,
│                   HMAC-SHA256 body signing, VerifyCallback
├── jar/            public jars: /bank/jar/{id} + send.monobank.ua/api/handler
├── webhook/        Verify, Parse, http.Handler, Deduper for personal webhook
├── monobanktest/   fake monobank server + builders for testing
├── money/          typed Money + currency-aware arithmetic
├── currency/       ISO 4217 numeric code + alpha-3 name
├── mcc/            ISO 18245 enum + Category (Groceries, Fuel, Restaurants…)
└── otelmonobank/   (separate sub-module) OpenTelemetry integration
```

Each subpackage imports independently — pull only what you need.

## Quick start: Personal API

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

    // A statement window > 31 days is paginated transparently.
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

Get a token at <https://api.monobank.ua/>.

## Quick start: Webhook (receive events)

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
    keys := bank.New() // KeyProvider — fetches /bank/sync public key and caches it

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

Subscribe a URL once: `personal.New(token).SetWebHook(ctx, "https://you/webhook")`.

`webhook.Handler` automatically:
- loads the public key via `Keys.ServerKey` on startup and refreshes it
  when `X-Key-Id` rotates,
- verifies the ECDSA signature in `X-Sign`,
- parses the payload into a typed `Response`,
- consults `Deduper` so monobank retries don't run your callback twice,
- returns 200/400/500 according to the result.

## Quick start: Corporate API (ECDSA)

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
    // → redirect the user to tok.AcceptURL,
    //   then poll cli.CheckAuth(ctx, tok.RequestID).
}
```

The `corporate` subpackage also covers monoКЕП — digital document signatures
in the mobile app: `cli.SignatureCreate / SignatureStatus / SignatureCancel`.

## Quick start: Business API (legal entities)

```go
import "github.com/OlexiyOdarchuk/go-monobank-sdk/business"

cli := business.New(os.Getenv("MONO_BUSINESS_TOKEN"))
accs, _ := cli.Accounts(ctx)

key := business.NewIdempotencyKey() // UUID v4
out, _ := cli.PreparePayment(ctx, key, &business.PaymentRequest{...})
```

All mutating endpoints accept an `Idempotency-Key` — generate one with
`business.NewIdempotencyKey()` per logical attempt. Retries with the same
key are guaranteed safe — the bank returns the same response without
duplicating the operation.

## Quick start: Acquiring

```go
import "github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring"

cli := acquiring.New(os.Getenv("MONO_ACQUIRING_TOKEN"))

inv, _ := cli.CreateInvoice(ctx, &acquiring.CreateInvoiceRequest{
    Amount: 10000, // minor units → 100.00 UAH
    Ccy:    980,   // UAH
    MerchantPaymInfo: &acquiring.MerchantPaymInfo{
        Reference:   "order-42",
        Destination: "Order #42",
    },
    Validity:    600,
    PaymentType: acquiring.PaymentDebit,
})
// → hand inv.PageURL to the customer; track status via webhook or InvoiceStatus.

// Verify acquiring webhook (ECDSA-SHA256, ASN.1 DER).
// /api/merchant/pubkey returns base64(PEM(SPKI)).
keyResp, _ := cli.PubKey(ctx)
pub, _ := keyResp.Public() // or acquiring.ParsePubKey([]byte(keyResp.Key))
if err := acquiring.VerifyWebhook(pub, body, r.Header.Get("X-Sign")); err != nil {
    http.Error(w, "bad signature", 400)
    return
}
```

Covered scenarios: one-shot debit, auth-then-capture (hold + finalize),
recurrent payments via tokenized cards, scheduled subscriptions
(`SubscriptionCreate/Edit/Remove/Status/List/Payments`), QR cashboxes,
refunds, fiscal receipts, statement export for reconciliation, direct PAN
flows (for merchants in PCI DSS scope), monopay button
(`MonoPayKeyImport/Delete/List`), split payments (`SplitReceivers`),
T2P terminals (`Terminals`).

## Quick start: Installment ("Покупка частинами")

```go
import "github.com/OlexiyOdarchuk/go-monobank-sdk/installment"

cli := installment.New(
    os.Getenv("CHAST_STORE_ID"),
    os.Getenv("CHAST_SECRET"),
    installment.WithBaseURL(installment.BaseURLSandbox),
)

order, _ := cli.CreateOrder(ctx, &installment.CreateOrderRequest{
    StoreOrderID: "ORD-001",
    ClientPhone:  "+380501234561", // ...1 = sandbox success
    TotalSum:     2499.99,         // hryvnia + kopecks, NOT minor units
    Invoice: installment.CreateOrderInvoice{
        Number: "INV-001", Date: "2026-05-13",
        Source: installment.SourceInternet,
    },
    AvailablePrograms: []installment.Program{{AvailablePartsCount: []int{3, 6}}},
    Products:          []installment.Product{{Name: "Kit", Count: 1, Sum: 2499.99}},
    ResultCallback:    "https://shop/api/chast/cb",
})
// → poll cli.OrderState(ctx, order.OrderID) until WAITING_FOR_STORE_CONFIRM
// → ship the goods → cli.ConfirmOrder(ctx, order.OrderID)
```

Body signing is automatic (HMAC-SHA256 with your store secret). For
incoming bank callbacks: `cli.VerifyCallback(body, signatureHeader)`.

## Quick start: Jar

```go
import "github.com/OlexiyOdarchuk/go-monobank-sdk/jar"

cli := jar.New()

// 1) If you know the longJarId (from the widget URL) — straight to /bank/jar/{id}:
info, _ := cli.ByLongID(ctx, "2zQL6sqnKgTYi7e69271YYWKTXTfMK8g")
fmt.Println(info.Title, info.Amount, "/", info.Goal)

// 2) If you only have a short share link send.monobank.ua/<id>:
short, _ := cli.ByShortID(ctx, "clientIdFromShareLink")
// → cache short.LongJarID and use ByLongID for repeated polls
```

Both endpoints are unauthenticated and read-only. `send.monobank.ua` has
tighter rate limits — cache `LongJarID` and only call `ByLongID` for
recurring updates.

## Features

### Base client ([root])

- `monobank.New(opts...)` — `*http.Client`, `HTTPDoer`, base URL, retry,
  auth, slog logger, request/response hooks, client-side rate limiter.
- `WithRetry(attempts, base, max)` — exponential backoff with full jitter,
  honors `Retry-After`, retries only 5xx/429.
- `WithRateLimiter(RateLimiter)` — client-side throttle (see "Rate limits"
  below).
- `APIError` — typed error with method, URL, received/expected status,
  parsed `ErrorDescription`, and the raw body.
- `WithRequestHook` / `WithResponseHook` — plug in metrics or tracing
  without replacing the transport.

### Rate limits

Mono throttles most endpoints — for example, `/personal/client-info` and
`/personal/statement/{account}/…` allow only 1 call per 60 seconds (per
token / per account respectively). Past the limit the server returns `429`
with `Retry-After`, which `WithRetry` honors automatically.

To avoid hitting `429` from the start, add `WithRateLimiter`:

```go
import (
    "time"

    monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
    "github.com/OlexiyOdarchuk/go-monobank-sdk/personal"
)

lim := monobank.NewLimiter(time.Minute, 1) // 1 request / 60s
cli := personal.New(token,
    monobank.WithRateLimiter(lim),
    monobank.WithRetry(3, 0, 0), // in case 429 still slips through
)
```

`Wait` runs once per logical `Do` (not per retry attempt), so a token is
never spent twice. `RateLimiter.Wait(ctx)` matches the signature of
`*golang.org/x/time/rate.Limiter` — drop yours in if you need finer
controls.

For per-account limits (each account's statement has its own quota), use
`KeyedLimiter`:

```go
klim := monobank.NewKeyedLimiter(time.Minute, 1)
cli := personal.New(token, monobank.WithRateLimiter(klim))

for _, acc := range info.Accounts {
    ctx := monobank.WithLimiterKey(ctx, acc.ID)
    txs, _ := cli.Transactions(ctx, acc.ID, from, to)
    // …
}
```

### Structured errors

Personal/corporate/business/acquiring APIs all return errors as
`{"errorDescription": "..."}`. The SDK parses that into
`APIError.ErrorDescription`:

```go
_, err := cli.ClientInfo(ctx)
var apiErr *monobank.APIError
if errors.As(err, &apiErr) {
    if apiErr.StatusCode == http.StatusForbidden {
        log.Printf("token rejected: %s", apiErr.ErrorDescription)
    }
    // apiErr.Body — raw bytes if you need custom parsing.
}
```

`installment` has its own format and its own `installment.APIError` with
`Message`, `TraceID` fields.

### `bank` — public endpoints

- `Rates(ctx)` — current bank rates.
- `Rates.Convert(amount Money, to Code)` — converts an amount through the
  bank's rate (via UAH cross if needed).
- `ServerKey(ctx)` — public ECDSA key for webhook verification; cached
  and auto-refreshed inside `webhook.Handler`.

### `webhook` — server side

- `Verify(pub, body, signatureB64)` — accepts both raw `r||s` and ASN.1 DER.
- `Parse(body)` → `*Response` with typed fields (transaction, mcc,
  amount, balance, opAmount, currencyCode, hold, etc.).
- `NewHandler(ctx, Options)` — drop-in `http.Handler`: verification,
  parsing, dedup, key auto-refresh.
- `NewMemoryDeduper(n)` — LRU set keyed on `Response.UID`; swap in your
  own `Deduper` for Redis/SQL.

### `money` — typed amounts

```go
m := money.New(12550, currency.UAH) // 125.50 UAH
m = m.Mul(3)                         // 376.50 UAH
sum, err := m.Add(other)             // errors out on currency mismatch
fmt.Println(m)                       // "125.50 UAH"
```

`Money` serializes as a plain integer (minor units) — wire-compatible with
mono's format.

### `currency`, `mcc` — enums

- `currency.Code(980).String()` → `"UAH"`; `currency.FromAlpha3("USD")` → `840`.
- `mcc.Code(5411).Category()` → `mcc.CategoryGroceries`.

### `monobanktest` — fake monobank for tests

```go
srv := monobanktest.NewServer(t)
srv.WithClientInfo(&bank.ClientInfo{ID: "c1", Name: "Test"})
cli := personal.New("token", srv.Option())

info, _ := cli.ClientInfo(ctx) // returns whatever you wired above
```

Builders cover the typical scenarios: client info, statements over a
range, rates, webhook events. Concurrency-safe.

### `otelmonobank` — OpenTelemetry

```go
import (
    "go.opentelemetry.io/otel"
    "github.com/OlexiyOdarchuk/go-monobank-sdk/otelmonobank"
    "github.com/OlexiyOdarchuk/go-monobank-sdk/personal"
)

cli := personal.New(token, otelmonobank.WithTracer(otel.Tracer("my-app")))
```

Every HTTP call (retries included) becomes its own span with attributes
`http.method`, `http.url`, `http.status_code`. Lives in a separate
sub-module so OTel doesn't get pulled into projects that don't use it.

## Examples

Runnable programs in [`examples/`](examples/):

| Directory | What it shows |
|---|---|
| `examples/personal` | ClientInfo, 7-day statement, MCC categories |
| `examples/corporate` | ECDSA auth, polling for client approval |
| `examples/business` | Account list + paginated statement |
| `examples/acquiring` | Create invoice, poll to a final state |
| `examples/installment` | Sandbox flow: create → state → confirm |
| `examples/jar` | Lookup a jar by longJarId or short clientId |
| `examples/webhook` | HTTP server with ECDSA verification + dedup |

Each one runs as `go run ./examples/<name>` with the matching env
(`MONO_TOKEN`, `MONO_ACQUIRING_TOKEN`, `CHAST_STORE_ID`+`CHAST_SECRET`,
`LONG_JAR_ID`, etc.).

## Documentation

Full godoc per package, type, and function (in Ukrainian) at
<https://pkg.go.dev/github.com/OlexiyOdarchuk/go-monobank-sdk>.

Wire identifiers (HTTP headers, JSON keys) stay in English — exactly as
the API itself uses them.

- [`CHANGELOG.md`](CHANGELOG.md) — release history (Keep a Changelog).
- [`SECURITY.md`](SECURITY.md) — how to privately report a vulnerability.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — how to send a PR.

## Compatibility

- Go: 1.23+ (requires `iter.Seq2`). `otelmonobank` — 1.25+.
- Verified on 1.23.x – 1.26.x in CI.
- Race-clean — `go test -race ./...` passes.

## Relation to vtopc/go-monobank

The upstream is intentionally minimal: thin wrappers around Personal and
Corporate Open APIs only, no server-side helpers, no enums, no corp-api
or acquiring. This SDK leaves every upstream endpoint where it is and
adds the rest. Want batteries included — pick this; want a thin layer —
pick the upstream.

The MIT license carries upstream copyright forward (see [LICENSE](LICENSE)).

## License

MIT — see [LICENSE](LICENSE).
