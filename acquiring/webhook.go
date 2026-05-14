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

// Webhook verification errors.
var (
	// ErrBadSignature indicates that the signature does not match the
	// body.
	ErrBadSignature = errors.New("acquiring: webhook signature is invalid")
	// ErrBadSignatureEncoding indicates that X-Sign is not valid
	// base64.
	ErrBadSignatureEncoding = errors.New("acquiring: X-Sign is not valid base64")
	// ErrMissingPubKey indicates an attempt to verify with a nil key.
	ErrMissingPubKey = errors.New("acquiring: missing public key")
	// ErrInvalidPubKey indicates that the key field does not contain
	// a valid ECDSA public key.
	ErrInvalidPubKey = errors.New("acquiring: invalid public key")
)

// ParsePubKey parses the `key` field of the response of
// GET /api/merchant/pubkey ([Client.PubKey]). Mono sends a
// base64-encoded "PUBLIC KEY" PEM block holding an x.509
// SubjectPublicKeyInfo with an ECDSA public key (NIST P-256). The
// function strips both wrappers and returns a ready
// *ecdsa.PublicKey.
//
//	keyResp, _ := cli.PubKey(ctx)
//	pub, err := acquiring.ParsePubKey([]byte(keyResp.Key))
//	// ... use pub for VerifyWebhook
//
// See also [ServerKey.Public] for convenience.
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
	// Mono acquiring always signs on P-256 (NIST secp256r1). Other
	// curves are rejected so a MITM cannot substitute a foreign key
	// (for example P-384) on which verify might accidentally pass.
	if ecPub.Curve != elliptic.P256() {
		return nil, fmt.Errorf("%w: expected P-256, got %s", ErrInvalidPubKey, ecPub.Curve.Params().Name)
	}
	return ecPub, nil
}

// Public is a convenience getter: it parses the Key field via
// [ParsePubKey] and returns a ready *ecdsa.PublicKey. Call after
// [Client.PubKey].
func (k *ServerKey) Public() (*ecdsa.PublicKey, error) {
	if k == nil {
		return nil, ErrMissingPubKey
	}
	return ParsePubKey([]byte(k.Key))
}

// VerifyWebhook returns nil when xSign is a valid ECDSA-SHA256
// signature (ASN.1 DER) over body produced by Mono's private key that
// corresponds to pub. The signature in the webhook is base64-encoded
// in the X-Sign header.
//
// The algorithm is ECDSA with SHA-256 in ASN.1 format — the way the
// Mono acquiring gateway signs webhooks. Fetch the key via
// [Client.PubKey] and cache it (it rotates rarely, unlike /bank/sync).
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

// ParseWebhook decodes the body of an acquiring webhook into
// [InvoiceStatusResponse] — the same shape that [Client.InvoiceStatus]
// returns. Mono sends the full invoice state on every change.
//
// Always verify the signature with [VerifyWebhook] before parsing —
// otherwise anyone can send a fake payload.
func ParseWebhook(body []byte) (*InvoiceStatusResponse, error) {
	var out InvoiceStatusResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("acquiring: decode webhook: %w", err)
	}
	return &out, nil
}
