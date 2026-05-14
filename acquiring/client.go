// Package acquiring — Go-клієнт для еквайрингового API monobank
// (api.monobank.ua/api/merchant/*). Покриває:
//
//   - інвойси, холди, QR-каси, токенізовані картки (wallet);
//   - регулярні платежі (subscriptions: create/edit/remove/status/list/payments);
//   - monopay-кнопка: імпорт/видалення/перегляд ключів торговця;
//   - розщеплення платежів (split-receivers) та T2P-термінали (термінал
//     у смартфоні);
//   - періодичні виписки, фіскальні чеки, квитанції, субмерчантів.
//
// Авторизація — один X-Token, отриманий для конкретного мерчанта.
// Цей токен НЕ той самий, що Personal API і не той, що business
// (corp-api) — тримай їх окремо.
//
// Верифікація webhook-ів — ECDSA-SHA256 з NIST P-256 (ASN.1 DER підпис у
// заголовку X-Sign). Ключ із [Client.PubKey] приходить як base64(PEM(SPKI));
// [ServerKey.Public] / [ParsePubKey] зніме обидві обгортки і поверне
// готовий *ecdsa.PublicKey для [VerifyWebhook].
//
// Типові сценарії:
//
//   - Одношагова оплата (debit): [Client.CreateInvoice] із
//     PaymentType: PaymentDebit → показати inv.PageURL → чекати
//     webhook або поллити [Client.InvoiceStatus].
//   - Auth-then-capture: [Client.CreateInvoice] із PaymentType:
//     PaymentHold → клієнт оплачує → статус "hold" → [Client.FinalizeInvoice]
//     знімає частину або всю заавторизовану суму.
//   - Рекурент через токенізацію: перший інвойс із SaveCardData.SaveCard,
//     на success-webhook прийде WalletData.CardToken — далі плати через
//     [Client.WalletPayment].
//   - Підписки (регулярні платежі): [Client.SubscriptionCreate] → клієнт
//     платить перший раз → банк автоматично списує далі за interval.
//     Слухай WebHookURLs.ChargeURL / StatusURL.
//   - QR-каси: [Client.QRList] / [Client.QRDetails] /
//     [Client.QRResetAmount] для терміналоподібних сценаріїв.
//   - Повернення: [Client.CancelInvoice] (повне або часткове).
//   - Звірка: [Client.Statement] за період; у кожному рядку CancelList
//     несе історію повернень.
//
// Прямі PAN-потоки ([Client.PaymentDirect], [Client.SyncPayment])
// вимагають PCI DSS scope — мерчант має тримати або проксіювати через
// сертифіковане оточення, яке може приймати сирі дані карток.
package acquiring

import (
	"log/slog"
	"net/http"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

// BaseURL — хост еквайрингового API. Перевизнач через
// [monobank.WithBaseURL] у тестах.
const BaseURL = "https://api.monobank.ua"

// Client спілкується з api.monobank.ua/api/merchant/*. Обгортка над
// [monobank.Client] для HTTP-плюмбінгу (retry, transport, маппінг
// помилок) плюс типізовані методи й DTO-шки для еквайрингу.
type Client struct {
	c monobank.Client
}

// New повертає [Client], авторизований вказаним X-Token. Додаткові
// опції (HTTP-клієнт, retry-політика) пробрасуються в [monobank.New].
//
//	cli := acquiring.New(os.Getenv("MONO_ACQUIRING_TOKEN"))
//	out, err := cli.MerchantDetails(ctx)
func New(token string, opts ...monobank.Option) *Client {
	base := []monobank.Option{
		monobank.WithBaseURL(BaseURL),
		monobank.WithAuth(TokenAuth{Token: token}),
	}
	return &Client{c: monobank.New(append(base, opts...)...)}
}

// Close звільняє фонові ресурси клієнта (див. [monobank.Client.Close]).
func (c *Client) Close() error { return c.c.Close() }

// TokenAuth реалізує [auth.Authorizer] для X-Token-авторизації
// еквайрингу.
type TokenAuth struct {
	Token string
}

// SetAuth додає X-Token до вихідного запиту. nil-request — no-op.
func (a TokenAuth) SetAuth(r *http.Request) error {
	if r == nil {
		return nil
	}
	r.Header.Set("X-Token", a.Token)
	return nil
}

// LogValue приховує токен у slog-виводі.
func (a TokenAuth) LogValue() slog.Value {
	return slog.StringValue("acquiring.TokenAuth{Token:***}")
}
