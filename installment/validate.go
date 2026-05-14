package installment

import (
	"context"
	"net/http"
)

// ValidateClientLegacy is the deprecated client validation. It
// returns the full information (full name, INN) when the client is
// found.
//
// POST /api/client/validate  (200 → ValidateClientResponse)
//
// Deprecated: use [Client.ValidateClient] (v2) for every new
// integration.
func (c *Client) ValidateClientLegacy(ctx context.Context, phone string) (*ValidateClientResponse, error) {
	var out ValidateClientResponse
	if err := c.doJSON(ctx, "/api/client/validate",
		ValidateClientRequest{Phone: phone}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// ValidateClient checks whether the phone belongs to a monobank
// client. It returns only the found flag, with no personal data
// (unlike the deprecated version).
//
// POST /api/v2/client/validate  (200 → ValidateClientSimpleResponse)
func (c *Client) ValidateClient(ctx context.Context, phone string) (bool, error) {
	var out ValidateClientSimpleResponse
	if err := c.doJSON(ctx, "/api/v2/client/validate",
		ValidateClientRequest{Phone: phone}, &out, http.StatusOK); err != nil {
		return false, err
	}
	return out.Found, nil
}
