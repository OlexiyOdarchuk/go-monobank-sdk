package corporate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Settings is the payload of /personal/corp/settings: the profile of
// the company registered with Mono. Permission is the list of rights
// the service may request in [Client.Auth]; the spec says only "1
// letter per permission". The SDK's constants map the letters as
// [auth.PermSt] = "s", [auth.PermPI] = "p", [auth.PermFOP] = "f", so
// "spf" would mean all three. Webhook is the currently configured URL
// (nil when none is set).
type Settings struct {
	// ID is the company/registration identifier. The spec lists it as
	// required but ships no property schema; omitempty so an absent
	// value decodes cleanly rather than forcing a zero string.
	ID         string  `json:"id,omitempty"`
	Pubkey     string  `json:"pubkey"`
	Name       string  `json:"name"`
	Permission string  `json:"permission"`
	Logo       string  `json:"logo"`
	Webhook    *string `json:"webhook"`
}

// GetSettings returns the profile of the company registered with the
// bank. Useful for diagnostics (which permissions are approved, and
// whether the webhook is configured).
//
// The spec lists a required X-Request-Id header here, yet its X-Sign
// covers only "X-Time | URL" and no company-level request-id exists;
// the SDK sends no X-Request-Id. Unverified against the live API.
// https://api.monobank.ua/docs/corporate.html#tag/Avtoryzaciya-ta-nalashtuvannya-kompaniyi/paths/~1personal~1corp~1settings/get
func (c *Client) GetSettings(ctx context.Context) (*Settings, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/personal/corp/settings", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out Settings
	if err := c.do(req, c.authMaker.New(""), &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// webhookRequest is the body of POST /personal/corp/webhook.
type webhookRequest struct {
	WebHookURL string `json:"webHookUrl"`
}

// SetWebHook subscribes the given URI to StatementItem events for
// EVERY client this service has access to. Unlike a personal webhook
// (one per user), a corporate webhook is one per service. The event
// carries an AccountID so you can map it back to the client.
//
// Per the spec, the call sends "a test POST request to this address;
// if it is not answered with 200 OK, the API call fails" — so the
// endpoint must already be live and answering 200 when you call this.
//
// The spec's 200 response reads "webhook set or deleted", but it does
// not say how deletion is requested. Passing "" is the likely way and
// would affect every client of the service; this is not verified.
//
// The spec also lists a required X-Request-Id header here (with an
// X-Sign over "X-Time | URL" only); the SDK sends none, as for
// [Client.GetSettings]. Unverified against the live API.
// https://api.monobank.ua/docs/corporate.html#tag/Avtoryzaciya-ta-nalashtuvannya-kompaniyi/paths/~1personal~1corp~1webhook/post
func (c *Client) SetWebHook(ctx context.Context, uri string) error {
	body, err := json.Marshal(webhookRequest{WebHookURL: uri})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/personal/corp/webhook", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.do(req, c.authMaker.New(""), nil, http.StatusOK)
}
