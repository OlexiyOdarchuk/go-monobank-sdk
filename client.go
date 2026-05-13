package monobank

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
)

// Помилки рівня клієнта.
var (
	// ErrEmptyRequest — у [Client.Do] передано nil-request.
	ErrEmptyRequest = errors.New("empty request")
	// ErrInvalidURL — baseURL клієнта не валідний (зазвичай не настає,
	// бо [New] завжди ставить дефолт; може виникнути після
	// [Client.SetBaseURL] з невалідним рядком).
	ErrInvalidURL = errors.New("invalid URL")
)

// HTTPDoer — мінімальна підмножина *http.Client, від якої залежить
// [Client]. Будь-який транспорт, що реалізує цей інтерфейс (стандартний
// клієнт, кастомний round-tripper, тестовий фейк), підключається через
// [WithHTTPDoer].
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// APIError повертається, коли HTTP-відповідь monobank не збіглася з
// жодним зі статусів, які очікував викликач. Зберігає метод, повний
// URL, отриманий і очікуваний статус-коди, плюс перші 256 символів body
// для діагностики.
//
// Якщо тіло відповіді — це JSON виду {"errorDescription": "..."} (стандартний
// формат помилок Mono для personal/corporate/business/acquiring API), то
// поле [APIError.ErrorDescription] містить розпарсене повідомлення; у інших
// випадках воно порожнє, а оригінальні байти залишаються в [APIError.Body].
type APIError struct {
	Method              string
	URL                 string
	StatusCode          int
	ExpectedStatusCodes []int
	// ErrorDescription — значення поля errorDescription з JSON-тіла
	// відповіді Mono, якщо тіло вдалося розпарсити; інакше порожнє.
	ErrorDescription string
	Body             []byte
}

func (e *APIError) Error() string {
	detail := e.ErrorDescription
	if detail == "" {
		detail = truncate(e.Body, 256)
	}
	return fmt.Sprintf("%s %s: HTTP %d (expected %v): %s",
		e.Method, e.URL, e.StatusCode, e.ExpectedStatusCodes, detail)
}

// errorBody — JSON-форма помилки, яку повертають personal/corporate/
// business/acquiring API Mono. Інші підпакети (наприклад, installment)
// мають власні формати й використовують власні типи помилок.
type errorBody struct {
	ErrorDescription string `json:"errorDescription"`
}

// parseErrorDescription витягує errorDescription з JSON-тіла. Повертає
// "" якщо тіло не JSON або поле відсутнє/порожнє.
func parseErrorDescription(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var e errorBody
	if err := json.Unmarshal(body, &e); err != nil {
		return ""
	}
	return e.ErrorDescription
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// Client — базовий HTTP-транспорт для всіх поверхонь monobank. Кожен
// підпакет (bank, personal, corporate, business, acquiring) композує
// [Client] із [auth.Authorizer] із пакета auth та власним base URL
// під свій API. Прямо з рутинного коду цей тип зазвичай не
// конструюється — використовуй фабрики підпакетів ([personal.New],
// [bank.New] тощо).
type Client struct {
	http    HTTPDoer
	auth    auth.Authorizer
	baseURL *url.URL
	retry   retryPolicy
	limiter RateLimiter

	logger *slog.Logger
	onReq  func(*http.Request)
	onResp func(*http.Response, error)
}

// SetBaseURL перевизначає base URL уже сконструйованого клієнта.
// Якщо uri не парситься як URL — лишає попереднє значення. Рутинно
// використовуй [WithBaseURL] під час конструювання через [New]; цей
// метод потрібен підпакетам, що збирають [Client] інкрементально.
func (c *Client) SetBaseURL(uri string) {
	u, err := url.Parse(uri)
	if err != nil || u == nil {
		return
	}
	c.baseURL = u
}

// Do виконує req проти c.baseURL і декодує відповідь у v. Кількість
// очікуваних статус-кодів довільна; за замовчуванням — http.StatusOK.
// Якщо відповідь має інший код — повертає [*APIError].
//
// Тип v впливає на режим декодування:
//   - nil — body просто читається й викидається;
//   - *[]byte — у v записуються сирі байти body;
//   - io.Writer — body копіюється у Writer;
//   - інакше — декодується як JSON у v.
//
// Транзитивні відмови (5xx, 429) ретраяться згідно з [WithRetry]
// (з повагою до Retry-After). Скасування контексту — миттєвий вихід.
//
// Метод експортовано, щоб підпакети (bank, personal, corporate,
// business, acquiring) використовували один HTTP-плюмбінг (retry,
// base-URL resolution, маппінг помилок), не реалізуючи його повторно.
// Передавай *http.Request з path-only URL — він resolve-иться проти
// налаштованого base URL.
func (c Client) Do(req *http.Request, v any, expectedStatusCodes ...int) error {
	if req == nil {
		return ErrEmptyRequest
	}
	if c.baseURL == nil {
		return ErrInvalidURL
	}
	if len(expectedStatusCodes) == 0 {
		expectedStatusCodes = []int{http.StatusOK}
	}

	target, err := url.Parse(req.URL.String())
	if err != nil {
		return fmt.Errorf("parse request URL: %w", err)
	}
	req.URL = c.baseURL.ResolveReference(target)
	if req.Header.Get("Content-Type") == "" && req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.auth != nil {
		if err := c.auth.SetAuth(req); err != nil {
			return fmt.Errorf("SetAuth: %w", err)
		}
	}

	if c.limiter != nil {
		if err := c.limiter.Wait(req.Context()); err != nil {
			return err
		}
	}

	return c.retry.run(req.Context(), func() error {
		return c.attempt(req, v, expectedStatusCodes)
	})
}

func (c Client) attempt(req *http.Request, v any, expectedStatusCodes []int) error {
	if c.onReq != nil {
		c.onReq(req)
	}
	if c.logger != nil {
		c.logger.Debug("monobank: sending request",
			slog.String("method", req.Method),
			slog.String("url", req.URL.String()))
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	dur := time.Since(start)

	if c.onResp != nil {
		c.onResp(resp, err)
	}
	if c.logger != nil {
		if err != nil {
			c.logger.Warn("monobank: http error",
				slog.String("method", req.Method),
				slog.String("url", req.URL.String()),
				slog.Duration("duration", dur),
				slog.Any("err", err))
		} else {
			c.logger.Debug("monobank: http response",
				slog.String("method", req.Method),
				slog.String("url", req.URL.String()),
				slog.Int("status", resp.StatusCode),
				slog.Duration("duration", dur))
		}
	}

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if !slices.Contains(expectedStatusCodes, resp.StatusCode) {
		body, _ := io.ReadAll(resp.Body)
		apiErr := &APIError{
			Method:              req.Method,
			URL:                 req.URL.String(),
			StatusCode:          resp.StatusCode,
			ExpectedStatusCodes: expectedStatusCodes,
			ErrorDescription:    parseErrorDescription(body),
			Body:                body,
		}
		if isTransientStatus(resp.StatusCode) {
			return &transientStatus{
				code:       resp.StatusCode,
				retryAfter: parseRetryAfter(resp.Header),
				cause:      apiErr,
			}
		}
		return apiErr
	}

	switch out := v.(type) {
	case nil:
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	case *[]byte:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response body: %w", err)
		}
		*out = body
		return nil
	case io.Writer:
		if _, err := io.Copy(out, resp.Body); err != nil {
			return fmt.Errorf("copy response body: %w", err)
		}
		return nil
	default:
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		return nil
	}
}

// isTransientStatus reports whether an HTTP status code is worth retrying.
func isTransientStatus(code int) bool {
	return code == http.StatusTooManyRequests || (code >= 500 && code <= 599)
}
