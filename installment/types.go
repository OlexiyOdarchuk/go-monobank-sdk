package installment

import "log/slog"

// OrderState is the overall state of an order (the state field in
// the callback and in [OrderStateInfo]).
type OrderState string

// Possible OrderState values.
const (
	StateSuccess   OrderState = "SUCCESS"
	StateFail      OrderState = "FAIL"
	StateInProcess OrderState = "IN_PROCESS"
)

// OrderSubState is the detailed state of an order.
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

// InvoiceSource is the sales channel for CreateOrderRequest.Invoice.
type InvoiceSource string

// Possible InvoiceSource values.
const (
	SourceInternet InvoiceSource = "INTERNET"
	SourceStore    InvoiceSource = "STORE"
)

// CreateOrderRequest is the body of POST /api/order/create.
type CreateOrderRequest struct {
	StoreOrderID                 string                        `json:"store_order_id"`
	ClientPhone                  string                        `json:"client_phone"`
	TotalSum                     Money                         `json:"total_sum"`
	Invoice                      CreateOrderInvoice            `json:"invoice"`
	AvailablePrograms            []Program                     `json:"available_programs"`
	Products                     []Product                     `json:"products"`
	ResultCallback               string                        `json:"result_callback,omitempty"`
	AdditionalParams             *CreateAdditionalParams       `json:"additional_params,omitempty"`
	FinancialCompanyMerchantInfo *FinancialCompanyMerchantInfo `json:"financial_company_merchant_info,omitempty"`
}

// CreateOrderInvoice is the invoice that travels with the order.
// Source is the sales channel (INTERNET or STORE).
type CreateOrderInvoice struct {
	Number  string        `json:"number"`
	Date    string        `json:"date"`
	Source  InvoiceSource `json:"source"`
	PointID string        `json:"point_id,omitempty"`
}

// Program is one available installment program: a list of the
// number-of-parts options (3 to 25 inclusive).
type Program struct {
	Type                string `json:"type,omitempty"` // deprecated
	AvailablePartsCount []int  `json:"available_parts_count"`
}

// Product is one item in the order.
type Product struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Sum   Money  `json:"sum"`
}

// CreateAdditionalParams holds optional parameters of the create
// request.
type CreateAdditionalParams struct {
	SellerPhone   string `json:"seller_phone,omitempty"`
	NDS           Money  `json:"nds,omitempty"`
	ExtInitialSum Money  `json:"ext_initial_sum,omitempty"`
}

// FinancialCompanyMerchantInfo holds the merchant's details for the
// financial company.
type FinancialCompanyMerchantInfo struct {
	StoreName   string `json:"store_name,omitempty"`
	EDRPOUCode  string `json:"edrpou_code,omitempty"`
	IBANAccount string `json:"iban_account,omitempty"`
}

// CreateOrderResponse is the response of /api/order/create.
type CreateOrderResponse struct {
	OrderID string `json:"order_id"`
}

// RequestWithOrderIdentifier is the shared body for endpoints that
// operate on order_id (state/info/data/confirm/reject/check_paid).
type RequestWithOrderIdentifier struct {
	OrderID string `json:"order_id"`
}

// OrderStateInfo is the response of /api/order/state, /confirm,
// /reject.
type OrderStateInfo struct {
	OrderID       string        `json:"order_id"`
	State         OrderState    `json:"state"`
	OrderSubState OrderSubState `json:"order_sub_state"`
	Message       string        `json:"message,omitempty"`
}

// OrderShortInfo is the response of /api/order/info (deprecated) and
// /api/order/data.
type OrderShortInfo struct {
	TotalSum        Money         `json:"total_sum"`
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

// Reverse is a single refund in the OrderShortInfo.ReverseList.
type Reverse struct {
	Sum       Money  `json:"sum"`
	Timestamp string `json:"timestamp"`
}

// CheckInstallmentsResponse is the response of /api/order/check/paid.
type CheckInstallmentsResponse struct {
	FullyPaid                bool `json:"fully_paid"`
	BankCanReturnMoneyToCard bool `json:"bank_can_return_money_to_card"`
}

// ReturnRequest is the body of /api/order/return.
type ReturnRequest struct {
	OrderID           string                  `json:"order_id"`
	Sum               Money                   `json:"sum"`
	StoreReturnID     string                  `json:"store_return_id"`
	ReturnMoneyToCard bool                    `json:"return_money_to_card"`
	AdditionalParams  *ReturnAdditionalParams `json:"additional_params,omitempty"`
}

// ReturnAdditionalParams holds optional return parameters.
type ReturnAdditionalParams struct {
	NDS Money `json:"nds,omitempty"`
}

// ReturnResponse is the response of /api/order/return.
type ReturnResponse struct {
	Status string `json:"status"`
}

// ValidateClientRequest is the body of /api/client/validate and
// /api/v2/client/validate.
type ValidateClientRequest struct {
	Phone string `json:"phone"`
}

// ValidateClientResponse is the response of /api/client/validate
// (legacy). It carries the full client info when found.
type ValidateClientResponse struct {
	Found  bool        `json:"found"`
	Client *ClientInfo `json:"client,omitempty"`
}

// ClientInfo holds the basic client info (for the legacy
// /api/client/validate). Not to be confused with [Client] — that is
// the SDK client itself.
type ClientInfo struct {
	FirstName  string `json:"first_name,omitempty"`
	LastName   string `json:"last_name,omitempty"`
	MiddleName string `json:"middle_name,omitempty"`
	INN        string `json:"inn,omitempty"`
}

// LogValue redacts the personal data on a ClientInfo so a
// Debug-level slog.Info call does not leak full name / INN into
// the log aggregator. The first letter of each name is kept as a
// visual anchor; the INN keeps only its last 4 digits.
func (c ClientInfo) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("first_name", maskNameInitial(c.FirstName)),
		slog.String("last_name", maskNameInitial(c.LastName)),
		slog.String("middle_name", maskNameInitial(c.MiddleName)),
		slog.String("inn", maskINN(c.INN)),
	)
}

func maskNameInitial(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) == 1 {
		return "*"
	}
	return string(r[0]) + "***"
}

func maskINN(inn string) string {
	if len(inn) <= 4 {
		return "***"
	}
	return "***" + inn[len(inn)-4:]
}

// ValidateClientSimpleResponse is the response of
// /api/v2/client/validate (the new endpoint, only the found flag).
type ValidateClientSimpleResponse struct {
	Found bool `json:"found"`
}

// DailyReportRequest is the body of /api/store/report.
type DailyReportRequest struct {
	Date string `json:"date"`
}

// DailyReportResponse is the response of /api/store/report.
type DailyReportResponse struct {
	Orders []ReportOrder `json:"orders"`
}

// ReportOrder is a single operation in the daily report.
type ReportOrder struct {
	OrderID            string  `json:"order_id"`
	InvoiceNumber      string  `json:"invoice_number"`
	OrderDate          string  `json:"order_date"`
	PayParts           int     `json:"pay_parts"`
	CommissionPercent  float64 `json:"commission_percent"` // % rate, not money
	TotalSum           Money   `json:"total_sum"`
	TransferredSum     Money   `json:"transferred_sum"`
	Commission         Money   `json:"commission"`
	OperationTimestamp string  `json:"operation_timestamp"`
	ODBContractNumber  string  `json:"odb_contract_number"`
}

// OrderDataRequest is the body of
// /api/order/(v2/)data/for/guarantee/letter and
// /api/order/guarantee/letter.
type OrderDataRequest struct {
	OrderID string                   `json:"order_id"`
	Invoice *OrderDataRequestInvoice `json:"invoice,omitempty"`
}

// OrderDataRequestInvoice carries the invoice details added to the
// guarantee-letter request.
type OrderDataRequestInvoice struct {
	Number string `json:"number,omitempty"`
	Date   string `json:"date,omitempty"`
}

// OrderData is the JSON response of
// /api/order/data/for/guarantee/letter (+v2), i.e. the structured
// data for client-side generation of the guarantee letter.
type OrderData struct {
	Header    Header    `json:"header"`
	Expansion Expansion `json:"expansion"`
}

// Header is the header of the guarantee letter.
type Header struct {
	RequestID        string `json:"request_id"`
	AnswerDatetime   string `json:"answer_datetime,omitempty"`
	FromOrganization string `json:"from_organization,omitempty"`
	OrganizationID   string `json:"organization_id,omitempty"`
	ContractNumber   string `json:"contract_number,omitempty"`
	ContractDate     string `json:"contract_date,omitempty"`
}

// Expansion is the main block of the guarantee letter.
type Expansion struct {
	Customer           *Customer `json:"customer,omitempty"`
	Invoice            *Invoice  `json:"invoice,omitempty"`
	PaymentDestination *Payment  `json:"payment_destination,omitempty"`
	Bank               *Bank     `json:"bank,omitempty"`
	Sign               string    `json:"sign,omitempty"`
	Stamp              string    `json:"stamp,omitempty"`
}

// Customer is the client in the guarantee letter.
type Customer struct {
	FirstName  string           `json:"first_name,omitempty"`
	LastName   string           `json:"last_name,omitempty"`
	MiddleName string           `json:"middle_name,omitempty"`
	INN        string           `json:"inn,omitempty"`
	Document   *ClientDocuments `json:"document,omitempty"`
}

// ClientDocuments holds the full set of client document types; in
// practice only one of them is populated.
type ClientDocuments struct {
	Passport              *PassportOrResidencePermitDocument `json:"passport,omitempty"`
	IDCard                *IDCardDocument                    `json:"id_card,omitempty"`
	ResidencePermit       *PassportOrResidencePermitDocument `json:"residence_permit,omitempty"`
	InternationalPassport *InternationalPassport             `json:"international_passport,omitempty"`
}

// PassportOrResidencePermitDocument is a passport or residence
// permit.
type PassportOrResidencePermitDocument struct {
	Number      string `json:"number,omitempty"`
	Issued      string `json:"issued,omitempty"`
	DateOfIssue string `json:"date_of_issue,omitempty"`
	Series      string `json:"series,omitempty"`
}

// IDCardDocument is an ID card (the modern passport format).
type IDCardDocument struct {
	Number         string `json:"number,omitempty"`
	Issued         string `json:"issued,omitempty"`
	DateOfIssue    string `json:"date_of_issue,omitempty"`
	ValidUntil     string `json:"valid_until,omitempty"`
	RegistryNumber string `json:"registry_number,omitempty"`
}

// InternationalPassport is an international (foreign-travel)
// passport.
type InternationalPassport struct {
	Number      string `json:"number,omitempty"`
	DateOfIssue string `json:"date_of_issue,omitempty"`
	Series      string `json:"series,omitempty"`
	ValidUntil  string `json:"valid_until,omitempty"`
}

// Invoice is the invoice in the guarantee letter.
type Invoice struct {
	InvoiceNumber string `json:"invoice_number,omitempty"`
	InvoiceDate   string `json:"invoice_date,omitempty"`
	InvoiceAmount Money  `json:"invoice_amount,omitempty"`
}

// Payment holds the recipient's payment details.
type Payment struct {
	DestID       string `json:"dest_id,omitempty"`
	DestName     string `json:"dest_name,omitempty"`
	DestMFO      string `json:"dest_mfo,omitempty"`
	DestBankName string `json:"dest_bank_name,omitempty"`
	DestAccNo    string `json:"dest_acc_number,omitempty"`
}

// Bank holds the creditor-bank information.
type Bank struct {
	Agreement           string `json:"agreement,omitempty"`
	AgreementDate       string `json:"agreement_date,omitempty"`
	CreditAmount        Money  `json:"credit_amount,omitempty"`
	CustomerPayAmount   Money  `json:"customer_pay_amount,omitempty"`
	ProductTypes        string  `json:"product_types,omitempty"`
	BankID              string  `json:"bank_id,omitempty"`
	BankName            string  `json:"bank_name,omitempty"`
	BankExecutive       string  `json:"bank_executive,omitempty"`
	AvailablePartsCount int     `json:"available_parts_count,omitempty"`
	CreditProduct       string  `json:"credit_product,omitempty"`
}
