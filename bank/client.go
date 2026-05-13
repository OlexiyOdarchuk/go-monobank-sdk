package bank

import (
	"context"
	"fmt"
	"net/http"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

// Client — обгортка над базовим [monobank.Client], що відкриває два
// публічні (неавторизовані) endpoint-и: курси валют і серверний ключ
// для верифікації webhook-ів. Зазвичай користувач створює один [Client]
// на застосунок і шерить його між викликами Rates та як KeyProvider для
// [webhook.Handler].
type Client struct {
	c monobank.Client
}

// New повертає [Client] для неавторизованих endpoint-ів. Опції
// (HTTP-клієнт, retry-політика, base URL для тестів) пробрасуються в
// базовий [monobank.New].
//
//	cli := bank.New()
//	rates, err := cli.Rates(ctx)
func New(opts ...monobank.Option) *Client {
	return &Client{c: monobank.New(opts...)}
}

// Rates тягне поточну таблицю курсів обміну з /bank/currency.
// Mono обмежує цей endpoint частотою — кешуй результат на хвилину-дві.
// Документація: https://api.monobank.ua/docs/#operation/getCurrency
func (c *Client) Rates(ctx context.Context) (Rates, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/bank/currency", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out Rates
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out, nil
}

// ServerKey тягне поточний публічний ключ банку з /bank/sync. Він
// використовується для верифікації підпису вхідних webhook-ів. Кешуй
// результат і перевикликай тільки тоді, коли вхідний X-Key-Id перестає
// збігатися з [ServerKey.ID]. [webhook.Handler] робить це автоматично.
// Документація: https://api.monobank.ua/docs/#operation/getServerKey
func (c *Client) ServerKey(ctx context.Context) (*ServerKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/bank/sync", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var raw bankSyncResponse
	if err := c.c.Do(req, &raw, http.StatusOK); err != nil {
		return nil, err
	}
	return raw.asServerKey()
}
