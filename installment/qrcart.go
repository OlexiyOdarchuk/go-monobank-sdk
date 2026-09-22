package installment

import (
	"context"
	"net/http"
)

// CreateQRCart puts a cart of goods behind one of the store's QR
// codes, for the client to scan in the Mono app. Unlike
// [Client.CreateOrder] the request carries no phone number and no
// program list — the documented schema has no field for either.
//
// https://monobank.ua/api-docs/chast/servisy/qr-koshyk/post--api--v1--qr--cart
//
// The bank confirms only that the cart was accepted; the outcome of
// processing it arrives asynchronously at
// CreateQRCartRequest.ResultCallback. monobank publishes no
// machine-readable schema for the 201 body, hence the error-only
// return.
//
// Errors worth handling separately: 400 (malformed request, or the
// qr_id does not belong to this store), 401 (invalid signature),
// 403 (the store has been switched off by the bank).
//
// POST /api/v1/qr/cart  (201, no body)
func (c *Client) CreateQRCart(ctx context.Context, in *CreateQRCartRequest) error {
	if in == nil {
		return ErrNilRequest
	}
	if in.QRID == "" {
		return ErrEmptyQRID
	}
	return c.doJSON(ctx, "/api/v1/qr/cart", in, nil, http.StatusCreated)
}

// CancelQRCart cancels the cart currently attached to the QR code.
// The docs do not say what a client who is already looking at the
// cart sees afterwards, nor what happens once they have confirmed
// it — do not rely on either.
//
// Errors: 400 (the qr_id does not belong to this store), 401
// (invalid signature).
//
// https://monobank.ua/api-docs/chast/servisy/qr-koshyk/post--api--v1--qr--cart--cancel
//
// POST /api/v1/qr/cart/cancel  (200, no body)
func (c *Client) CancelQRCart(ctx context.Context, qrID string) error {
	if qrID == "" {
		return ErrEmptyQRID
	}
	return c.doJSON(ctx, "/api/v1/qr/cart/cancel",
		CancelQRCartRequest{QRID: qrID}, nil, http.StatusOK)
}
