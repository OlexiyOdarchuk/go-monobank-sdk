package corporate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Settings — payload /personal/corp/settings: профіль компанії,
// зареєстрованої в Mono. Permission — рядок із літер дозволів, які
// можна запитувати в [Client.Auth] (наприклад "spf" означає Statements,
// PersonalInfo і ФОП). Webhook — поточний підписаний URL (nil, якщо не
// налаштовано).
type Settings struct {
	Pubkey     string  `json:"pubkey"`
	Name       string  `json:"name"`
	Permission string  `json:"permission"`
	Logo       string  `json:"logo"`
	Webhook    *string `json:"webhook"`
}

// GetSettings повертає профіль компанії, зареєстрованої в банку.
// Корисно для діагностики (бачити, які permissions схвалені, і чи
// налаштований webhook).
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

// webhookRequest — body POST /personal/corp/webhook.
type webhookRequest struct {
	WebHookURL string `json:"webHookUrl"`
}

// SetWebHook підписує вказаний URI на отримання подій StatementItem для
// КОЖНОГО клієнта, доступ до якого має цей сервіс. На відміну від
// personal-webhook (один на користувача), corporate-webhook — один на
// сервіс. Подія несе AccountID, щоб ти зміг звести її назад до клієнта.
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
