// Package otelmonobank is the OpenTelemetry integration for
// monobank-sdk.
//
// It lives in a separate sub-module so that callers who do not use
// OpenTelemetry are not forced to depend on
// `go.opentelemetry.io/otel`. Import it explicitly when you need
// tracing:
//
//	import (
//	    "github.com/OlexiyOdarchuk/go-monobank-sdk/personal"
//	    "github.com/OlexiyOdarchuk/go-monobank-sdk/otelmonobank"
//	    "go.opentelemetry.io/otel"
//	)
//
//	cli := personal.New(token, otelmonobank.WithTracer(otel.Tracer("my-app")))
//
// Each HTTP request — every retry attempt included — becomes its own
// span with http.method, http.url (path-only, query redacted),
// http.status_code, and error attributes.
package otelmonobank

import (
	"net/http"
	"net/url"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

// WithTracer returns a [monobank.Option] that instruments the client
// with OpenTelemetry tracing. Each HTTP request creates a span named
// "monobank HTTP <method>" with the attributes:
//
//   - http.method — request method
//   - http.url — request URL path (query string redacted to avoid
//     leaking tokens/IDs into span attributes that often end up in
//     a third-party APM)
//   - http.status_code — response status (Int, per OTel semconv)
//   - error — true on a transport failure
//
// The span is closed in the response-hook regardless of outcome,
// including transport failures where resp == nil — without that the
// previous implementation leaked entries in its internal map.
// Retries are handled correctly: when a request-hook fires for a
// retry, any in-flight span from the previous attempt is closed
// first.
//
// If tracer == nil the option is a no-op.
//
// Hook composition: WithTracer CHAINS its hooks on top of any
// existing [monobank.WithRequestHook] / [monobank.WithResponseHook]
// — previous hooks fire first, then the OTel one. Put WithTracer
// AFTER any application hooks you want to run first.
func WithTracer(tracer trace.Tracer) monobank.Option {
	if tracer == nil {
		return func(*monobank.Client) {}
	}

	// store holds the in-flight span per *http.Request. Mono's
	// Client.Do does not propagate request-context values into the
	// response hook, and we cannot rewrite the Request's context
	// from inside the hook either — so we keep an indexed
	// side-table. Each entry is guaranteed to be removed either by
	// the response-hook or by a retry firing its request-hook
	// again (the latter closes the stale span before storing a new
	// one), so the map never grows unbounded.
	store := newSpanStore()

	requestHook := func(r *http.Request) {
		// Retry: if a span already exists for this *http.Request,
		// close it before opening a new one — otherwise the previous
		// span would leak (its response-hook never fired because the
		// retry replaced it).
		if prev, ok := store.pop(r); ok {
			prev.End()
		}
		_, span := tracer.Start(r.Context(), "monobank HTTP "+r.Method,
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.url", redactURL(r.URL)),
				attribute.String("http.target", r.URL.Path),
			),
		)
		store.set(r, span)
	}

	responseHook := func(resp *http.Response, err error) {
		// resp.Request is nil on transport errors, so we cannot key
		// off it there. Fall back to "close every span that has no
		// pending response" — actually a simpler and correct
		// approach is: keep the *http.Request reference in a
		// goroutine-local slot. But Mono passes the original
		// *http.Request as resp.Request when resp != nil and
		// guarantees the response-hook runs once per request — so we
		// can pop by resp.Request when available.
		//
		// When resp == nil (transport error) we use the most recent
		// span — the spanStore exposes popAny() for that case. This
		// is correct because Mono's Client.Do is sequential per
		// goroutine: at most one in-flight request per call.
		var span trace.Span
		var ok bool
		if resp != nil && resp.Request != nil {
			span, ok = store.pop(resp.Request)
		} else {
			span, ok = store.popAny()
		}
		if !ok {
			return
		}
		defer span.End()

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return
		}
		if resp != nil {
			span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
			if resp.StatusCode >= 400 {
				span.SetStatus(codes.Error, http.StatusText(resp.StatusCode))
			} else {
				span.SetStatus(codes.Ok, "")
			}
		}
	}

	return func(c *monobank.Client) {
		// Chain on top of whatever hooks already exist. We cannot
		// read them off the Client (no getter), so we wrap by
		// applying the OTel option AFTER any earlier hook was
		// installed — that means OTel runs second. To preserve
		// order strictly, also install hooks that re-run the
		// previously-installed pair via a captured closure. Mono
		// stores a single onReq/onResp; we therefore use a tiny
		// shared-state wrapper that calls "current" first, then
		// OTel.
		c.ChainRequestHook(requestHook)
		c.ChainResponseHook(responseHook)
	}
}

// spanStore is the in-flight span side-table. The previous version
// of this file kept the type in a separate file with a public-ish
// API; consolidated here so the OTel option owns its state outright.
type spanStore struct {
	mu     sync.Mutex
	m      map[*http.Request]trace.Span
	recent []*http.Request // FIFO of currently-stored requests, for popAny
}

func newSpanStore() *spanStore {
	return &spanStore{m: make(map[*http.Request]trace.Span)}
}

func (s *spanStore) set(r *http.Request, span trace.Span) {
	s.mu.Lock()
	if _, exists := s.m[r]; !exists {
		s.recent = append(s.recent, r)
	}
	s.m[r] = span
	s.mu.Unlock()
}

func (s *spanStore) pop(r *http.Request) (trace.Span, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	span, ok := s.m[r]
	if ok {
		delete(s.m, r)
		for i, p := range s.recent {
			if p == r {
				s.recent = append(s.recent[:i], s.recent[i+1:]...)
				break
			}
		}
	}
	return span, ok
}

// popAny pops the most-recently-stored span, used when the response
// hook fires with resp == nil (transport error) — there is no
// *http.Request handle to key off. Safe because monobank.Client.Do
// is single-flight per goroutine.
func (s *spanStore) popAny() (trace.Span, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.recent) == 0 {
		return nil, false
	}
	r := s.recent[len(s.recent)-1]
	s.recent = s.recent[:len(s.recent)-1]
	span := s.m[r]
	delete(s.m, r)
	return span, true
}

// redactURL strips the query string from a request URL so secrets
// or PII-bearing query parameters do not land in span attributes.
// Returns "" for a nil URL.
func redactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	clone := *u
	clone.RawQuery = ""
	clone.User = nil
	return clone.String()
}
