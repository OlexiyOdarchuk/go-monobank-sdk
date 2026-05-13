package corporate

import (
	"context"
	"fmt"
	"net/http"
)

// TokenRequest — відповідь POST /personal/auth/request. RequestID —
// короткоживучий ідентифікатор запиту на доступ; AcceptURL — посилання,
// на яке треба перенаправити клієнта (або вшити в QR-код), щоб він
// підтвердив доступ у застосунку Mono.
type TokenRequest struct {
	RequestID string `json:"tokenRequestId"`
	AcceptURL string `json:"acceptUrl"`
}

const urlPathAuth = "/personal/auth/request"

// Auth ініціює запит на доступ до даних клієнта. callbackURL шлеться у
// X-Callback — Mono POST-не на нього, коли клієнт підтвердить доступ
// (як альтернатива поллінгу через [Client.CheckAuth]). Порожній
// permissions означає «усі дозволи»; передай комбінацію з
// [auth.PermSt], [auth.PermPI], [auth.PermFOP] для звуження scope.
// https://api.monobank.ua/docs/corporate.html#tag/Klyentski-personalni-dani/paths/~1personal~1auth~1request/post
func (c *Client) Auth(ctx context.Context, callbackURL string, permissions ...string) (*TokenRequest, error) {
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

// CheckAuth перевіряє статус запиту доступу за requestID. Повертає nil,
// коли клієнт підтвердив доступ; інакше — [monobank.APIError] зі
// StatusCode 403 (запит ще не схвалений) або іншим кодом для більш
// фатальних помилок. Полінг кожні 3-5 секунд — типова стратегія.
// https://api.monobank.ua/docs/corporate.html#tag/Klyentski-personalni-dani/paths/~1personal~1auth~1request/get
func (c *Client) CheckAuth(ctx context.Context, requestID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPathAuth, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.do(req, c.authMaker.New(requestID), nil, http.StatusOK)
}
