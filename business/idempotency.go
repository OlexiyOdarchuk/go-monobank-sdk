package business

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewIdempotencyKey produces a fresh UUID v4 suitable for the
// Idempotency-Key header expected by [Client.PreparePayment] and
// [Client.CreateSalaryRegistry]. Entropy comes from crypto/rand — no
// external dependencies.
//
// Semantics: the key identifies an operation "attempt". If the
// network fails and you retry the call with the same key, the bank
// returns the same response without duplicating the operation. A new
// logical payment requires a new key.
//
//	key, err := business.NewIdempotencyKey()
//	if err != nil { return err }
//	out, err := cli.PreparePayment(ctx, key, &business.PaymentRequest{...})
//
// Returns an error when crypto/rand fails — historically a panic;
// now the caller can decide whether to abort or retry on entropy
// starvation. The error is wrapped, use errors.Unwrap if you care
// about the underlying io.Reader failure.
func NewIdempotencyKey() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("business: crypto/rand failed: %w", err)
	}
	// Set the version (4) and variant (RFC 4122) bits per the UUID v4
	// spec (RFC 4122 §4.4).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	// 8-4-4-4-12 hex-digit format.
	var dst [36]byte
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])
	return string(dst[:]), nil
}
