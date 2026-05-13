package installment

import "context"

// API — інтерфейс клієнта ПЧ для моків. Сам [Client] його реалізує
// (перевіряється compile-time assert-ом нижче).
type API interface {
	// Заявки.
	CreateOrder(ctx context.Context, in *CreateOrderRequest) (*CreateOrderResponse, error)
	OrderState(ctx context.Context, orderID string) (*OrderStateInfo, error)
	ConfirmOrder(ctx context.Context, orderID string) (*OrderStateInfo, error)
	RejectOrder(ctx context.Context, orderID string) (*OrderStateInfo, error)
	ReturnOrder(ctx context.Context, in *ReturnRequest) (*ReturnResponse, error)
	OrderInfo(ctx context.Context, orderID string) (*OrderShortInfo, error)
	OrderData(ctx context.Context, orderID string) (*OrderShortInfo, error)
	CheckPaid(ctx context.Context, orderID string) (*CheckInstallmentsResponse, error)

	// Гарантійний лист.
	GuaranteeLetterPDF(ctx context.Context, in *OrderDataRequest) ([]byte, error)
	GuaranteeLetterData(ctx context.Context, in *OrderDataRequest) (*OrderData, error)
	GuaranteeLetterDataV2(ctx context.Context, in *OrderDataRequest) (*OrderData, error)

	// Валідація клієнта.
	ValidateClient(ctx context.Context, phone string) (bool, error)
	ValidateClientLegacy(ctx context.Context, phone string) (*ValidateClientResponse, error)

	// Звітність.
	DailyReport(ctx context.Context, date string) ([]ReportOrder, error)
}

var _ API = (*Client)(nil)
