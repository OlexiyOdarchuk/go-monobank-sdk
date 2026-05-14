package auth

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersonal_setsXTokenHeader(t *testing.T) {
	a := NewPersonal("secret-token")

	r, err := http.NewRequest(http.MethodGet, "/personal/client-info", http.NoBody)
	assert.NoError(t, err)

	assert.NoError(t, a.SetAuth(r))
	assert.Equal(t, "secret-token", r.Header.Get("X-Token"))
}

func TestPersonal_nilRequestIsNoOp(t *testing.T) {
	a := NewPersonal("x")
	// Must not panic; must return nil.
	assert.NoError(t, a.SetAuth(nil))
}

func TestPublic_isNoOp(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "/bank/currency", http.NoBody)

	assert.NoError(t, NewPublic().SetAuth(r))
	// No headers should have been set.
	assert.Empty(t, r.Header.Get("X-Token"))
	assert.Empty(t, r.Header.Get("X-Key-Id"))
	assert.Empty(t, r.Header.Get("X-Sign"))
	assert.Empty(t, r.Header.Get("X-Time"))
}

func TestPublic_nilRequestIsNoOp(t *testing.T) {
	assert.NoError(t, NewPublic().SetAuth(nil))
}

// Token приховується від slog через LogValuer-інтерфейс.
func TestPersonal_LogValueRedactsToken(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	logger.Info("auth ready", "creds", NewPersonal("super-secret-token"))

	out := buf.String()
	assert.NotContains(t, out, "super-secret-token", "токен НЕ повинен потрапити в логи")
	assert.Contains(t, out, "***")
}

// Той самий захист — для CorpAuthMaker із приватним ECDSA-ключем.
func TestCorpAuthMaker_LogValueRedactsPrivateKey(t *testing.T) {
	m, err := NewCorpAuthMaker(secKey)
	require.NoError(t, err)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logger.Info("ready", "maker", m)

	out := buf.String()
	assert.Contains(t, out, m.KeyID, "KeyID — публічний, має бути видимий")
	// Перевіряємо, що жодне 32-байтне поле приватного ключа не
	// потрапило у вивід випадково.
	assert.NotContains(t, strings.ToLower(out), "privatekey:0x")
	assert.Contains(t, out, "privateKey=***")
}

// Compile-time check that all three authorizers satisfy the interface.
var (
	_ Authorizer = Public{}
	_ Authorizer = Personal{}
	_ Authorizer = Corp{}
)
