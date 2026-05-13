package auth

import "net/http"

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

// NewPersonal створює Authorizer для вказаного токена Personal API.
// Токен можна отримати на https://api.monobank.ua/.
func NewPersonal(token string) Personal { return Personal{token: token} }
