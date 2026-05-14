package acquiring

import (
	"context"
	"time"
)

// API is the interface of the acquiring client. It exists separately
// from *[Client] so callers can mock it via mockgen/testify-mock in
// their own tests. [Client] implements this interface (verified by
// the compile-time assert below).
//
// Grouped by topic for convenience.
type API interface {
	// Merchant.
	MerchantDetails(ctx context.Context) (*MerchantDetails, error)
	Employees(ctx context.Context) ([]Employee, error)
	PubKey(ctx context.Context) (*ServerKey, error)
	Submerchants(ctx context.Context) ([]Submerchant, error)
	Statement(ctx context.Context, from, to time.Time, code string) ([]StatementInvoice, error)

	// Invoices.
	CreateInvoice(ctx context.Context, in *CreateInvoiceRequest) (*CreateInvoiceResponse, error)
	InvoiceStatus(ctx context.Context, invoiceID string) (*InvoiceStatusResponse, error)
	CancelInvoice(ctx context.Context, in *CancelRequest) (*CancelResponse, error)
	FinalizeInvoice(ctx context.Context, in *FinalizeRequest) (*FinalizeResponse, error)
	RemoveInvoice(ctx context.Context, invoiceID string) error
	FiscalChecks(ctx context.Context, invoiceID string) ([]FiscalCheck, error)
	Receipt(ctx context.Context, invoiceID, email string) (*ReceiptResponse, error)
	PaymentDirect(ctx context.Context, in *PaymentDirectRequest) (*PaymentDirectResponse, error)
	SyncPayment(ctx context.Context, in *SyncPaymentRequest) (*SyncPaymentResponse, error)

	// QR cash desks.
	QRList(ctx context.Context) ([]QR, error)
	QRDetails(ctx context.Context, qrID string) (*QRDetails, error)
	QRResetAmount(ctx context.Context, qrID string) error

	// Wallet (tokenized cards).
	Wallet(ctx context.Context, walletID string) ([]WalletCard, error)
	DeleteCard(ctx context.Context, cardToken string) error
	WalletPayment(ctx context.Context, in *WalletPaymentRequest) (*WalletPaymentResponse, error)

	// Subscriptions (recurring payments).
	SubscriptionCreate(ctx context.Context, in *SubscriptionCreateRequest) (*SubscriptionCreateResponse, error)
	SubscriptionEdit(ctx context.Context, in *SubscriptionEditRequest) error
	SubscriptionRemove(ctx context.Context, subscriptionID string) error
	SubscriptionStatus(ctx context.Context, subscriptionID string) (*SubscriptionStatusResponse, error)
	SubscriptionList(ctx context.Context, opts SubscriptionListOptions) (*SubscriptionsListResponse, error)
	SubscriptionPayments(ctx context.Context, opts SubscriptionPaymentsOptions) (*SubscriptionPaymentsResponse, error)

	// monopay keys (signing requests to the monopay widget).
	MonoPayKeyImport(ctx context.Context, in *MonoPayKeyImportRequest) (string, error)
	MonoPayKeyDelete(ctx context.Context, keyID string) error
	MonoPayKeyList(ctx context.Context) ([]MonoPayKey, error)

	// Split receivers (payment splitting) and T2P terminals.
	SplitReceivers(ctx context.Context) ([]SplitReceiver, error)
	Terminals(ctx context.Context) ([]Terminal, error)
}

// Compile-time assert: *Client satisfies [API].
var _ API = (*Client)(nil)
