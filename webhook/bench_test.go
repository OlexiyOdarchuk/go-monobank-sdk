package webhook

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func BenchmarkVerify(b *testing.B) {
	seed := [32]byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
	}
	priv := secp256k1.PrivKeyFromBytes(seed[:]).ToECDSA()

	body := []byte(`{"type":"StatementItem","data":{"account":"acc","statementItem":{"id":"x","time":1700000000,"description":"","mcc":5411,"originalMcc":5411,"hold":false,"amount":-1000,"operationAmount":-1000,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":50000}}}`)
	digest := sha256.Sum256(body)
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		b.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	signB64 := base64.StdEncoding.EncodeToString(sig)

	b.ResetTimer()
	for range b.N {
		if err := Verify(&priv.PublicKey, body, signB64); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParse(b *testing.B) {
	body := []byte(`{"type":"StatementItem","data":{"account":"acc","statementItem":{"id":"x","time":1700000000,"description":"shop","mcc":5411,"originalMcc":5411,"hold":false,"amount":-1000,"operationAmount":-1000,"currencyCode":980,"commissionRate":0,"cashbackAmount":50,"balance":50000}}}`)
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for range b.N {
		if _, err := Parse(body); err != nil {
			b.Fatal(err)
		}
	}
}
