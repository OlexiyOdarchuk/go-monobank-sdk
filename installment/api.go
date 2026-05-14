package installment

import "context"

// API is the installment client interface for mocks. [Client]
// implements it (verified by the compile-time assert below).
type API interface {
	// Orders.
	CreateOrder(ctx context.Context, in *CreateOrderRequest) (*CreateOrderResponse, error)
	OrderState(ctx context.Context, orderID string) (*OrderStateInfo, error)
	ConfirmOrder(ctx context.Context, orderID string) (*OrderStateInfo, error)
	RejectOrder(ctx context.Context, orderID string) (*OrderStateInfo, error)
	ReturnOrder(ctx context.Context, in *ReturnRequest) (*ReturnResponse, error)
	OrderInfo(ctx context.Context, orderID string) (*OrderShortInfo, error)
	OrderData(ctx context.Context, orderID string) (*OrderShortInfo, error)
	CheckPaid(ctx context.Context, orderID string) (*CheckInstallmentsResponse, error)

	// Guarantee letter.
	GuaranteeLetterPDF(ctx context.Context, in *OrderDataRequest) ([]byte, error)
	GuaranteeLetterData(ctx context.Context, in *OrderDataRequest) (*OrderData, error)
	GuaranteeLetterDataV2(ctx context.Context, in *OrderDataRequest) (*OrderData, error)

	// Client validation.
	ValidateClient(ctx context.Context, phone string) (bool, error)
	ValidateClientLegacy(ctx context.Context, phone string) (*ValidateClientResponse, error)

	// Reporting.
	DailyReport(ctx context.Context, date string) ([]ReportOrder, error)
}

var _ API = (*Client)(nil)
