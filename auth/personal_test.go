package auth

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
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

// Compile-time check that all three authorizers satisfy the interface.
var (
	_ Authorizer = Public{}
	_ Authorizer = Personal{}
	_ Authorizer = Corp{}
)
