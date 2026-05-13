package webhook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// KeyProvider — будь-що, що вміє віддати поточний публічний ключ банку.
// [bank.Client] його реалізує; у тестах можна підставити фейк.
type KeyProvider interface {
	ServerKey(ctx context.Context) (*bank.ServerKey, error)
}

// Options конфігурує [NewHandler].
type Options struct {
	// Keys — обов'язковий. Handler викликає Keys.ServerKey при старті,
	// щоб завантажити поточний публічний ключ банку, і ще раз — щоразу,
	// коли вхідний X-Key-Id перестає збігатися з кешованим (тобто Mono
	// провернула ключ).
	Keys KeyProvider

	// OnEvent — обов'язковий. Отримує верифікований і розпарсений
	// payload. Якщо повертає не-nil error — handler відповідає 500, і
	// Mono ретраїть доставку; nil → 200 (ack).
	OnEvent func(ctx context.Context, event *Response) error

	// OnUnknownType — необов'язковий. Викликається для payload-ів, які
	// пройшли верифікацію підпису, але мають невідомий "type" (не
	// входить у Type*-константи). Якщо nil — handler тихо ack-ить 200.
	// Payload автентичний, просто його тип ще не представлений у SDK.
	OnUnknownType func(ctx context.Context, raw []byte)

	// OnError — необов'язковий. Викликається для внутрішніх збоїв, які
	// handler не може показати через HTTP-відповідь (наприклад, провал
	// рефрешу ключа). Використовуй для логування / метрик.
	OnError func(err error)

	// Dedup — необов'язковий. Якщо вказаний, handler консультується з
	// ним перед OnEvent і скіпає дублікати (відповідає 200, щоб Mono
	// припинила ретраї). Без deduper-а подія, яку OnEvent успішно
	// обробив, але клієнт не встиг отримати 200 — буде оброблена двічі.
	Dedup Deduper
}

// Handler — готовий http.Handler, який приймає підписані webhook-и Mono.
// Поведінка:
//
//   - GET /твій-шлях — повертає 200. Mono пінгує URL через GET при
//     підписці, щоб переконатися, що він живий.
//   - POST — читає body, верифікує X-Sign проти кешованого ключа
//     (перевикликає [KeyProvider.ServerKey] якщо X-Key-Id змінився),
//     парсить payload і запускає OnEvent. Поганий підпис → 401;
//     OnEvent повернув error → 500 (Mono ретраїть); інакше 200.
//
// Монтується на будь-який роутер: net/http, chi, gin через
// http.HandlerFunc — це звичайний http.Handler.
type Handler struct {
	opts Options

	mu  sync.RWMutex
	key *bank.ServerKey
}

// Помилки конфігурації Handler-а.
var (
	// ErrNilKeyProvider — у Options не задано Keys.
	ErrNilKeyProvider = errors.New("webhook.Options.Keys is required")
	// ErrNilOnEvent — у Options не задано OnEvent.
	ErrNilOnEvent = errors.New("webhook.Options.OnEvent is required")
)

// NewHandler валідує opts і прогріває кеш ключа (синхронно тягне
// серверний ключ через Keys.ServerKey). Передавай некасельовуваний
// контекст, якщо немає вимоги обмежити час старту — без початкового
// ключа handler непрацездатний.
func NewHandler(ctx context.Context, opts Options) (*Handler, error) {
	if opts.Keys == nil {
		return nil, ErrNilKeyProvider
	}
	if opts.OnEvent == nil {
		return nil, ErrNilOnEvent
	}
	h := &Handler{opts: opts}
	if err := h.refresh(ctx); err != nil {
		return nil, fmt.Errorf("initial ServerKey fetch: %w", err)
	}
	return h, nil
}

// KeyID повертає кешований ідентифікатор серверного ключа. Корисно для
// діагностики/метрик (бачити, як часто ключ ротується).
func (h *Handler) KeyID() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.key == nil {
		return ""
	}
	return h.key.ID
}

func (h *Handler) refresh(ctx context.Context) error {
	sk, err := h.opts.Keys.ServerKey(ctx)
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.key = sk
	h.mu.Unlock()
	return nil
}

func (h *Handler) currentKey() *bank.ServerKey {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.key
}

// ServeHTTP реалізує http.Handler — вся логіка прийому вебхука: GET
// підтвердження, POST з верифікацією + парсингом + dedup + OnEvent.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Mono pings the URL with GET to verify the subscription.
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodPost:
		// fall through
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	key := h.currentKey()
	if incomingKeyID := r.Header.Get("X-Key-Id"); incomingKeyID != "" && incomingKeyID != key.ID {
		if err := h.refresh(r.Context()); err != nil {
			h.reportError(fmt.Errorf("refresh ServerKey on X-Key-Id mismatch: %w", err))
		} else {
			key = h.currentKey()
		}
	}

	if err := Verify(key.PubKey, body, r.Header.Get("X-Sign")); err != nil {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	event, err := Parse(body)
	if errors.Is(err, ErrUnknownType) {
		if h.opts.OnUnknownType != nil {
			h.opts.OnUnknownType(r.Context(), body)
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	if err != nil {
		h.reportError(fmt.Errorf("parse webhook: %w", err))
		http.Error(w, "malformed payload", http.StatusBadRequest)
		return
	}

	id := event.Data.Transaction.ID
	if h.opts.Dedup != nil && h.opts.Dedup.Has(id) {
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := h.opts.OnEvent(r.Context(), event); err != nil {
		h.reportError(fmt.Errorf("OnEvent: %w", err))
		// 5xx → mono will retry (after 60s and 600s). We deliberately do
		// NOT mark id as seen — let the retry actually run OnEvent again.
		http.Error(w, "callback failed", http.StatusInternalServerError)
		return
	}
	if h.opts.Dedup != nil {
		h.opts.Dedup.Add(id)
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) reportError(err error) {
	if h.opts.OnError != nil {
		h.opts.OnError(err)
	}
}
