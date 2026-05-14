package auth

import (
	"log/slog"
	"net/http"
)

const personalTokenHeader = "X-Token"

// Personal is the Authorizer that sends a Personal API token in the
// X-Token header.
type Personal struct {
	token string
}

// SetAuth adds the X-Token header to the request. A nil request is a
// no-op.
func (a Personal) SetAuth(r *http.Request) error {
	if r == nil {
		return nil
	}
	r.Header.Set(personalTokenHeader, a.token)
	return nil
}

// LogValue implements [slog.LogValuer] so the token does not leak
// into logs:
//
//	slog.Info("ready", "auth", auth.NewPersonal(tok))
//	// → ... auth=auth.Personal{token:***}
//
// Without it, slog would render the raw token as the struct's value.
func (a Personal) LogValue() slog.Value {
	return slog.StringValue("auth.Personal{token:***}")
}

// NewPersonal builds an Authorizer for the given Personal API token.
// You can issue a token at https://api.monobank.ua/.
func NewPersonal(token string) Personal { return Personal{token: token} }
