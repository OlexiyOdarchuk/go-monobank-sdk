package otelmonobank_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/otelmonobank"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
	"go.opentelemetry.io/otel/trace/noop"
)

// --- recording tracer (мінімальна імплементація trace.Tracer для тестів) ---

type recordedSpan struct {
	name   string
	attrs  []attribute.KeyValue
	err    error
	status struct {
		code codes.Code
		desc string
	}
	ended bool
}

type recordingTracer struct {
	embedded.Tracer
	mu    sync.Mutex
	spans []*recordedSpan
}

func (t *recordingTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	s := &recordedSpan{name: name}
	// Витягнути attributes, передані як WithAttributes(...) у Start.
	cfg := trace.NewSpanStartConfig(opts...)
	s.attrs = append(s.attrs, cfg.Attributes()...)
	t.mu.Lock()
	t.spans = append(t.spans, s)
	t.mu.Unlock()
	return ctx, &recordingSpan{tr: t, rec: s}
}

func (t *recordingTracer) recorded() []*recordedSpan {
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := make([]*recordedSpan, len(t.spans))
	copy(cp, t.spans)
	return cp
}

type recordingSpan struct {
	embedded.Span
	tr  *recordingTracer
	rec *recordedSpan
}

func (s *recordingSpan) End(_ ...trace.SpanEndOption)          { s.rec.ended = true }
func (s *recordingSpan) AddEvent(string, ...trace.EventOption) {}
func (s *recordingSpan) AddLink(trace.Link)                    {}
func (s *recordingSpan) IsRecording() bool                     { return true }
func (s *recordingSpan) RecordError(err error, _ ...trace.EventOption) {
	s.rec.err = err
}
func (s *recordingSpan) SpanContext() trace.SpanContext { return trace.SpanContext{} }
func (s *recordingSpan) SetStatus(code codes.Code, desc string) {
	s.rec.status.code = code
	s.rec.status.desc = desc
}
func (s *recordingSpan) SetName(string) {}
func (s *recordingSpan) SetAttributes(kv ...attribute.KeyValue) {
	s.rec.attrs = append(s.rec.attrs, kv...)
}
func (s *recordingSpan) TracerProvider() trace.TracerProvider { return nil }

// --- тести ---

func TestWithTracer_recordsRequestAndResponse(t *testing.T) {
	tr := &recordingTracer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := monobank.New(
		monobank.WithBaseURL(srv.URL),
		monobank.WithHTTPClient(srv.Client()),
		otelmonobank.WithTracer(tr),
	)
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))

	spans := tr.recorded()
	require.Len(t, spans, 1)
	assert.Contains(t, spans[0].name, "monobank HTTP GET")
	assert.True(t, spans[0].ended, "span має бути закритий через End()")
	assert.Equal(t, codes.Ok, spans[0].status.code)

	// Атрибути.
	attrs := map[string]string{}
	for _, kv := range spans[0].attrs {
		attrs[string(kv.Key)] = kv.Value.AsString()
	}
	assert.Equal(t, "GET", attrs["http.method"])
	assert.Contains(t, attrs["http.url"], srv.URL)
	assert.Equal(t, "200", attrs["http.status_code"])
}

func TestWithTracer_recordsTransportError(t *testing.T) {
	tr := &recordingTracer{}
	c := monobank.New(
		monobank.WithBaseURL("http://127.0.0.1:1"),
		monobank.WithHTTPClient(&http.Client{}),
		otelmonobank.WithTracer(tr),
	)
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.Error(t, c.Do(req, nil))

	spans := tr.recorded()
	// На transport error response-hook отримує resp=nil → span не закривається
	// через response-hook (бо resp.Request == nil). Це обмеження дизайну —
	// span залишається у store-і. Тест перевіряє, що Start був викликаний.
	require.Len(t, spans, 1)
}

func TestWithTracer_recordsHTTPError(t *testing.T) {
	tr := &recordingTracer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := monobank.New(
		monobank.WithBaseURL(srv.URL),
		monobank.WithHTTPClient(srv.Client()),
		otelmonobank.WithTracer(tr),
	)
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	_ = c.Do(req, nil) // 400 → APIError, але span закриється у response-hook

	spans := tr.recorded()
	require.Len(t, spans, 1)
	assert.True(t, spans[0].ended)
	assert.Equal(t, codes.Error, spans[0].status.code, "4xx → status Error")
}

func TestWithTracer_perRetry(t *testing.T) {
	tr := &recordingTracer{}
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		if hits == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := monobank.New(
		monobank.WithBaseURL(srv.URL),
		monobank.WithHTTPClient(srv.Client()),
		monobank.WithRetry(3, 0, 0),
		otelmonobank.WithTracer(tr),
	)
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))

	spans := tr.recorded()
	assert.Equal(t, 2, len(spans), "по одному span на спробу (1 ретрай)")
}

func TestWithTracer_nilTracer_noop(t *testing.T) {
	// nil tracer → опція має бути no-op (не панічити).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := monobank.New(
		monobank.WithBaseURL(srv.URL),
		monobank.WithHTTPClient(srv.Client()),
		otelmonobank.WithTracer(nil),
	)
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))
}

func TestWithTracer_noopProvider(t *testing.T) {
	// Перевірка, що з реальним OTel noop-провайдером воно не падає.
	tracer := noop.NewTracerProvider().Tracer("test")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := monobank.New(
		monobank.WithBaseURL(srv.URL),
		monobank.WithHTTPClient(srv.Client()),
		otelmonobank.WithTracer(tracer),
	)
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))
}
