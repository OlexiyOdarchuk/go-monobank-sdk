package corporate

import (
	"context"
	"fmt"
	"net/http"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
)

// TokenRequest is the response from POST /personal/auth/request.
// RequestID is the short-lived access-request identifier; AcceptURL
// is the link to redirect the client to (or embed in a QR code) so
// they can approve access in the Mono mobile app.
type TokenRequest struct {
	RequestID string `json:"tokenRequestId"`
	AcceptURL string `json:"acceptUrl"`
}

const urlPathAuth = "/personal/auth/request"

// Auth initiates a request for access to a client's data. callbackURL
// is sent in X-Callback — Mono POSTs to it when the client approves
// access (an alternative to polling via [Client.CheckAuth]). An empty
// permissions list means "all permissions"; pass a combination of
// [auth.PermSt], [auth.PermPI], [auth.PermFOP] to narrow the scope.
// https://api.monobank.ua/docs/corporate.html#tag/Klyentski-personalni-dani/paths/~1personal~1auth~1request/post
func (c *Client) Auth(ctx context.Context, callbackURL string, permissions ...auth.Permission) (*TokenRequest, error) {
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
