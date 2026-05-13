package installment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Стандартні базові URL для трьох середовищ ПЧ.
const (
	BaseURLSandbox    = "https://u2-demo-ext.mono.st4g3.com"
	BaseURLStage      = "https://u2-ext.mono.st4g3.com"
	BaseURLProduction = "https://u2.monobank.com.ua"
)

// HeaderStoreID — назва HTTP-заголовка ідентифікатора магазину.
const HeaderStoreID = "store-id"

// HeaderSignature — назва HTTP-заголовка з підписом.
const HeaderSignature = "signature"

// ErrNilRequest повертається з мутаційних методів, коли body == nil.
var ErrNilRequest = errors.New("installment: request body is nil")

// APIError — структура помилки сервера (тіло відповіді {"message": "..."}).
type APIError struct {
	StatusCode int
	Message    string `json:"message"`
	TraceID    string
}

// Error реалізує error.
func (e *APIError) Error() string {
	if e.TraceID != "" {
		return fmt.Sprintf("installment: %d: %s (trace=%s)", e.StatusCode, e.Message, e.TraceID)
	}
	return fmt.Sprintf("installment: %d: %s", e.StatusCode, e.Message)
}

// Option — функціональна опція [New].
type Option func(*Client)

// WithBaseURL перевизначає базовий URL (за дефолтом — production).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithHTTPClient підставляє кастомний http.Client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.h = h } }

// Client — клієнт API «Покупка частинами». Будуй через [New].
type Client struct {
	h       *http.Client
	baseURL string
	storeID string
	secret  []byte
}

// New повертає клієнт із дефолтним таймаутом 30с, налаштований на
// production. Для тестів використовуй [WithBaseURL] зі значенням
// [BaseURLSandbox] або [BaseURLStage].
//
//	cli := installment.New("test_store_with_confirm", "secret_98765432--123-123",
//	    installment.WithBaseURL(installment.BaseURLSandbox))
func New(storeID, secret string, opts ...Option) *Client {
	c := &Client{
		h:       &http.Client{Timeout: 30 * time.Second},
		baseURL: BaseURLProduction,
		storeID: storeID,
		secret:  []byte(secret),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Sign обчислює base64(HMAC-SHA256(body, secret)). Експортовано для
// тестів та для верифікації вхідних callback-запитів (підпис у заголовку
// signature рахується тим самим способом).
func (c *Client) Sign(body []byte) string {
	mac := hmac.New(sha256.New, c.secret)
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyCallback повертає nil, якщо HMAC-SHA256(body, secret) збігається з
// значенням заголовка signature, прийнятим у вхідному callback. Викликай
// перед обробкою тіла, інакше будь-хто може прислати фейковий запит.
func (c *Client) VerifyCallback(body []byte, signatureHeader string) error {
	want := c.Sign(body)
	if !hmac.Equal([]byte(want), []byte(signatureHeader)) {
		return errors.New("installment: callback signature mismatch")
	}
	return nil
}

// doJSON виконує POST з JSON-тілом, підписує його, перевіряє очікуваний
// статус і декодує відповідь у out (якщо out != nil).
func (c *Client) doJSON(ctx context.Context, path string, in, out any, wantStatus int) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("installment: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("installment: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(HeaderStoreID, c.storeID)
	req.Header.Set(HeaderSignature, c.Sign(body))
	resp, err := c.h.Do(req)
	if err != nil {
		return fmt.Errorf("installment: do request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		return decodeAPIError(resp, respBody)
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("installment: decode response: %w", err)
	}
	return nil
}

// doPDF виконує POST з JSON-тілом і повертає сире тіло відповіді (PDF).
func (c *Client) doPDF(ctx context.Context, path string, in any) ([]byte, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("installment: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("installment: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/pdf")
	req.Header.Set(HeaderStoreID, c.storeID)
	req.Header.Set(HeaderSignature, c.Sign(body))
	resp, err := c.h.Do(req)
	if err != nil {
		return nil, fmt.Errorf("installment: do request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp, respBody)
	}
	return respBody, nil
}

func decodeAPIError(resp *http.Response, body []byte) error {
	e := &APIError{StatusCode: resp.StatusCode, TraceID: resp.Header.Get("Trace-Id")}
	_ = json.Unmarshal(body, e)
	if e.Message == "" {
		e.Message = string(body)
	}
	return e
}
