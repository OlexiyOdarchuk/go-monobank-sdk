package acquiring

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
)

// Помилки верифікації вебхука.
var (
	// ErrBadSignature — підпис не збігається з body.
	ErrBadSignature = errors.New("acquiring: webhook signature is invalid")
	// ErrBadSignatureEncoding — X-Sign не валідний base64.
	ErrBadSignatureEncoding = errors.New("acquiring: X-Sign is not valid base64")
	// ErrMissingPubKey — спроба верифікувати з nil-ключем.
	ErrMissingPubKey = errors.New("acquiring: missing public key")
	// ErrInvalidPubKey — поле key не містить валідний ECDSA публічний ключ.
	ErrInvalidPubKey = errors.New("acquiring: invalid public key")
)

// ParsePubKey розбирає значення поля `key` з відповіді
// GET /api/merchant/pubkey ([Client.PubKey]). Mono шле base64-кодований
// PEM-блок "PUBLIC KEY" з x.509 SubjectPublicKeyInfo, в якому лежить
// ECDSA публічний ключ (NIST P-256). Функція знімає обидві обгортки
// й повертає готовий *ecdsa.PublicKey.
//
//	keyResp, _ := cli.PubKey(ctx)
//	pub, err := acquiring.ParsePubKey([]byte(keyResp.Key))
//	// ... use pub for VerifyWebhook
//
// Для зручності див. також [ServerKey.Public].
func ParsePubKey(keyB64 []byte) (*ecdsa.PublicKey, error) {
	if len(keyB64) == 0 {
		return nil, fmt.Errorf("%w: empty key", ErrInvalidPubKey)
	}
	pemBytes, err := base64.StdEncoding.DecodeString(string(keyB64))
	if err != nil {
		return nil, fmt.Errorf("%w: base64 decode: %v", ErrInvalidPubKey, err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("%w: no PEM block found", ErrInvalidPubKey)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPubKey, err)
	}
	ecPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%w: expected ECDSA, got %T", ErrInvalidPubKey, pub)
	}
	// Mono acquiring завжди підписує P-256 (NIST secp256r1). Інші
	// криві відхиляємо, щоб MITM не міг підсунути сторонній ключ
	// (наприклад P-384), на якому verify випадково пройде.
	if ecPub.Curve != elliptic.P256() {
		return nil, fmt.Errorf("%w: expected P-256, got %s", ErrInvalidPubKey, ecPub.Curve.Params().Name)
	}
	return ecPub, nil
}

// Public — зручний геттер: розбирає поле Key через [ParsePubKey] і повертає
// готовий *ecdsa.PublicKey. Викликай після [Client.PubKey].
func (k *ServerKey) Public() (*ecdsa.PublicKey, error) {
	if k == nil {
		return nil, ErrMissingPubKey
	}
	return ParsePubKey([]byte(k.Key))
}

// VerifyWebhook повертає nil, якщо xSign — валідний ECDSA-SHA256 підпис
// (ASN.1 DER) тіла body, зроблений приватним ключем Mono, що відповідає
// pub. Підпис у вебхуку — base64-кодований у заголовку X-Sign.
//
// Алгоритм — ECDSA з SHA-256 у форматі ASN.1, що відповідає тому, як
// еквайринговий шлюз Mono підписує webhook-и. Ключ отримуєш через
// [Client.PubKey] і кешуєш (rotates рідко, на відміну від /bank/sync).
//
//	pubResp, _ := cli.PubKey(ctx)
//	pub, _ := pubResp.Public()
//	if err := acquiring.VerifyWebhook(pub, body, r.Header.Get("X-Sign")); err != nil {
//	    http.Error(w, "bad sig", http.StatusUnauthorized); return
//	}
func VerifyWebhook(pub *ecdsa.PublicKey, body []byte, xSign string) error {
	if pub == nil {
		return ErrMissingPubKey
	}
	sig, err := base64.StdEncoding.DecodeString(xSign)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadSignatureEncoding, err)
	}
	hash := sha256.Sum256(body)
	if !ecdsa.VerifyASN1(pub, hash[:], sig) {
		return ErrBadSignature
	}
	return nil
}

// ParseWebhook декодує body еквайрингового вебхука у
// [InvoiceStatusResponse] — та сама форма, що повертає [Client.InvoiceStatus].
// Mono шле повний стан інвойсу на кожну зміну.
//
// Перед викликом завжди верифікуй підпис через [VerifyWebhook] — інакше
// будь-хто може прислати фейковий payload.
func ParseWebhook(body []byte) (*InvoiceStatusResponse, error) {
	var out InvoiceStatusResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("acquiring: decode webhook: %w", err)
	}
	return &out, nil
}
