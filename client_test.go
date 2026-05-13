package monobank

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testPayload struct {
	Message string `json:"message"`
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
