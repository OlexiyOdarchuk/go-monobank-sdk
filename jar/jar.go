// Package jar — клієнт для двох публічних (без авторизації) ендпоінтів
// Monobank, що повертають інформацію про «банки» (jars):
//
//   - GET  https://api.monobank.ua/bank/jar/{longJarId}     — повна
//     інформація по jar за «довгим» ідентифікатором.
//   - POST https://send.monobank.ua/api/handler              — пошук
//     jar за clientId зі share-посилання (https://send.monobank.ua/<id>);
//     відповідь містить longJarId/extJarId для подальшого використання у
//     [Client.ByLongID].
//
// Обидва ендпоінти задокументовані лише в community-нотатках (див.
// pinned-повідомлення в каналі @api_mono_chat). Це read-only API:
// ні створювати, ні редагувати jars через них не можна.
//
// Ліміти на send.monobank.ua більш агресивні, ніж на /bank/jar — для
// повторних запитів краще кешувати longJarId і ходити лише в /bank/jar.
package jar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Default endpoints — переозначити можна через [WithAPIBaseURL] /
// [WithSendBaseURL].
const (
	defaultAPIBaseURL  = "https://api.monobank.ua"
	defaultSendBaseURL = "https://send.monobank.ua"
)

// ErrNotFound — банка не існує або ID невалідний.
var ErrNotFound = errors.New("jar: not found")

// APIError — структурована помилка з тіла відповіді monobank (errCode/errText).
type APIError struct {
	StatusCode int
	ErrCode    string `json:"errCode"`
	ErrText    string `json:"errText"`
}

// Error реалізує error.
func (e *APIError) Error() string {
	return fmt.Sprintf("jar: %d %s: %s", e.StatusCode, e.ErrCode, e.ErrText)
}

// Info — стабільний підмножина полів /bank/jar/{longJarId}.
//
// Сума у мінорних одиницях валюти (копійках для UAH). Currency — ISO 4217
// numeric (980 = UAH).
type Info struct {
	JarID     string `json:"jarId"`
	Title     string `json:"title"`
	OwnerName string `json:"ownerName"`
	OwnerIcon string `json:"ownerIcon"`
	Amount    int64  `json:"amount"`
	Goal      int64  `json:"goal"`
	Currency  int    `json:"currency"`
}

// SendInfo — підмножина полів від send.monobank.ua/api/handler (c="hello").
// Цей endpoint віддає не той самий формат, що /bank/jar — поля інші,
// тому ми тримаємо окремий тип.
type SendInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Avatar      string `json:"avatar"`
	JarAmount   int64  `json:"jarAmount"`
	JarGoal     int64  `json:"jarGoal"`
	JarStatus   string `json:"jarStatus"`
	IsTrusted   bool   `json:"isTrusted"`
	// LongJarID/ExtJarID — поле для подальшої взаємодії з /bank/jar.
	// У різних версіях API може приходити під різними назвами; ми
	// заповнюємо його з extJarId або longJarId, що першим знайдеться у
	// тілі (див. UnmarshalJSON).
	LongJarID string `json:"-"`
}

// UnmarshalJSON акуратно витягує longJarId/extJarId — у різних версіях
// поле має різну назву.
func (s *SendInfo) UnmarshalJSON(data []byte) error {
	type raw SendInfo
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*s = SendInfo(r)
	// Шукаємо longJarId або extJarId окремо.
	var aux struct {
		LongJarID string `json:"longJarId"`
		ExtJarID  string `json:"extJarId"`
	}
	_ = json.Unmarshal(data, &aux)
	if aux.LongJarID != "" {
		s.LongJarID = aux.LongJarID
	} else if aux.ExtJarID != "" {
		s.LongJarID = aux.ExtJarID
	}
	return nil
}

// Option — функціональна опція конструктора [New].
type Option func(*Client)

// WithHTTPClient підставляє кастомний *http.Client (за дефолтом — *http.Client
// з 15-секундним таймаутом).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.h = h }
}

// WithAPIBaseURL перевизначає базовий URL для api.monobank.ua (для тестів).
func WithAPIBaseURL(u string) Option {
	return func(c *Client) { c.apiBase = u }
}

// WithSendBaseURL перевизначає базовий URL для send.monobank.ua (для тестів).
func WithSendBaseURL(u string) Option {
	return func(c *Client) { c.sendBase = u }
}

// Client — read-only клієнт двох публічних jar-ендпоінтів. Створюй через
// [New]. Без авторизації — обидва ендпоінти публічні.
type Client struct {
	h        *http.Client
	apiBase  string
	sendBase string
}

// New повертає клієнт із дефолтним таймаутом 15с.
func New(opts ...Option) *Client {
	c := &Client{
		h:        &http.Client{Timeout: 15 * time.Second},
		apiBase:  defaultAPIBaseURL,
		sendBase: defaultSendBaseURL,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// ByLongID повертає поточну інформацію по jar за довгим (longJarId /
// extJarId) ідентифікатором. Цей ID можна знайти у URL віджета банки або
// отримати з [Client.ByShortID].
func (c *Client) ByLongID(ctx context.Context, longJarID string) (*Info, error) {
	if longJarID == "" {
		return nil, fmt.Errorf("jar: empty longJarID")
	}
	u := c.apiBase + "/bank/jar/" + url.PathEscape(longJarID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("jar: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.h.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jar: do request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp.StatusCode, body)
	}
	var out Info
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("jar: decode response: %w", err)
	}
	return &out, nil
}

// shortIDRequest — тіло POST на send.monobank.ua/api/handler.
type shortIDRequest struct {
	C        string `json:"c"`
	ClientID string `json:"clientId"`
	Pc       string `json:"Pc"`
}

// ByShortID шукає інформацію по jar за коротким clientId (тим, що в
// URL share-посилання https://send.monobank.ua/{clientId}). Цей виклик
// йде на send.monobank.ua — ліміти суворіші, кешуй результат.
//
// Корисний для одноразового отримання longJarId з коротким посиланням.
// Для регулярних оновлень балансу використовуй [Client.ByLongID].
func (c *Client) ByShortID(ctx context.Context, clientID string) (*SendInfo, error) {
	if clientID == "" {
		return nil, fmt.Errorf("jar: empty clientID")
	}
	body, err := json.Marshal(shortIDRequest{C: "hello", ClientID: clientID, Pc: "random"})
	if err != nil {
		return nil, fmt.Errorf("jar: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.sendBase+"/api/handler", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jar: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.h.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jar: do request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp.StatusCode, respBody)
	}
	// send.monobank.ua може віддати { "errCode": "..."} зі статусом 200.
	var maybeErr APIError
	if json.Unmarshal(respBody, &maybeErr) == nil && maybeErr.ErrCode != "" {
		maybeErr.StatusCode = resp.StatusCode
		return nil, &maybeErr
	}
	var out SendInfo
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("jar: decode response: %w", err)
	}
	return &out, nil
}

func decodeAPIError(status int, body []byte) error {
	var e APIError
	if err := json.Unmarshal(body, &e); err != nil {
		return fmt.Errorf("jar: http %d: %s", status, string(body))
	}
	e.StatusCode = status
	return &e
}
