package installment

import (
	"context"
	"net/http"
)

// QRCartIdentifierResponse is the 201 body of POST /api/v1/qr/cart.
//
// ID is the cart's identifier, and per the docs it is also the
// order_id of the order created once the client scans the QR code:
// use it with [Client.OrderState] and to match incoming callbacks.
// There is no separate request to inspect a QR cart.
type QRCartIdentifierResponse struct {
	ID string `json:"id"`
}

// CreateQRCartWithResponse puts a cart of goods behind one of the
// store's QR codes, for the client to scan in the Mono app, and
// returns the cart's id. Unlike [Client.CreateOrder] the request
// carries no phone number and no program list — the documented
// schema has no field for either.
//
// https://monobank.ua/api-docs/chast/servisy/qr-koshyk/post--api--v1--qr--cart
//
// How the QR flow differs from [Client.CreateOrder], per the docs:
//
//   - The returned ID is the order_id. Until the client scans the
//     code, [Client.OrderState] returns 404 — that is expected.
//   - No [Client.ConfirmOrder] call is needed: the order is confirmed
//     automatically, and no WAITING_FOR_STORE_CONFIRM callback is sent.
//   - The cart lives 15 minutes. If the code is not scanned, a
//     FAIL/CONFIRM_TIME_EXPIRED callback ([SubConfirmTimeExpired]) is
//     sent. If the client scanned it but did not confirm within 15
//     minutes, no callback is sent at all, so poll
//     [Client.OrderState] until a final state.
//
// Errors worth handling separately: 400 (malformed request, or the
// qr_id does not belong to this store), 401 (invalid signature),
// 403 (the store has been switched off by the bank), 409 (the
// store_order_id was already used — even for a cancelled cart — or
// the QR code already has an active cart).
//
// POST /api/v1/qr/cart  (201 → QRCartIdentifierResponse)
func (c *Client) CreateQRCartWithResponse(ctx context.Context, in *CreateQRCartRequest) (*QRCartIdentifierResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	if in.QRID == "" {
		return nil, ErrEmptyQRID
	}
	var out QRCartIdentifierResponse
	if err := c.doJSON(ctx, "/api/v1/qr/cart", in, &out, http.StatusCreated); err != nil {
		return nil, err
	}

	return &out, nil
}

// CreateQRCart puts a cart of goods behind one of the store's QR
// codes and discards the cart id the bank returns.
//
// Deprecated: use [Client.CreateQRCartWithResponse] — the returned id
// is the order_id needed for [Client.OrderState] and for matching
// callbacks, and this method throws it away.
func (c *Client) CreateQRCart(ctx context.Context, in *CreateQRCartRequest) error {
	if in == nil {
		return ErrNilRequest
	}
	if in.QRID == "" {
		return ErrEmptyQRID
	}
	// The body is not decoded, so a 201 the SDK cannot parse still
	// counts as success here, as it always has.
	return c.doJSON(ctx, "/api/v1/qr/cart", in, nil, http.StatusCreated)
}

// CancelQRCart clears the cart currently attached to the QR code and
// frees the code for a new cart. Per the docs, if the client has
// already scanned the code, the purchase continues regardless; if a
// SUCCESS/ACTIVE callback arrives after the cancellation and the
// goods were not handed over, issue a refund with
// [Client.ReturnOrder].
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
