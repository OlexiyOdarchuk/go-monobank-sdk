// Package personal — клієнт Personal Open API monobank: окрема людина,
// авторизується одним X-Token, що видається на https://api.monobank.ua/.
//
// Усі типи відповідей беруться з підпакета [bank] (ClientInfo, Account,
// Jar, Transaction…) — вони спільні з corporate-клієнтом, бо банк
// повертає однакові форми незалежно від способу авторизації.
//
// Rate limits: Mono обмежує /personal/client-info — один виклик на
// 60 с; /personal/statement — один виклик на акаунт на 60 с. Інші
// endpoint-и теж лімітуються — обробляй 429 з backoff-ом
// ([monobank.WithRetry] робить це автоматично через Retry-After).
//
// Вікно виписки — максимум 31 доба за один виклик. Для ширших
// діапазонів є [Client.TransactionsRange] — він прозоро пагінується.
package personal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// Client — Open API клієнт із персональним токеном.
type Client struct {
	c monobank.Client
}

// New повертає [Client] із вказаним персональним токеном. Додаткові
// опції (HTTP-клієнт, retry-політика, base URL для тестів) пробрасуються
// у базовий [monobank.New].
//
//	cli := personal.New(os.Getenv("MONO_TOKEN"))
//	info, err := cli.ClientInfo(ctx)
func New(token string, opts ...monobank.Option) *Client {
	base := []monobank.Option{monobank.WithAuth(auth.NewPersonal(token))}
	return &Client{c: monobank.New(append(base, opts...)...)}
}

// ClientInfo повертає те, що банк знає про авторизованого користувача
// (ім'я, рахунки, банки). Rate limit: 1 виклик на 60 с.
// https://api.monobank.ua/docs/#tag/Klientski-personalni-dani/paths/~1personal~1client-info/get
func (c *Client) ClientInfo(ctx context.Context) (*bank.ClientInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/personal/client-info", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out bank.ClientInfo
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// Transactions повертає записи виписки по рахунку accountID за період
// [from, to] (включно). Mono приймає максимум 31 добу за один виклик;
// для ширших діапазонів використовуй [Client.TransactionsRange].
// Rate limit: 1 виклик на рахунок на 60 с.
// https://api.monobank.ua/docs/#tag/Klientski-personalni-dani/paths/~1personal~1statement~1{account}~1{from}~1{to}/get
func (c *Client) Transactions(ctx context.Context, accountID string, from, to time.Time) (bank.Transactions, error) {
	uri := "/personal/statement/" + accountID +
		"/" + strconv.FormatInt(from.Unix(), 10) +
		"/" + strconv.FormatInt(to.Unix(), 10)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out bank.Transactions
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out, nil
}

// TransactionsRange повертає виписку за довільний діапазон, нарізаючи
// його на послідовні 31-денні вікна (ліміт Mono на один виклик) і
// зчіплюючи результати у хронологічному порядку.
//
// Якщо to нульовий або раніше за from — повертає nil, nil (без помилки).
// Зважай на rate limit: 1 виклик на рахунок на 60 с — для тижневих
// діапазонів це 1 запит, для квартальних може бути 3-4.
func (c *Client) TransactionsRange(ctx context.Context, accountID string, from, to time.Time) (bank.Transactions, error) {
	if to.IsZero() || !to.After(from) {
		return nil, nil
	}

	var all bank.Transactions
	for cursor := from; cursor.Before(to); {
		end := cursor.Add(bank.MaxStatementWindow)
		if end.After(to) {
			end = to
		}
		chunk, err := c.Transactions(ctx, accountID, cursor, end)
		if err != nil {
			return nil, fmt.Errorf("range %s..%s: %w", cursor.Format(time.RFC3339), end.Format(time.RFC3339), err)
		}
		all = append(all, chunk...)
		cursor = end
	}
	return all, nil
}

// webhookRequest — body POST /personal/webhook.
type webhookRequest struct {
	WebHookURL string `json:"webHookUrl"`
}

// SetWebHook підписує вказаний URI на отримання подій типу
// StatementItem. Mono пінгне URI через GET одразу після підписки, щоб
// перевірити, що він живий (відповідай 200). Передай порожній рядок,
// щоб скасувати підписку.
// https://api.monobank.ua/docs/#tag/Klientski-personalni-dani/paths/~1personal~1webhook/post
func (c *Client) SetWebHook(ctx context.Context, uri string) error {
	body, err := json.Marshal(webhookRequest{WebHookURL: uri})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/personal/webhook", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}
