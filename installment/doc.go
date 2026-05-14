// Package installment is the Go client for monobank's "Pay in
// installments" API (u2.monobank.com.ua). It lets a merchant create
// interest-free installment orders, track their status, hand over
// the goods, issue refunds, and produce guarantee letters.
//
// Authorization uses an HMAC-SHA256 signature of the request body
// paired with the headers store-id (store identifier) and signature
// (base64 of the HMAC). The bank issues the secret separately for
// the sandbox, stage, and production environments.
//
// Environments (these are API hostnames, not documentation pages;
// opening them in a browser is pointless — sandbox-root returns 404,
// stage/prod return 503 because they are closed without
// store-id+signature):
//
//   - Sandbox          — https://u2-demo-ext.mono.st4g3.com
//     Publicly accessible, works with the demo credentials
//     test_store_with_confirm / secret_98765432--123-123. Docs in
//     Swagger UI:
//     https://u2-demo-ext.mono.st4g3.com/docs/index.html
//   - Stage (pre-prod) — https://u2-ext.mono.st4g3.com
//     Available only after the bank opens your store-id; otherwise
//     503.
//   - Production       — https://u2.monobank.com.ua
//     Available only with production credentials issued by the bank;
//     otherwise 503.
//
// Override the base URL via [WithBaseURL]; the default is production.
//
// Typical flow (see /api/order/create -> callback / polling):
//
//  1. [Client.CreateOrder] — create an order with items and the
//     client's phone number; you receive an order_id.
//  2. The client receives a push in the Mono app and confirms the
//     contract.
//  3. The merchant gets a callback (POST to result_callback) or
//     polls [Client.OrderState] until it reaches
//     IN_PROCESS/WAITING_FOR_STORE_CONFIRM.
//  4. The merchant ships the goods and calls [Client.ConfirmOrder]
//     — this activates the installment and the first payment is
//     charged.
//  5. On rejection — [Client.RejectOrder]; for returns —
//     [Client.ReturnOrder].
//
// Every monetary field is hryvnias-with-kopecks as a `number`
// (float64). Unlike the other monobank-sdk APIs, the installment API
// does not use minor units; pass 2499.99, not 249999.
package installment
