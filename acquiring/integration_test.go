//go:build integration

// Package acquiring integration tests hit the real api.monobank.ua.
// They are excluded from the default `go test ./...` run; enable with:
//
//	MONO_ACQUIRING_TOKEN=xxx go test -tags=integration -run Integration ./acquiring/...
//
// Every test skips when its inputs are absent, so the suite is a green
// no-op until someone supplies credentials. CI runs it weekly — see
// .github/workflows/integration.yaml.
//
// Read-only by default. The one call that moves money
// ([Client.POSTransactionCancel]) additionally requires an explicit
// opt-in and an RRN the operator chose; it will never pick one itself.
package acquiring_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/acquiring"
)

// refundOptIn is the value MONO_ALLOW_REFUND must carry before a live
// refund is attempted. A long, deliberate string rather than "1": this
// switch spends real money and should never be flipped by accident.
const refundOptIn = "yes-refund-real-money"

func envOrSkip(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("%s is not set", key)
	}
	return v
}

func liveClient(t *testing.T) *acquiring.Client {
	t.Helper()
	cli := acquiring.New(envOrSkip(t, "MONO_ACQUIRING_TOKEN"))
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// The cheapest proof that the token and the transport work at all —
// run this first when the rest of the file starts failing.
func TestIntegration_MerchantDetails(t *testing.T) {
	out, err := liveClient(t).MerchantDetails(liveContext(t))
	require.NoError(t, err, "live /api/merchant/details call must succeed")
	assert.NotEmpty(t, out.MerchantID, "merchant id must come back")
}

// Terminals is read-only and costs nothing; an empty list is a valid
// answer for a merchant without tap-to-pay.
func TestIntegration_Terminals(t *testing.T) {
	list, err := liveClient(t).Terminals(liveContext(t))
	require.NoError(t, err, "live /api/merchant/t2p/terminal/list call must succeed")
	for _, term := range list {
		assert.NotEmpty(t, term.Terminal, "every terminal must carry its id")
	}
}

// The SDK's T2PPaymentStatusResponse is modelled on the documented
// webhook payload, because monobank publishes no machine-readable
// schema for this endpoint. This test is what closes that gap: set
// MONO_T2P_EXTERNAL_PAYMENT_ID to a payment from the last 90 days and
// it reports which documented fields the bank actually sends.
func TestIntegration_T2PPaymentStatus(t *testing.T) {
	id := envOrSkip(t, "MONO_T2P_EXTERNAL_PAYMENT_ID")

	out, err := liveClient(t).T2PPaymentStatus(liveContext(t), id)
	require.NoError(t, err, "live T2P status call must succeed")
	assert.Equal(t, id, out.ExternalPaymentID, "the id must echo back")
	assert.NotEmpty(t, out.Status, "status is the point of the call")

	// Not assertions: the fields below are carried over from the
	// callback table and this is the only place we learn whether the
	// bank really sends them. Read the log, then tighten the godoc.
	t.Logf("bank=%q terminal=%q paymentType=%q ccy=%q internalPaymentId=%q "+
		"transactionId=%q dataTime=%q responseCode=%q errorMessage=%q",
		out.Bank, out.Terminal, out.PaymentType, out.Currency,
		out.InternalPaymentID, out.TransactionID, out.DataTime,
		out.ResponseCode, out.ErrorMessage)
	t.Logf("statement-delayed fields: maskedPan=%q cardMask=%q approvalCode=%q "+
		"rrn=%q countryCard=%q", out.MaskedPan, out.CardMask,
		out.ApprovalCode, out.RRN, out.CountryCard)
}

// An id the bank has never seen must map onto ErrNotFound rather than
// a bare 4xx — cheap to check and it needs no real payment.
func TestIntegration_T2PPaymentStatus_unknownID(t *testing.T) {
	_, err := liveClient(t).T2PPaymentStatus(liveContext(t),
		"sdk-integration-probe-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	require.Error(t, err, "an unknown externalPaymentId must not succeed")
	assert.True(t, acquiring.IsNotFound(err) || acquiring.IsBadRequest(err),
		"unknown id should map to a typed 404/400, got %v", err)
}

// TestIntegration_POSTransactionCancel issues a REAL REFUND against a
// REAL transaction. It needs three things and skips unless all are
// present: the RRN to refund, the amount in minor units, and
// MONO_ALLOW_REFUND set to the opt-in constant.
//
// Pick an RRN deliberately — the test refuses to discover one on its
// own, because "find a recent POS sale and reverse part of it" is not
// something a test suite should decide.
func TestIntegration_POSTransactionCancel(t *testing.T) {
	if os.Getenv("MONO_ALLOW_REFUND") != refundOptIn {
		t.Skipf("refunds move real money; set MONO_ALLOW_REFUND=%s to run", refundOptIn)
	}
	rrn := envOrSkip(t, "MONO_POS_RRN")
	raw := envOrSkip(t, "MONO_POS_AMOUNT")

	amount, err := strconv.ParseInt(raw, 10, 64)
	require.NoError(t, err, "MONO_POS_AMOUNT must be an integer in minor units")
	require.Positive(t, amount, "a refund of zero or less makes no sense")

	err = liveClient(t).POSTransactionCancel(liveContext(t),
		&acquiring.POSTransactionCancelRequest{RRN: rrn, Amount: amount})
	require.NoError(t, err, "live POS refund must be accepted")
	t.Logf("refund of %d minor units accepted for RRN %s", amount, rrn)
}
