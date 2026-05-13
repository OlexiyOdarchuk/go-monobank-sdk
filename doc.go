// Package monobank — базовий HTTP-транспорт для всіх поверхонь monobank API.
// Він публікує тип [Client], тип [Option] і набір конструкторів та опцій
// ([New], [WithHTTPClient], [WithHTTPDoer], [WithBaseURL], [WithRetry],
// [WithAuth]), а також спільний тип помилки [APIError].
//
// Прикладний код зазвичай тягне не цей пакет напряму, а тематичні
// підпакети, що сидять на ньому згори:
//
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/auth — інтерфейс
//     Authorizer та реалізації для персонального токена і корпоративного
//     ECDSA-підпису.
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/bank — публічні endpoint-и
//     банку (курси валют, серверний ключ) і спільна модель даних
//     (ClientInfo, Account, Jar, Transaction).
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/personal — Personal Open API
//     (авторизація через X-Token).
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/corporate — Corporate Open
//     API (ECDSA-підписи), включно з monoКЕП.
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/business — corp-api
//     (юр. особи): зарплатні контакти й відомості, платежі,
//     розрахункові листи.
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring — еквайринг
//     (/api/merchant/*): інвойси, холди, QR-каси, токенізовані картки.
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/webhook — серверна сторона:
//     верифікація підпису, парсер payload-у, готовий http.Handler і
//     in-memory deduper.
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/mcc — типізований ISO 18245
//     MCC enum із групуванням у категорії ([mcc.Code.Category]).
//   - github.com/OlexiyOdarchuk/go-monobank-sdk/currency — типізований
//     ISO 4217 числовий код валюти з alpha-3 ім'ям.
//
// Базовий клієнт ([Client]) уже всередині кожного підпакета: ти не
// конструюєш його окремо для рутинного коду.
package monobank
