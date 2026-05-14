package auth

import (
	"log/slog"
	"net/http"
)

const personalTokenHeader = "X-Token"

// Personal — Authorizer, який передає токен Personal API у заголовку X-Token.
type Personal struct {
	token string
}

// SetAuth додає заголовок X-Token до запиту. nil-request — no-op.
func (a Personal) SetAuth(r *http.Request) error {
	if r == nil {
		return nil
	}
	r.Header.Set(personalTokenHeader, a.token)
	return nil
}

// LogValue реалізує [slog.LogValuer], щоб токен не потрапляв у логи:
//
//	slog.Info("ready", "auth", auth.NewPersonal(tok))
//	// → ... auth=auth.Personal{token:***}
//
// Без цього slog рендерив би сирий токен як значення struct-у.
func (a Personal) LogValue() slog.Value {
	return slog.StringValue("auth.Personal{token:***}")
}

// NewPersonal створює Authorizer для вказаного токена Personal API.
// Токен можна отримати на https://api.monobank.ua/.
func NewPersonal(token string) Personal { return Personal{token: token} }
