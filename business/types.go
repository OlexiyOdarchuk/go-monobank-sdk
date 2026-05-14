package business

import (
	"encoding/json"
	"math"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
	"github.com/vtopc/epoch"
)

// DocumentType is the kind of ID document a salary contact / recipient can
// be identified by when an ІПН (РНОКПП) is not provided.
type DocumentType string

// Possible DocumentType values.
const (
	OldPassport     DocumentType = "OLD_PASSPORT"
	IDCard          DocumentType = "ID_CARD"
	ForeignPassport DocumentType = "FOREIGN_PASSPORT"
)

// Account is one of the company's IBAN-addressed accounts. Currency is the
// ISO-4217 numeric code (980 = UAH, 840 = USD, 978 = EUR). Balance is in
// units of that currency (not coins) — corp-api represents money as a
// decimal number, so this is float64. Use [Account.BalanceMoney] for a
// typed [money.Money] (round-tripped через множення на 100).
type Account struct {
	IBAN     string  `json:"iban"`
	Currency int     `json:"currency"`
	Balance  float64 `json:"balance"`
}

// BalanceMoney повертає Balance як типізовану [money.Money] (мінорні
// одиниці, з прив'язкою валюти). Округлення — до найближчої копійки
// (припускає 2 десяткових місця, що валідно для UAH/USD/EUR).
func (a Account) BalanceMoney() money.Money {
	return money.New(int64(math.Round(a.Balance*100)), currency.Code(a.Currency))
}

// BalancePoint is a single day's balance from the account-history series.
type BalancePoint struct {
	Date    string  `json:"date"` // YYYY-MM-DD
	Balance float64 `json:"balance"`
	IsFinal bool    `json:"isFinal"`
}

// Money повертає Balance як [money.Money]. currencyCode цього BalancePoint
// успадковується від батьківського запиту [Client.AccountBalances] —
// передай його окремо (з Account.Currency).
func (b BalancePoint) Money(code currency.Code) money.Money {
	return money.New(int64(math.Round(b.Balance*100)), code)
}

// Contact is a row from the salary-contacts directory.
type Contact struct {
	ID             string       `json:"id"` // UUID
	LEClientID     int64        `json:"leClientID"`
	FullName       string       `json:"fullName"`
	INN            string       `json:"inn"`
	DocumentType   DocumentType `json:"documentType,omitempty"`
	DocumentNumber string       `json:"documentNumber,omitempty"`
	DocumentSeries string       `json:"documentSeries,omitempty"`
	IBAN           string       `json:"iban"`
	PAN            string       `json:"pan,omitempty"`
}

// ContactsPage is a paginated result of [Client.Contacts] / [Client.SearchContacts].
type ContactsPage struct {
	HasMore  bool      `json:"hasMore"`
	Contacts []Contact `json:"contacts"`
}

// CreateContactRequest is the body of POST /ext/v1/salary-contacts.
// Either INN or (DocumentType + DocumentNumber) must be provided.
type CreateContactRequest struct {
	FirstName      string       `json:"firstName,omitempty"`
	LastName       string       `json:"lastName,omitempty"`
	MiddleName     string       `json:"middleName,omitempty"`
	INN            string       `json:"inn,omitempty"`
	DocumentType   DocumentType `json:"documentType,omitempty"`
	DocumentNumber string       `json:"documentNumber,omitempty"`
	DocumentSeries string       `json:"documentSeries,omitempty"`
	IBAN           string       `json:"iban,omitempty"`
	PAN            string       `json:"pan,omitempty"`
}

// SalaryRecipient is one row in a salary registry.
type SalaryRecipient struct {
	FullName       string       `json:"fullName"`
	INN            string       `json:"inn,omitempty"`
	DocumentType   DocumentType `json:"documentType,omitempty"`
	DocumentNumber string       `json:"documentNumber,omitempty"`
	DocumentSeries string       `json:"documentSeries,omitempty"`
	IBAN           string       `json:"iban"`
	// Amount is in minor units (kopecks for UAH).
	Amount int64 `json:"amount"`
}

// CreateSalaryRegistryRequest is the body of POST /ext/v1/payments/salary/registries.
type CreateSalaryRegistryRequest struct {
	RegistryName       string            `json:"registryName"`
	SenderIBAN         string            `json:"senderIban"`
	SalaryRegistryType string            `json:"salaryRegistryType"`
	From               string            `json:"from"` // YYYY-MM-DD
	To                 string            `json:"to"`   // YYYY-MM-DD
	Recipients         []SalaryRecipient `json:"recipients"`
}

// SalaryRegistryCreated is the response to POST /ext/v1/payments/salary/registries.
type SalaryRegistryCreated struct {
	ID              string `json:"id"`
	State           string `json:"state"`
	CreateTimestamp string `json:"createTimestamp"` // ISO-8601
}

// SalaryRegistryType is one alias from /payments/salary/registries/types.
type SalaryRegistryType struct {
	Alias       string `json:"alias"`
	Description string `json:"description"`
}

// RegistryStatus is the lifecycle state of a salary registry.
type RegistryStatus string

// Possible RegistryStatus values.
const (
	RegistrySaved          RegistryStatus = "SAVED"
	RegistryInProgress     RegistryStatus = "IN_PROGRESS"
	RegistryPrepared       RegistryStatus = "PREPARED"
	RegistryChangeRequired RegistryStatus = "CHANGE_REQUIRED"
	RegistryFail           RegistryStatus = "FAIL"
	RegistryPaymentsDone   RegistryStatus = "PAYMENTS_DONE"
)

// SalaryRegistryStatus is the response to GET /ext/v1/payments/salary/registries/{id}/status.
type SalaryRegistryStatus struct {
	Status        RegistryStatus `json:"status"`
	UpdatedAt     string         `json:"updatedAt"` // ISO-8601
	DeclineReason string         `json:"declineReason,omitempty"`
}

// OperationStatus is the per-operation state reported by /ext/v1/statement.
type OperationStatus string

// Possible OperationStatus values.
const (
	OperationDone     OperationStatus = "DONE"
	OperationPending  OperationStatus = "PENDING"
	OperationDeclined OperationStatus = "DECLINED"
)

// StatementItem is one operation in an account's statement.
type StatementItem struct {
	ID                string          `json:"id"`
	ExternalReference string          `json:"externalReference,omitempty"`
	Time              epoch.Seconds   `json:"time"`
	CompletedTime     epoch.Seconds   `json:"completedTime,omitempty"`
	Description       string          `json:"description"`
	Amount            money.Money     `json:"amount"`
	// CurrencyAlpha3 — валюта операції у форматі ISO-4217 alpha-3
	// (наприклад, "UAH"). На відміну від [bank.Account.Currency] чи
	// [acquiring.InvoiceStatusResponse.Currency], тут wire-формат
	// рядковий — corp-api саме так шле currencyCode у виписці. Для
	// типізованого порівняння конвертуй через [currency.FromAlpha3]
	// (UnmarshalJSON робить це автоматично для Amount.Code).
	CurrencyAlpha3 string          `json:"currencyCode"`
	ReceiptID      string          `json:"receiptId,omitempty"`
	CounterEdrpou     string          `json:"counterEdrpou,omitempty"`
	CounterIBAN       string          `json:"counterIban,omitempty"`
	CounterName       string          `json:"counterName,omitempty"`
	Reverse           bool            `json:"reverse,omitempty"`
	Status            OperationStatus `json:"status"`
}

// UnmarshalJSON прив'язує currency.Code до Amount, конвертуючи з
// alpha-3 рядка (`"UAH"`) у числовий код. Для невідомих валют Code
// лишається нульовим (Amount.Minor усе одно коректно прочитується).
func (s *StatementItem) UnmarshalJSON(data []byte) error {
	type raw StatementItem
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*s = StatementItem(r)
	if c, ok := currency.FromAlpha3(s.CurrencyAlpha3); ok {
		s.Amount.Code = c
	}
	return nil
}

// StatementDirection is the iteration direction passed to [Client.Statement].
// UP returns items chronologically newer than `from`; DOWN — older.
type StatementDirection string

// Possible StatementDirection values.
const (
	StatementUp   StatementDirection = "UP"
	StatementDown StatementDirection = "DOWN"
)

// PaymentReceiver is the destination side of a payment.
type PaymentReceiver struct {
	IBAN   string `json:"iban"`
	EDRPOU string `json:"edrpou"`
	Name   string `json:"name"`
}

// PaymentRequest is the body of POST /ext/v1/payment/prepare.
type PaymentRequest struct {
	SenderIBAN        string          `json:"senderIban"`
	Receiver          PaymentReceiver `json:"receiver"`
	Destination       string          `json:"destination"`
	Amount            int64           `json:"amount"` // minor units
	Currency          string          `json:"currency"`
	PayCode           string          `json:"payCode,omitempty"`
	AdditionalInfo    string          `json:"additionalInfo,omitempty"`
	ExternalReference string          `json:"externalReference,omitempty"`
}

// PaymentPrepared is the response to POST /ext/v1/payment/prepare.
type PaymentPrepared struct {
	ID string `json:"id"`
}

// PaymentStateValue is the lifecycle state of a prepared payment.
type PaymentStateValue string

// Possible PaymentStateValue values.
const (
	PaymentDraft       PaymentStateValue = "DRAFT"
	PaymentDeclined    PaymentStateValue = "DECLINED"
	PaymentInStatement PaymentStateValue = "IN_STATEMENT"
)

// PaymentState is the response to GET /ext/v1/payment/{id}/state and
// GET /ext/v1/payment/state.
type PaymentState struct {
	ID    string            `json:"id"`
	State PaymentStateValue `json:"state"`
}

// BatchAttribute is one attribute (line item) on an employee's payslip.
type BatchAttribute struct {
	AttributeName   string `json:"attributeName"`
	Value           string `json:"value"`
	SortOrder       int    `json:"sortOrder"`
	AttributeSuffix string `json:"attributeSuffix,omitempty"`
}

// BatchEmployee is one employee in a payslip batch.
type BatchEmployee struct {
	Identification string           `json:"identification"`
	Attributes     []BatchAttribute `json:"attributes"`
}

// BatchPayslipRequest is the body of POST /ext/v1/payslips/batch.
type BatchPayslipRequest struct {
	Period    string          `json:"period"` // YYYY-MM
	Employees []BatchEmployee `json:"employees"`
}

// DeletePayslipsRequest is the body of DELETE /ext/v1/payslips/batch.
type DeletePayslipsRequest struct {
	Period          string   `json:"period"` // YYYY-MM
	Identifications []string `json:"identifications"`
}

// BatchStats summarises a single payslip upload batch.
type BatchStats struct {
	EmployeesInBatch int `json:"employeesInBatch"`
	SuccessInBatch   int `json:"successInBatch"`
	FailedInBatch    int `json:"failedInBatch"`
}

// OverallStats summarises everything uploaded for a period.
type OverallStats struct {
	TotalEmployees        int `json:"totalEmployees"`
	TotalSuccessEmployees int `json:"totalSuccessEmployees"`
	TotalFailedEmployees  int `json:"totalFailedEmployees"`
}

// FailedEmployeeReason enumerates per-row failure causes.
type FailedEmployeeReason string

// Possible FailedEmployeeReason values.
const (
	ContactNotFound FailedEmployeeReason = "CONTACT_NOT_FOUND"
)

// FailedEmployee is one row that didn't make it into a payslip batch.
type FailedEmployee struct {
	Identification string               `json:"identification"`
	Reason         FailedEmployeeReason `json:"reason"`
}

// BatchPayslipResponse is the inner result of POST /ext/v1/payslips/batch.
// Mono wraps it in `{"result": ...}` on the wire; methods on Client unwrap it.
type BatchPayslipResponse struct {
	Period          string           `json:"period"`
	Status          string           `json:"status"` // always "LOADED"
	BatchStats      BatchStats       `json:"batchStats"`
	OverallStats    OverallStats     `json:"overallStats"`
	FailedEmployees []FailedEmployee `json:"failedEmployees"`
	CreatedAt       string           `json:"createdAt"`
	UpdatedAt       string           `json:"updatedAt"`
}

// ImportStatus is the lifecycle state of a payslip import for a period.
type ImportStatus string

// Possible ImportStatus values.
const (
	ImportLoading ImportStatus = "LOADING"
	ImportLoaded  ImportStatus = "LOADED"
	ImportFailed  ImportStatus = "FAILED"
	ImportSent    ImportStatus = "SENT"
	ImportDeleted ImportStatus = "DELETED"
)

// ImportStatusResponse is the inner result of GET /ext/v1/payslip-imports/status.
type ImportStatusResponse struct {
	Period                string           `json:"period"`
	Status                ImportStatus     `json:"status"`
	TotalEmployees        int              `json:"totalEmployees"`
	TotalSuccessEmployees int              `json:"totalSuccessEmployees"`
	TotalFailedEmployees  int              `json:"totalFailedEmployees"`
	FailedEmployees       []FailedEmployee `json:"failedEmployees"`
	CreatedAt             string           `json:"createdAt"`
	UpdatedAt             string           `json:"updatedAt"`
}

// SendResult is the inner result of POST /ext/v1/payslip-imports/send.
type SendResult struct {
	Period        string `json:"period"`
	Status        string `json:"status"` // always "SENT"
	EmployeesSent int    `json:"employeesSent"`
}

// resultWrapper unwraps mono's `{"result": ...}` envelope used by
// /ext/v1/payslips/* and /ext/v1/payslip-imports/* responses.
type resultWrapper[T any] struct {
	Result T `json:"result"`
}
