// Package business — Go-клієнт для corp-api.monobank.ua, «API для
// роботи з рахунками юридичних осіб». Це окремий API від corporate
// Open API (підпакет [corporate]): інший хост, простіша авторизація
// (один X-Token), інша поверхня (свої рахунки/платежі компанії
// замість делегованих даних клієнтів).
//
// Авторизація — один заголовок X-Token, який видається у web-кабінеті
// https://web.monobank.ua/?modal=tokens. Без ECDSA-підпису.
//
// 23 endpoint-и групуються у шість тем:
//
//   - Рахунки: [Client.Accounts], [Client.Account], [Client.AccountBalances]
//   - Виписка: [Client.Statement] (пагінована), [Client.Operation] (одна
//     операція)
//   - Платежі: [Client.PreparePayment], [Client.PaymentState],
//     [Client.PaymentStateByReference]
//   - Зарплатні контакти: [Client.Contacts], [Client.SearchContacts],
//     [Client.ContactByID], [Client.CreateContact],
//     [Client.DeleteContact], [Client.DeleteContactsBatch]
//   - Зарплатні відомості: [Client.CreateSalaryRegistry],
//     [Client.SalaryRegistryTypes], [Client.SalaryRegistryStatus]
//   - Розрахункові листи (payslips): [Client.UploadPayslips],
//     [Client.DeletePayslips], [Client.ImportStatus],
//     [Client.DeleteImport], [Client.SendPayslipsToMobile],
//     [Client.PayslipPDF]
//
// Ідемпотентність: мутаційні endpoint-и з Idempotency-Key
// (PreparePayment, CreateSalaryRegistry) очікують свіжий UUID v4 на
// кожну логічну спробу — повтор з тим самим ключем безпечний.
//
// Rate limits: corp-api рахує квоти per-company. Поточний залишок —
// у заголовку X-Rate-Limit-Remaining; 429-відповіді несуть
// X-Rate-Limit-Retry-After-Seconds.
package business

import (
	"log/slog"
	"net/http"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

// BaseURL — дефолтний хост corp-api.monobank.ua. Перевизнач через
// [monobank.WithBaseURL] при створенні клієнта для тестів.
const BaseURL = "https://corp-api.monobank.ua"

// Client спілкується з corp-api.monobank.ua. Це тонка обгортка над
// [monobank.Client] — retry, transport та error-decoding переробляти
// не доводиться; цей пакет додає лише доменні типи й методи.
type Client struct {
	c monobank.Client
}

// New повертає [Client], авторизований вказаним X-Token. Додаткові
// опції (HTTP-клієнт, retry-політика тощо) пробрасуються в [monobank.New].
//
//	cli := business.New(os.Getenv("MONO_BUSINESS_TOKEN"))
//	accs, err := cli.Accounts(ctx)
func New(token string, opts ...monobank.Option) *Client {
	base := []monobank.Option{
		monobank.WithBaseURL(BaseURL),
		monobank.WithAuth(TokenAuth{Token: token}),
	}
	return &Client{c: monobank.New(append(base, opts...)...)}
}

// Close звільняє фонові ресурси клієнта (див. [monobank.Client.Close]).
func (c *Client) Close() error { return c.c.Close() }

// TokenAuth реалізує [auth.Authorizer] для X-Token-авторизації corp-api.
// Окрім X-Token, виставляє обов'язковий заголовок `Accept: application/json`,
// якого очікує corp-api на кожному запиті.
type TokenAuth struct {
	Token string
}

// SetAuth додає X-Token і Accept до запиту. nil-request — no-op.
func (a TokenAuth) SetAuth(r *http.Request) error {
	if r == nil {
		return nil
	}
	r.Header.Set("X-Token", a.Token)
	if r.Header.Get("Accept") == "" {
		r.Header.Set("Accept", "application/json")
	}
	return nil
}

// LogValue приховує токен у slog-виводі.
func (a TokenAuth) LogValue() slog.Value {
	return slog.StringValue("business.TokenAuth{Token:***}")
}
