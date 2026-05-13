package installment

// OrderState — загальний стан заявки (поле state у callback та
// [OrderStateInfo]).
type OrderState string

// Possible OrderState values.
const (
	StateSuccess   OrderState = "SUCCESS"
	StateFail      OrderState = "FAIL"
	StateInProcess OrderState = "IN_PROCESS"
)

// OrderSubState — деталізований стан заявки.
type OrderSubState string

// Possible OrderSubState values.
const (
	// Success branch.
	SubActive   OrderSubState = "ACTIVE"
	SubDone     OrderSubState = "DONE"
	SubReturned OrderSubState = "RETURNED"

	// IN_PROCESS branch.
	SubWaitingForClient       OrderSubState = "WAITING_FOR_CLIENT"
	SubWaitingForStoreConfirm OrderSubState = "WAITING_FOR_STORE_CONFIRM"

	// FAIL branch.
	SubClientNotFound           OrderSubState = "CLIENT_NOT_FOUND"
	SubExceededSumLimit         OrderSubState = "EXCEEDED_SUM_LIMIT"
	SubPayPartsAreNotAcceptable OrderSubState = "PAY_PARTS_ARE_NOT_ACCEPTABLE"
	SubExistsOtherOpenOrder     OrderSubState = "EXISTS_OTHER_OPEN_ORDER"
	SubNotEnoughMoneyForDebit   OrderSubState = "NOT_ENOUGH_MONEY_FOR_INIT_DEBIT"
	SubClientPushTimeout        OrderSubState = "CLIENT_PUSH_TIMEOUT"
	SubFraudRejected            OrderSubState = "FRAUD_REJECTED"
	SubRejectedByClient         OrderSubState = "REJECTED_BY_CLIENT"
	SubRejectedByStore          OrderSubState = "REJECTED_BY_STORE"
	SubFail                     OrderSubState = "FAIL"
	SubRestrictedByRisks        OrderSubState = "RESTRICTED_BY_RISKS"
)

// InvoiceSource — канал продажу для CreateOrderRequest.Invoice.
type InvoiceSource string

// Possible InvoiceSource values.
const (
	SourceInternet InvoiceSource = "INTERNET"
	SourceStore    InvoiceSource = "STORE"
)

// CreateOrderRequest — тіло POST /api/order/create.
type CreateOrderRequest struct {
	StoreOrderID                 string                        `json:"store_order_id"`
	ClientPhone                  string                        `json:"client_phone"`
	TotalSum                     float64                       `json:"total_sum"`
	Invoice                      CreateOrderInvoice            `json:"invoice"`
	AvailablePrograms            []Program                     `json:"available_programs"`
	Products                     []Product                     `json:"products"`
	ResultCallback               string                        `json:"result_callback,omitempty"`
	AdditionalParams             *CreateAdditionalParams       `json:"additional_params,omitempty"`
	FinancialCompanyMerchantInfo *FinancialCompanyMerchantInfo `json:"financial_company_merchant_info,omitempty"`
}

// CreateOrderInvoice — рахунок-фактура, що йде з заявкою.
// Source — канал продажу (INTERNET або STORE).
type CreateOrderInvoice struct {
	Number  string        `json:"number"`
	Date    string        `json:"date"`
	Source  InvoiceSource `json:"source"`
	PointID string        `json:"point_id,omitempty"`
}

// Program — одна доступна програма ПЧ: список варіантів кількості частин
// (від 3 до 25 включно).
type Program struct {
	Type                string `json:"type,omitempty"` // deprecated
	AvailablePartsCount []int  `json:"available_parts_count"`
}

// Product — товар у заявці.
type Product struct {
	Name  string  `json:"name"`
	Count int     `json:"count"`
	Sum   float64 `json:"sum"`
}

// CreateAdditionalParams — необов'язкові параметри create-заявки.
type CreateAdditionalParams struct {
	SellerPhone   string  `json:"seller_phone,omitempty"`
	NDS           float64 `json:"nds,omitempty"`
	ExtInitialSum float64 `json:"ext_initial_sum,omitempty"`
}

// FinancialCompanyMerchantInfo — реквізити магазину для фінкомпанії.
type FinancialCompanyMerchantInfo struct {
	StoreName   string `json:"store_name,omitempty"`
	EDRPOUCode  string `json:"edrpou_code,omitempty"`
	IBANAccount string `json:"iban_account,omitempty"`
}

// CreateOrderResponse — відповідь /api/order/create.
type CreateOrderResponse struct {
	OrderID string `json:"order_id"`
}

// RequestWithOrderIdentifier — спільне тіло для endpoint-ів, що
// працюють по order_id (state/info/data/confirm/reject/check_paid).
type RequestWithOrderIdentifier struct {
	OrderID string `json:"order_id"`
}

// OrderStateInfo — відповідь /api/order/state, /confirm, /reject.
type OrderStateInfo struct {
	OrderID       string        `json:"order_id"`
	State         OrderState    `json:"state"`
	OrderSubState OrderSubState `json:"order_sub_state"`
	Message       string        `json:"message,omitempty"`
}

// OrderShortInfo — відповідь /api/order/info (deprecated) та /api/order/data.
type OrderShortInfo struct {
	TotalSum        float64       `json:"total_sum"`
	Source          InvoiceSource `json:"source,omitempty"`
	InvoiceNumber   string        `json:"invoice_number,omitempty"`
	InvoiceDate     string        `json:"invoice_date,omitempty"`
	PointID         string        `json:"point_id,omitempty"`
	StoreOrderID    string        `json:"store_order_id,omitempty"`
	CreateTimestamp string        `json:"create_timestamp,omitempty"`
	ReverseList     []Reverse     `json:"reverse_list,omitempty"`
	MaskedCard      string        `json:"maskedCard,omitempty"`
	IBAN            string        `json:"iban,omitempty"`
}

// Reverse — одне повернення у списку OrderShortInfo.ReverseList.
type Reverse struct {
	Sum       float64 `json:"sum"`
	Timestamp string  `json:"timestamp"`
}

// CheckInstallmentsResponse — відповідь /api/order/check/paid.
type CheckInstallmentsResponse struct {
	FullyPaid                bool `json:"fully_paid"`
	BankCanReturnMoneyToCard bool `json:"bank_can_return_money_to_card"`
}

// ReturnRequest — тіло /api/order/return.
type ReturnRequest struct {
	OrderID           string                  `json:"order_id"`
	Sum               float64                 `json:"sum"`
	StoreReturnID     string                  `json:"store_return_id"`
	ReturnMoneyToCard bool                    `json:"return_money_to_card"`
	AdditionalParams  *ReturnAdditionalParams `json:"additional_params,omitempty"`
}

// ReturnAdditionalParams — необов'язкові параметри повернення.
type ReturnAdditionalParams struct {
	NDS float64 `json:"nds,omitempty"`
}

// ReturnResponse — відповідь /api/order/return.
type ReturnResponse struct {
	Status string `json:"status"`
}

// ValidateClientRequest — тіло /api/client/validate, /api/v2/client/validate.
type ValidateClientRequest struct {
	Phone string `json:"phone"`
}

// ValidateClientResponse — відповідь /api/client/validate (legacy).
// Містить повну інформацію про клієнта, якщо знайдено.
type ValidateClientResponse struct {
	Found  bool        `json:"found"`
	Client *ClientInfo `json:"client,omitempty"`
}

// ClientInfo — базова інформація про клієнта (для legacy /api/client/validate).
// Не плутати з [Client] — це сам SDK-клієнт.
type ClientInfo struct {
	FirstName  string `json:"first_name,omitempty"`
	LastName   string `json:"last_name,omitempty"`
	MiddleName string `json:"middle_name,omitempty"`
	INN        string `json:"inn,omitempty"`
}

// ValidateClientSimpleResponse — відповідь /api/v2/client/validate (новий,
// лише прапор found).
type ValidateClientSimpleResponse struct {
	Found bool `json:"found"`
}

// DailyReportRequest — тіло /api/store/report.
type DailyReportRequest struct {
	Date string `json:"date"`
}

// DailyReportResponse — відповідь /api/store/report.
type DailyReportResponse struct {
	Orders []ReportOrder `json:"orders"`
}

// ReportOrder — одна операція у денному звіті.
type ReportOrder struct {
	OrderID            string  `json:"order_id"`
	InvoiceNumber      string  `json:"invoice_number"`
	OrderDate          string  `json:"order_date"`
	PayParts           int     `json:"pay_parts"`
	CommissionPercent  float64 `json:"commission_percent"`
	TotalSum           float64 `json:"total_sum"`
	TransferredSum     float64 `json:"transferred_sum"`
	Commission         float64 `json:"commission"`
	OperationTimestamp string  `json:"operation_timestamp"`
	ODBContractNumber  string  `json:"odb_contract_number"`
}

// OrderDataRequest — тіло /api/order/(v2/)data/for/guarantee/letter та
// /api/order/guarantee/letter.
type OrderDataRequest struct {
	OrderID string                   `json:"order_id"`
	Invoice *OrderDataRequestInvoice `json:"invoice,omitempty"`
}

// OrderDataRequestInvoice — реквізити рахунку, які додаються до запиту
// на гарантійний лист.
type OrderDataRequestInvoice struct {
	Number string `json:"number,omitempty"`
	Date   string `json:"date,omitempty"`
}

// OrderData — JSON-відповідь /api/order/data/for/guarantee/letter (+v2),
// тобто структуровані дані для генерації гарантійного листа на стороні
// клієнта.
type OrderData struct {
	Header    Header    `json:"header"`
	Expansion Expansion `json:"expansion"`
}

// Header — заголовок гарантійного листа.
type Header struct {
	RequestID        string `json:"request_id"`
	AnswerDatetime   string `json:"answer_datetime,omitempty"`
	FromOrganization string `json:"from_organization,omitempty"`
	OrganizationID   string `json:"organization_id,omitempty"`
	ContractNumber   string `json:"contract_number,omitempty"`
	ContractDate     string `json:"contract_date,omitempty"`
}

// Expansion — основний блок гарантійного листа.
type Expansion struct {
	Customer           *Customer `json:"customer,omitempty"`
	Invoice            *Invoice  `json:"invoice,omitempty"`
	PaymentDestination *Payment  `json:"payment_destination,omitempty"`
	Bank               *Bank     `json:"bank,omitempty"`
	Sign               string    `json:"sign,omitempty"`
	Stamp              string    `json:"stamp,omitempty"`
}

// Customer — клієнт у гарантійному листі.
type Customer struct {
	FirstName  string           `json:"first_name,omitempty"`
	LastName   string           `json:"last_name,omitempty"`
	MiddleName string           `json:"middle_name,omitempty"`
	INN        string           `json:"inn,omitempty"`
	Document   *ClientDocuments `json:"document,omitempty"`
}

// ClientDocuments — оверкомплект документів клієнта; реально приходить
// один із них.
type ClientDocuments struct {
	Passport              *PassportOrResidencePermitDocument `json:"passport,omitempty"`
	IDCard                *IDCardDocument                    `json:"id_card,omitempty"`
	ResidencePermit       *PassportOrResidencePermitDocument `json:"residence_permit,omitempty"`
	InternationalPassport *InternationalPassport             `json:"international_passport,omitempty"`
}

// PassportOrResidencePermitDocument — паспорт або посвідка.
type PassportOrResidencePermitDocument struct {
	Number      string `json:"number,omitempty"`
	Issued      string `json:"issued,omitempty"`
	DateOfIssue string `json:"date_of_issue,omitempty"`
	Series      string `json:"series,omitempty"`
}

// IDCardDocument — ID-картка (паспорт нового зразка).
type IDCardDocument struct {
	Number         string `json:"number,omitempty"`
	Issued         string `json:"issued,omitempty"`
	DateOfIssue    string `json:"date_of_issue,omitempty"`
	ValidUntil     string `json:"valid_until,omitempty"`
	RegistryNumber string `json:"registry_number,omitempty"`
}

// InternationalPassport — закордонний паспорт.
type InternationalPassport struct {
	Number      string `json:"number,omitempty"`
	DateOfIssue string `json:"date_of_issue,omitempty"`
	Series      string `json:"series,omitempty"`
	ValidUntil  string `json:"valid_until,omitempty"`
}

// Invoice — рахунок-фактура у гарантійному листі.
type Invoice struct {
	InvoiceNumber string  `json:"invoice_number,omitempty"`
	InvoiceDate   string  `json:"invoice_date,omitempty"`
	InvoiceAmount float64 `json:"invoice_amount,omitempty"`
}

// Payment — реквізити отримувача коштів.
type Payment struct {
	DestID       string `json:"dest_id,omitempty"`
	DestName     string `json:"dest_name,omitempty"`
	DestMFO      string `json:"dest_mfo,omitempty"`
	DestBankName string `json:"dest_bank_name,omitempty"`
	DestAccNo    string `json:"dest_acc_number,omitempty"`
}

// Bank — інформація про банк-кредитор.
type Bank struct {
	Agreement           string  `json:"agreement,omitempty"`
	AgreementDate       string  `json:"agreement_date,omitempty"`
	CreditAmount        float64 `json:"credit_amount,omitempty"`
	CustomerPayAmount   float64 `json:"customer_pay_amount,omitempty"`
	ProductTypes        string  `json:"product_types,omitempty"`
	BankID              string  `json:"bank_id,omitempty"`
	BankName            string  `json:"bank_name,omitempty"`
	BankExecutive       string  `json:"bank_executive,omitempty"`
	AvailablePartsCount int     `json:"available_parts_count,omitempty"`
	CreditProduct       string  `json:"credit_product,omitempty"`
}
