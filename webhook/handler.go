package webhook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
	"golang.org/x/sync/singleflight"
)

// DefaultMaxBodyBytes — стеля на розмір webhook-body, якщо не вказано
// інше через [Options.MaxBodyBytes]. Реальні Mono-payload-и — десятки
// KB; 1 MiB з великим запасом і захищає від OOM, коли атакуючий шле
// гігабайтне тіло на твій webhook-URL.
const DefaultMaxBodyBytes = 1 << 20

// minRefreshInterval — мінімальна пауза між викликами /bank/sync для
// оновлення серверного ключа. Захист від DoS-амплифікації, коли
// атакуючий заливає handler POST-ами з випадковим X-Key-Id.
const minRefreshInterval = 30 * time.Second

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
	//
	// Для production обов'язково використовуй ПЕРСИСТЕНТНИЙ deduper
	// (Redis/SQL) — in-memory [NewMemoryDeduper] втрачає стан при
	// рестарті і дозволяє replay-атаку: атакуючий, що раз перехопив
	// валідно підписаний webhook, може повторно його надіслати після
	// твого рестарту, бо Mono не включає freshness-токен у payload.
	Dedup Deduper

	// MaxBodyBytes — верхня межа розміру тіла webhook-запиту. 0 або
	// негативне значення → [DefaultMaxBodyBytes] (1 MiB). Реальні
	// payload-и Mono — десятки KB, тож стандартний ліміт безпечний.
	// При перевищенні handler відповідає 413 Request Entity Too Large.
	MaxBodyBytes int64
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
	opts         Options
	maxBodyBytes int64

	mu  sync.RWMutex
	key *bank.ServerKey

	refreshGroup singleflight.Group

	refreshMu     sync.Mutex
	lastRefreshAt time.Time
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
	max := opts.MaxBodyBytes
	if max <= 0 {
		max = DefaultMaxBodyBytes
	}
	h := &Handler{opts: opts, maxBodyBytes: max}
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

// refreshCoalesced виконує refresh, об'єднуючи паралельні виклики через
// singleflight (один похід у /bank/sync на пак конкурентних запитів) і
// дроссельуючи частоту до [minRefreshInterval] (захист від DoS-
// амплифікації, коли атакуючий шле POST з випадковим X-Key-Id).
//
// Внутрішній double-check на lastRefreshAt захищає від рейсу: коли
// singleflight завершує одну Do-функцію, а друга хвиля goroutine-ів
// уже встигла пройти зовнішній throttle-check і потрапити у Do —
// другий fn виявить, що lastRefreshAt свіжий, і одразу вийде без
// нового виклику ServerKey().
func (h *Handler) refreshCoalesced(ctx context.Context) error {
	h.refreshMu.Lock()
	if time.Since(h.lastRefreshAt) < minRefreshInterval {
		h.refreshMu.Unlock()
		return nil
	}
	h.refreshMu.Unlock()

	_, err, _ := h.refreshGroup.Do("refresh", func() (any, error) {
		h.refreshMu.Lock()
		if time.Since(h.lastRefreshAt) < minRefreshInterval {
			h.refreshMu.Unlock()
			return nil, nil
		}
		h.refreshMu.Unlock()

		if e := h.refresh(ctx); e != nil {
			return nil, e
		}
		h.refreshMu.Lock()
		h.lastRefreshAt = time.Now()
		h.refreshMu.Unlock()
		return nil, nil
	})
	return err
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

	limited := http.MaxBytesReader(w, r.Body, h.maxBodyBytes)
	body, err := io.ReadAll(limited)
	if err != nil {
		// MaxBytesReader повертає *http.MaxBytesError при перевищенні.
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	key := h.currentKey()
	if key == nil {
		// Захист на випадок Handler{} напряму (без NewHandler) — без
		// ключа верифікувати неможливо.
		h.reportError(errors.New("ServerKey not loaded"))
		http.Error(w, "key not ready", http.StatusServiceUnavailable)
		return
	}
	if incomingKeyID := r.Header.Get("X-Key-Id"); incomingKeyID != "" && incomingKeyID != key.ID {
		if err := h.refreshCoalesced(r.Context()); err != nil {
			h.reportError(fmt.Errorf("refresh ServerKey on X-Key-Id mismatch: %w", err))
		} else if k := h.currentKey(); k != nil {
			key = k
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
