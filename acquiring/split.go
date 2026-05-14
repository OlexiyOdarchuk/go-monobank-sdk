package acquiring

import (
	"context"
	"fmt"
	"net/http"
)

// SplitReceiver is a single recipient in a payment-splitting scheme.
// SplitReceiverID is used in CreateInvoiceRequest when splitting (it
// is wired up via a separate agent-agreement mechanism on the bank's
// side — unavailable for default merchants).
type SplitReceiver struct {
	SplitReceiverID string `json:"splitReceiverId"`
	OKPO            string `json:"okpo"`
	Name            string `json:"name"`
}

// SplitReceiverListResponse wraps the /split-receiver/list response.
type SplitReceiverListResponse struct {
	List []SplitReceiver `json:"list"`
}

// SplitReceivers returns the list of recipients for payment
// splitting (submerchants). Only available to merchants whose
// agent / split-acquiring scheme has been configured.
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
