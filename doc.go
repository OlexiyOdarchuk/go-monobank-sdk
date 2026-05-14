// Package monobank is the base HTTP transport for every monobank API
// surface. It exposes the [Client] type, the [Option] type, and a set
// of constructors and options ([New], [WithHTTPClient], [WithHTTPDoer],
// [WithBaseURL], [WithRetry], [WithAuth], [WithRateLimiter],
// [WithUnsafeRetries], [WithLogger], [WithRequestHook],
// [WithResponseHook]), plus the shared error type [APIError] and
// sentinel errors keyed by HTTP status ([ErrUnauthorized],
// [ErrForbidden], [ErrNotFound], [ErrTooManyRequests]).
//
// Throttling: [NewLimiter] is a simple token bucket; [NewKeyedLimiter]
// gives per-key buckets (for example, per accountID) with optional
// TTL eviction; [WithLimiterKey] propagates the key through
// context.Context.
//
// Application code usually does not pull in this package directly,
// but rather the topical sub-packages built on top of it:
//
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/auth — the Authorizer
//     interface plus implementations for the personal token and the
//     corporate ECDSA signature.
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/bank — the bank's
//     public endpoints (currency rates, server key) and the shared
//     data model (ClientInfo, Account, Jar, Transaction).
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/personal — Personal
//     Open API (authorization via X-Token).
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/corporate — Corporate
//     Open API (ECDSA signatures), including monoKEP.
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/business — corp-api
//     (legal entities): payroll contacts and rosters, payments,
//     payslips.
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring — acquiring
//     (/api/merchant/*): invoices, holds, QR cash desks, tokenized
//     cards.
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/webhook — the server
//     side: signature verification, payload parser, a ready
//     http.Handler, and an in-memory deduper.
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/mcc — a typed ISO 18245
//     MCC enum with grouping into categories ([mcc.Code.Category]).
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/currency — a typed
//     ISO 4217 numeric currency code with its alpha-3 name.
//
// The base client ([Client]) is already embedded in each sub-package:
// you don't construct it separately for routine code.
package monobank
