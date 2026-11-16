// Package corporate is the client for monobank's Corporate Open API,
// including monoKEP (digital document signatures). Every request is
// ECDSA-signed with X-Time and X-Sign. The SDK also sends X-Key-Id on
// every call, although the spec lists it for neither registration
// endpoint (there is no approved key id yet at that point). Calls
// about an approved client also carry its request-id in X-Request-Id;
// the initial /personal/auth/request carries the requested
// permissions in X-Permissions (see [Client.Auth]).
//
// Full client onboarding:
//
//  1. [Client.Register] (POST /personal/auth/registration) — send
//     your pubkey + company metadata to the bank. Wait for manual
//     approval.
//  2. [Client.RegistrationStatus] — poll periodically until Status
//     becomes "Approved".
//  3. [Client.Auth] — request access to a specific client's data,
//     limited by permissions ([auth.PermSt], [auth.PermPI],
//     [auth.PermFOP]). Show the returned AcceptURL to the client.
//  4. [Client.CheckAuth] — poll periodically until the client
//     approves (a 401 means still pending, see [IsAuthPending]), or
//     wait for the bank's GET request to your X-Callback URL.
//  5. [Client.ClientInfo] / [Client.Transactions] / [Client.SetWebHook]
//     — call with the request-id from step 3.
//
// Auxiliary surfaces:
//
//   - [Client.GetSettings] — your company's profile (pubkey, name,
//     logo, which permissions you may request, configured webhook).
//   - monoKEP: [Client.SignatureCreate] / [Client.SignatureStatus] /
//     [Client.SignatureCancel] — create a deeplink that the signer
//     opens in the Mono mobile app to sign documents.
package corporate

import (
	"errors"
	"net/http"
	"time"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/auth"
)

// ErrEmptyAuthMaker is returned from [New] when authMaker is nil.
var ErrEmptyAuthMaker = errors.New("authMaker is nil")

// ErrNilRequest is returned from endpoints that take a body when nil
// is passed.
var ErrNilRequest = errors.New("request body is nil")

// Client is the corporate Open API client.
type Client struct {
	c         monobank.Client
	authMaker auth.CorpAuthMakerAPI
	// allowInsecureCallback bypasses the https check for X-Callback
	// in [Client.Auth]. Default false.
	allowInsecureCallback bool

	// now is a test seam for the range helpers; nil means time.Now.
	now func() time.Time
}

func (c *Client) clock() time.Time {
	if c.now != nil {
		return c.now()
	}

	return time.Now()
}

// New returns a corporate [Client] that uses authMaker to sign every
// outgoing request. authMaker is usually built via
// [auth.NewCorpAuthMaker].
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

// Close releases the client's background resources (see
// [monobank.Client.Close]).
func (c *Client) Close() error { return c.c.Close() }

// do applies the given per-request authorizer to req and then dispatches
// it through the base client (whose own auth is auth.Public — a no-op).
func (c *Client) do(req *http.Request, a auth.Authorizer, v any, statuses ...int) error {
	if err := a.SetAuth(req); err != nil {
		return err
	}
	return c.c.Do(req, v, statuses...)
}
