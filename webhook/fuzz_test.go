package webhook

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// fuzzKey is a deterministic ECDSA key on secp256k1 used by Verify fuzz —
// each fuzz worker gets the same key, so reproducibility is preserved.
var fuzzKey = secp256k1.PrivKeyFromBytes([]byte{
	0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
	0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
	0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
}).ToECDSA()

func FuzzParse(f *testing.F) {
	f.Add([]byte(`{"type":"StatementItem","data":{"account":"acc"}}`))
	f.Add([]byte(`{"type":"StatementItem","data":{}}`))
	f.Add([]byte(``))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"type":42}`))
	f.Add([]byte(`{"type":"StatementItem","data":null}`))

	f.Fuzz(func(t *testing.T, body []byte) {
		// Property: Parse must never panic on arbitrary input.
		// Errors are fine; crashes are not.
		_, _ = Parse(body)
	})
}

func FuzzVerify(f *testing.F) {
	// Seed with a known-valid signature so the fuzzer occasionally hits
	// the accept path before mutating the bytes around it.
	body := []byte(`{"type":"StatementItem","data":{}}`)
	digest := sha256.Sum256(body)
	r, s, err := ecdsa.Sign(rand.Reader, fuzzKey, digest[:])
	if err != nil {
		f.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	validB64 := base64.StdEncoding.EncodeToString(sig)

	f.Add(body, validB64)
	f.Add(body, "")
	f.Add(body, "not-base64!!!")
	f.Add([]byte(``), validB64)
	f.Add([]byte(`{}`), "MEUCIQDxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxAIgYxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx=")

	f.Fuzz(func(t *testing.T, body []byte, xSign string) {
		// Property: Verify must never panic. It returns nil only if the
		// signature is valid for body under fuzzKey; otherwise some
		// non-nil error. Random fuzz mutations virtually never produce a
		// valid ECDSA signature, so we don't need to check the accept
		// branch — just that errors are returned cleanly.
		err := Verify(&fuzzKey.PublicKey, body, xSign)
		if err == nil {
			// Re-verification must still pass — Verify is pure.
			if err2 := Verify(&fuzzKey.PublicKey, body, xSign); err2 != nil {
				t.Fatalf("Verify is non-deterministic: first=nil, second=%v", err2)
			}
		}
	})
}
