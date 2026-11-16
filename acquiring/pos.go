package acquiring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// DefaultPOSLanguage is the Accept-Language sent on
// POST /api/merchant/pos-transaction-cancel when
// [POSTransactionCancelRequest.Language] is empty.
const DefaultPOSLanguage = "uk"

// POSTransactionCancelRequest is the body of
// POST /api/merchant/pos-transaction-cancel. RRN is the reference
// retrieval number of the original POS transaction — the only
// identifier this endpoint accepts. Amount is the refund in minor
// units and must not exceed what is left of the transaction after the
// refunds already made against it.
type POSTransactionCancelRequest struct {
	RRN    string `json:"rrn"`
	Amount int64  `json:"amount"`
	// Language goes out as the Accept-Language header, which the docs
	// mark as required: the language of the response, "uk" or "en".
	// Empty means [DefaultPOSLanguage]. It is not part of the body.
	Language string `json:"-"`
}

// POSTransactionCancelResponse is the 200 body of
// POST /api/merchant/pos-transaction-cancel: the refund was initiated,
// not completed.
type POSTransactionCancelResponse struct {
	// Status is the status of initiating the refund (the docs' sample
	// sends "ok").
	Status string `json:"status"`
	// TranID identifies the refund transaction. Optional in the docs.
	TranID string `json:"tranId,omitempty"`
}

// POSRefundStatus is the state of a POS refund as reported by
// [Client.POSTransactionCancelStatus].
type POSRefundStatus string

// Possible POSRefundStatus values, as documented. The docs do not say
// which of them are final.
const (
	POSRefundNew        POSRefundStatus = "new"
	POSRefundProcessing POSRefundStatus = "processing"
	POSRefundSuccess    POSRefundStatus = "success"
	POSRefundRejected   POSRefundStatus = "rejected"
	POSRefundReversed   POSRefundStatus = "reversed"
)

// POSTransactionCancelStatusResponse is the response of
// GET /api/merchant/pos-transaction-cancel/status. Every field is
// optional in the docs.
type POSTransactionCancelStatusResponse struct {
	// Amount is the refund in minor units.
	Amount int64 `json:"amount"`
	// CancelRef is the cancel request's id the refund was found by.
	CancelRef string `json:"cancelRef"`
	// CreateTime and ModifyTime are RFC 3339 date-times.
	CreateTime string `json:"createTime"`
	ModifyTime string `json:"modifyTime"`
	// OriginalTranID identifies the transaction being refunded.
	OriginalTranID string `json:"originalTranId"`
	// RRN is the original transaction's RRN, the one sent in the
	// cancel request.
	RRN    string          `json:"rrn"`
	Status POSRefundStatus `json:"status"`
}

// POSTransactionCancel refunds a transaction made on a physical POS
// terminal and discards the response body.
//
// Deprecated: use [Client.POSTransactionCancelWithResponse], which
// returns the documented 200 body (the refund transaction id and the
// initiation status).
func (c *Client) POSTransactionCancel(ctx context.Context, in *POSTransactionCancelRequest) error {
	_, err := c.posTransactionCancel(ctx, in, false)

	return err
}

// POSTransactionCancelWithResponse refunds a transaction made on a
// physical POS terminal, addressing it by the RRN of the original
// operation. A POS sale does not go through [Client.CreateInvoice],
// so it has no invoiceId for [Client.CancelInvoice] to work with.
//
// https://monobank.ua/api-docs/acquiring/methods/pos/post--api--merchant--pos-transaction-cancel
//
// A 200 means the refund was initiated; the final outcome comes later.
// The docs refer to a pos_refund webhook for it but publish no schema
// for that webhook, and offer [Client.POSTransactionCancelStatus] for
// when it was not received.
//
// Partial refunds are allowed: Amount must not exceed what remains
// after previous refunds. Documented errors: 400 (invalid parameters,
// the transaction belongs to another merchant, the amount exceeds the
// remainder, or the merchant's balance is insufficient), 404
// ([IsNotFound]: no transaction with this RRN), 429
// ([IsTooManyRequests]).
//
// This call moves money outward, so the retry rules matter. Being a
// POST it is not retried automatically, and monobank documents no
// idempotency key for it — the bank has nothing to deduplicate
// against. Enabling [monobank.WithUnsafeRetries] therefore makes a
// timed-out refund eligible for a second attempt that the bank may
// well honor a second time. Leave it off for this client, and on an
// ambiguous failure check the refund's status instead of retrying.
func (c *Client) POSTransactionCancelWithResponse(
	ctx context.Context, in *POSTransactionCancelRequest,
) (*POSTransactionCancelResponse, error) {
	return c.posTransactionCancel(ctx, in, true)
}

func (c *Client) posTransactionCancel(
	ctx context.Context, in *POSTransactionCancelRequest, decode bool,
) (*POSTransactionCancelResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	if in.RRN == "" {
		return nil, ErrEmptyID
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/pos-transaction-cancel", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	lang := in.Language
	if lang == "" {
		lang = DefaultPOSLanguage
	}
	req.Header.Set("Accept-Language", lang)
	if !decode {
		return nil, c.c.Do(req, nil, http.StatusOK)
	}
	var out POSTransactionCancelResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}

	return &out, nil
}

// POSTransactionCancelStatus returns a refund created by
// [Client.POSTransactionCancelWithResponse], looked up by its
// cancelRef. It searches only this merchant's refunds and only over a
// recent window whose length the docs leave unrendered (the page says
// "за останні PosRefundMaxDays днів"). The docs position it as the way
// to learn the final status when the pos_refund webhook was not
// received.
//
// https://monobank.ua/api-docs/acquiring/methods/pos/get--api--merchant--pos-transaction-cancel--status
//
// The docs do not say where cancelRef comes from: the cancel request
// carries only rrn and amount, and its response only status and
// tranId. Whether tranId is the cancelRef is not stated (the samples
// on the two pages use different values), so verify it against your
// own refunds before relying on it.
//
// A 404 ([IsNotFound]) means no refund with this cancelRef exists for
// this merchant.
func (c *Client) POSTransactionCancelStatus(ctx context.Context, cancelRef string) (*POSTransactionCancelStatusResponse, error) {
	if cancelRef == "" {
		return nil, ErrEmptyID
	}
	q := url.Values{}
	q.Set("cancelRef", cancelRef)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/merchant/pos-transaction-cancel/status?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out POSTransactionCancelStatusResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}

	return &out, nil
}
