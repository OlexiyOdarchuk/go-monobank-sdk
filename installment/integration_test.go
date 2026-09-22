//go:build integration

// Package installment integration tests hit a real "Покупка
// частинами" environment. They are excluded from the default
// `go test ./...` run; enable with:
//
//	CHAST_STORE_ID=xxx CHAST_SECRET=yyy \
//	    go test -tags=integration -run Integration ./installment/...
//
// The tests skip when credentials are absent, so the suite is a green
// no-op until someone supplies them. CI runs it weekly — see
// .github/workflows/integration.yaml.
//
// They default to [installment.BaseURLSandbox]. Point CHAST_BASE_URL
// at production only if you mean it: these calls create and cancel
// real carts.
package installment_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/installment"
)

// newUUID builds a v4 UUID for store_order_id, which the docs type as
// a uuid. Local rather than a dependency: this is the only place in
// the module that needs one.
func newUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	_, err := rand.Read(b[:])
	require.NoError(t, err)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func envOrSkip(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("%s is not set", key)
	}
	return v
}

func liveClient(t *testing.T) *installment.Client {
	t.Helper()
	base := os.Getenv("CHAST_BASE_URL")
	if base == "" {
		base = installment.BaseURLSandbox
	}
	cli, err := installment.New(
		envOrSkip(t, "CHAST_STORE_ID"),
		envOrSkip(t, "CHAST_SECRET"),
		installment.WithBaseURL(base),
	)
	require.NoError(t, err, "building the client must not fail")
	return cli
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// ValidateClient is the cheapest call that proves the HMAC signature
// and the store-id header are accepted. A phone nobody registered is
// a perfectly good input: we assert on the call succeeding, not on the
// answer.
func TestIntegration_ValidateClient(t *testing.T) {
	_, err := liveClient(t).ValidateClient(liveContext(t), "+380000000000")
	require.NoError(t, err, "live client validation must succeed")
}

// The QR cart is the pair this SDK added without ever calling it. The
// create side is the interesting half: the docs publish no schema for
// the 201 body and the element shape of `products` is an inference
// from /api/order/create, so a 400 here means the inference was wrong.
//
// MONO_QR_ID must be a QR code that belongs to CHAST_STORE_ID.
func TestIntegration_QRCart(t *testing.T) {
	qrID := envOrSkip(t, "MONO_QR_ID")
	callback := os.Getenv("MONO_CALLBACK")
	if callback == "" {
		callback = "https://example.com/chast/callback"
	}
	cli, ctx := liveClient(t), liveContext(t)

	err := cli.CreateQRCart(ctx, &installment.CreateQRCartRequest{
		QRID:         qrID,
		StoreOrderID: newUUID(t),
		Products: []installment.Product{{
			Name:  "SDK integration probe",
			Count: 1,
			Sum:   installment.NewMoney(1, 0),
		}},
		ResultCallback: callback,
	})
	require.NoError(t, err, "creating a QR cart must be accepted (201)")

	// Always clean up: a cart left on the code would greet the next
	// person who scans it.
	err = cli.CancelQRCart(ctx, qrID)
	assert.NoError(t, err, "cancelling the cart must be accepted (200)")
}

// Cancelling a QR code that carries no cart tells us how the bank
// answers the empty case — the docs do not say.
func TestIntegration_CancelQRCart_noCart(t *testing.T) {
	qrID := envOrSkip(t, "MONO_QR_ID")

	err := liveClient(t).CancelQRCart(liveContext(t), qrID)
	t.Logf("cancelling an empty QR cart returned: %v", err)
}
