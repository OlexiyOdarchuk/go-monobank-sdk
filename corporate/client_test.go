package corporate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAuth is a no-op authorizer used by tests that don't care about
// actual signing.
type stubAuth struct{}

func (stubAuth) SetAuth(_ *http.Request) error { return nil }

// stubMaker satisfies auth.CorpAuthMakerAPI for tests.
type stubMaker struct{}

func (stubMaker) New(_ string) auth.Authorizer                        { return stubAuth{} }
func (stubMaker) NewPermissions(_ ...auth.Permission) auth.Authorizer { return stubAuth{} }

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(stubMaker{}, monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	return c
}

func TestNew_nilMaker(t *testing.T) {
	_, err := New(nil)
	assert.ErrorIs(t, err, ErrEmptyAuthMaker)
}

func TestAuth(t *testing.T) {
	var seenCallback string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/personal/auth/request", r.URL.Path)
		seenCallback = r.Header.Get("X-Callback")
		_, _ = w.Write([]byte(`{"tokenRequestId":"rq-1","acceptUrl":"https://accept"}`))
	})

	out, err := c.Auth(context.Background(), "https://yourapp/cb", auth.PermSt)
	require.NoError(t, err)
	assert.Equal(t, "rq-1", out.RequestID)
	assert.Equal(t, "https://yourapp/cb", seenCallback)
}

func TestCheckAuth(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/personal/auth/request", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})

	require.NoError(t, c.CheckAuth(context.Background(), "rq-1"))
}

func TestRegister(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/personal/auth/registration", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var got RegistrationRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "Acme", got.Name)
		_, _ = w.Write([]byte(`{"status":"New"}`))
	})

	out, err := c.Register(context.Background(), &RegistrationRequest{
		Pubkey: []byte("PEM"), Name: "Acme", Description: "x",
		ContactPerson: "P", Phone: "+380", Email: "e@x", Logo: []byte{1},
	})
	require.NoError(t, err)
	assert.Equal(t, RegistrationNew, out.Status)
}

func TestRegister_nil(t *testing.T) {
	c, _ := New(stubMaker{})
	_, err := c.Register(context.Background(), nil)
	assert.ErrorIs(t, err, ErrNilRequest)
}

func TestRegistrationStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/personal/auth/registration/status", r.URL.Path)
		_, _ = w.Write([]byte(`{"status":"Approved","keyId":"abc"}`))
	})

	out, err := c.RegistrationStatus(context.Background(), []byte("PEM"))
	require.NoError(t, err)
	assert.Equal(t, RegistrationApproved, out.Status)
	assert.Equal(t, "abc", out.KeyID)
}

func TestSignatureCreate(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/personal/signature/create", r.URL.Path)
		_, _ = w.Write([]byte(`{"requestId":"r1","deeplink":"https://mbnk.app/x"}`))
	})

	out, err := c.SignatureCreate(context.Background(), &SignatureCreateRequest{
		Documents: []Document{{Name: "Договір", Hash: "A4"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "r1", out.RequestID)
}

func TestSignatureStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/personal/signature/status", r.URL.Path)
		assert.Equal(t, "r-42", r.URL.Query().Get("requestId"))
		_, _ = w.Write([]byte(`{"documents":[{"name":"x","hash":"A","status":"signed"}]}`))
	})

	out, err := c.SignatureStatus(context.Background(), "r-42")
	require.NoError(t, err)
	require.Len(t, out.Documents, 1)
	assert.Equal(t, DocSigned, out.Documents[0].Status)
}

func TestSignatureCancel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/personal/signature/cancel", r.URL.Path)
		assert.Equal(t, "r-9", r.URL.Query().Get("requestId"))
		_, _ = w.Write([]byte(`{}`))
	})

	require.NoError(t, c.SignatureCancel(context.Background(), "r-9"))
}

func TestGetSettings(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/personal/corp/settings", r.URL.Path)
		_, _ = w.Write([]byte(`{"name":"Acme","permission":"sp"}`))
	})

	out, err := c.GetSettings(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Acme", out.Name)
}

func TestSetWebHook(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/personal/corp/webhook", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var got webhookRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "https://x", got.WebHookURL)
		w.WriteHeader(http.StatusOK)
	})

	require.NoError(t, c.SetWebHook(context.Background(), "https://x"))
}
