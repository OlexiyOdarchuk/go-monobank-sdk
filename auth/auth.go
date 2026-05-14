// Package auth provides authorizers for monobank API requests.
//
// The [Authorizer] interface lets the base HTTP client stay agnostic
// about how a given request is authorized. Three implementations cover
// every public Mono surface:
//
//   - [Public] — for endpoints without authorization (rates, server key).
//   - [Personal] — Personal API token (X-Token header).
//   - [Corp] — ECDSA-signed Corporate API access (X-Key-Id, X-Time,
//     X-Sign, plus either X-Request-Id or X-Permissions depending on
//     the endpoint).
//
// The business (legal-entity corp-api) and acquiring sub-packages use
// a simpler single X-Token scheme and have their own authorizers in
// their respective packages.
package auth

import "net/http"

// Authorizer mutates an outgoing request to add the credentials a
// specific monobank surface expects.
type Authorizer interface {
	SetAuth(*http.Request) error
}

// Public is a no-op Authorizer for endpoints that do not require
// authorization (for example /bank/currency, /bank/sync).
type Public struct{}

// SetAuth implements [Authorizer]; it does nothing.
func (Public) SetAuth(_ *http.Request) error { return nil }

// NewPublic returns a no-op Authorizer for unauthorized calls.
func NewPublic() Public { return Public{} }
