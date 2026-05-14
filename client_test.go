package monobank

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testPayload struct {
	Message string `json:"message"`
}

// roundTripperFunc — функціональний адаптер для http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestClient_Do(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	req, err := http.NewRequest(http.MethodGet, "/test", http.NoBody)
	require.NoError(t, err)

	var out testPayload
	require.NoError(t, c.Do(req, &out, http.StatusOK))
	assert.Equal(t, "ok", out.Message)
}

func TestClient_Do_apiError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errorDescription":"Missing X-Token"}`))
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test", http.NoBody)

	err := c.Do(req, nil)
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Contains(t, string(apiErr.Body), "Missing X-Token")
}

func TestClient_Do_multipleExpectedStatusCodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil, http.StatusOK, http.StatusCreated))

	// Only accept 200 — same response now fails.
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/x", http.NoBody)
	var apiErr *APIError
	require.ErrorAs(t, c.Do(req2, nil, http.StatusOK), &apiErr)
	assert.Equal(t, http.StatusCreated, apiErr.StatusCode)
}

// fakeAuth lets us observe that the configured Authorizer is invoked.
type fakeAuth struct{ called bool }

func (f *fakeAuth) SetAuth(_ *http.Request) error { f.called = true; return nil }

func TestClient_Do_callsAuthorizer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	fa := &fakeAuth{}
	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithAuth(fa))
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))
	assert.True(t, fa.called, "configured Authorizer must run")
}

func TestClient_Do_rawBytes(t *testing.T) {
	want := []byte("%PDF-1.4 fake")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(want)
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	req, _ := http.NewRequest(http.MethodGet, "/pdf", http.NoBody)
	var got []byte
	require.NoError(t, c.Do(req, &got))
	assert.Equal(t, want, got)
}

func TestNew_defaultsToPublicAuth(t *testing.T) {
	c := New()
	_, ok := c.auth.(auth.Public)
	assert.True(t, ok, "default auth must be auth.Public")
}

func TestNew_appliesOptions(t *testing.T) {
	base, _ := url.Parse("https://example.com")
	c := New(WithBaseURL("https://example.com"))
	assert.Equal(t, base.String(), c.baseURL.String())
}

// тіло POST мусить бути валідним на КОЖНІЙ retry-спробі.
// До фіксу другий attempt отримував порожнє тіло, бо http.Transport
// повністю споживав Body на першому Do.
func TestClient_Do_retriesPreserveBody(t *testing.T) {
	const want = `{"hello":"world"}`
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, want, string(body),
			"attempt #%d body mismatch", hits.Load()+1)

		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithRetry(3, time.Millisecond, 10*time.Millisecond),
	)
	req, _ := http.NewRequest(http.MethodPost, "/x", strings.NewReader(want))
	req.Header.Set("Idempotency-Key", "test-key") // інакше POST не ретраїться (C2)
	require.NoError(t, c.Do(req, nil))
	assert.Equal(t, int32(2), hits.Load())
}

// POST без Idempotency-Key НЕ повинен ретраїтись на 503.
func TestClient_Do_postWithoutIdempotencyKeyNotRetried(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := New(
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithRetry(5, time.Millisecond, 10*time.Millisecond),
	)
	req, _ := http.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
	err := c.Do(req, nil)
	require.Error(t, err)
	assert.Equal(t, int32(1), hits.Load(),
		"POST без Idempotency-Key мусить виконатися рівно один раз")
}

// POST з Idempotency-Key ретраїться як ідемпотентний.
func TestClient_Do_postWithIdempotencyKeyRetried(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithRetry(3, time.Millisecond, 10*time.Millisecond),
	)
	req, _ := http.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
	req.Header.Set("Idempotency-Key", "k1")
	require.NoError(t, c.Do(req, nil))
	assert.Equal(t, int32(2), hits.Load())
}

// 204 No Content із non-nil v — не повинно бути EOF-помилки.
func TestClient_Do_204NoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	req, _ := http.NewRequest(http.MethodDelete, "/x", http.NoBody)

	var out testPayload
	err := c.Do(req, &out, http.StatusNoContent)
	assert.NoError(t, err, "204 must not cause decode EOF")
	assert.Equal(t, "", out.Message, "out unchanged when body is empty")
}

// Content-Length: 0 з очікуваним 200 (нестандартно, але
// можливо) — також не EOF.
func TestClient_Do_emptyBody200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	var out testPayload
	assert.NoError(t, c.Do(req, &out))
}

// APIError.Is має зробити errors.Is(err, ErrUnauthorized)
// тощо валідним для відповідних HTTP-статусів.
func TestAPIError_IsSentinels(t *testing.T) {
	cases := map[int]error{
		http.StatusUnauthorized:    ErrUnauthorized,
		http.StatusForbidden:       ErrForbidden,
		http.StatusNotFound:        ErrNotFound,
		http.StatusTooManyRequests: ErrTooManyRequests,
	}
	for code, sentinel := range cases {
		t.Run(http.StatusText(code), func(t *testing.T) {
			apiErr := &APIError{StatusCode: code}
			assert.True(t, apiErr.Is(sentinel),
				"APIError(%d).Is must match its sentinel", code)
			assert.True(t, errors.Is(apiErr, sentinel),
				"errors.Is must chain through APIError")
		})
	}

	// Status, який не має sentinel-у, — не матчиться.
	apiErr := &APIError{StatusCode: http.StatusInternalServerError}
	assert.False(t, errors.Is(apiErr, ErrUnauthorized))
}

// errors.As повинен витягати APIError навіть коли
// користувач робив errors.Is(err, ErrUnauthorized) — тобто і chain, і
// sentinel працюють одночасно.
func TestAPIError_AsAndIsBoth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorDescription":"Unknown 'X-Token'"}`))
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	err := c.Do(req, nil)

	assert.True(t, errors.Is(err, ErrUnauthorized))

	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	assert.Equal(t, "Unknown 'X-Token'", apiErr.ErrorDescription)
}

// WithBaseURL мусить warn-логнути при не-https + не-localhost.
func TestWithBaseURL_warnsOnInsecureScheme(t *testing.T) {
	cases := map[string]bool{
		"http://api.example.com":  true,  // має варн
		"http://localhost:8080":   false, // ОК
		"http://127.0.0.1":        false, // ОК
		"https://api.example.com": false, // ОК
	}
	for uri, wantInsecure := range cases {
		t.Run(uri, func(t *testing.T) {
			assert.Equal(t, wantInsecure, isInsecureBaseURL(uri))
		})
	}
}

// User-Agent виставляється автоматично у форматі "go-monobank-sdk/...".
func TestClient_Do_setsUserAgent(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))

	assert.Contains(t, ua, "go-monobank-sdk/")
	assert.Contains(t, ua, runtime.Version(), "має містити Go-версію")
}

// WithUserAgent перевизначає дефолтний User-Agent.
func TestClient_Do_withUserAgentOverride(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithUserAgent("acme/1.2.3 go-monobank-sdk/v1.2.0"),
	)
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))
	assert.Equal(t, "acme/1.2.3 go-monobank-sdk/v1.2.0", ua)
}

// Якщо користувач сам виставив User-Agent на req, SDK його не зачепить.
func TestClient_Do_userProvidedUserAgentWins(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	req.Header.Set("User-Agent", "mine/1")
	require.NoError(t, c.Do(req, nil))
	assert.Equal(t, "mine/1", ua)
}

// http://-URL на не-loopback-хост відхиляється як ErrInsecureBaseURL.
func TestWithBaseURL_rejectsInsecureForNonLoopback(t *testing.T) {
	c := New(WithBaseURL("http://api.example.com"))
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	err := c.Do(req, nil)
	assert.ErrorIs(t, err, ErrInsecureBaseURL)
}

// localhost / 127.0.0.1 / [::1] пропускаються без помилки (для httptest).
func TestWithBaseURL_allowsLoopback(t *testing.T) {
	cases := []string{"http://localhost:8080", "http://127.0.0.1:8080", "http://[::1]:8080"}
	for _, uri := range cases {
		t.Run(uri, func(t *testing.T) {
			c := New(WithBaseURL(uri))
			assert.NoError(t, c.optErr)
		})
	}
}

// WithInsecureBaseURL дозволяє http:// на зовнішній хост свідомо.
func TestWithInsecureBaseURL_overridesGuard(t *testing.T) {
	c := New(
		WithInsecureBaseURL(true),
		WithBaseURL("http://staging.example.com"),
	)
	assert.NoError(t, c.optErr)
}

// Regression: option order must NOT matter — putting
// WithInsecureBaseURL after WithBaseURL used to leave the guard in
// place because options ran sequentially. New now uses a two-pass
// apply.
func TestWithInsecureBaseURL_orderDoesNotMatter(t *testing.T) {
	for name, opts := range map[string][]Option{
		"insecure-first": {
			WithInsecureBaseURL(true),
			WithBaseURL("http://staging.example.com"),
		},
		"insecure-last": {
			WithBaseURL("http://staging.example.com"),
			WithInsecureBaseURL(true),
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := New(opts...)
			assert.NoError(t, c.optErr,
				"WithInsecureBaseURL must take effect regardless of position")
		})
	}
}

// Regression: 127.0.0.2 is loopback per net.IP.IsLoopback (whole
// 127.0.0.0/8), but the old hostname-equality check rejected it.
func TestWithBaseURL_loopbackIsBroaderThan127001(t *testing.T) {
	for _, uri := range []string{
		"http://127.0.0.2:8080",
		"http://127.42.0.1:8080",
	} {
		t.Run(uri, func(t *testing.T) {
			c := New(WithBaseURL(uri))
			assert.NoError(t, c.optErr,
				"the whole 127.0.0.0/8 must be treated as loopback")
		})
	}
}

// WithRoundTripper підмінює http.Transport, зберігаючи інші
// налаштування http.Client (наприклад, таймаут).
func TestWithRoundTripper_appliesMiddleware(t *testing.T) {
	var seen []string
	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		seen = append(seen, r.URL.Path)
		return http.DefaultTransport.RoundTrip(r)
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithRoundTripper(rt))
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	require.NoError(t, c.Do(req, nil))
	assert.Contains(t, seen, "/x")
}

// WithRoundTripper зберігає таймаут і Cookie-jar вбудованого http.Client.
func TestWithRoundTripper_preservesHTTPClientSettings(t *testing.T) {
	custom := &http.Client{Timeout: 7 * time.Second}
	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return http.DefaultTransport.RoundTrip(r)
	})

	c := New(WithHTTPClient(custom), WithRoundTripper(rt))
	hc, ok := c.http.(*http.Client)
	require.True(t, ok)
	assert.Equal(t, 7*time.Second, hc.Timeout, "WithRoundTripper мусить зберегти Timeout")
	assert.NotNil(t, hc.Transport)
}

// nil — ігнорується.
func TestWithRoundTripper_nilIgnored(t *testing.T) {
	c := New(WithRoundTripper(nil))
	assert.NotNil(t, c.http)
}

// Close() зупиняє sweeper KeyedLimiter-а (немає leak goroutine-у).
func TestClient_Close_stopsKeyedLimiter(t *testing.T) {
	klim := NewKeyedLimiter(time.Minute, 1, 50*time.Millisecond)
	c := New(WithRateLimiter(klim))
	require.NoError(t, c.Close())
}

// shouldRetry правильно різнить методи.
func TestShouldRetry_methodMatrix(t *testing.T) {
	mkReq := func(method string, idem bool) *http.Request {
		r, _ := http.NewRequest(method, "/x", nil)
		if idem {
			r.Header.Set("Idempotency-Key", "k1")
		}
		return r
	}
	c := New()
	assert.True(t, c.shouldRetry(mkReq(http.MethodGet, false)))
	assert.True(t, c.shouldRetry(mkReq(http.MethodHead, false)))
	assert.True(t, c.shouldRetry(mkReq(http.MethodPut, false)))
	assert.True(t, c.shouldRetry(mkReq(http.MethodDelete, false)))
	assert.False(t, c.shouldRetry(mkReq(http.MethodPost, false)))
	assert.True(t, c.shouldRetry(mkReq(http.MethodPost, true)), "POST з Idempotency-Key — ретрай дозволений")
	assert.False(t, c.shouldRetry(mkReq(http.MethodPatch, false)))
	assert.True(t, c.shouldRetry(mkReq(http.MethodPatch, true)))

	cUnsafe := New(WithUnsafeRetries(true))
	assert.True(t, cUnsafe.shouldRetry(mkReq(http.MethodPost, false)),
		"WithUnsafeRetries дозволяє POST без ключа")
}

// WithUnsafeRetries(true) дозволяє ретраїти POST без ключа.
func TestClient_Do_unsafeRetriesAllowsPostRetry(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithRetry(3, time.Millisecond, 10*time.Millisecond),
		WithUnsafeRetries(true),
	)
	req, _ := http.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
	require.NoError(t, c.Do(req, nil))
	assert.Equal(t, int32(2), hits.Load())
}
