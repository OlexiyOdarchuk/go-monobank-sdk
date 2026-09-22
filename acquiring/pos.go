package acquiring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// POSTransactionCancelRequest is the body of
// POST /api/merchant/pos-transaction-cancel. RRN is the reference
// retrieval number of the original POS transaction — the only
// identifier this endpoint accepts. Amount is the refund in minor
// units and must not exceed what is left of the transaction after the
// refunds already made against it.
type POSTransactionCancelRequest struct {
	RRN    string `json:"rrn"`
	Amount int64  `json:"amount"`
}

// POSTransactionCancel refunds a transaction made on a physical POS
// terminal, addressing it by the RRN of the original operation. A POS
// sale does not go through [Client.CreateInvoice], so it has no
// invoiceId for [Client.CancelInvoice] to work with.
//
// https://monobank.ua/api-docs/acquiring/methods/pos/post--api--merchant--pos-transaction-cancel
//
// Partial refunds are allowed: call the method repeatedly while the
// remaining balance still covers Amount.
//
// The endpoint only acknowledges that the refund was initiated, and
// monobank publishes no machine-readable schema for the 200 body (the
// samples on the docs site render client-side), so the method reports
// that acknowledgement as a nil error — the same shape as
// [Client.QRResetAmount] or [Client.RemoveInvoice]. A 404
// ([IsNotFound]) means no transaction with this RRN was found; a 429
// ([IsTooManyRequests]) means the endpoint is being called too often.
//
// This is the one call in the package that moves money outward, so
// the retry rules matter. Being a POST it is not retried
// automatically, and monobank documents no idempotency key for it —
// the bank has nothing to deduplicate against. Enabling
// [monobank.WithUnsafeRetries] therefore makes a timed-out refund
// eligible for a second attempt that the bank may well honour a
// second time. Leave it off for this client, and on an ambiguous
// failure reconcile against [Client.Statement] instead of retrying.
func (c *Client) POSTransactionCancel(ctx context.Context, in *POSTransactionCancelRequest) error {
	if in == nil {
		return ErrNilRequest
	}
	if in.RRN == "" {
		return ErrEmptyID
	}
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/pos-transaction-cancel", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}
