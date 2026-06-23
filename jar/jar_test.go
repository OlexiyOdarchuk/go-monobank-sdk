package jar_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/jar"
)

func TestByLongID_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/bank/jar/abc123", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jarId":"abc123","title":"Test","ownerName":"Doe","amount":12345,"goal":100000,"currency":980,"ownerIcon":"https://x"}`))
	}))
	defer srv.Close()

	c, errNew := jar.New(jar.WithAPIBaseURL(srv.URL))
	require.NoError(t, errNew)
	info, err := c.ByLongID(context.Background(), "abc123")
	require.NoError(t, err)
	assert.Equal(t, "abc123", info.JarID)
	assert.Equal(t, "Test", info.Title)
	assert.Equal(t, int64(12345), info.Amount)
	assert.Equal(t, int64(100000), info.Goal)
	assert.Equal(t, 980, info.Currency)
}

func TestByLongID_notFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errCode":"NOT_FOUND","errText":"jar not found"}`))
	}))
	defer srv.Close()

	c, errNew := jar.New(jar.WithAPIBaseURL(srv.URL))
	require.NoError(t, errNew)
	_, err := c.ByLongID(context.Background(), "missing")
	assert.ErrorIs(t, err, jar.ErrNotFound)
}

func TestByLongID_tmrError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"errCode":"TMR","errText":"Too many requests"}`))
	}))
	defer srv.Close()

	c, errNew := jar.New(jar.WithAPIBaseURL(srv.URL))
	require.NoError(t, errNew)
	_, err := c.ByLongID(context.Background(), "x")
	var apiErr *jar.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "TMR", apiErr.ErrCode)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
}

func TestByLongID_emptyID(t *testing.T) {
	c, errNew := jar.New()
	require.NoError(t, errNew)
	_, err := c.ByLongID(context.Background(), "")
	require.Error(t, err)
}

func TestByShortID_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/handler", r.URL.Path)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "hello", body["c"])
		assert.Equal(t, "shortXY", body["clientId"])
		assert.NotEmpty(t, body["Pc"])

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name":"Bake Sale",
			"description":"Cake for kids",
			"avatar":"https://x",
			"jarAmount":4242,
			"jarGoal":100000,
			"jarStatus":"ACTIVE",
			"isTrusted":true,
			"longJarId":"longABC123"
		}`))
	}))
	defer srv.Close()

	c, errNew := jar.New(jar.WithSendBaseURL(srv.URL))
	require.NoError(t, errNew)
	info, err := c.ByShortID(context.Background(), "shortXY")
	require.NoError(t, err)
	assert.Equal(t, "Bake Sale", info.Name)
	assert.Equal(t, int64(4242), info.JarAmount)
	assert.Equal(t, int64(100000), info.JarGoal)
	assert.True(t, info.IsTrusted)
	assert.Equal(t, "longABC123", info.LongJarID)
}

func TestByShortID_extJarId(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"x","extJarId":"extXYZ"}`))
	}))
	defer srv.Close()

	c, errNew := jar.New(jar.WithSendBaseURL(srv.URL))
	require.NoError(t, errNew)
	info, err := c.ByShortID(context.Background(), "y")
	require.NoError(t, err)
	assert.Equal(t, "extXYZ", info.LongJarID)
}

func TestByShortID_errCodeIn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errCode":"NOT_FOUND","errText":"jar not found"}`))
	}))
	defer srv.Close()

	c, errNew := jar.New(jar.WithSendBaseURL(srv.URL))
	require.NoError(t, errNew)
	_, err := c.ByShortID(context.Background(), "x")
	var apiErr *jar.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "NOT_FOUND", apiErr.ErrCode)
}

func TestByShortID_emptyID(t *testing.T) {
	c, errNew := jar.New()
	require.NoError(t, errNew)
	_, err := c.ByShortID(context.Background(), "")
	require.Error(t, err)
}

func TestAPIError_Error(t *testing.T) {
	e := &jar.APIError{StatusCode: 429, ErrCode: "TMR", ErrText: "boom"}
	assert.Contains(t, e.Error(), "429")
	assert.Contains(t, e.Error(), "TMR")
}

func TestErrNotFoundExported(t *testing.T) {
	// Sanity: errors.Is plays nicely with ErrNotFound.
	assert.True(t, errors.Is(jar.ErrNotFound, jar.ErrNotFound))
}
