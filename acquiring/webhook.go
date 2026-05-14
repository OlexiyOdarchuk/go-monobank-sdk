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
	"time"
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
	// ErrWebhookStale is returned by [VerifyWebhookFresh] when the
	// modifiedDate of the payload is older than maxAge. This is a
	// freshness-check on top of the signature check: the signature
	// being valid only proves the body came from Mono, not when. A
	// reasonably tight maxAge (a few minutes) makes replaying an old
	// captured webhook far less useful to an attacker.
	ErrWebhookStale = errors.New("acquiring: webhook modifiedDate is older than maxAge")
	// ErrWebhookNoTimestamp is returned by [VerifyWebhookFresh] when
	// modifiedDate is absent or cannot be parsed.
	ErrWebhookNoTimestamp = errors.New("acquiring: webhook is missing a parseable modifiedDate")
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
	// Avoid the string(keyB64) copy by decoding directly into a
	// pre-sized buffer.
	pemBytes := make([]byte, base64.StdEncoding.DecodedLen(len(keyB64)))
	n, err := base64.StdEncoding.Decode(pemBytes, keyB64)
	if err != nil {
		return nil, fmt.Errorf("%w: base64 decode: %v", ErrInvalidPubKey, err)
	}
	pemBytes = pemBytes[:n]
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
//
// IMPORTANT: a valid signature only proves the body came from Mono
// — not when. Mono itself re-sends webhooks (60s and 600s after the
// initial delivery for transient 5xx). To guard against replay, the
// caller MUST persistently deduplicate by (invoiceId, modifiedDate)
// across process restarts. The signature does not encode a
// timestamp; nothing in this signature check prevents an attacker
// from re-submitting an old captured (body, X-Sign) pair. See
// [VerifyWebhookFresh] for a freshness-bounded variant.
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

// parseModifiedDate tolerates the small set of layouts Mono has been
// seen to use for modifiedDate over the years. Accepts RFC3339 with
// optional milli/nano fractions and an optional trailing Z.
func parseModifiedDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, ErrWebhookNoTimestamp
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: %q", ErrWebhookNoTimestamp, s)
}

// VerifyWebhookFresh is VerifyWebhook plus a freshness check on the
// payload's modifiedDate. It rejects bodies whose timestamp is more
// than maxAge in the past — a cheap mitigation against replay of an
// old captured (body, X-Sign) pair.
//
// Tradeoffs:
//   - The signature check still runs first; an attacker cannot use
//     this entry point as a partial-trust oracle.
//   - maxAge needs slack for legitimate retries: Mono itself re-sends
//     after 60s and 600s on a 5xx. A maxAge of ~15 minutes is a sane
//     starting point for production.
//   - This is NOT a substitute for persistent dedup on (invoiceId,
//     modifiedDate) — within maxAge a duplicate is still a duplicate.
//
// Returns [ErrBadSignature] / [ErrBadSignatureEncoding] /
// [ErrMissingPubKey] for crypto failures, [ErrWebhookNoTimestamp]
// when modifiedDate is missing or unparseable, and [ErrWebhookStale]
// when the payload is too old.
func VerifyWebhookFresh(pub *ecdsa.PublicKey, body []byte, xSign string, maxAge time.Duration) error {
	if err := VerifyWebhook(pub, body, xSign); err != nil {
		return err
	}
	// Parse just modifiedDate without forcing a full ParseWebhook so
	// callers cannot accidentally rely on partially-trusted data.
	var probe struct {
		ModifiedDate string `json:"modifiedDate"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return fmt.Errorf("%w: %v", ErrWebhookNoTimestamp, err)
	}
	t, err := parseModifiedDate(probe.ModifiedDate)
	if err != nil {
		return err
	}
	if maxAge > 0 && time.Since(t) > maxAge {
		return fmt.Errorf("%w: modifiedDate=%s, age=%s, maxAge=%s",
			ErrWebhookStale, t.UTC().Format(time.RFC3339), time.Since(t), maxAge)
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
