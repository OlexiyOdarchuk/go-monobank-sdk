// Package installment is the Go client for monobank's "Pay in
// installments" API (u2.monobank.com.ua). It lets a merchant create
// interest-free installment orders, track their status, hand over
// the goods, issue refunds, produce guarantee letters, and attach a
// QR cart to a store QR code for the client to scan
// ([Client.CreateQRCartWithResponse] / [Client.CancelQRCart]).
//
// Authorization uses an HMAC-SHA256 signature of the request body
// paired with the headers store-id (store identifier) and signature
// (base64 of the HMAC). The bank issues the secret separately for
// the sandbox, stage, and production environments.
//
// Environments (these are API hostnames, not documentation pages):
//
//   - Sandbox          — https://u2-demo-ext.mono.st4g3.com
//     Publicly accessible, works with the demo credentials
//     test_store_with_confirm / secret_98765432--123-123. Docs in
//     Swagger UI:
//     https://u2-demo-ext.mono.st4g3.com/docs/index.html
//   - Stage (pre-prod) — https://u2-ext.mono.st4g3.com
//     Integration with the real Mono app. The docs publish shared
//     credentials COMFY / sign_key and a list of test QR ids (see
//     the "Інтеграція через QR" section). The COMFY store is shared by
//     all partners: on a 409 for a QR code pick another one, cancel
//     only your own carts, and use a fresh store_order_id per request.
//   - Production       — https://u2.monobank.com.ua
//     Production credentials are issued by the bank after testing.
//
// Override the base URL via [WithBaseURL]; the default is production.
//
// Typical flow (see /api/order/create -> callback / polling):
//
//  1. [Client.CreateOrder] — create an order with items and the
//     client's phone number; you receive an order_id.
//  2. The client receives a push in the Mono app, confirms the
//     contract, and the first payment is charged.
//  3. The merchant gets a callback (POST to result_callback) or
//     polls [Client.OrderState] until it reaches
//     IN_PROCESS/WAITING_FOR_STORE_CONFIRM.
//  4. The merchant hands over the goods and calls
//     [Client.ConfirmOrder]; the order moves to SUCCESS/ACTIVE and
//     the installment becomes active.
//  5. On rejection — [Client.RejectOrder]; for returns —
//     [Client.ReturnOrder].
//
// On the wire every monetary field is a JSON number of hryvnias with
// kopecks (2499.99), not minor units as in the other monobank APIs.
// The SDK models them as [Money], an exact count of kopecks:
// [NewMoney](2499, 99) or [MoneyFromKopecks](249999) both send
// 2499.99.
package installment
