package installment

import (
	"context"
	"net/http"
)

// CreateOrder creates an installment order. It returns order_id; the
// rest of the information arrives asynchronously: a callback to
// CreateOrderRequest.ResultCallback (when provided) or polling via
// [Client.OrderState].
//
// An order with the same store_order_id is idempotent — repeating
// the call returns the same order_id instead of creating a new one.
//
// POST /api/order/create  (201 → CreateOrderResponse)
func (c *Client) CreateOrder(ctx context.Context, in *CreateOrderRequest) (*CreateOrderResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	var out CreateOrderResponse
	if err := c.doJSON(ctx, "/api/order/create", in, &out, http.StatusCreated); err != nil {
		return nil, err
	}
	return &out, nil
}

// OrderState returns the order's current state.
// IN_PROCESS/WAITING_FOR_STORE_CONFIRM is the moment to call
// [Client.ConfirmOrder] after handing over the goods. All FAIL/*
// states are terminal.
//
// POST /api/order/state  (200 → OrderStateInfo)
func (c *Client) OrderState(ctx context.Context, orderID string) (*OrderStateInfo, error) {
	if orderID == "" {
		return nil, ErrEmptyOrderID
	}
	var out OrderStateInfo
	if err := c.doJSON(ctx, "/api/order/state",
		RequestWithOrderIdentifier{OrderID: orderID}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// ConfirmOrder confirms that the goods have been handed over — this
// activates the installment plan. Call in response to a
// WAITING_FOR_STORE_CONFIRM state.
//
// POST /api/order/confirm  (200 → OrderStateInfo)
func (c *Client) ConfirmOrder(ctx context.Context, orderID string) (*OrderStateInfo, error) {
	if orderID == "" {
		return nil, ErrEmptyOrderID
	}
	var out OrderStateInfo
	if err := c.doJSON(ctx, "/api/order/confirm",
		RequestWithOrderIdentifier{OrderID: orderID}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// RejectOrder cancels the order from the merchant's side (for
// example, the goods are out of stock). Allowed before the goods are
// handed over.
//
// POST /api/order/reject  (200 → OrderStateInfo)
func (c *Client) RejectOrder(ctx context.Context, orderID string) (*OrderStateInfo, error) {
	if orderID == "" {
		return nil, ErrEmptyOrderID
	}
	var out OrderStateInfo
	if err := c.doJSON(ctx, "/api/order/reject",
		RequestWithOrderIdentifier{OrderID: orderID}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReturnOrder records a return of goods (full or partial).
// ReturnMoneyToCard: true sends the money back to the client's card;
// false means the client collects cash at the store.
//
// POST /api/order/return  (200 → ReturnResponse)
func (c *Client) ReturnOrder(ctx context.Context, in *ReturnRequest) (*ReturnResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	var out ReturnResponse
	if err := c.doJSON(ctx, "/api/order/return", in, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// OrderInfo is a deprecated version of OrderData. In new
// integrations use [Client.OrderData].
//
// POST /api/order/info  (200 → OrderShortInfo)
//
// Deprecated: use [Client.OrderData].
func (c *Client) OrderInfo(ctx context.Context, orderID string) (*OrderShortInfo, error) {
	if orderID == "" {
		return nil, ErrEmptyOrderID
	}
	var out OrderShortInfo
	if err := c.doJSON(ctx, "/api/order/info",
		RequestWithOrderIdentifier{OrderID: orderID}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// OrderData returns detailed information about an order, including
// the return list and the masked card.
//
// POST /api/order/data  (200 → OrderShortInfo)
func (c *Client) OrderData(ctx context.Context, orderID string) (*OrderShortInfo, error) {
	if orderID == "" {
		return nil, ErrEmptyOrderID
	}
	var out OrderShortInfo
	if err := c.doJSON(ctx, "/api/order/data",
		RequestWithOrderIdentifier{OrderID: orderID}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// CheckPaid reports whether the client has fully paid the order and
// whether the bank can refund the money to the card on a return.
//
// POST /api/order/check/paid  (200 → CheckInstallmentsResponse)
func (c *Client) CheckPaid(ctx context.Context, orderID string) (*CheckInstallmentsResponse, error) {
	if orderID == "" {
		return nil, ErrEmptyOrderID
	}
	var out CheckInstallmentsResponse
	if err := c.doJSON(ctx, "/api/order/check/paid",
		RequestWithOrderIdentifier{OrderID: orderID}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}
