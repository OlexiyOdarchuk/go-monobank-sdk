package corporate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
)

// ErrInsecureCallback is returned by [Client.Auth] when the
// callbackURL points at a non-loopback host over an http:// scheme.
// Mono will POST your full requestID to that URL when the client
// approves access — over plain http, anyone on the path can read it
// and impersonate your service. Override with
// [Client.AllowInsecureCallback] when you really need an http
// callback (debugging through a proxy).
var ErrInsecureCallback = errors.New("corporate: X-Callback must be https for non-loopback hosts")

// ErrInvalidCallback is returned by [Client.Auth] when callbackURL
// is empty, missing a scheme, or fails to parse.
var ErrInvalidCallback = errors.New("corporate: X-Callback is empty or unparseable")

// TokenRequest is the response from POST /personal/auth/request.
// RequestID is the short-lived access-request identifier; AcceptURL
// is the link to redirect the client to (or embed in a QR code) so
// they can approve access in the Mono mobile app.
type TokenRequest struct {
	RequestID string `json:"tokenRequestId"`
	AcceptURL string `json:"acceptUrl"`
}

const urlPathAuth = "/personal/auth/request"

// AllowInsecureCallback toggles the opt-out for the X-Callback
// scheme guard in [Client.Auth]. Call it once during setup if you
// really need an http callback (debugging via a recorded proxy).
//
//	cli.AllowInsecureCallback(true)
//	tok, err := cli.Auth(ctx, "http://debug.internal/cb", auth.PermSt)
func (c *Client) AllowInsecureCallback(allow bool) {
	c.allowInsecureCallback = allow
}

// Auth initiates a request for access to a client's data. callbackURL
// is sent in X-Callback — Mono POSTs to it when the client approves
// access (an alternative to polling via [Client.CheckAuth]). An empty
// permissions list means "all permissions"; pass a combination of
// [auth.PermSt], [auth.PermPI], [auth.PermFOP] to narrow the scope.
//
// SECURITY: callbackURL must be https on non-loopback hosts. Over
// plain http the requestID Mono returns in the callback can be
// captured and replayed against the bank to mimic your service.
// Override deliberately via [Client.AllowInsecureCallback] for
// debugging through a recorded proxy.
// https://api.monobank.ua/docs/corporate.html#tag/Klyentski-personalni-dani/paths/~1personal~1auth~1request/post
func (c *Client) Auth(ctx context.Context, callbackURL string, permissions ...auth.Permission) (*TokenRequest, error) {
	if err := validateCallbackURL(callbackURL, c.allowInsecureCallback); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlPathAuth, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Callback", callbackURL)

	var out TokenRequest
	if err := c.do(req, c.authMaker.NewPermissions(permissions...), &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// validateCallbackURL refuses empty / unparseable URLs and http:// on
// non-loopback hosts unless explicitly allowed. Mirrors the loopback
// definition used by [monobank.WithBaseURL].
func validateCallbackURL(callbackURL string, allowInsecure bool) error {
	if callbackURL == "" {
		return fmt.Errorf("%w: empty", ErrInvalidCallback)
	}
	u, err := url.Parse(callbackURL)
	if err != nil || u == nil || u.Scheme == "" {
		return fmt.Errorf("%w: %q", ErrInvalidCallback, callbackURL)
	}
	if u.Scheme == "https" {
		return nil
	}
	if allowInsecure {
		return nil
	}
	host := u.Hostname()
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("%w: got %q", ErrInsecureCallback, callbackURL)
}

// CheckAuth checks the status of an access request by requestID. It
// returns nil when the client has approved access; otherwise it
// returns a [monobank.APIError] with StatusCode 403 (request not yet
// approved) or another code for more fatal failures. Polling every
// 3-5 seconds is a typical strategy.
// https://api.monobank.ua/docs/corporate.html#tag/Klyentski-personalni-dani/paths/~1personal~1auth~1request/get
func (c *Client) CheckAuth(ctx context.Context, requestID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPathAuth, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.do(req, c.authMaker.New(requestID), nil, http.StatusOK)
}
