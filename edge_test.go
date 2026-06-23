package monobank

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
)

// --- APIError ---

func TestAPIError_ErrorContainsMethodURLAndStatus(t *testing.T) {
	e := &APIError{
		Method:              http.MethodPost,
		URL:                 "https://api.monobank.ua/personal/webhook",
		StatusCode:          http.StatusUnauthorized,
		ExpectedStatusCodes: []int{http.StatusOK},
		Body:                []byte(`{"errorDescription":"bad token"}`),
	}
	s := e.Error()
	assert.Contains(t, s, http.MethodPost)
	assert.Contains(t, s, "/personal/webhook")
	assert.Contains(t, s, "HTTP 401")
	assert.Contains(t, s, "bad token")
}

func TestAPIError_truncatesLongBody(t *testing.T) {
	long := bytes.Repeat([]byte{'x'}, 1024)
	e := &APIError{StatusCode: 500, Body: long}
	s := e.Error()
	// truncate keeps 256 chars + the ellipsis marker.
	assert.Contains(t, s, "…")
	assert.Less(t, len(s), 1024)
}

// --- SetBaseURL / WithBaseURL ---

func TestSetBaseURL_invalidNoOp(t *testing.T) {
	c := New()
	before := c.baseURL.String()
	c.SetBaseURL("://broken url with spaces")
	// Implementation no-ops on parse failure; baseURL remains the default.
	assert.Equal(t, before, c.baseURL.String())
}

// --- Options ---

func TestWithHTTPDoer(t *testing.T) {
	doer := http.DefaultClient
	c := New(WithHTTPDoer(doer))
	assert.NotNil(t, c.http)
}

func TestWithHTTPDoer_nilIgnored(t *testing.T) {
	c := New(WithHTTPDoer(nil))
	assert.NotNil(t, c.http, "nil doer must not zero out the field")
}

func TestWithHTTPClient_nilIgnored(t *testing.T) {
	c := New(WithHTTPClient(nil))
	assert.NotNil(t, c.http, "nil client must not zero out the field")
}

func TestWithAuth_nilIgnored(t *testing.T) {
	c := New(WithAuth(nil))
	// Default auth.Public stays in place.
	_, ok := c.auth.(auth.Public)
	assert.True(t, ok)
}

func TestWithRetry_zeroAttemptsInheritsDefaults(t *testing.T) {
	c := New(WithRetry(0, 0, 0))
	// defaultRetry has maxAttempts=4, baseDelay=500ms, maxDelay=30s.
	assert.Equal(t, defaultRetry.maxAttempts, c.retry.maxAttempts)
	assert.Equal(t, defaultRetry.baseDelay, c.retry.baseDelay)
}

func TestWithRetry_explicit(t *testing.T) {
	c := New(WithRetry(7, 100*time.Millisecond, 5*time.Second))
	assert.Equal(t, 7, c.retry.maxAttempts)
	assert.Equal(t, 100*time.Millisecond, c.retry.baseDelay)
	assert.Equal(t, 5*time.Second, c.retry.maxDelay)
}

func TestWithRetry_inheritsZeroSubFields(t *testing.T) {
	// Setting only attempts must inherit base/max from defaults.
	c := New(WithRetry(5, 0, 0))
	assert.Equal(t, 5, c.retry.maxAttempts)
	assert.Equal(t, defaultRetry.baseDelay, c.retry.baseDelay)
	assert.Equal(t, defaultRetry.maxDelay, c.retry.maxDelay)
}

// --- Do edge cases ---

func TestDo_nilRequest(t *testing.T) {
	c := New()
	err := c.Do(nil, nil)
	assert.ErrorIs(t, err, ErrEmptyRequest)
}

func TestDo_writesToIOWriter(t *testing.T) {
	want := "binary-blob"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(want))
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)

	var buf bytes.Buffer
	require.NoError(t, c.Do(req, &buf))
	assert.Equal(t, want, buf.String())
}

func TestDo_authErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("server must not be hit when SetAuth errors out")
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithAuth(errAuth{}))
	req, _ := http.NewRequest(http.MethodGet, "/x", http.NoBody)
	err := c.Do(req, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SetAuth")
}

// errAuth always fails — used to drive the SetAuth error branch.
type errAuth struct{}

func (errAuth) SetAuth(_ *http.Request) error { return errors.New("auth boom") }

// --- transient + retry edge ---

func TestTransientStatus_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	e := &transientStatus{code: 503, cause: cause}
	assert.ErrorIs(t, e, cause, "transientStatus must unwrap to cause")
}

func TestBackoff_baseZeroReturnsZero(t *testing.T) {
	assert.Equal(t, time.Duration(0), backoff(0, time.Second, 3))
}

func TestBackoff_respectsMax(t *testing.T) {
	// attempt=10 with base=1s would be 1024s; max caps it at 5s.
	got := backoff(time.Second, 5*time.Second, 10)
	assert.LessOrEqual(t, got, 5*time.Second)
}

// --- HTTPDoer custom transport ---

type recordingDoer struct {
	calls int
	last  *http.Request
}

func (d *recordingDoer) Do(req *http.Request) (*http.Response, error) {
	d.calls++
	d.last = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Header:     make(http.Header),
	}, nil
}

func TestWithHTTPDoer_usedForRequests(t *testing.T) {
	doer := &recordingDoer{}
	c := New(WithHTTPDoer(doer))

	req, _ := http.NewRequest(http.MethodGet, "/path", http.NoBody)
	require.NoError(t, c.Do(req, nil))
	assert.Equal(t, 1, doer.calls)
	require.NotNil(t, doer.last)
	assert.Equal(t, "/path", doer.last.URL.Path)
}
