package acquiring

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
)

// IsTerminal reports whether the status is a final invoice state —
// one Mono will not move away from on its own. success, failure,
// reversed and expired are terminal; created, processing and hold are
// transient (hold is awaiting a finalize/cancel decision from the
// merchant).
func (s InvoiceStatus) IsTerminal() bool {
	switch s {
	case InvoiceSuccess, InvoiceFailure, InvoiceReversed, InvoiceExpired:
		return true
	default:
		return false
	}
}

// PollOptions tunes [Client.PollInvoice].
type PollOptions struct {
	// Interval between status polls. Default 2s; values below 500ms
	// are raised to 500ms to stay friendly to the rate limiter.
	Interval time.Duration
	// Timeout caps the total wait. Zero relies entirely on the
	// context's own deadline/cancellation.
	Timeout time.Duration
	// TreatHoldAsTerminal stops polling when the invoice reaches the
	// "hold" state. Use it for auth-then-capture flows where "hold" is
	// the state you are waiting for (you then call FinalizeInvoice).
	// By default hold is treated as transient.
	TreatHoldAsTerminal bool
}

const (
	defaultPollInterval = 2 * time.Second
	minPollInterval     = 500 * time.Millisecond
)

// ErrPollTimeout is returned by [Client.PollInvoice] when the invoice
// did not reach a terminal state before Timeout / the context
// deadline elapsed. The last observed status, if any, is wrapped for
// context.
var ErrPollTimeout = errors.New("acquiring: invoice did not reach a terminal status before timeout")

// PollInvoice polls [Client.InvoiceStatus] until the invoice reaches
// a terminal state ([InvoiceStatus.IsTerminal], plus "hold" when
// [PollOptions.TreatHoldAsTerminal] is set) and returns that final
// state. It is the reconciliation fallback for a missed webhook (and
// the only way to observe "expired", which never triggers a webhook).
//
// It returns:
//   - the terminal [InvoiceStatusResponse] and nil on success;
//   - [ErrPollTimeout] (wrapping the last status) when Timeout / the
//     context deadline elapses first;
//   - ctx.Err() if the context is cancelled;
//   - any non-transient API error from InvoiceStatus (transient ones
//     are already retried by the underlying client).
//
// Example:
//
//	inv, err := cli.PollInvoice(ctx, id, acquiring.PollOptions{
//	    Interval: time.Second, Timeout: 2 * time.Minute,
//	})
func (c *Client) PollInvoice(ctx context.Context, invoiceID string, opts PollOptions) (*InvoiceStatusResponse, error) {
	if invoiceID == "" {
		return nil, ErrEmptyID
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	if interval < minPollInterval {
		interval = minPollInterval
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	terminal := func(s InvoiceStatus) bool {
		return s.IsTerminal() || (opts.TreatHoldAsTerminal && s == InvoiceHold)
	}

	timer := time.NewTimer(0) // fire immediately for the first poll
	defer timer.Stop()

	var last *InvoiceStatusResponse
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) && last != nil {
				return last, fmt.Errorf("%w (last status %q)", ErrPollTimeout, last.Status)
			}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, ErrPollTimeout
			}
			return last, ctx.Err()
		case <-timer.C:
			inv, err := c.InvoiceStatus(ctx, invoiceID)
			if err != nil {
				return nil, err
			}
			last = inv
			if terminal(inv.Status) {
				return inv, nil
			}
			timer.Reset(interval)
		}
	}
}

// LocalPayment is the merchant-side record of an invoice, used by
// [ReconcileStatement] to find rows that drifted from the bank's
// view. Status is optional: leave it "" to compare amounts only.
type LocalPayment struct {
	Amount money.Money
	Status ProcessingStatus
}

// Mismatch is one invoice whose local record disagrees with the bank
// statement. Exactly one kind of disagreement is described per
// Mismatch (amount or status); the Reason names which.
type Mismatch struct {
	InvoiceID    string
	Reason       string // "amount" or "status"
	LocalAmount  money.Money
	RemoteAmount money.Money
	LocalStatus  ProcessingStatus
	RemoteStatus ProcessingStatus
}

// Reconciliation is the result of comparing a bank statement against
// local records. A clean reconciliation has all three slices empty
// ([Reconciliation.Clean] is true).
type Reconciliation struct {
	// OnlyRemote are statement rows with no matching local record —
	// payments the bank knows about but the merchant DB does not
	// (missed webhook, dropped write).
	OnlyRemote []StatementInvoice
	// OnlyLocal are local invoiceIDs absent from the statement —
	// records the merchant has but the bank did not report for the
	// period (wrong period, never actually charged).
	OnlyLocal []string
	// Mismatches are invoices present on both sides whose amount or
	// status disagree.
	Mismatches []Mismatch
}

// Clean reports whether the two sides fully agree.
func (r *Reconciliation) Clean() bool {
	return len(r.OnlyRemote) == 0 && len(r.OnlyLocal) == 0 && len(r.Mismatches) == 0
}

// ReconcileStatement diffs a bank statement (from [Client.Statement])
// against local records keyed by invoiceId. It is a pure function —
// no I/O — so it is trivial to unit-test and can run over a statement
// page you already fetched.
//
// For each invoiceId it reports: present only in the statement
// (OnlyRemote), present only locally (OnlyLocal), or present on both
// with a different amount or status (Mismatches). Status is compared
// only when the local record sets a non-empty Status. Amount is
// compared on the minor-unit value; currency is taken from the
// statement row.
//
//	local := map[string]acquiring.LocalPayment{
//	    "inv-1": {Amount: money.UAH(149.99), Status: acquiring.StatusSuccess},
//	}
//	rec := acquiring.ReconcileStatement(stmt, local)
//	if !rec.Clean() { /* alert / investigate */ }
func ReconcileStatement(statement []StatementInvoice, local map[string]LocalPayment) *Reconciliation {
	rec := &Reconciliation{}
	seen := make(map[string]struct{}, len(statement))

	for _, row := range statement {
		seen[row.InvoiceID] = struct{}{}
		lp, ok := local[row.InvoiceID]
		if !ok {
			rec.OnlyRemote = append(rec.OnlyRemote, row)
			continue
		}
		if lp.Amount.Minor != row.Amount.Minor {
			rec.Mismatches = append(rec.Mismatches, Mismatch{
				InvoiceID:    row.InvoiceID,
				Reason:       "amount",
				LocalAmount:  lp.Amount,
				RemoteAmount: row.Amount,
			})
		}
		if lp.Status != "" && lp.Status != row.Status {
			rec.Mismatches = append(rec.Mismatches, Mismatch{
				InvoiceID:    row.InvoiceID,
				Reason:       "status",
				LocalStatus:  lp.Status,
				RemoteStatus: row.Status,
			})
		}
	}

	for id := range local {
		if _, ok := seen[id]; !ok {
			rec.OnlyLocal = append(rec.OnlyLocal, id)
		}
	}
	// Map iteration order is random; sort so the result is
	// deterministic (stable logs, comparable in tests).
	sort.Strings(rec.OnlyLocal)
	return rec
}
