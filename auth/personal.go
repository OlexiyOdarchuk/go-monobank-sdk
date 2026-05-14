package auth

import (
	"errors"
	"log/slog"
	"net/http"
)

const personalTokenHeader = "X-Token"

// ErrEmptyToken is returned by [Personal.SetAuth] when the Personal
// API token is empty. Returning the error at request time (rather
// than at construction) keeps [NewPersonal]'s signature unchanged
// while still preventing an unauthenticated request from going on
// the wire.
var ErrEmptyToken = errors.New("auth: Personal token is empty")

// Personal is the Authorizer that sends a Personal API token in the
// X-Token header.
type Personal struct {
	token string
}

// SetAuth adds the X-Token header to the request. A nil request is a
// no-op. An empty token surfaces as [ErrEmptyToken] — without this
// guard the SDK would happily send an empty X-Token, which Mono
// replies to with a generic 403, masking the real problem
// (forgotten/empty env var).
func (a Personal) SetAuth(r *http.Request) error {
	if r == nil {
		return nil
	}
	if a.token == "" {
		return ErrEmptyToken
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
// You can issue a token at https://api.monobank.ua/. An empty token
// is accepted at construction but rejected at request time with
// [ErrEmptyToken] — fail-fast on the request rather than at every
// place the token might be read from config.
func NewPersonal(token string) Personal { return Personal{token: token} }
