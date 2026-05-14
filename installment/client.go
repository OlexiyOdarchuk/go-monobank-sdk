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

// ErrCallbackSignatureMismatch повертається з [Client.VerifyCallback],
// коли підпис у заголовку signature не збігається з очікуваним.
var ErrCallbackSignatureMismatch = errors.New("installment: callback signature mismatch")

// MaxResponseBytes — стеля на розмір відповіді, після якої тіло
// обрізається. Захист від OOM у разі зловмисного/glitched проксі.
// PDF-документи (payslips, інвойси) можуть досягати кількох MB; 50 MiB
// — з великим запасом.
const MaxResponseBytes = 50 << 20

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
// УВАГА: дефолт — production. Якщо забути [WithBaseURL] у тестовому
// середовищі, перший виклик уже вдарить по бойовому API. Sandbox- і
// production-secrets різні, тож автентифікація провалиться при
// несумісності — але якщо у тебе production-secret в тестовому коді,
// проведеться реальна операція. Завжди передавай явний BaseURL у
// non-prod коді.
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

// VerifyCallback повертає nil, якщо HMAC-SHA256(body, secret) збігається
// з [ErrCallbackSignatureMismatch] значенням заголовка signature,
// прийнятим у вхідному callback. Викликай перед обробкою тіла, інакше
// будь-хто може прислати фейковий запит.
//
// Реалізація відхиляє підпис із некоректною довжиною (base64 від
// 32-байтного HMAC-SHA256 завжди має 44 символи) ДО обчислення HMAC —
// це захист від CPU-DoS, коли атакуючий шле гігабайтне тіло з порожнім
// або довільним signature. Незалежно від довжини, фінальне порівняння
// constant-time через [hmac.Equal].
//
// Окремо рекомендую загорнути запит у [http.MaxBytesReader] перед
// викликом VerifyCallback, щоб обмежити верхню межу читання тіла.
func (c *Client) VerifyCallback(body []byte, signatureHeader string) error {
	const wantLen = 44 // base64.StdEncoding.EncodedLen(sha256.Size)
	if len(signatureHeader) != wantLen {
		return ErrCallbackSignatureMismatch
	}
	want := c.Sign(body)
	if !hmac.Equal([]byte(want), []byte(signatureHeader)) {
		return ErrCallbackSignatureMismatch
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
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes))
	if err != nil {
		return fmt.Errorf("installment: read response body: %w", err)
	}
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
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("installment: read response body: %w", err)
	}
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
