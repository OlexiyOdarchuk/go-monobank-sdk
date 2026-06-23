package monobanktest

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"testing"
)

// AcquiringWebhookSigner is a self-contained ECDSA P-256 key pair
// that signs payloads exactly the way Mono's acquiring gateway does
// (ECDSA-SHA256, ASN.1 DER signature, base64 in the X-Sign header) —
// so an integration test can exercise the full verify → parse →
// dedup → handle path without a real bank.
//
// It pairs with the [Server.WithAcquiringPubKey] builder, which
// serves this signer's public key on GET /api/merchant/pubkey in the
// exact base64(PEM(SPKI)) shape acquiring.ParsePubKey expects.
//
//	signer := monobanktest.NewAcquiringWebhookSigner(t)
//	srv := monobanktest.NewServer(t).WithAcquiringPubKey(signer)
//	body := signer.MarshalBody(t, &acquiring.InvoiceStatusResponse{
//	    InvoiceID: "p2_x", Status: acquiring.InvoiceSuccess,
//	})
//	req := signer.Request(t, "POST", "/webhook", body)
//	handler.ServeHTTP(rec, req) // X-Sign verifies against the served key
type AcquiringWebhookSigner struct {
	priv     *ecdsa.PrivateKey
	keyField string // base64(PEM(SPKI)) — the /api/merchant/pubkey `key`
}

// NewAcquiringWebhookSigner generates a fresh P-256 key pair for the
// test. It fails the test (t.Fatalf) on any crypto error.
func NewAcquiringWebhookSigner(t testing.TB) *AcquiringWebhookSigner {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("monobanktest: generate ECDSA key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("monobanktest: marshal public key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	return &AcquiringWebhookSigner{
		priv:     priv,
		keyField: base64.StdEncoding.EncodeToString(pemBytes),
	}
}

// PubKeyField returns the value Mono puts in the `key` field of
// GET /api/merchant/pubkey: base64(PEM(SPKI)) of the public key.
// Feed it to acquiring.ServerKey{Key: ...}.Public() or serve it via
// [Server.WithAcquiringPubKey].
func (s *AcquiringWebhookSigner) PubKeyField() string { return s.keyField }

// Sign returns the X-Sign header value for body: a base64-encoded
// ECDSA-SHA256 ASN.1 signature, matching acquiring.VerifyWebhook.
func (s *AcquiringWebhookSigner) Sign(t testing.TB, body []byte) string {
	t.Helper()
	hash := sha256.Sum256(body)
	sig, err := ecdsa.SignASN1(rand.Reader, s.priv, hash[:])
	if err != nil {
		t.Fatalf("monobanktest: sign webhook body: %v", err)
	}
	return base64.StdEncoding.EncodeToString(sig)
}

// MarshalBody JSON-encodes v into the canonical webhook body bytes.
// IMPORTANT: sign and send THESE bytes — re-marshaling elsewhere can
// reorder fields and invalidate the signature, since the signature is
// over the exact bytes.
func (s *AcquiringWebhookSigner) MarshalBody(t testing.TB, v any) []byte {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("monobanktest: marshal webhook body: %v", err)
	}
	return body
}

// Request builds a POST *http.Request to target carrying body and a
// matching X-Sign header — ready to hand to a handler's ServeHTTP.
func (s *AcquiringWebhookSigner) Request(t testing.TB, method, target string, body []byte) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("monobanktest: build request: %v", err)
	}
	req.Header.Set("X-Sign", s.Sign(t, body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// WithAcquiringPubKey binds GET /api/merchant/pubkey to the signer's
// public key, so an acquiring client (or webhook handler built from
// one) fetching the key over this server gets a key that verifies the
// signer's signatures. Returns the *Server for chaining.
func (s *Server) WithAcquiringPubKey(signer *AcquiringWebhookSigner) *Server {
	s.Handle(http.MethodGet, "/api/merchant/pubkey",
		JSON(map[string]string{"key": signer.PubKeyField()}))
	return s
}
