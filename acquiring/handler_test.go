package acquiring_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/monobanktest"
)

// newSignedHandler будує webhook-handler, прив'язаний до тестового
// сервера, що віддає публічний ключ підписувача.
func newSignedHandler(t *testing.T, opts acquiring.WebhookHandlerOptions) (*acquiring.WebhookHandler, *monobanktest.AcquiringWebhookSigner) {
	t.Helper()
	signer := monobanktest.NewAcquiringWebhookSigner(t)
	srv := monobanktest.NewServer(t).WithAcquiringPubKey(signer)
	cli := acquiring.New("tok", srv.Option())
	opts.Keys = cli
	h, err := acquiring.NewWebhookHandler(context.Background(), opts)
	require.NoError(t, err)
	return h, signer
}

func TestWebhookHandler_happyPath(t *testing.T) {
	var got *acquiring.InvoiceStatusResponse
	h, signer := newSignedHandler(t, acquiring.WebhookHandlerOptions{
		OnEvent: func(_ context.Context, inv *acquiring.InvoiceStatusResponse) error {
			got = inv
			return nil
		},
	})

	body := signer.MarshalBody(t, &acquiring.InvoiceStatusResponse{
		InvoiceID: "p2_x", Status: acquiring.InvoiceSuccess, ModifiedDate: "2099-01-01T00:00:00Z",
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, signer.Request(t, http.MethodPost, "/webhook", body))

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, got)
	assert.Equal(t, "p2_x", got.InvoiceID)
	assert.Equal(t, acquiring.InvoiceSuccess, got.Status)
}

func TestWebhookHandler_getPing(t *testing.T) {
	h, _ := newSignedHandler(t, acquiring.WebhookHandlerOptions{
		OnEvent: func(context.Context, *acquiring.InvoiceStatusResponse) error { return nil },
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/webhook", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestWebhookHandler_badSignature(t *testing.T) {
	called := false
	h, signer := newSignedHandler(t, acquiring.WebhookHandlerOptions{
		OnEvent: func(context.Context, *acquiring.InvoiceStatusResponse) error { called = true; return nil },
	})
	body := signer.MarshalBody(t, &acquiring.InvoiceStatusResponse{InvoiceID: "p2_x"})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Sign", "Zm9vYmFy") // не валідний підпис

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.False(t, called)
}

func TestWebhookHandler_dedup(t *testing.T) {
	var count int
	h, signer := newSignedHandler(t, acquiring.WebhookHandlerOptions{
		Dedup: acquiring.NewMemoryDeduper(16),
		OnEvent: func(context.Context, *acquiring.InvoiceStatusResponse) error {
			count++
			return nil
		},
	})

	body := signer.MarshalBody(t, &acquiring.InvoiceStatusResponse{
		InvoiceID: "p2_x", Status: acquiring.InvoiceSuccess, ModifiedDate: "2099-01-01T00:00:00Z",
	})
	// Дві однакові доставки (invoiceId + modifiedDate) -> OnEvent раз.
	for range 2 {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, signer.Request(t, http.MethodPost, "/webhook", body))
		assert.Equal(t, http.StatusOK, rec.Code)
	}
	assert.Equal(t, 1, count)

	// Інший modifiedDate = новий стан -> OnEvent ще раз.
	body2 := signer.MarshalBody(t, &acquiring.InvoiceStatusResponse{
		InvoiceID: "p2_x", Status: acquiring.InvoiceSuccess, ModifiedDate: "2099-01-01T00:05:00Z",
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, signer.Request(t, http.MethodPost, "/webhook", body2))
	assert.Equal(t, 2, count)
}

func TestWebhookHandler_onEventErrorNoDedup(t *testing.T) {
	var count int
	dedup := acquiring.NewMemoryDeduper(16)
	h, signer := newSignedHandler(t, acquiring.WebhookHandlerOptions{
		Dedup: dedup,
		OnEvent: func(context.Context, *acquiring.InvoiceStatusResponse) error {
			count++
			if count == 1 {
				return assert.AnError // перша спроба падає
			}
			return nil
		},
	})
	body := signer.MarshalBody(t, &acquiring.InvoiceStatusResponse{
		InvoiceID: "p2_x", Status: acquiring.InvoiceSuccess, ModifiedDate: "2099-01-01T00:00:00Z",
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, signer.Request(t, http.MethodPost, "/webhook", body))
	assert.Equal(t, http.StatusInternalServerError, rec.Code) // 5xx -> Mono ретраїть

	// Ретрай реально виконує OnEvent знову (ключ не "отруєно").
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, signer.Request(t, http.MethodPost, "/webhook", body))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 2, count)
}

func TestWebhookHandler_staleRejected(t *testing.T) {
	h, signer := newSignedHandler(t, acquiring.WebhookHandlerOptions{
		MaxAge:  15 * time.Minute,
		OnEvent: func(context.Context, *acquiring.InvoiceStatusResponse) error { return nil },
	})
	body := signer.MarshalBody(t, &acquiring.InvoiceStatusResponse{
		InvoiceID: "p2_x", Status: acquiring.InvoiceSuccess,
		ModifiedDate: "2000-01-01T00:00:00Z", // давно
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, signer.Request(t, http.MethodPost, "/webhook", body))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestWebhookHandler_validation(t *testing.T) {
	_, err := acquiring.NewWebhookHandler(context.Background(), acquiring.WebhookHandlerOptions{
		OnEvent: func(context.Context, *acquiring.InvoiceStatusResponse) error { return nil },
	})
	require.ErrorIs(t, err, acquiring.ErrNilKeyProvider)

	signer := monobanktest.NewAcquiringWebhookSigner(t)
	srv := monobanktest.NewServer(t).WithAcquiringPubKey(signer)
	cli := acquiring.New("tok", srv.Option())
	_, err = acquiring.NewWebhookHandler(context.Background(), acquiring.WebhookHandlerOptions{Keys: cli})
	require.ErrorIs(t, err, acquiring.ErrNilOnEvent)
}

func TestMemoryDeduper(t *testing.T) {
	d := acquiring.NewMemoryDeduper(2)
	d.Add("a")
	d.Add("b")
	assert.True(t, d.Has("a"))
	d.Add("c") // витісняє LRU; "a" щойно читали -> витісниться "b"
	assert.True(t, d.Has("a"))
	assert.False(t, d.Has("b"))
	assert.True(t, d.Has("c"))
	assert.False(t, d.Has(""))
}

func TestDedupKey(t *testing.T) {
	assert.Equal(t, "p2_x|2099-01-01T00:00:00Z", acquiring.DedupKey(&acquiring.InvoiceStatusResponse{
		InvoiceID: "p2_x", ModifiedDate: "2099-01-01T00:00:00Z",
	}))
	assert.Equal(t, "p2_x", acquiring.DedupKey(&acquiring.InvoiceStatusResponse{InvoiceID: "p2_x"}))
	assert.Equal(t, "", acquiring.DedupKey(nil))
}
