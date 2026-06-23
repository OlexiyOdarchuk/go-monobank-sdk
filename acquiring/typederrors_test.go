package acquiring_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring"
)

func apiErr(status int, body string) *monobank.APIError {
	return &monobank.APIError{StatusCode: status, Body: []byte(body)}
}

func TestAsAPIError_parsesErrCode(t *testing.T) {
	e, ok := acquiring.AsAPIError(apiErr(404, `{"errCode":"NOT_FOUND","errText":"invalid 'invoiceId'"}`))
	require.True(t, ok)
	assert.Equal(t, acquiring.CodeNotFound, e.Code)
	assert.Equal(t, "invalid 'invoiceId'", e.Text)
	assert.Equal(t, 404, e.StatusCode)
}

func TestAsAPIError_nonAPIError(t *testing.T) {
	_, ok := acquiring.AsAPIError(errors.New("boom"))
	assert.False(t, ok)
	_, ok = acquiring.AsAPIError(nil)
	assert.False(t, ok)
	_, ok = acquiring.AsAPIError(context.Canceled)
	assert.False(t, ok)
}

func TestAsAPIError_wrappedChain(t *testing.T) {
	wrapped := errors.Join(errors.New("ctx"), apiErr(429, `{"errCode":"TMR","errText":"too many requests"}`))
	e, ok := acquiring.AsAPIError(wrapped)
	require.True(t, ok)
	assert.Equal(t, acquiring.CodeTooManyRequests, e.Code)
}

func TestPredicates(t *testing.T) {
	assert.True(t, acquiring.IsNotFound(apiErr(404, `{"errCode":"NOT_FOUND"}`)))
	assert.True(t, acquiring.IsTooManyRequests(apiErr(429, `{"errCode":"TMR"}`)))
	assert.True(t, acquiring.IsBadRequest(apiErr(400, `{"errCode":"BAD_REQUEST"}`)))
	assert.True(t, acquiring.IsForbidden(apiErr(403, `{"errCode":"FORBIDDEN"}`)))
	assert.True(t, acquiring.IsInternalError(apiErr(500, `{"errCode":"INTERNAL_ERROR"}`)))

	assert.False(t, acquiring.IsNotFound(apiErr(429, `{"errCode":"TMR"}`)))
	assert.False(t, acquiring.IsNotFound(errors.New("nope")))
}

func TestPredicates_fallBackToStatus(t *testing.T) {
	// Тіло без errCode (наприклад HTML 404 від проксі) — падаємо на статус.
	assert.True(t, acquiring.IsNotFound(apiErr(404, `<html>not found</html>`)))
	assert.True(t, acquiring.IsTooManyRequests(apiErr(http.StatusTooManyRequests, ``)))
}

func TestAPIError_unwrapKeepsSentinels(t *testing.T) {
	e, ok := acquiring.AsAPIError(apiErr(404, `{"errCode":"NOT_FOUND"}`))
	require.True(t, ok)
	// Стандартні sentinel-и monobank досі матчаться крізь Unwrap.
	assert.True(t, errors.Is(e, monobank.ErrNotFound))
}

func TestCode_helper(t *testing.T) {
	assert.Equal(t, acquiring.CodeBadRequest, acquiring.Code(apiErr(400, `{"errCode":"BAD_REQUEST"}`)))
	assert.Equal(t, acquiring.ErrCode(""), acquiring.Code(errors.New("x")))
}
