// Package corporate — клієнт Corporate Open API monobank, включно з
// monoКЕП (цифровим підписом документів). Кожен запит ECDSA-підписаний
// (X-Key-Id, X-Time, X-Sign); scope per-call — або request-id (після
// схвалення клієнтом), або набір permissions (для початкового
// /personal/auth/request).
//
// Повний онбординг клієнта:
//
//  1. [Client.Register] (POST /personal/auth/registration) — відправ
//     свій pubkey + метадані компанії банку. Чекай ручного схвалення.
//  2. [Client.RegistrationStatus] — періодично перевіряй, поки
//     Status не стане "Approved".
//  3. [Client.Auth] — запит на доступ до даних конкретного клієнта,
//     обмежений permissions ([auth.PermSt], [auth.PermPI],
//     [auth.PermFOP]). Покажи клієнту повернутий AcceptURL.
//  4. [Client.CheckAuth] — періодично перевіряй, поки клієнт не
//     підтвердить (або чекай POST від банку на твій X-Callback URL).
//  5. [Client.ClientInfo] / [Client.Transactions] / [Client.SetWebHook]
//     — викликай із request-id з кроку 3.
//
// Допоміжні поверхні:
//
//   - [Client.GetSettings] — профіль твоєї компанії (pubkey, назва,
//     логотип, які permissions можна запитувати, налаштований webhook).
//   - monoКЕП: [Client.SignatureCreate] / [Client.SignatureStatus] /
//     [Client.SignatureCancel] — створити deeplink, який підписант
//     відкриває у mobile-апі Mono для підпису документів.
package corporate

import (
	"errors"
	"net/http"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
)

// ErrEmptyAuthMaker повертається з [New], коли authMaker — nil.
var ErrEmptyAuthMaker = errors.New("authMaker is nil")

// ErrNilRequest повертається з endpoint-ів, що приймають body, коли
// передано nil.
var ErrNilRequest = errors.New("request body is nil")

// Client — корпоративний Open API клієнт.
type Client struct {
	c         monobank.Client
	authMaker auth.CorpAuthMakerAPI
}

// New повертає корпоративний [Client], використовуючи authMaker для
// підпису кожного вихідного запиту. authMaker зазвичай створюється
// через [auth.NewCorpAuthMaker].
//
//	maker, _ := auth.NewCorpAuthMaker(privPEM)
//	cli, _ := corporate.New(maker)
//	tok, err := cli.Auth(ctx, "https://yourapp/cb", auth.PermSt)
func New(authMaker auth.CorpAuthMakerAPI, opts ...monobank.Option) (*Client, error) {
	if authMaker == nil {
		return nil, ErrEmptyAuthMaker
	}
	return &Client{
		c:         monobank.New(opts...),
		authMaker: authMaker,
	}, nil
}

// do applies the given per-request authorizer to req and then dispatches
// it through the base client (whose own auth is auth.Public — a no-op).
func (c *Client) do(req *http.Request, a auth.Authorizer, v any, statuses ...int) error {
	if err := a.SetAuth(req); err != nil {
		return err
	}
	return c.c.Do(req, v, statuses...)
}
