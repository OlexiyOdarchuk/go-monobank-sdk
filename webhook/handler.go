package webhook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// DefaultMaxBodyBytes is the cap on webhook body size used when
// [Options.MaxBodyBytes] is not set. Real Mono payloads are tens of
// KB; 1 MiB leaves headroom and guards against OOM when an attacker
// posts a gigabyte body to your webhook URL.
const DefaultMaxBodyBytes = 1 << 20

// minRefreshInterval is the minimum pause between calls to
// /bank/sync to refresh the server key. Guards against DoS
// amplification when an attacker floods the handler with POSTs that
// have a random X-Key-Id.
const minRefreshInterval = 30 * time.Second

// KeyProvider is anything that knows how to return the bank's current
// public key. [bank.Client] implements it; in tests, plug in a fake.
type KeyProvider interface {
	ServerKey(ctx context.Context) (*bank.ServerKey, error)
}

// Options configures [NewHandler].
type Options struct {
	// Keys is required. The handler calls Keys.ServerKey at startup
	// to load the current bank public key, and again whenever an
	// incoming X-Key-Id stops matching the cached one (i.e. Mono
	// rotated the key).
	Keys KeyProvider

	// OnEvent is required. It receives the verified and parsed
	// payload. If it returns a non-nil error, the handler responds
	// 500 and Mono retries delivery; nil → 200 (ack).
	OnEvent func(ctx context.Context, event *Response) error

	// OnUnknownType is optional. It is invoked for payloads that
	// pass signature verification but have an unknown "type" (not
	// in the Type* constants). When nil, the handler silently acks
	// with 200. The payload is authentic — its type just is not
	// represented in the SDK yet.
	OnUnknownType func(ctx context.Context, raw []byte)

	// OnError is optional. It is invoked for internal failures the
	// handler cannot surface through the HTTP response (for example,
	// a key-refresh failure). Use for logging / metrics.
	OnError func(err error)

	// Dedup is optional. When set, the handler consults it before
	// OnEvent and skips duplicates (responding 200 so Mono stops
	// retrying). Without a deduper, an event OnEvent processed
	// successfully but for which the client did not get a 200 in
	// time will be processed twice.
	//
	// For production always use a PERSISTENT deduper (Redis/SQL) —
	// the in-memory [NewMemoryDeduper] loses state on restart and
	// allows a replay attack: an attacker that intercepted a
	// validly signed webhook once can re-send it after your
	// restart, because Mono does not include a freshness token in
	// the payload.
	Dedup Deduper

	// MaxBodyBytes is the upper bound on the webhook request body.
	// 0 or a negative value falls back to [DefaultMaxBodyBytes]
	// (1 MiB). Real Mono payloads are tens of KB, so the default
	// limit is safe. When exceeded, the handler responds with 413
	// Request Entity Too Large.
	MaxBodyBytes int64
}

// Handler is a ready http.Handler that accepts signed Mono webhooks.
// Behavior:
//
//   - GET /your-path returns 200. Mono pings the URL with a GET on
//     subscription to verify that it is alive.
//   - POST reads the body, verifies X-Sign against the cached key
//     (calling [KeyProvider.ServerKey] again if X-Key-Id changed),
//     parses the payload and runs OnEvent. Bad signature → 401;
//     OnEvent returned an error → 500 (Mono retries); otherwise 200.
//
// Mounts on any router: net/http, chi, gin via http.HandlerFunc — it
// is a regular http.Handler.
type Handler struct {
	opts         Options
	maxBodyBytes int64

	mu  sync.RWMutex
	key *bank.ServerKey

	refreshGroup singleflight.Group

	refreshMu     sync.Mutex
	lastRefreshAt time.Time
}

// Handler configuration errors.
var (
	// ErrNilKeyProvider indicates that Options.Keys is not set.
	ErrNilKeyProvider = errors.New("webhook.Options.Keys is required")
	// ErrNilOnEvent indicates that Options.OnEvent is not set.
	ErrNilOnEvent = errors.New("webhook.Options.OnEvent is required")
)

// NewHandler validates opts and warms the key cache (it
// synchronously fetches the server key via Keys.ServerKey). Pass a
// non-cancelable context if you have no need to bound startup time
// — the handler is not usable without the initial key.
func NewHandler(ctx context.Context, opts Options) (*Handler, error) {
	if opts.Keys == nil {
		return nil, ErrNilKeyProvider
	}
	if opts.OnEvent == nil {
		return nil, ErrNilOnEvent
	}
	maxBody := opts.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = DefaultMaxBodyBytes
	}
	h := &Handler{opts: opts, maxBodyBytes: maxBody}
	if err := h.refresh(ctx); err != nil {
		return nil, fmt.Errorf("initial ServerKey fetch: %w", err)
	}
	return h, nil
}

// KeyID returns the cached server-key identifier. Handy for
// diagnostics/metrics (to see how often the key rotates).
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

// refreshCoalesced runs refresh, coalescing parallel calls via
// singleflight (one hit to /bank/sync per burst of concurrent
// requests) and throttling the frequency to [minRefreshInterval]
// (guards against DoS amplification when an attacker posts with a
// random X-Key-Id).
//
// The internal double-check on lastRefreshAt protects against a
// race: when singleflight finishes one Do function and a second
// wave of goroutines has already cleared the outer throttle check
// and entered Do, the second fn notices lastRefreshAt is fresh and
// exits immediately without another call to ServerKey().
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

// ServeHTTP implements http.Handler — the full webhook-ingest logic:
// GET confirmation, POST with verification + parsing + dedup +
// OnEvent.
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
		// MaxBytesReader returns *http.MaxBytesError on overflow.
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
		// Guard against constructing Handler{} directly (without
		// NewHandler) — verification is impossible without a key.
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
	if id == "" {
		// Mono very rarely sends a webhook whose Transaction.ID is
		// empty (older event types, jar top-ups in some flows).
		// Without an ID dedup is a no-op, so every Mono retry of the
		// same body would fire OnEvent again. Warn the caller —
		// they can decide whether to write to a fallback dedup key
		// based on a hash of the body — but still ack 200 so the
		// bank stops retrying.
		h.reportError(errors.New("webhook: empty Transaction.ID, dedup skipped"))
	} else if h.opts.Dedup != nil && h.opts.Dedup.Has(id) {
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
