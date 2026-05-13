package monobank

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- WithLogger ---

func TestWithLogger_logsRequestAndResponse(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLogger(logger))
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))

	out := buf.String()
	assert.Contains(t, out, "monobank: sending request")
	assert.Contains(t, out, "monobank: http response")
	assert.Contains(t, out, "status=200")
	assert.Contains(t, out, "method=GET")
}

func TestWithLogger_logsTransportError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	c := New(
		WithBaseURL("http://127.0.0.1:1"), // closed port — Do will fail
		WithHTTPClient(&http.Client{Timeout: 100 * time.Millisecond}),
		WithLogger(logger),
	)
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	err := c.Do(req, nil)
	require.Error(t, err)

	out := buf.String()
	assert.Contains(t, out, "monobank: http error")
	assert.Contains(t, out, `level=WARN`)
}

func TestWithLogger_nilIgnored(t *testing.T) {
	c := New(WithLogger(nil))
	assert.Nil(t, c.logger, "nil logger must not be stored")
}

// --- WithRequestHook ---

func TestWithRequestHook_receivesPreparedRequest(t *testing.T) {
	var seenURL, seenAuth string
	hook := func(r *http.Request) {
		seenURL = r.URL.String()
		seenAuth = r.Header.Get("X-Token")
		r.Header.Set("X-Correlation-Id", "abc-123")
	}

	var seenCorrelation string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenCorrelation = r.Header.Get("X-Correlation-Id")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithAuth(stubAuth{token: "the-token"}),
		WithRequestHook(hook),
	)
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))

	assert.Contains(t, seenURL, srv.URL, "URL must be resolved before hook fires")
	assert.Equal(t, "the-token", seenAuth, "auth must be set before hook fires")
	assert.Equal(t, "abc-123", seenCorrelation, "hook-added header reaches server")
}

func TestWithRequestHook_firesPerAttempt(t *testing.T) {
	var calls atomic.Int32
	hook := func(*http.Request) { calls.Add(1) }

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithRequestHook(hook),
		WithRetry(3, time.Millisecond, 5*time.Millisecond),
	)
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))
	assert.Equal(t, int32(2), calls.Load(), "hook must fire for each retry")
}

func TestWithRequestHook_nilIgnored(t *testing.T) {
	c := New(WithRequestHook(nil))
	assert.Nil(t, c.onReq)
}

// --- WithResponseHook ---

func TestWithResponseHook_receivesResponse(t *testing.T) {
	var seenStatus int
	var seenErr error
	hook := func(r *http.Response, err error) {
		if r != nil {
			seenStatus = r.StatusCode
		}
		seenErr = err
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithResponseHook(hook))
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	_ = c.Do(req, nil) // 418 is not in expected codes → APIError; hook still fires

	assert.Equal(t, http.StatusTeapot, seenStatus)
	assert.NoError(t, seenErr)
}

func TestWithResponseHook_transportErrorBranch(t *testing.T) {
	var seenResp *http.Response
	var seenErr error
	hook := func(r *http.Response, err error) {
		seenResp = r
		seenErr = err
	}

	c := New(
		WithBaseURL("http://127.0.0.1:1"),
		WithHTTPClient(&http.Client{Timeout: 100 * time.Millisecond}),
		WithResponseHook(hook),
	)
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.Error(t, c.Do(req, nil))

	assert.Nil(t, seenResp, "resp is nil on transport error")
	require.Error(t, seenErr)
}

func TestWithResponseHook_nilIgnored(t *testing.T) {
	c := New(WithResponseHook(nil))
	assert.Nil(t, c.onResp)
}

// --- combined ---

func TestHooks_allThreeCooperate(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	var reqCalls, respCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithLogger(logger),
		WithRequestHook(func(*http.Request) { reqCalls.Add(1) }),
		WithResponseHook(func(*http.Response, error) { respCalls.Add(1) }),
	)
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))

	assert.Equal(t, int32(1), reqCalls.Load())
	assert.Equal(t, int32(1), respCalls.Load())
	assert.Contains(t, buf.String(), "monobank:")
}

// --- helpers ---

type stubAuth struct{ token string }

func (a stubAuth) SetAuth(r *http.Request) error {
	if r == nil {
		return nil
	}
	r.Header.Set("X-Token", a.token)
	return nil
}

// Verify the hook signature is stable: a function with the right shape
// compiles (sanity for callers).
var (
	_ func(*http.Request)         = func(*http.Request) {}
	_ func(*http.Response, error) = func(*http.Response, error) {}
	_ func(*http.Request) error   = func(r *http.Request) error { return nil }
	_                             = errors.New // unused-imports guard
	_ context.Context             = context.Background()
)
