package installment

import (
	"context"
	"net/http"
)

// CreateOrder створює заявку на «Покупка частинами». Повертає order_id;
// решту інформації магазин отримує асинхронно: callback на
// CreateOrderRequest.ResultCallback (якщо переданий), або polling через
// [Client.OrderState].
//
// Замовлення з тим самим store_order_id ідемпотентне — повторний виклик
// поверне той самий order_id замість створення нового.
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

// OrderState повертає поточний стан заявки.
// IN_PROCESS/WAITING_FOR_STORE_CONFIRM — момент, коли треба викликати
// [Client.ConfirmOrder] після видачі товару. Усі FAIL/* — фінальні.
//
// POST /api/order/state  (200 → OrderStateInfo)
func (c *Client) OrderState(ctx context.Context, orderID string) (*OrderStateInfo, error) {
	var out OrderStateInfo
	if err := c.doJSON(ctx, "/api/order/state",
		RequestWithOrderIdentifier{OrderID: orderID}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// ConfirmOrder підтверджує видачу товару — активує розстрочку. Викликай
// у відповідь на стан WAITING_FOR_STORE_CONFIRM.
//
// POST /api/order/confirm  (200 → OrderStateInfo)
func (c *Client) ConfirmOrder(ctx context.Context, orderID string) (*OrderStateInfo, error) {
	var out OrderStateInfo
	if err := c.doJSON(ctx, "/api/order/confirm",
		RequestWithOrderIdentifier{OrderID: orderID}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// RejectOrder скасовує заявку з боку магазину (наприклад, товару немає
// на складі). Допустимо до видачі товару.
//
// POST /api/order/reject  (200 → OrderStateInfo)
func (c *Client) RejectOrder(ctx context.Context, orderID string) (*OrderStateInfo, error) {
	var out OrderStateInfo
	if err := c.doJSON(ctx, "/api/order/reject",
		RequestWithOrderIdentifier{OrderID: orderID}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReturnOrder реєструє повернення товару (повне або часткове).
// ReturnMoneyToCard: true — гроші повертаються на картку клієнта;
// false — клієнт забирає готівкою в магазині.
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

// OrderInfo — застаріла версія OrderData. У нових інтеграціях використовуй
// [Client.OrderData].
//
// POST /api/order/info  (200 → OrderShortInfo)
//
// Deprecated: використовуй [Client.OrderData].
func (c *Client) OrderInfo(ctx context.Context, orderID string) (*OrderShortInfo, error) {
	var out OrderShortInfo
	if err := c.doJSON(ctx, "/api/order/info",
		RequestWithOrderIdentifier{OrderID: orderID}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// OrderData повертає детальну інформацію по заявці, включно зі списком
// повернень та маскованою карткою.
//
// POST /api/order/data  (200 → OrderShortInfo)
func (c *Client) OrderData(ctx context.Context, orderID string) (*OrderShortInfo, error) {
	var out OrderShortInfo
	if err := c.doJSON(ctx, "/api/order/data",
		RequestWithOrderIdentifier{OrderID: orderID}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// CheckPaid каже, чи повністю сплачена заявка клієнтом і чи може банк
// повернути кошти на картку при поверненні товару.
//
// POST /api/order/check/paid  (200 → CheckInstallmentsResponse)
func (c *Client) CheckPaid(ctx context.Context, orderID string) (*CheckInstallmentsResponse, error) {
	var out CheckInstallmentsResponse
	if err := c.doJSON(ctx, "/api/order/check/paid",
		RequestWithOrderIdentifier{OrderID: orderID}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}
