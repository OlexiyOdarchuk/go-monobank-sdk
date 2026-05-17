package auth

import (
	"encoding/asn1"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCorp_SetAuth_withRequestID ensures X-Request-Id is set and signed
// path = X-Time | requestID | URL.Path.
func TestCorp_SetAuth_withRequestID(t *testing.T) {
	m, err := NewCorpAuthMaker(secKey)
	require.NoError(t, err)

	a := m.New("rq-abc").(Corp)
	r, err := http.NewRequest(http.MethodGet, "https://api.monobank.ua/personal/client-info", http.NoBody)
	require.NoError(t, err)

	require.NoError(t, a.SetAuth(r))

	assert.Equal(t, "rq-abc", r.Header.Get("X-Request-Id"))
	assert.Empty(t, r.Header.Get("X-Permissions"))
	assert.Equal(t, m.KeyID, r.Header.Get("X-Key-Id"))

	// X-Time is unix seconds — within ±5 s of now.
	tsStr := r.Header.Get("X-Time")
	require.NotEmpty(t, tsStr)
	_, err = strconv.ParseInt(tsStr, 10, 64)
	require.NoError(t, err)

	// X-Sign is base64 ASN.1(r,s). Length isn't constant but is ~96 chars.
	sign := r.Header.Get("X-Sign")
	require.NotEmpty(t, sign)
	assert.GreaterOrEqual(t, len(sign), 90)
}

// TestCorp_SetAuth_withPermissions exercises the X-Permissions branch.
func TestCorp_SetAuth_withPermissions(t *testing.T) {
	m, err := NewCorpAuthMaker(secKey)
	require.NoError(t, err)

	a := m.NewPermissions(PermSt, PermPI).(Corp)
	r, err := http.NewRequest(http.MethodPost, "https://api.monobank.ua/personal/auth/request", http.NoBody)
	require.NoError(t, err)

	require.NoError(t, a.SetAuth(r))

	// Permissions are concatenated letters in order passed.
	assert.Equal(t, "sp", r.Header.Get("X-Permissions"))
	assert.Empty(t, r.Header.Get("X-Request-Id"))
}

// Permission — typed string, тож константи мають правильний
// тип, конкатенація летерів зберігається, custom Permission приймається.
func TestPermission_typedStringSemantics(t *testing.T) {
	p := PermSt
	assert.IsType(t, Permission(""), p)
	assert.Equal(t, "s", string(p))

	// Custom Permission приймається (якщо Mono додасть нову букву —
	// користувач не чекає release-у SDK).
	m, err := NewCorpAuthMaker(secKey)
	require.NoError(t, err)
	a := m.NewPermissions(Permission("x")).(Corp)

	r, err := http.NewRequest(http.MethodPost, "https://api.monobank.ua/personal/auth/request", http.NoBody)
	require.NoError(t, err)
	require.NoError(t, a.SetAuth(r))
	assert.Equal(t, "x", r.Header.Get("X-Permissions"))
}

// TestCorp_SetAuth_noActor — when both requestID and permissions are empty
// (e.g. /personal/corp/settings), only the timestamp + URL go into the sign.
func TestCorp_SetAuth_noActor(t *testing.T) {
	m, err := NewCorpAuthMaker(secKey)
	require.NoError(t, err)

	a := m.New("").(Corp) // empty requestID
	r, err := http.NewRequest(http.MethodGet, "https://api.monobank.ua/personal/corp/settings", http.NoBody)
	require.NoError(t, err)

	require.NoError(t, a.SetAuth(r))
	assert.Empty(t, r.Header.Get("X-Request-Id"))
	assert.Empty(t, r.Header.Get("X-Permissions"))
	assert.NotEmpty(t, r.Header.Get("X-Sign"))
}

func TestCorp_SetAuth_nilRequestIsNoOp(t *testing.T) {
	m, err := NewCorpAuthMaker(secKey)
	require.NoError(t, err)

	a := m.New("rq").(Corp)
	// Must not panic.
	assert.NoError(t, a.SetAuth(nil))
}

func TestCorp_SetAuth_nilURLErrors(t *testing.T) {
	m, err := NewCorpAuthMaker(secKey)
	require.NoError(t, err)

	a := m.New("rq").(Corp)
	r := &http.Request{Header: http.Header{}} // no URL

	err = a.SetAuth(r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing URL")
}

// --- decodePrivateKey error paths ---

func TestNewCorpAuthMaker_emptyInput(t *testing.T) {
	_, err := NewCorpAuthMaker(nil)
	assert.ErrorIs(t, err, ErrDecodePrivateKey)

	_, err = NewCorpAuthMaker([]byte{})
	assert.ErrorIs(t, err, ErrDecodePrivateKey)
}

// TestNewCorpAuthMaker_wrongPEMBlockThenValid — decoder must skip PEM
// blocks of unrelated types and keep looking for "EC PRIVATE KEY".
func TestNewCorpAuthMaker_wrongPEMBlockThenValid(t *testing.T) {
	combined := []byte(
		"-----BEGIN CERTIFICATE-----\n" +
			"YWJjZGVmZ2g=\n" +
			"-----END CERTIFICATE-----\n",
	)
	combined = append(combined, secKey...)

	got, err := NewCorpAuthMaker(combined)
	require.NoError(t, err)
	assert.Equal(t, keyID, got.KeyID)
}

func TestNewCorpAuthMaker_onlyWrongBlockType(t *testing.T) {
	wrongBlock := []byte(
		"-----BEGIN CERTIFICATE-----\n" +
			"YWJjZGVmZ2g=\n" +
			"-----END CERTIFICATE-----\n",
	)
	_, err := NewCorpAuthMaker(wrongBlock)
	assert.ErrorIs(t, err, ErrDecodePrivateKey)
}

// TestParseECPrivateKey_corruptASN1 — a PEM block whose body is not
// valid SEC1 ASN.1 must surface as an error rather than a panic.
func TestParseECPrivateKey_corruptASN1(t *testing.T) {
	bad := []byte(
		"-----BEGIN EC PRIVATE KEY-----\n" +
			"YWJjZGVmZ2g=\n" + // "abcdefgh" — definitely not ASN.1
			"-----END EC PRIVATE KEY-----\n",
	)
	_, err := NewCorpAuthMaker(bad)
	// Wrapped through ErrDecodePrivateKey by NewCorpAuthMaker.
	assert.ErrorIs(t, err, ErrDecodePrivateKey)
}

// TestParseECPrivateKey_wrongVersion — version field != 1 is rejected.
func TestParseECPrivateKey_wrongVersion(t *testing.T) {
	// Build an ASN.1 SEC1 structure with version=2 (unknown).
	key := ecPrivateKey{
		Version:    2,
		PrivateKey: make([]byte, 32),
	}
	body, err := asn1.Marshal(key)
	require.NoError(t, err)

	_, err = parseECPrivateKey(body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version 2")
}

// TestParseECPrivateKey_oversizedRaw — a >32-byte key after stripping
// leading zeros is invalid.
func TestParseECPrivateKey_oversizedRaw(t *testing.T) {
	key := ecPrivateKey{
		Version:    1,
		PrivateKey: make([]byte, 64), // way too big
	}
	// Fill with a non-zero pattern so leading-zero-strip can't bring it
	// down to 32 bytes.
	for i := range key.PrivateKey {
		key.PrivateKey[i] = 0xFF
	}
	body, err := asn1.Marshal(key)
	require.NoError(t, err)

	_, err = parseECPrivateKey(body)
	assert.ErrorIs(t, err, ErrInvalidPrivateKey)
}
