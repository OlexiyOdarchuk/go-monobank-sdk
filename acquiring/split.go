package acquiring

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// SplitReceiver is a single recipient in a payment-splitting scheme.
// SplitReceiverID is what CreateInvoiceRequest's basket lines carry
// as splitReceiverId when splitting.
//
// https://monobank.ua/api-docs/acquiring/methods/split/get--api--merchant--split-receiver--list
type SplitReceiver struct {
	// SplitReceiverID is the receiver's reference. The wire field is
	// `id`; the Go name is kept for compatibility.
	SplitReceiverID string `json:"id"`
	// OKPO is the seller's tax id. The wire field is `edrpou`; the
	// list endpoint describes it as ЄДРПОУ, the get-by-id endpoint as
	// ІПН.
	OKPO string `json:"edrpou"`
	Name string `json:"name"`
}

// SplitReceiverListResponse wraps the /split-receiver/list response.
type SplitReceiverListResponse struct {
	List []SplitReceiver `json:"list"`
}

// SplitReceiverStatus is the state of a split receiver as reported
// by [Client.SplitReceiverByID].
type SplitReceiverStatus string

// Possible SplitReceiverStatus values, as documented.
const (
	SplitReceiverActive     SplitReceiverStatus = "active"
	SplitReceiverProcessing SplitReceiverStatus = "processing"
	SplitReceiverNotActive  SplitReceiverStatus = "not_active"
	SplitReceiverBlocked    SplitReceiverStatus = "blocked"
	SplitReceiverRestricted SplitReceiverStatus = "restricted"
)

// SplitReceiverDetails is the response of
// GET /api/merchant/split-receiver/get-by-id. It carries the same
// id/edrpou/name as a list entry plus the receiver's status and IBAN.
// The docs note that the name is shortened to its first two words.
type SplitReceiverDetails struct {
	SplitReceiver
	Status SplitReceiverStatus `json:"status"`
	IBAN   string              `json:"iban"`
}

// SplitReceivers returns the list of recipients for payment
// splitting (submerchants). Only available to merchants whose
// split scheme has been configured.
func (c *Client) SplitReceivers(ctx context.Context) ([]SplitReceiver, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/merchant/split-receiver/list", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out SplitReceiverListResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.List, nil
}

// SplitReceiverByID returns one split receiver by its id. The docs
// say it returns the receiver in any status. An unknown id is a 404
// ([IsNotFound]).
//
// https://monobank.ua/api-docs/acquiring/methods/split/get--api--merchant--split-receiver--get-by-id
func (c *Client) SplitReceiverByID(ctx context.Context, splitReceiverID string) (*SplitReceiverDetails, error) {
	if splitReceiverID == "" {
		return nil, ErrEmptyID
	}
	q := url.Values{}
	q.Set("splitReceiverId", splitReceiverID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/merchant/split-receiver/get-by-id?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out SplitReceiverDetails
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}

	return &out, nil
}
