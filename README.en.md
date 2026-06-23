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
| corp-api for legal entities | [corp-api.monobank.ua](https://corp-api.monobank.ua/) | `business` | 23 |
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
├── business/       corp-api.monobank.ua — 23 endpoints
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

key, err := business.NewIdempotencyKey() // UUID v4
if err != nil { return err }
out, _ := cli.PreparePayment(ctx, key, &business.PaymentRequest{...})
```

All mutating endpoints accept an `Idempotency-Key` — generate one with
`business.NewIdempotencyKey()` per logical attempt. Retries with the same
key are guaranteed safe — the bank returns the same response without
duplicating the operation. An empty key is refused locally with
`ErrIdempotencyKeyRequired`, because the bank treats a missing header as
"no dedupe" and would happily create a duplicate.

## Quick start: Acquiring

```go
import "github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring"

cli := acquiring.New(os.Getenv("MONO_ACQUIRING_TOKEN"))

inv, _ := cli.CreateInvoice(ctx, &acquiring.CreateInvoiceRequest{
    Amount:   10000,           // minor units → 100.00 UAH
    Currency: currency.UAH,    // or just 980
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

// Optional replay defence — reject webhooks older than 5 minutes by
// modifiedDate. This is ON TOP OF the persistent (invoiceId,
// modifiedDate) dedupe that you MUST keep (Mono retries on 60s/600s).
if err := acquiring.VerifyWebhookFresh(pub, body, sig, 5*time.Minute); err != nil {
    // ErrWebhookStale / ErrWebhookNoTimestamp / ErrBadSignature
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

cli, err := installment.New(
    os.Getenv("CHAST_STORE_ID"),
    os.Getenv("CHAST_SECRET"),
    installment.WithBaseURL(installment.BaseURLSandbox),
)
if err != nil { return err } // ErrEmptyStoreID / ErrEmptySecret / ErrInsecureBaseURL

order, _ := cli.CreateOrder(ctx, &installment.CreateOrderRequest{
    StoreOrderID: "ORD-001",
    ClientPhone:  "+380501234561",                // ...1 = sandbox success
    TotalSum:     installment.NewMoney(2499, 99), // 2499.99 UAH (typed Money)
    Invoice: installment.CreateOrderInvoice{
        Number: "INV-001", Date: "2026-05-13",
        Source: installment.SourceInternet,
    },
    AvailablePrograms: []installment.Program{{AvailablePartsCount: []int{3, 6}}},
    Products: []installment.Product{
        {Name: "Kit", Count: 1, Sum: installment.NewMoney(2499, 99)},
    },
    ResultCallback: "https://shop/api/chast/cb",
})
// → poll cli.OrderState(ctx, order.OrderID) until WAITING_FOR_STORE_CONFIRM
// → ship the goods → cli.ConfirmOrder(ctx, order.OrderID)
```

Body signing is automatic (HMAC-SHA256 with your store secret). For
incoming bank callbacks: `cli.VerifyCallback(body, signatureHeader)`.
Wrong-length signature → `ErrCallbackBadLength`; wrong MAC →
`ErrCallbackSignatureMismatch` — separate sentinels for security
telemetry.

Every money field in installment (`TotalSum`, `Sum`, `Reverse.Sum`,
`Commission`, `CreditAmount`, …) is `installment.Money` — int64 kopecks
inside, exact-decimal MarshalJSON/UnmarshalJSON (string-based parsing,
no float64 in the hot path). `0.10`/`0.20`/`0.30` round-trip exactly.

## Quick start: Jar

```go
import "github.com/OlexiyOdarchuk/go-monobank-sdk/jar"

cli, err := jar.New()
if err != nil { return err }

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
- `WithUnsafeRetries(bool)` — by default POST/PATCH retries are gated on
  the `Idempotency-Key` header (a load-balancer 502 may arrive after the
  upstream already processed the request, so a blind retry would create
  a duplicate). Enable when you know your endpoint is idempotent or
  duplicates are acceptable.
- `APIError` — typed error with method, URL, received/expected status,
  parsed `ErrorDescription`, and the raw body. Implements `errors.Is`
  against the package sentinels (see below).
- `WithRequestHook` / `WithResponseHook` — plug in metrics or tracing
  without replacing the transport. These options overwrite each other —
  to ADD a hook on top of an existing one (e.g. application hook +
  OpenTelemetry): `Client.ChainRequestHook(fn)` / `ChainResponseHook(fn)`.
  For a full middleware chain use `WithRoundTripper(http.RoundTripper)`:
  a clean slot for OpenTelemetry / Datadog / circuit-breaker that
  preserves the inner `*http.Client`'s timeout and cookie jar.
- `WithUserAgent(string)` — overrides the default User-Agent
  (`go-monobank-sdk/vX.Y.Z (linux; goX.Y.Z)`). Mono uses it in
  support to disambiguate your service from other SDK users.
  Exported `monobank.UserAgent()` lets you keep the SDK part and
  prepend your own prefix.
- `Client.Close() error` — implements `io.Closer`; stops the
  background sweeper of a `KeyedLimiter`. Safe to call on a client
  without a limiter.

### Insecure base URL

By default, `WithBaseURL("http://...")` for a non-loopback host
returns `monobank.ErrInsecureBaseURL` from the first `Do` — a
config mistake should not silently send the X-Token in the clear.
Loopback is always allowed — both `localhost` and any IP for which
`net.IP.IsLoopback` is true (`127.0.0.0/8`, `::1`). Opt in
explicitly for MITM-proxy debugging or VPN-internal staging:

```go
cli := personal.New(token,
    monobank.WithInsecureBaseURL(true),
    monobank.WithBaseURL("http://staging.example.com"),
)
// Option order does NOT matter — New does a two-pass apply.
```

The same guard lives in `installment.New`, `jar.New`, and
`corporate.Client.Auth` (for the `X-Callback URL`). Each package has
its own `WithInsecureBaseURL` / `AllowInsecureCallback` for opt-out.

### Token redaction in logs

`auth.Personal`, `business.TokenAuth`, `acquiring.TokenAuth`,
`auth.CorpAuthMaker`, `installment.Client` implement
`slog.LogValuer` and render as `***` in slog output:

```go
slog.Info("auth", "creds", auth.NewPersonal(token))
// → ... creds=auth.Personal{token:***}
```

Protects against accidental token leaks in DEBUG logs via
`slog.Info("token", token)`. If you log the token explicitly via
`slog.String("token", token)` — that's on you.

The SDK also redacts PII in logs: `bank.ClientInfo` (Name → first
letter + `***`, WebHookURL path → `***`), `bank.Account` (IBAN →
country + last4, card masks → `****` + last4), and
`installment.ClientInfo` (full name / INN). All via `LogValue` —
the raw struct still behaves normally, but
`slog.Info("info", info)` never spills the customer's name into a
log aggregator.

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
// idleTTL=10*time.Minute — buckets idle for >10 min are GC'd by a
// background sweeper so the map stays bounded under churn.
klim := monobank.NewKeyedLimiter(time.Minute, 1, 10*time.Minute)
defer klim.Stop()

cli := personal.New(token, monobank.WithRateLimiter(klim))

for _, acc := range info.Accounts {
    ctx := monobank.WithLimiterKey(ctx, acc.AccountID)
    txs, _ := cli.Transactions(ctx, acc.AccountID, from, to)
    // …
}
```

Pass `idleTTL <= 0` to disable eviction (fine for short-lived CLIs;
long-running services should always pass a sensible TTL, e.g. ~10× the
`every` interval).

### Structured errors

Personal/corporate/business/acquiring APIs all return errors as
`{"errorDescription": "..."}`. The SDK parses that into
`APIError.ErrorDescription`:

```go
_, err := cli.ClientInfo(ctx)

// Sentinel errors for common control flow:
switch {
case errors.Is(err, monobank.ErrUnauthorized):    // 401
    log.Println("invalid token")
case errors.Is(err, monobank.ErrForbidden):       // 403
    log.Println("token lacks endpoint permission")
case errors.Is(err, monobank.ErrNotFound):        // 404
    log.Println("entity not found")
case errors.Is(err, monobank.ErrTooManyRequests): // 429
    log.Println("rate limit exceeded")
}

// Full APIError when you need ErrorDescription / raw body:
var apiErr *monobank.APIError
if errors.As(err, &apiErr) {
    log.Printf("HTTP %d: %s", apiErr.StatusCode, apiErr.ErrorDescription)
}
```

`installment` has its own format and its own `installment.APIError` with
`Message`, `TraceID` fields. Callback failures expose two sentinels:
`installment.ErrCallbackBadLength` (wrong-length header) and
`installment.ErrCallbackSignatureMismatch` (wrong MAC) — separate so
security telemetry can tell "malformed" from "forgery attempt".

Other useful sentinel errors:

- `monobank.{ErrEmptyRequest, ErrInvalidURL, ErrInsecureBaseURL}`
- `auth.ErrEmptyToken` — empty X-Token is refused on the first `Do`.
- `business.{ErrIdempotencyKeyRequired, ErrInvalidTimeRange, ErrNilRequest}`
- `installment.{ErrEmptyOrderID, ErrEmptyDate, ErrEmptyPhone, ErrInvalidPhone, ErrEmptyStoreID, ErrEmptySecret, ErrInsecureBaseURL}`
- `acquiring.{ErrEmptyID, ErrBadSignature, ErrWebhookStale, ErrWebhookNoTimestamp}`
- `corporate.{ErrInsecureCallback, ErrInvalidCallback, ErrLogoTooLarge, ErrEmptyAuthMaker, ErrNilRequest}`
- `jar.{ErrNotFound, ErrEmptyJarID, ErrEmptyClientID, ErrInsecureBaseURL}`
- `money.ErrOverflow` — int64 overflow in `Add/Sub/Mul`.

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
- `NewMemoryDeduper(n)` — LRU set keyed on `Data.Transaction.ID` (the
  transaction id inside the payload). Swap in your own `Deduper` for
  Redis/SQL — the in-memory variant loses state on restart, which makes
  replay of a previously-valid webhook possible.
- `Options.MaxBodyBytes` — request-body cap (default 1 MiB via
  `DefaultMaxBodyBytes`). Protects a publicly-exposed webhook URL from
  OOM by a hostile POST.
- ServerKey refreshes are coalesced via `singleflight` and throttled to
  once per 30 s — an attacker spamming POSTs with random `X-Key-Id`
  cannot amplify a DoS against `/bank/sync`.

### `acquiring` — server-side helpers & reconciliation

Beyond the raw endpoints, acquiring ships batteries for the common
production tasks:

```go
// 1. A drop-in http.Handler for acquiring webhooks (the acquiring twin of
//    webhook.NewHandler): verify → freshness → parse → dedup → callback.
//    Dedup by (invoiceId, modifiedDate); the key auto-refreshes on a bad sig.
h, _ := acquiring.NewWebhookHandler(ctx, acquiring.WebhookHandlerOptions{
    Keys:   cli,                              // *acquiring.Client (PubKeyProvider)
    Dedup:  acquiring.NewMemoryDeduper(4096), // use a persistent one in prod
    MaxAge: 15 * time.Minute,                 // anti-replay on top of dedup
    OnEvent: func(ctx context.Context, inv *acquiring.InvoiceStatusResponse) error {
        return fulfil(ctx, inv) // error → 500 → Mono retries; nil → 200
    },
})
mux.Handle("/mono/webhook", h)

// 2. Reconciliation: finish the status when a webhook is missed (the only way
//    to observe "expired", which never sends a webhook) + diff a statement.
inv, err := cli.PollInvoice(ctx, invoiceID, acquiring.PollOptions{
    Interval: time.Second, Timeout: 2 * time.Minute,
}) // → terminal state or acquiring.ErrPollTimeout
stmt, _ := cli.Statement(ctx, from, time.Time{}, "")
rec := acquiring.ReconcileStatement(stmt, local) // local: map[invoiceID]LocalPayment
if !rec.Clean() { /* rec.OnlyRemote / OnlyLocal / Mismatches */ }

// 3. Basket builder with validation (the classic mistakes: missing code, or
//    total != qty*sum). Build cross-checks the basket sum vs the invoice amount.
items, err := acquiring.NewBasket().
    AddItem("Coffee", "SKU-1", 2, money.UAH(45).Minor, acquiring.WithUnit("pcs")).
    AddItem("Tea", "SKU-2", 1, money.UAH(30).Minor).
    Build(money.UAH(120).Minor)

// 4. Typed API errors ({errCode, errText}) with predicates.
if _, err := cli.InvoiceStatus(ctx, id); err != nil {
    switch {
    case acquiring.IsNotFound(err):        // unknown invoiceId
    case acquiring.IsTooManyRequests(err): // back off
    }
    if e, ok := acquiring.AsAPIError(err); ok { log.Println(e.Code, e.Text) }
}

// 5. Subscription lifecycle classifier that drives grace logic.
switch acquiring.ClassifyCharge(payment.Status, sub.WalletData.Status) {
case acquiring.HealthChargeFailed: // transient — keep access in a grace window
case acquiring.HealthWalletDead:   // token dead — prompt the customer to re-link
case acquiring.HealthActive:       // ok
}
```

- `NewWebhookHandler` mirrors `webhook.NewHandler`: `singleflight` + throttle
  on key refresh, `MaxBodyBytes` (default 1 MiB), an `OnError` hook, ack-200
  for the GET subscription ping.
- `Deduper` / `NewMemoryDeduper(n)` — LRU keyed on `DedupKey(inv)` =
  `invoiceId|modifiedDate`.
- `InvoiceStatus.IsTerminal()` and `PollOptions.TreatHoldAsTerminal` — for
  auth-then-capture, where the target state is `hold`.
- `AsAPIError` / `Code` / `Is{BadRequest,Forbidden,NotFound,TooManyRequests,InternalError}`
  — fall back to the HTTP status when the body carries no `errCode`; keep the
  monobank sentinels matchable via `Unwrap`.
- `SubscriptionHealth`: `GraceEligible()`, `Terminal()`, `RetainAccess()`.

### `money` — typed amounts

```go
m := money.New(12550, currency.UAH)        // 125.50 UAH
k := money.UAH(125.50)                      // 12550 kopecks — no ×100 confusion
p, _ := money.ParseMajor("0.10", currency.UAH) // 10 kopecks, integer math (no float error)
fmt.Println(money.UAH(125.50).Major())     // 125.5 — back to major units
trip, err := m.Mul(3)                      // → ErrOverflow on MaxInt64*N
if err != nil { return err }
sum, err := m.Add(other)                   // currency mismatch → error
                                           // overflow → ErrOverflow
fmt.Println(trip)                          // "376.50 UAH"
fmt.Println(money.New(1250, currency.JPY)) // "1250 JPY" — 0 decimals
```

`Money` serializes as a plain integer (minor units) — wire-compatible with
mono's format. `Money.Major` / `String` honor `currency.Code.Decimals()`:
JPY/KRW (0 decimals), BHD/JOD/KWD/OMR/TND (3 decimals), UAH/USD/EUR/...
(2 decimals) all format correctly without a hardcoded ÷100. The helper
`currency.Code.MinorPerMajor()` returns `10^Decimals` for currency-aware
conversion between decimal-float and int64 minor.

`Add/Sub/Mul` return `ErrOverflow` on int64 boundary — silent wrap on
quarterly-statement-scale aggregates is no longer possible.

### `installment.Money` — UAH with exact-decimal wire format

The installment API is unusual in that it ships money on the wire as
`"total_sum": 2499.99` (decimal float), not as int64 minor units. To
avoid float64 precision loss, the package has a dedicated type:

```go
amt := installment.NewMoney(2499, 99)     // 2499.99 UAH
amt = installment.MoneyFromKopecks(249999)
amt = installment.MoneyFromMajor(2499.99) // rounds half away from zero

// MarshalJSON emits `2499.99` (no quotes); UnmarshalJSON parses via
// the digit string, no float64. 0.10, 0.20, 0.30 round-trip exactly.
```

### `currency`, `mcc` — enums

- `currency.Code(980).String()` → `"UAH"`; `currency.FromAlpha3("USD")` → `840`.
- `currency.UAH.Decimals()` → `2`; `currency.JPY.Decimals()` → `0`;
  `currency.BHD.Decimals()` → `3`.
- `mcc.Code(5411).Category()` → `mcc.CategoryGroceries`.
- `mcc.Code(3500).Category()` → `mcc.CategoryHotels` (3000-3299 airlines,
  3300-3499 car rental, 3500-3999 lodging).
- `mcc.Code(5912).Category()` → `mcc.CategoryHealth` (pharmacy override).

### Paginators

Every endpoint with a natural cursor ships an `iter.Seq2` wrapper:

- `personal.Client.TransactionsRangeIter` — lazy paging over a
  multi-window statement.
- `corporate.Client.TransactionsRangeIter` — same for corp-API.
- `business.Client.ContactsAll` / `SearchContactsAll` — salary
  contacts.
- `business.Client.StatementAll` — DOWN-cursor over time.
- `acquiring.Client.SubscriptionListAll` / `SubscriptionPaymentsAll` —
  subscriptions and their payments.

All honor `break` (stop further HTTP calls) and `ctx.Done()`.

### `monobanktest` — fake monobank for tests

```go
srv := monobanktest.NewServer(t)
srv.WithClientInfo(&bank.ClientInfo{ID: "c1", Name: "Test"})
cli := personal.New("token", srv.Option())

info, _ := cli.ClientInfo(ctx) // returns whatever you wired above
```

Builders cover the typical scenarios: client info, statements over a
range, rates, webhook events. Concurrency-safe.

For integration tests of acquiring webhook verification there is a signed-body
generator — no need to hand-roll ECDSA keys:

```go
signer := monobanktest.NewAcquiringWebhookSigner(t)         // P-256 keypair
srv := monobanktest.NewServer(t).WithAcquiringPubKey(signer) // serves /api/merchant/pubkey
cli := acquiring.New("tok", srv.Option())
h, _ := acquiring.NewWebhookHandler(ctx, acquiring.WebhookHandlerOptions{
    Keys: cli, OnEvent: onEvent,
})

body := signer.MarshalBody(t, &acquiring.InvoiceStatusResponse{
    InvoiceID: "p2_x", Status: acquiring.InvoiceSuccess,
})
req := signer.Request(t, "POST", "/webhook", body) // X-Sign set
h.ServeHTTP(rec, req)                              // signature verifies against the real key
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

Full godoc per package, type, and function at
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
