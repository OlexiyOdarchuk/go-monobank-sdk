package corporate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Settings is the payload of /personal/corp/settings: the profile of
// the company registered with Mono. Permission is a string of
// permission letters that may be requested in [Client.Auth] (for
// example "spf" means Statements, PersonalInfo, and FOP). Webhook is
// the currently configured URL (nil when none is set).
type Settings struct {
	Pubkey     string  `json:"pubkey"`
	Name       string  `json:"name"`
	Permission string  `json:"permission"`
	Logo       string  `json:"logo"`
	Webhook    *string `json:"webhook"`
}

// GetSettings returns the profile of the company registered with the
// bank. Useful for diagnostics (which permissions are approved, and
// whether the webhook is configured).
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
// Pass an empty string to UNSUBSCRIBE — the bank deletes the
// existing subscription. This is a destructive operation across
// every client of the service; if you only meant to validate the
// URI, do not pass "".
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
