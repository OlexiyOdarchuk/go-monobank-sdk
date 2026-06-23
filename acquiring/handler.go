package acquiring

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// DefaultMaxBodyBytes caps the webhook body when
// [WebhookHandlerOptions.MaxBodyBytes] is unset. Real acquiring
// payloads are a few KB; 1 MiB leaves ample headroom and guards
// against an attacker posting a giant body to the public webhook URL.
const DefaultMaxBodyBytes = 1 << 20

// minRefreshInterval throttles refetching the merchant public key.
// Acquiring webhooks carry no key-id header, so the only refresh
// trigger is a verification failure — without throttling, a flood of
// bad-signature POSTs would amplify into a flood of /pubkey calls.
const minRefreshInterval = 30 * time.Second

// PubKeyProvider is anything that can return the merchant's current
// acquiring public key (the response of GET /api/merchant/pubkey).
// *[Client] implements it; tests can plug in a fake.
type PubKeyProvider interface {
	PubKey(ctx context.Context) (*ServerKey, error)
}

// WebhookHandlerOptions configures [NewWebhookHandler].
type WebhookHandlerOptions struct {
	// Keys is required. The handler calls Keys.PubKey at startup to
	// load and parse the current key, and again (throttled) whenever a
	// signature fails to verify — the documented way to pick up a
	// rotated key.
	Keys PubKeyProvider

	// OnEvent is required. It receives the verified, parsed invoice
	// state (the same shape as [Client.InvoiceStatus]). Returning a
	// non-nil error makes the handler respond 500 so Mono retries
	// (after 60s and 600s); nil → 200 (ack).
	OnEvent func(ctx context.Context, inv *InvoiceStatusResponse) error

	// Dedup is optional but strongly recommended. When set, the
	// handler skips events whose [DedupKey] was already processed,
	// acking 200 so Mono stops retrying. Use a PERSISTENT deduper in
	// production; the in-memory [NewMemoryDeduper] loses state on
	// restart and re-opens the replay window.
	Dedup Deduper

	// MaxAge, when > 0, rejects payloads whose modifiedDate is older
	// than MaxAge (via [VerifyWebhookFresh]) — a cheap replay
	// mitigation. Leave room for Mono's own 60s/600s retries; ~15
	// minutes is a sane production value. Zero disables the check.
	MaxAge time.Duration

	// OnError is optional. It receives internal failures the handler
	// cannot surface in the HTTP response (key-refresh failure, parse
	// failure, an empty invoiceId). Use for logging/metrics.
	OnError func(err error)

	// MaxBodyBytes bounds the request body. 0 or negative falls back
	// to [DefaultMaxBodyBytes]. On overflow the handler responds 413.
	MaxBodyBytes int64
}

// WebhookHandler is a ready http.Handler for acquiring webhooks.
//
//   - GET returns 200 — Mono pings the URL to confirm the
//     subscription is alive.
//   - POST reads the body (size-capped), verifies X-Sign against the
//     cached key (refetching the key once, throttled, if the first
//     verify fails), optionally enforces freshness, parses the
//     invoice state, filters duplicates via Dedup, and runs OnEvent.
//     Bad signature → 401; stale → 401; OnEvent error → 500 (Mono
//     retries); otherwise 200.
//
// It is a plain http.Handler — mount it on net/http, chi, gin, etc.
type WebhookHandler struct {
	opts         WebhookHandlerOptions
	maxBodyBytes int64

	mu  sync.RWMutex
	key *ecdsa.PublicKey

	refreshGroup  singleflight.Group
	refreshMu     sync.Mutex
	lastRefreshAt time.Time
}

// WebhookHandler configuration errors.
var (
	// ErrNilKeyProvider indicates that Options.Keys is not set.
	ErrNilKeyProvider = errors.New("acquiring: WebhookHandlerOptions.Keys is required")
	// ErrNilOnEvent indicates that Options.OnEvent is not set.
	ErrNilOnEvent = errors.New("acquiring: WebhookHandlerOptions.OnEvent is required")
)

// NewWebhookHandler validates opts and warms the key cache by
// synchronously fetching and parsing the merchant public key. The
// handler is unusable without that key, so the initial fetch is part
// of construction; pass a context with a timeout to bound startup.
func NewWebhookHandler(ctx context.Context, opts WebhookHandlerOptions) (*WebhookHandler, error) {
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
	h := &WebhookHandler{opts: opts, maxBodyBytes: maxBody}
	if err := h.refresh(ctx); err != nil {
		return nil, fmt.Errorf("initial PubKey fetch: %w", err)
	}
	return h, nil
}

func (h *WebhookHandler) refresh(ctx context.Context) error {
	sk, err := h.opts.Keys.PubKey(ctx)
	if err != nil {
		return err
	}
	pub, err := sk.Public()
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.key = pub
	h.mu.Unlock()
	return nil
}

// refreshCoalesced refetches the key, coalescing concurrent callers
// via singleflight and throttling to [minRefreshInterval]. The
// double-check on lastRefreshAt closes the race where a second wave
// of goroutines clears the outer throttle and enters Do after the
// first refresh already completed.
func (h *WebhookHandler) refreshCoalesced(ctx context.Context) error {
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

func (h *WebhookHandler) currentKey() *ecdsa.PublicKey {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.key
}

// ServeHTTP implements http.Handler.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
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
		// Guard against a zero-value WebhookHandler{} built without
		// NewWebhookHandler.
		h.reportError(errors.New("acquiring: PubKey not loaded"))
		http.Error(w, "key not ready", http.StatusServiceUnavailable)
		return
	}

	xSign := r.Header.Get("X-Sign")
	if err := h.verify(r.Context(), key, body, xSign); err != nil {
		if errors.Is(err, ErrWebhookStale) {
			http.Error(w, "stale webhook", http.StatusUnauthorized)
			return
		}
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	inv, err := ParseWebhook(body)
	if err != nil {
		h.reportError(fmt.Errorf("parse webhook: %w", err))
		http.Error(w, "malformed payload", http.StatusBadRequest)
		return
	}

	dkey := DedupKey(inv)
	if dkey == "" {
		h.reportError(errors.New("acquiring: empty invoiceId, dedup skipped"))
	} else if h.opts.Dedup != nil && h.opts.Dedup.Has(dkey) {
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := h.opts.OnEvent(r.Context(), inv); err != nil {
		h.reportError(fmt.Errorf("OnEvent: %w", err))
		// 5xx → Mono retries. Deliberately do NOT mark the key as
		// seen, so the retry actually runs OnEvent again.
		http.Error(w, "callback failed", http.StatusInternalServerError)
		return
	}
	if h.opts.Dedup != nil && dkey != "" {
		h.opts.Dedup.Add(dkey)
	}
	w.WriteHeader(http.StatusOK)
}

// verify checks the signature against the cached key; on failure it
// refetches the key once (throttled) and retries — the documented
// recovery for a rotated key. When MaxAge is set, it then enforces
// freshness on the parsed modifiedDate.
func (h *WebhookHandler) verify(ctx context.Context, key *ecdsa.PublicKey, body []byte, xSign string) error {
	err := VerifyWebhook(key, body, xSign)
	if errors.Is(err, ErrBadSignature) {
		if rerr := h.refreshCoalesced(ctx); rerr != nil {
			h.reportError(fmt.Errorf("refresh PubKey after bad signature: %w", rerr))
		} else if k := h.currentKey(); k != nil && k != key {
			key, err = k, VerifyWebhook(k, body, xSign)
		}
	}
	if err != nil {
		return err
	}
	if h.opts.MaxAge > 0 {
		// Signature already verified above; reuse the fresh check's
		// timestamp logic without re-running the (cheap) crypto.
		return VerifyWebhookFresh(key, body, xSign, h.opts.MaxAge)
	}
	return nil
}

func (h *WebhookHandler) reportError(err error) {
	if h.opts.OnError != nil {
		h.opts.OnError(err)
	}
}
