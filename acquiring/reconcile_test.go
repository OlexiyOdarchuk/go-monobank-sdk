package acquiring_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/monobanktest"
)

func TestInvoiceStatus_IsTerminal(t *testing.T) {
	for _, s := range []acquiring.InvoiceStatus{
		acquiring.InvoiceSuccess, acquiring.InvoiceFailure,
		acquiring.InvoiceReversed, acquiring.InvoiceExpired,
	} {
		assert.True(t, s.IsTerminal(), string(s))
	}
	for _, s := range []acquiring.InvoiceStatus{
		acquiring.InvoiceCreated, acquiring.InvoiceProcessing, acquiring.InvoiceHold,
	} {
		assert.False(t, s.IsTerminal(), string(s))
	}
}

func TestPollInvoice_reachesTerminal(t *testing.T) {
	srv := monobanktest.NewServer(t)
	// Перші дві відповіді — processing, третя — success.
	srv.Handle(http.MethodGet, "/api/merchant/invoice/status", monobanktest.Sequence(
		monobanktest.JSON(`{"invoiceId":"p2_x","status":"processing","amount":4200,"ccy":980}`),
		monobanktest.JSON(`{"invoiceId":"p2_x","status":"processing","amount":4200,"ccy":980}`),
		monobanktest.JSON(`{"invoiceId":"p2_x","status":"success","amount":4200,"ccy":980}`),
	))
	cli := acquiring.New("tok", srv.Option())

	inv, err := cli.PollInvoice(context.Background(), "p2_x", acquiring.PollOptions{
		Interval: 10 * time.Millisecond,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, acquiring.InvoiceSuccess, inv.Status)
}

func TestPollInvoice_timeout(t *testing.T) {
	srv := monobanktest.NewServer(t)
	srv.Handle(http.MethodGet, "/api/merchant/invoice/status",
		monobanktest.JSON(`{"invoiceId":"p2_x","status":"processing","amount":4200,"ccy":980}`))
	cli := acquiring.New("tok", srv.Option())

	inv, err := cli.PollInvoice(context.Background(), "p2_x", acquiring.PollOptions{
		Interval: 10 * time.Millisecond,
		Timeout:  60 * time.Millisecond,
	})
	require.ErrorIs(t, err, acquiring.ErrPollTimeout)
	require.NotNil(t, inv) // останній побачений стан повертається
	assert.Equal(t, acquiring.InvoiceProcessing, inv.Status)
}

func TestPollInvoice_holdAsTerminal(t *testing.T) {
	srv := monobanktest.NewServer(t)
	srv.Handle(http.MethodGet, "/api/merchant/invoice/status",
		monobanktest.JSON(`{"invoiceId":"p2_x","status":"hold","amount":4200,"ccy":980}`))
	cli := acquiring.New("tok", srv.Option())

	inv, err := cli.PollInvoice(context.Background(), "p2_x", acquiring.PollOptions{
		Interval:            10 * time.Millisecond,
		Timeout:             2 * time.Second,
		TreatHoldAsTerminal: true,
	})
	require.NoError(t, err)
	assert.Equal(t, acquiring.InvoiceHold, inv.Status)
}

func TestPollInvoice_emptyID(t *testing.T) {
	cli := acquiring.New("tok")
	_, err := cli.PollInvoice(context.Background(), "", acquiring.PollOptions{})
	require.ErrorIs(t, err, acquiring.ErrEmptyID)
}

func TestPollInvoice_ctxCancel(t *testing.T) {
	srv := monobanktest.NewServer(t)
	srv.Handle(http.MethodGet, "/api/merchant/invoice/status",
		monobanktest.JSON(`{"invoiceId":"p2_x","status":"processing","amount":4200,"ccy":980}`))
	cli := acquiring.New("tok", srv.Option())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := cli.PollInvoice(ctx, "p2_x", acquiring.PollOptions{Interval: 10 * time.Millisecond})
	require.True(t, errors.Is(err, context.Canceled))
}

func TestReconcileStatement(t *testing.T) {
	stmt := []acquiring.StatementInvoice{
		{InvoiceID: "inv-1", Amount: money.New(14999, currency.UAH), Status: acquiring.StatusSuccess},
		{InvoiceID: "inv-2", Amount: money.New(5000, currency.UAH), Status: acquiring.StatusSuccess},
		{InvoiceID: "inv-3", Amount: money.New(100, currency.UAH), Status: acquiring.StatusSuccess}, // only remote
	}
	local := map[string]acquiring.LocalPayment{
		"inv-1": {Amount: money.New(14999, currency.UAH), Status: acquiring.StatusSuccess}, // ok
		"inv-2": {Amount: money.New(9999, currency.UAH), Status: acquiring.StatusSuccess},  // amount mismatch
		"inv-9": {Amount: money.New(1, currency.UAH)},                                      // only local
	}

	rec := acquiring.ReconcileStatement(stmt, local)
	assert.False(t, rec.Clean())

	require.Len(t, rec.OnlyRemote, 1)
	assert.Equal(t, "inv-3", rec.OnlyRemote[0].InvoiceID)

	require.Len(t, rec.OnlyLocal, 1)
	assert.Equal(t, "inv-9", rec.OnlyLocal[0])

	require.Len(t, rec.Mismatches, 1)
	assert.Equal(t, "inv-2", rec.Mismatches[0].InvoiceID)
	assert.Equal(t, "amount", rec.Mismatches[0].Reason)
}

func TestReconcileStatement_statusMismatch(t *testing.T) {
	stmt := []acquiring.StatementInvoice{
		{InvoiceID: "inv-1", Amount: money.New(100, currency.UAH), Status: acquiring.StatusFailure},
	}
	local := map[string]acquiring.LocalPayment{
		"inv-1": {Amount: money.New(100, currency.UAH), Status: acquiring.StatusSuccess},
	}
	rec := acquiring.ReconcileStatement(stmt, local)
	require.Len(t, rec.Mismatches, 1)
	assert.Equal(t, "status", rec.Mismatches[0].Reason)
	assert.Equal(t, acquiring.StatusSuccess, rec.Mismatches[0].LocalStatus)
	assert.Equal(t, acquiring.StatusFailure, rec.Mismatches[0].RemoteStatus)
}

func TestReconcileStatement_clean(t *testing.T) {
	stmt := []acquiring.StatementInvoice{
		{InvoiceID: "inv-1", Amount: money.New(100, currency.UAH), Status: acquiring.StatusSuccess},
	}
	local := map[string]acquiring.LocalPayment{
		"inv-1": {Amount: money.New(100, currency.UAH)}, // статус не задано -> не звіряємо
	}
	rec := acquiring.ReconcileStatement(stmt, local)
	assert.True(t, rec.Clean())
}
