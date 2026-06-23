package acquiring_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring"
)

// genKey генерує тестовий ECDSA P-256 keypair і повертає приватний ключ
// разом зі значенням, яке Mono кладе у поле `key` відповіді
// /api/merchant/pubkey: base64(PEM(SPKI)).
func genKey(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	derPub, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	pemPub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: derPub})
	keyField := []byte(base64.StdEncoding.EncodeToString(pemPub))
	return priv, keyField
}

// signBody підписує body тим самим алгоритмом, який ми очікуємо від Mono.
func signBody(t *testing.T, priv *ecdsa.PrivateKey, body []byte) string {
	t.Helper()
	hash := sha256.Sum256(body)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, hash[:])
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(sig)
}

func TestParsePubKey_roundTrip(t *testing.T) {
	priv, keyField := genKey(t)

	pub, err := acquiring.ParsePubKey(keyField)
	require.NoError(t, err)
	assert.True(t, pub.Equal(&priv.PublicKey))
}

func TestServerKey_Public(t *testing.T) {
	priv, keyField := genKey(t)

	sk := &acquiring.ServerKey{Key: string(keyField)}
	pub, err := sk.Public()
	require.NoError(t, err)
	assert.True(t, pub.Equal(&priv.PublicKey))
}

func TestServerKey_Public_nil(t *testing.T) {
	var sk *acquiring.ServerKey
	_, err := sk.Public()
	assert.ErrorIs(t, err, acquiring.ErrMissingPubKey)
}

func TestParsePubKey_emptyInput(t *testing.T) {
	_, err := acquiring.ParsePubKey(nil)
	assert.ErrorIs(t, err, acquiring.ErrInvalidPubKey)
}

func TestParsePubKey_notBase64(t *testing.T) {
	_, err := acquiring.ParsePubKey([]byte("!!!not base64!!!"))
	assert.ErrorIs(t, err, acquiring.ErrInvalidPubKey)
}

func TestParsePubKey_notPEM(t *testing.T) {
	// Валідна base64 ("abc"), але всередині немає PEM-блоку.
	_, err := acquiring.ParsePubKey([]byte(base64.StdEncoding.EncodeToString([]byte("not a PEM"))))
	assert.ErrorIs(t, err, acquiring.ErrInvalidPubKey)
}

// ParsePubKey має відхиляти ECDSA-ключі на інших кривих
// крім P-256. Mono завжди підписує P-256; інша крива — або bug,
// або MITM, або hostile proxy.
func TestParsePubKey_rejectsNonP256(t *testing.T) {
	curves := map[string]elliptic.Curve{
		"P-384": elliptic.P384(),
		"P-521": elliptic.P521(),
	}
	for name, curve := range curves {
		t.Run(name, func(t *testing.T) {
			priv, err := ecdsa.GenerateKey(curve, rand.Reader)
			require.NoError(t, err)
			derPub, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
			require.NoError(t, err)
			pemPub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: derPub})
			keyField := []byte(base64.StdEncoding.EncodeToString(pemPub))

			_, err = acquiring.ParsePubKey(keyField)
			assert.ErrorIs(t, err, acquiring.ErrInvalidPubKey)
			assert.Contains(t, err.Error(), "P-256",
				"error must mention P-256 expectation, got %q", err.Error())
		})
	}
}

func TestParsePubKey_wrongAlgorithm(t *testing.T) {
	// PEM з RSA-ключем — не ECDSA, має падати.
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	derPub, err := x509.MarshalPKIXPublicKey(&rsaPriv.PublicKey)
	require.NoError(t, err)
	pemPub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: derPub})
	keyField := []byte(base64.StdEncoding.EncodeToString(pemPub))

	_, err = acquiring.ParsePubKey(keyField)
	assert.ErrorIs(t, err, acquiring.ErrInvalidPubKey)
}

func TestVerifyWebhook_validSignature(t *testing.T) {
	priv, _ := genKey(t)
	body := []byte(`{"invoiceId":"i-1","status":"success","amount":1000,"ccy":980}`)

	sig := signBody(t, priv, body)
	require.NoError(t, acquiring.VerifyWebhook(&priv.PublicKey, body, sig))
}

func TestVerifyWebhook_tamperedBody(t *testing.T) {
	priv, _ := genKey(t)
	body := []byte(`{"invoiceId":"i-1","status":"success","amount":1000,"ccy":980}`)
	sig := signBody(t, priv, body)

	tampered := []byte(`{"invoiceId":"i-1","status":"success","amount":9999,"ccy":980}`)
	err := acquiring.VerifyWebhook(&priv.PublicKey, tampered, sig)
	assert.ErrorIs(t, err, acquiring.ErrBadSignature)
}

func TestVerifyWebhook_garbageSignature(t *testing.T) {
	priv, _ := genKey(t)
	body := []byte(`{}`)

	// Validна base64, але невалідний підпис.
	err := acquiring.VerifyWebhook(&priv.PublicKey, body, "AAAA")
	assert.ErrorIs(t, err, acquiring.ErrBadSignature)
}

func TestVerifyWebhook_badBase64(t *testing.T) {
	priv, _ := genKey(t)
	err := acquiring.VerifyWebhook(&priv.PublicKey, []byte(`{}`), "!!!not base64!!!")
	assert.ErrorIs(t, err, acquiring.ErrBadSignatureEncoding)
}

func TestVerifyWebhook_nilKey(t *testing.T) {
	err := acquiring.VerifyWebhook(nil, []byte(`{}`), "AAAA")
	assert.ErrorIs(t, err, acquiring.ErrMissingPubKey)
}

func TestParseWebhook(t *testing.T) {
	body := []byte(`{"invoiceId":"i-42","status":"success","amount":4200,"ccy":980,"finalAmount":4200,"createdDate":"2026-01-15T10:00:00Z"}`)

	out, err := acquiring.ParseWebhook(body)
	require.NoError(t, err)
	assert.Equal(t, "i-42", out.InvoiceID)
	assert.Equal(t, acquiring.InvoiceSuccess, out.Status)
	assert.Equal(t, int64(4200), out.Amount.Minor)
}

func TestParseWebhook_malformed(t *testing.T) {
	_, err := acquiring.ParseWebhook([]byte(`{not json}`))
	require.Error(t, err)
}

// VerifyWebhookFresh: signature must be checked FIRST (so this
// helper can never be used as a partial-trust oracle), then
// modifiedDate freshness is enforced.
func TestVerifyWebhookFresh(t *testing.T) {
	priv, _ := genKey(t)
	now := time.Now().UTC()

	makeBody := func(modified time.Time) []byte {
		return []byte(`{"invoiceId":"i-1","status":"success","amount":1000,"ccy":980,"modifiedDate":"` +
			modified.Format(time.RFC3339) + `"}`)
	}

	t.Run("fresh body passes", func(t *testing.T) {
		body := makeBody(now.Add(-30 * time.Second))
		sig := signBody(t, priv, body)
		require.NoError(t, acquiring.VerifyWebhookFresh(&priv.PublicKey, body, sig, 5*time.Minute))
	})

	t.Run("stale body rejected", func(t *testing.T) {
		body := makeBody(now.Add(-1 * time.Hour))
		sig := signBody(t, priv, body)
		err := acquiring.VerifyWebhookFresh(&priv.PublicKey, body, sig, 5*time.Minute)
		assert.ErrorIs(t, err, acquiring.ErrWebhookStale)
	})

	t.Run("missing modifiedDate rejected", func(t *testing.T) {
		body := []byte(`{"invoiceId":"i-1","status":"success","amount":1000,"ccy":980}`)
		sig := signBody(t, priv, body)
		err := acquiring.VerifyWebhookFresh(&priv.PublicKey, body, sig, 5*time.Minute)
		assert.ErrorIs(t, err, acquiring.ErrWebhookNoTimestamp)
	})

	t.Run("tampered body never reaches the timestamp check", func(t *testing.T) {
		body := makeBody(now.Add(-30 * time.Second))
		sig := signBody(t, priv, body)
		tampered := append(body[:len(body)-1], `,"x":1}`...)
		err := acquiring.VerifyWebhookFresh(&priv.PublicKey, tampered, sig, 5*time.Minute)
		assert.ErrorIs(t, err, acquiring.ErrBadSignature,
			"signature failure must surface even when the body parses cleanly")
	})

	t.Run("RFC3339 with millis is accepted", func(t *testing.T) {
		body := []byte(`{"invoiceId":"i-1","modifiedDate":"` +
			now.Add(-10*time.Second).Format("2006-01-02T15:04:05.000Z") + `"}`)
		sig := signBody(t, priv, body)
		require.NoError(t, acquiring.VerifyWebhookFresh(&priv.PublicKey, body, sig, time.Minute))
	})

	t.Run("maxAge=0 disables freshness check", func(t *testing.T) {
		body := makeBody(now.Add(-30 * 24 * time.Hour))
		sig := signBody(t, priv, body)
		require.NoError(t, acquiring.VerifyWebhookFresh(&priv.PublicKey, body, sig, 0),
			"maxAge=0 must skip the staleness check (signature-only mode)")
	})
}
