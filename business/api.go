package business

import (
	"context"
	"iter"
	"time"
)

// API — інтерфейс corp-api клієнта (юр. особи). Існує окремо від
// *[Client], щоб користувачі могли мокувати його через
// mockgen/testify-mock у власних тестах. Сам [Client] цей інтерфейс
// реалізує (перевіряється compile-time assert-ом нижче).
//
// Згруповано за топіками для зручності.
type API interface {
	// Рахунки.
	Accounts(ctx context.Context) ([]Account, error)
	Account(ctx context.Context, iban string) (*Account, error)
	AccountBalances(ctx context.Context, iban, dateFrom, dateTo string) ([]BalancePoint, error)

	// Зарплатні контакти.
	Contacts(ctx context.Context, limit, offset int) (*ContactsPage, error)
	ContactsAll(ctx context.Context, pageSize int) iter.Seq2[Contact, error]
	SearchContacts(ctx context.Context, query string, limit, offset int) (*ContactsPage, error)
	SearchContactsAll(ctx context.Context, query string, pageSize int) iter.Seq2[Contact, error]
	ContactByID(ctx context.Context, id string) (*Contact, error)
	CreateContact(ctx context.Context, in *CreateContactRequest) error
	DeleteContact(ctx context.Context, id string) error
	DeleteContactsBatch(ctx context.Context, ids []string) error

	// Виписка та операції.
	Statement(ctx context.Context, account string, from, to time.Time,
		direction StatementDirection, limit int) ([]StatementItem, error)
	Operation(ctx context.Context, id, externalReference string) (*StatementItem, error)

	// Платежі.
	PreparePayment(ctx context.Context, idempotencyKey string,
		in *PaymentRequest) (*PaymentPrepared, error)
	PaymentState(ctx context.Context, id string) (*PaymentState, error)
	PaymentStateByReference(ctx context.Context, externalReference string) (*PaymentState, error)

	// Зарплатні відомості.
	CreateSalaryRegistry(ctx context.Context, idempotencyKey string,
		in *CreateSalaryRegistryRequest) (*SalaryRegistryCreated, error)
	SalaryRegistryTypes(ctx context.Context) ([]SalaryRegistryType, error)
	SalaryRegistryStatus(ctx context.Context, id string) (*SalaryRegistryStatus, error)

	// Розрахункові листи (payslips).
	UploadPayslips(ctx context.Context, in *BatchPayslipRequest) (*BatchPayslipResponse, error)
	DeletePayslips(ctx context.Context, in *DeletePayslipsRequest) error
	ImportStatus(ctx context.Context, period string) (*ImportStatusResponse, error)
	DeleteImport(ctx context.Context, period string) error
	SendPayslipsToMobile(ctx context.Context, period string) (*SendResult, error)
	PayslipPDF(ctx context.Context, identification, period string) ([]byte, error)
}

// Compile-time assert: *Client задовольняє [API].
var _ API = (*Client)(nil)
