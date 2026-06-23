package corporate

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

func newErrorClient(t *testing.T) *Client {
	t.Helper()
	return newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errorDescription":"boom"}`))
	})
}

func assertCorpAPIError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var apiErr *monobank.APIError
	require.True(t, errors.As(err, &apiErr), "want *monobank.APIError, got %T", err)
	assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
}

func TestCorporate_errorPaths(t *testing.T) {
	ctx := context.Background()
	from := time.Unix(1_700_000_000, 0)

	t.Run("Auth", func(t *testing.T) {
		_, err := newErrorClient(t).Auth(ctx, "https://cb")
		assertCorpAPIError(t, err)
	})
	t.Run("CheckAuth", func(t *testing.T) {
		err := newErrorClient(t).CheckAuth(ctx, "rq-1")
		assertCorpAPIError(t, err)
	})
	t.Run("GetSettings", func(t *testing.T) {
		_, err := newErrorClient(t).GetSettings(ctx)
		assertCorpAPIError(t, err)
	})
	t.Run("SetWebHook", func(t *testing.T) {
		err := newErrorClient(t).SetWebHook(ctx, "https://wh")
		assertCorpAPIError(t, err)
	})
	t.Run("SignatureCreate", func(t *testing.T) {
		_, err := newErrorClient(t).SignatureCreate(ctx, &SignatureCreateRequest{Documents: []Document{{}}})
		assertCorpAPIError(t, err)
	})
	t.Run("SignatureStatus", func(t *testing.T) {
		_, err := newErrorClient(t).SignatureStatus(ctx, "rq-1")
		assertCorpAPIError(t, err)
	})
	t.Run("SignatureCancel", func(t *testing.T) {
		err := newErrorClient(t).SignatureCancel(ctx, "rq-1")
		assertCorpAPIError(t, err)
	})
	t.Run("Register", func(t *testing.T) {
		_, err := newErrorClient(t).Register(ctx, &RegistrationRequest{Pubkey: []byte("pem")})
		assertCorpAPIError(t, err)
	})
	t.Run("RegistrationStatus", func(t *testing.T) {
		_, err := newErrorClient(t).RegistrationStatus(ctx, []byte("pem"))
		assertCorpAPIError(t, err)
	})
	t.Run("Transactions", func(t *testing.T) {
		_, err := newErrorClient(t).Transactions(ctx, "rq", "acc", from, from.Add(time.Hour))
		assertCorpAPIError(t, err)
	})
}
