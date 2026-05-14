// Package webhook provides server-side helpers for monobank webhooks:
// signature verification, payload parsing, a mountable http.Handler
// with automatic key rotation, plus an in-memory deduper that
// absorbs Mono's 60-second and 600-second redeliveries.
//
// Mono signs each webhook body with ECDSA on the secp256k1 curve and
// sends the key identifier in X-Key-Id and the signature itself in
// X-Sign. The key is rotated occasionally; the bank publishes the
// current one at /bank/sync ([bank.Client.ServerKey]).
//
// For "batteries included" use [NewHandler] — it keeps the key in a
// cache, refetches it when X-Key-Id stops matching, verifies the
// signature, parses the body, filters duplicates via Deduper, and
// invokes your callback. If you integrate it into your own HTTP
// framework with your own routing, use the lower-level [Verify] and
// [Parse] directly.
package webhook

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
)

// Signature verification errors.
var (
	// ErrBadSignature indicates the signature does not match the body
	// for the given key. Check via errors.Is.
	ErrBadSignature = errors.New("webhook signature is invalid")
	// ErrBadSignatureEncoding indicates X-Sign is not valid base64.
	ErrBadSignatureEncoding = errors.New("X-Sign is not valid base64")
	// ErrMissingPubKey indicates an attempt to verify with a nil key.
	ErrMissingPubKey = errors.New("missing public key")
)

// Length of the "raw" (r||s) signature for secp256k1: two 32-byte
// coordinates.
const rawSigLen = 64

// Verify returns nil only when xSign is a valid ECDSA signature over
// body produced by the private key that corresponds to pub. Both
// forms Mono has historically used are supported: raw r||s (64
// bytes, base64) and ASN.1 DER — both decode transparently.
//
// You typically do not call Verify directly because [Handler] does
// it for you. Exceptions: integration into an external HTTP
// framework or writing test helpers.
func Verify(pub *ecdsa.PublicKey, body []byte, xSign string) error {
	if pub == nil {
		return ErrMissingPubKey
	}
	sig, err := base64.StdEncoding.DecodeString(xSign)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadSignatureEncoding, err)
	}
	digest := sha256.Sum256(body)

	if len(sig) == rawSigLen {
		r := new(big.Int).SetBytes(sig[:rawSigLen/2])
		s := new(big.Int).SetBytes(sig[rawSigLen/2:])
		if ecdsa.Verify(pub, digest[:], r, s) {
			return nil
		}
		// raw r||s did not verify — fall through to ASN.1 DER, since some
		// encoders produce DER that also happens to be 64 bytes long.
	}

	var asn1Sig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(sig, &asn1Sig); err != nil {
		return ErrBadSignature
	}
	if asn1Sig.R == nil || asn1Sig.S == nil {
		return ErrBadSignature
	}
	if !ecdsa.Verify(pub, digest[:], asn1Sig.R, asn1Sig.S) {
		return ErrBadSignature
	}
	return nil
}
