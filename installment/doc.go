// Package installment — Go-клієнт для API «Покупка частинами» Monobank
// (u2.monobank.com.ua). Дозволяє магазину створювати заявки на
// безвідсоткову розстрочку, відслідковувати їх статус, видавати товар,
// робити повернення та формувати гарантійні листи.
//
// Авторизація — HMAC-SHA256 підпис тіла запиту з парним заголовком
// store-id (ідентифікатор магазину) та signature (base64 від HMAC).
// Секрет видається банком окремо для тестового середовища, stage та
// production.
//
// Середовища (це базові URL для API-викликів, не сторінки документації;
// відкривати їх у браузері безглуздо — sandbox-root повертає 404,
// stage/prod — 503, бо вони закриті без store-id+signature):
//
//   - Тестове (sandbox)   — https://u2-demo-ext.mono.st4g3.com
//     Доступ публічний, працює з демо-credentials test_store_with_confirm
//     / secret_98765432--123-123. Документація — у Swagger UI:
//     https://u2-demo-ext.mono.st4g3.com/docs/index.html
//   - Stage (передпрод)   — https://u2-ext.mono.st4g3.com
//     Доступ лише після того, як банк відкриє ваш store-id; інакше 503.
//   - Production          — https://u2.monobank.com.ua
//     Доступ лише за продакшн-credentials, виданими банком; інакше 503.
//
// Перевизнач базовий URL через [WithBaseURL]; за дефолтом — production.
//
// Типовий потік (див. /api/order/create -> callback / polling):
//
//  1. [Client.CreateOrder] — створюєш заявку із товарами та номером
//     телефону клієнта; повертається order_id.
//  2. Клієнт отримує push у застосунку Mono та підтверджує договір.
//  3. Магазин отримує callback (POST на result_callback) або поллить
//     [Client.OrderState] до стану IN_PROCESS/WAITING_FOR_STORE_CONFIRM.
//  4. Магазин видає товар і викликає [Client.ConfirmOrder] — це
//     активує розстрочку і списується перший платіж.
//  5. У разі відмови — [Client.RejectOrder]; повернення товару —
//     [Client.ReturnOrder].
//
// Усі грошові поля — це гривні з копійками як `number` (float64).
// На відміну від інших API monobank-sdk, ПЧ не використовує мінорні
// одиниці; передавай 2499.99, а не 249999.
package installment
