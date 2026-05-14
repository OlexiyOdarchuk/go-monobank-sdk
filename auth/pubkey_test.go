package auth

import (
	"bytes"
	"encoding/asn1"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicKeyPEM_roundtrip(t *testing.T) {
	m, err := NewCorpAuthMaker(secKey)
	require.NoError(t, err)

	pemBytes, err := m.PublicKeyPEM()
	require.NoError(t, err)

	// Розпарсити назад і впевнитися, що це правильний SPKI-блок із
	// очікуваною кривою.
	block, _ := pem.Decode(pemBytes)
	require.NotNil(t, block, "expected a decodable PEM block")
	assert.Equal(t, "PUBLIC KEY", block.Type)

	var info pkixPublicKey
	rest, err := asn1.Unmarshal(block.Bytes, &info)
	require.NoError(t, err)
	assert.Empty(t, rest, "no trailing bytes after SPKI")

	// Алгоритм — id-ecPublicKey.
	assert.True(t, info.Algorithm.Algorithm.Equal(oidPublicKeyECDSA))

	// Параметр — OID кривої secp256k1.
	var curveOID asn1.ObjectIdentifier
	_, err = asn1.Unmarshal(info.Algorithm.Parameters.FullBytes, &curveOID)
	require.NoError(t, err)
	assert.True(t, curveOID.Equal(oidSecp256k1), "curve must be secp256k1")

	// Public point — точно 65 байтів, починається з 0x04.
	require.Len(t, info.PublicKey.Bytes, 1+2*pointCoordinateBytes)
	assert.Equal(t, byte(0x04), info.PublicKey.Bytes[0])

	// X-координата точки збігається з тією, що випливає з приватного ключа.
	wantX := m.privateKey.PublicKey.X.Bytes() //nolint:staticcheck
	gotX := info.PublicKey.Bytes[1 : 1+pointCoordinateBytes]
	// Прибрати лівопадинг для порівняння.
	for len(gotX) > 0 && gotX[0] == 0x00 {
		gotX = gotX[1:]
	}
	assert.True(t, bytes.Equal(wantX, gotX), "X coordinate must match private key")
}

// TestPublicKeyPEM_matchesExpectedShape перевіряє, що вихід має точно
// той формат, на який чекає Mono при /personal/auth/registration:
// один PEM-блок "PUBLIC KEY", 174 байти (стандартна довжина для
// secp256k1 SPKI).
func TestPublicKeyPEM_matchesExpectedShape(t *testing.T) {
	m, err := NewCorpAuthMaker(secKey)
	require.NoError(t, err)

	pemBytes, err := m.PublicKeyPEM()
	require.NoError(t, err)

	// Шапка / закриваючий рядок.
	assert.Contains(t, string(pemBytes), "-----BEGIN PUBLIC KEY-----")
	assert.Contains(t, string(pemBytes), "-----END PUBLIC KEY-----")

	// Має бути рівно один PEM-блок.
	block, rest := pem.Decode(pemBytes)
	require.NotNil(t, block)
	rest2, _ := pem.Decode(rest)
	assert.Nil(t, rest2, "expected exactly one PEM block")
}
