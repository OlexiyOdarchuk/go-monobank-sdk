package otelmonobank

import (
	"net/http"
	"sync"

	"go.opentelemetry.io/otel/trace"
)

// spanStore — потоковобезпечне сховище span-ів, привʼязаних до
// конкретного *http.Request. Потрібно, бо monobank.Client.Do не дає
// способу пробросити дані з request-hook у response-hook інакше; а
// модифікувати context самого Request у hook-у не допоможе (response
// бачить той самий вказівник, але context уже зрезолвлений).
type spanStore struct {
	mu sync.Mutex
	m  map[*http.Request]trace.Span
}

func newSpanStore() *spanStore {
	return &spanStore{m: make(map[*http.Request]trace.Span)}
}

func (s *spanStore) set(r *http.Request, span trace.Span) {
	s.mu.Lock()
	s.m[r] = span
	s.mu.Unlock()
}

func (s *spanStore) pop(r *http.Request) (trace.Span, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	span, ok := s.m[r]
	if ok {
		delete(s.m, r)
	}
	return span, ok
}
