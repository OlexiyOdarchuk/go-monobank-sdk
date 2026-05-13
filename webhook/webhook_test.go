package webhook

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test vector captured from a real mono personal-API webhook (a jar top-up).
const (
	testServerPubKeyB64 = "BNDZP+AGoRC+ER1plDSUCHOw2/aBNIocmD2gS/v34/b0iQ1HBo+oS3/f402e3OXA5uCxakSjuxGMP6X0XP9VIUk="
	testServerKeyID     = "2626ff34473bb66260b930af946fa9641a06bcd4"

	testWebhookBody = `{"type":"StatementItem","data":{"account":"GdIXP9tJybhRwW4yl457iw","statementItem":{"id":"XfHmJ0KH0p1jVAw58w","time":1778612175,"description":"Поповнення «Донатики🥰»","mcc":4829,"originalMcc":4829,"amount":-20000,"operationAmount":-20000,"currencyCode":980,"commissionRate":0,"cashbackAmount":0,"balance":62360,"hold":true,"receiptId":"ACAB-X504-7TP3-143T"}}}`
	testWebhookSign = "hqiXbiDWs4lCkg/i9cZaqWBHgvio2PhGNnzkBiU7MBJxnODkMf8RYKsaLke8gbm+1XSvZgmMHPYFw3XswwL7Qw=="
)

func testServerPubKey(t *testing.T) *ecdsa.PublicKey {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(testServerPubKeyB64)
	require.NoError(t, err)
	require.Equal(t, 65, len(raw))
	require.Equal(t, byte(0x04), raw[0])
	return &ecdsa.PublicKey{
		Curve: secp256k1.S256(),
		X:     new(big.Int).SetBytes(raw[1:33]),
		Y:     new(big.Int).SetBytes(raw[33:]),
	}
}

func TestVerify(t *testing.T) {
	pub := testServerPubKey(t)

	t.Run("valid signature", func(t *testing.T) {
		assert.NoError(t, Verify(pub, []byte(testWebhookBody), testWebhookSign))
	})

	t.Run("tampered body", func(t *testing.T) {
		tampered := []byte(testWebhookBody[:len(testWebhookBody)-2] + "9}")
		assert.ErrorIs(t, Verify(pub, tampered, testWebhookSign), ErrBadSignature)
	})

	t.Run("garbage signature", func(t *testing.T) {
		assert.ErrorIs(t, Verify(pub, []byte(testWebhookBody), "AAAA"), ErrBadSignature)
	})

	t.Run("non-base64 signature", func(t *testing.T) {
		assert.ErrorIs(t, Verify(pub, []byte(testWebhookBody), "not base64!!!"),
			ErrBadSignatureEncoding)
	})

	t.Run("nil pubkey", func(t *testing.T) {
		assert.ErrorIs(t, Verify(nil, []byte(testWebhookBody), testWebhookSign),
			ErrMissingPubKey)
	})
}

func TestParse(t *testing.T) {
	t.Run("known type", func(t *testing.T) {
		w, err := Parse([]byte(testWebhookBody))
		require.NoError(t, err)
		require.NotNil(t, w)
		assert.Equal(t, TypeStatementItem, w.Type)
		assert.Equal(t, "GdIXP9tJybhRwW4yl457iw", w.Data.AccountID)
		assert.Equal(t, "XfHmJ0KH0p1jVAw58w", w.Data.Transaction.ID)
		assert.Equal(t, int64(-20000), w.Data.Transaction.Amount.Minor)
		assert.Equal(t, "UAH", w.Data.Transaction.Amount.Code.String())
		assert.Equal(t, "ACAB-X504-7TP3-143T", w.Data.Transaction.ReceiptID)
	})

	t.Run("unknown type still parsed, error flagged", func(t *testing.T) {
		w, err := Parse([]byte(`{"type":"NewEventKind","data":{}}`))
		assert.ErrorIs(t, err, ErrUnknownType)
		require.NotNil(t, w)
		assert.Equal(t, "NewEventKind", w.Type)
	})

	t.Run("malformed JSON", func(t *testing.T) {
		_, err := Parse([]byte(`{`))
		require.Error(t, err)
		assert.False(t, errors.Is(err, ErrUnknownType))
	})
}

func TestMemoryDeduper_basic(t *testing.T) {
	d := NewMemoryDeduper(3)

	assert.False(t, d.Has("a"))
	d.Add("a")
	assert.True(t, d.Has("a"))

	d.Add("b")
	d.Add("c")
	assert.Equal(t, 3, d.Len())

	d.Add("e") // evicts "a"
	assert.False(t, d.Has("a"), "evicted id must be reported as new")
	assert.True(t, d.Has("b"))
	assert.True(t, d.Has("c"))
	assert.True(t, d.Has("e"))
	assert.Equal(t, 3, d.Len())
}

func TestMemoryDeduper_emptyIdNoop(t *testing.T) {
	d := NewMemoryDeduper(8)
	d.Add("")
	assert.False(t, d.Has(""), "empty id is ignored")
	assert.Equal(t, 0, d.Len())
}

func TestMemoryDeduper_AddIsIdempotent(t *testing.T) {
	d := NewMemoryDeduper(3)
	d.Add("a")
	d.Add("a")
	d.Add("a")
	assert.Equal(t, 1, d.Len())
}

func TestMemoryDeduper_LRUEvictsLeastRecentlyUsed(t *testing.T) {
	d := NewMemoryDeduper(3)
	d.Add("a")
	d.Add("b")
	d.Add("c")

	assert.True(t, d.Has("a")) // refresh "a"
	d.Add("d")                 // evicts "b" (LRU), not "a"
	assert.True(t, d.Has("a"))
	assert.False(t, d.Has("b"))
	assert.True(t, d.Has("c"))
	assert.True(t, d.Has("d"))
}

func TestMemoryDeduper_concurrent(t *testing.T) {
	d := NewMemoryDeduper(1024)

	const workers = 16
	const opsPerWorker = 200

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				id := strconv.Itoa((w * opsPerWorker) + i)
				_ = d.Has(id)
				d.Add(id)
			}
		}(w)
	}
	wg.Wait()

	assert.LessOrEqual(t, d.Len(), 1024)
}

func TestMemoryDeduper_zeroCapacityFallback(t *testing.T) {
	d := NewMemoryDeduper(0)
	d.Add("x")
	assert.True(t, d.Has("x"))
}

// fakeKeyProvider is a KeyProvider whose returned key and error are
// settable at runtime. It also counts calls so tests can assert refresh
// behaviour.
type fakeKeyProvider struct {
	key   *bank.ServerKey
	err   error
	calls atomic.Int32
}

func (f *fakeKeyProvider) ServerKey(_ context.Context) (*bank.ServerKey, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	return f.key, nil
}

func newTestHandler(t *testing.T, opts Options) (*Handler, *fakeKeyProvider) {
	t.Helper()
	pub := testServerPubKey(t)
	prov := &fakeKeyProvider{key: &bank.ServerKey{ID: testServerKeyID, PubKey: pub}}
	opts.Keys = prov
	h, err := NewHandler(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, int32(1), prov.calls.Load())
	return h, prov
}

func signedPOST(body, sign, keyID string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	r.Header.Set("X-Sign", sign)
	r.Header.Set("X-Key-Id", keyID)
	return r
}

func TestNewHandler_validation(t *testing.T) {
	_, err := NewHandler(context.Background(), Options{
		OnEvent: func(context.Context, *Response) error { return nil },
	})
	assert.ErrorIs(t, err, ErrNilKeyProvider)

	_, err = NewHandler(context.Background(), Options{
		Keys: &fakeKeyProvider{key: &bank.ServerKey{}},
	})
	assert.ErrorIs(t, err, ErrNilOnEvent)
}

func TestNewHandler_initialFetchFails(t *testing.T) {
	prov := &fakeKeyProvider{err: errors.New("boom")}
	_, err := NewHandler(context.Background(), Options{
		Keys:    prov,
		OnEvent: func(context.Context, *Response) error { return nil },
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "initial ServerKey fetch")
}

func TestHandler_GET_returns200(t *testing.T) {
	h, _ := newTestHandler(t, Options{
		OnEvent: func(context.Context, *Response) error {
			t.Fatal("OnEvent must not be called for GET")
			return nil
		},
	})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/webhook", http.NoBody))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_validPOST_callsOnEvent(t *testing.T) {
	var seen *Response
	h, _ := newTestHandler(t, Options{
		OnEvent: func(_ context.Context, e *Response) error {
			seen = e
			return nil
		},
	})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedPOST(testWebhookBody, testWebhookSign, testServerKeyID))

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, seen)
	assert.Equal(t, "XfHmJ0KH0p1jVAw58w", seen.Data.Transaction.ID)
}

func TestHandler_badSignature_rejects401(t *testing.T) {
	called := false
	h, _ := newTestHandler(t, Options{
		OnEvent: func(context.Context, *Response) error {
			called = true
			return nil
		},
	})

	tampered := strings.Replace(testWebhookBody, "Поповнення", "Hijacked", 1)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedPOST(tampered, testWebhookSign, testServerKeyID))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.False(t, called, "OnEvent must not run on bad signature")
}

func TestHandler_callbackError_500(t *testing.T) {
	h, _ := newTestHandler(t, Options{
		OnEvent: func(context.Context, *Response) error {
			return errors.New("downstream unavailable")
		},
	})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedPOST(testWebhookBody, testWebhookSign, testServerKeyID))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_unknownType_ackedButCallbackSkipped(t *testing.T) {
	const unknown = `{"type":"FutureEvent","data":{}}`

	seed := [32]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}
	priv := secp256k1.PrivKeyFromBytes(seed[:]).ToECDSA()
	digest := sha256.Sum256([]byte(unknown))
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	require.NoError(t, err)
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	signB64 := base64.StdEncoding.EncodeToString(sig)

	var onEventCalled, onUnknownCalled bool
	prov := &fakeKeyProvider{key: &bank.ServerKey{ID: "test", PubKey: &priv.PublicKey}}
	h, err := NewHandler(context.Background(), Options{
		Keys: prov,
		OnEvent: func(context.Context, *Response) error {
			onEventCalled = true
			return nil
		},
		OnUnknownType: func(_ context.Context, raw []byte) {
			onUnknownCalled = true
			assert.Equal(t, unknown, string(raw))
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedPOST(unknown, signB64, "test"))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, onEventCalled, "OnEvent must not run for unknown type")
	assert.True(t, onUnknownCalled, "OnUnknownType must run")
}

func TestHandler_keyRotation(t *testing.T) {
	pub := testServerPubKey(t)
	prov := &fakeKeyProvider{key: &bank.ServerKey{ID: "stale", PubKey: pub}}

	h, err := NewHandler(context.Background(), Options{
		Keys:    prov,
		OnEvent: func(context.Context, *Response) error { return nil },
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), prov.calls.Load())
	assert.Equal(t, "stale", h.KeyID())

	prov.key = &bank.ServerKey{ID: testServerKeyID, PubKey: pub}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedPOST(testWebhookBody, testWebhookSign, testServerKeyID))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int32(2), prov.calls.Load(), "handler must re-fetch on X-Key-Id mismatch")
	assert.Equal(t, testServerKeyID, h.KeyID())
}

// Regression: a transient OnEvent failure (HTTP 500) must NOT mark the id
// as seen, so the next retry from mono actually runs OnEvent again.
func TestHandler_failureLetsMonoRetryWithDedup(t *testing.T) {
	var attempts atomic.Int32
	prov := &fakeKeyProvider{key: &bank.ServerKey{ID: testServerKeyID, PubKey: testServerPubKey(t)}}
	h, err := NewHandler(context.Background(), Options{
		Keys:  prov,
		Dedup: NewMemoryDeduper(64),
		OnEvent: func(context.Context, *Response) error {
			n := attempts.Add(1)
			if n == 1 {
				return errors.New("transient downstream failure")
			}
			return nil
		},
	})
	require.NoError(t, err)

	w1 := httptest.NewRecorder()
	h.ServeHTTP(w1, signedPOST(testWebhookBody, testWebhookSign, testServerKeyID))
	assert.Equal(t, http.StatusInternalServerError, w1.Code)

	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, signedPOST(testWebhookBody, testWebhookSign, testServerKeyID))
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, int32(2), attempts.Load(), "retry must actually invoke OnEvent")

	w3 := httptest.NewRecorder()
	h.ServeHTTP(w3, signedPOST(testWebhookBody, testWebhookSign, testServerKeyID))
	assert.Equal(t, http.StatusOK, w3.Code)
	assert.Equal(t, int32(2), attempts.Load(), "duplicates must not run OnEvent")
}

func TestHandler_endToEnd_overHTTP(t *testing.T) {
	var got *Response
	h, _ := newTestHandler(t, Options{
		OnEvent: func(_ context.Context, e *Response) error {
			got = e
			return nil
		},
	})
	server := httptest.NewServer(h)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/webhook", strings.NewReader(testWebhookBody))
	req.Header.Set("X-Sign", testWebhookSign)
	req.Header.Set("X-Key-Id", testServerKeyID)

	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, got)
	assert.Equal(t, "ACAB-X504-7TP3-143T", got.Data.Transaction.ReceiptID)
}
