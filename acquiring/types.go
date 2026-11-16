package acquiring

import (
	"encoding/json"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/money"
)

// MerchantDetails is the response to GET /api/merchant/details.
type MerchantDetails struct {
	MerchantID   string `json:"merchantId"`
	MerchantName string `json:"merchantName"`
	EDRPOU       string `json:"edrpou"`
}

// Employee is one row from /api/merchant/employee/list. The endpoint
// is described only in the older ReDoc spec
// (api.monobank.ua/docs/acquiring.html); monobank.ua/api-docs has no
// page for it.
type Employee struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	ExtRef string `json:"extRef"`
}

// EmployeeList is the wire shape of /api/merchant/employee/list.
type EmployeeList struct {
	List []Employee `json:"list"`
}

// ServerKey is the merchant-side pubkey returned by /api/merchant/pubkey.
// Use it to verify webhook signatures.
type ServerKey struct {
	Key string `json:"key"`
}

// QR is one row in /api/merchant/qr/list.
type QR struct {
	ShortQrID  string `json:"shortQrId"`
	QrID       string `json:"qrId"`
	AmountType string `json:"amountType"` // merchant | client | fix
	PageURL    string `json:"pageUrl"`
}

// QRList is the wire shape of /api/merchant/qr/list.
type QRList struct {
	List []QR `json:"list"`
}

// QRDetails is the response to /api/merchant/qr/details.
type QRDetails struct {
	ShortQrID string        `json:"shortQrId"`
	InvoiceID string        `json:"invoiceId,omitempty"`
	Amount    money.Money   `json:"amount,omitempty"`
	Currency  currency.Code `json:"ccy,omitempty"`
}

// UnmarshalJSON attaches Currency to Amount.Code.
func (q *QRDetails) UnmarshalJSON(data []byte) error {
	type raw QRDetails
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*q = QRDetails(r)
	q.Amount.Code = q.Currency
	return nil
}

// Submerchant is one row in /api/merchant/submerchant/list.
type Submerchant struct {
	Code   string `json:"code"`
	EDRPOU string `json:"edrpou,omitempty"`
	IBAN   string `json:"iban"`
	Owner  string `json:"owner,omitempty"`
}

// SubmerchantList is the wire shape of /api/merchant/submerchant/list.
type SubmerchantList struct {
	List []Submerchant `json:"list"`
}

// AdjustmentType — discount or extra charge.
type AdjustmentType string

// Possible AdjustmentType values.
const (
	AdjustmentDiscount    AdjustmentType = "DISCOUNT"
	AdjustmentExtraCharge AdjustmentType = "EXTRA_CHARGE"
)

// AdjustmentMode — percent or absolute value.
type AdjustmentMode string

// Possible AdjustmentMode values.
const (
	AdjustmentPercent AdjustmentMode = "PERCENT"
	AdjustmentValue   AdjustmentMode = "VALUE"
)

// Adjustment is one discount/extra-charge line attached to an invoice or
// basket item. Stored on the wire as `discounts` (the bank's term),
// renamed here because it encompasses both reductions and surcharges.
type Adjustment struct {
	Type  AdjustmentType `json:"type"`
	Mode  AdjustmentMode `json:"mode"`
	Value float64        `json:"value"`
	// Tax is the list of tax rates used for fiscalization. The docs are
	// ambiguous here: monobank.ua/api-docs types discounts[].tax as
	// "array of object" without an item schema, while basketOrder[].tax
	// is "array of integer"; the older ReDoc spec has no tax on
	// discounts at all. []int mirrors the basket tax array; if the
	// bank actually expects objects, a non-empty Tax here may be
	// rejected — leave it empty unless you have confirmed the format.
	Tax []int `json:"tax,omitempty"`
}

// BasketItem is one line in a basket order.
type BasketItem struct {
	Name      string       `json:"name"`
	Qty       float64      `json:"qty"`
	Sum       int64        `json:"sum"`
	Total     int64        `json:"total,omitempty"`
	Icon      string       `json:"icon,omitempty"`
	Unit      string       `json:"unit,omitempty"`
	Code      string       `json:"code"`
	Barcode   string       `json:"barcode,omitempty"`
	Header    string       `json:"header,omitempty"`
	Footer    string       `json:"footer,omitempty"`
	Tax       []int        `json:"tax,omitempty"`
	UKTZED    string       `json:"uktzed,omitempty"`
	Discounts []Adjustment `json:"discounts,omitempty"`
	// SplitReceiverID routes this line's funds to a registered split
	// receiver (see GET /api/merchant/split-receiver/list).
	SplitReceiverID string `json:"splitReceiverId,omitempty"`
}

// CancelItem is one line in a cancel/finalize request — a slimmer variant
// of [BasketItem] without basket-only metadata.
type CancelItem struct {
	Name    string  `json:"name"`
	Qty     float64 `json:"qty"`
	Sum     int64   `json:"sum"`
	Code    string  `json:"code"`
	Barcode string  `json:"barcode,omitempty"`
	Header  string  `json:"header,omitempty"`
	Footer  string  `json:"footer,omitempty"`
	Tax     []int   `json:"tax,omitempty"`
	UKTZED  string  `json:"uktzed,omitempty"`
	// SplitReceiverID names the split receiver the refunded line was
	// paid to. The split-scheme cancel docs mark it required for each
	// item ("при частковому поверненні … обовʼязково вказуємо: name,
	// qty, sum, code та splitReceiverId"); outside the split scheme
	// leave it empty. Not used by finalize.
	SplitReceiverID string `json:"splitReceiverId,omitempty"`
}

// MerchantPaymInfo is the marketing/checkout metadata attached to an invoice.
type MerchantPaymInfo struct {
	Reference      string       `json:"reference,omitempty"`
	Destination    string       `json:"destination,omitempty"`
	Comment        string       `json:"comment,omitempty"`
	CustomerEmails []string     `json:"customerEmails,omitempty"`
	Discounts      []Adjustment `json:"discounts,omitempty"`
	BasketOrder    []BasketItem `json:"basketOrder,omitempty"`
	// Metadata is additional order data. The docs say it "will be
	// available in the statement endpoint" ([StatementInvoice.Metadata]);
	// the invoice status response has no metadata field. The docs type
	// it only as "object" and say nothing about value types; this SDK
	// models it as string→string, so send numbers and booleans in their
	// string form.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PaymentType selects the operation type; the API default is debit.
//
//   - debit: capture immediately.
//   - hold: authorize now, capture later via [Client.FinalizeInvoice].
//     The hold lasts 9 days and is cancelled if not finalized by then.
//     Invoice create documents hold as working only with ccy 980.
//   - verification (invoice create only): checks the card without
//     moving funds. Amount must be 0 and SaveCardData.SaveCard must be
//     true.
//
// /wallet/payment accepts only debit and hold.
type PaymentType string

// Possible PaymentType values.
const (
	PaymentDebit        PaymentType = "debit"
	PaymentHold         PaymentType = "hold"
	PaymentVerification PaymentType = "verification"
)

// DisplayType controls how the checkout page is rendered. The only
// value the API currently defines is "iframe" (embeddable checkout);
// an empty value yields the default full-page checkout.
type DisplayType string

// Possible DisplayType values.
const (
	DisplayIframe DisplayType = "iframe"
)

// SaveCardData asks the bank to tokenize the card for future wallet
// payments. Tokenization is disabled by default; monobank support has
// to enable it for the merchant.
type SaveCardData struct {
	SaveCard bool `json:"saveCard"`
	// WalletID is the buyer's wallet identifier, generated by the
	// merchant's backend. The bank may not keep it: if the same card is
	// already tokenized (even under another walletId), the bank returns
	// the walletId of that first token and ignores this one. The
	// walletId in [WalletData] is temporary while its status is "new"
	// and final once it is "created" — persist that returned value and
	// use it for later invoices and [Client.Wallet], not your own.
	WalletID string `json:"walletId,omitempty"`
}

// CreateInvoiceRequest is the body of POST /api/merchant/invoice/create.
type CreateInvoiceRequest struct {
	Amount           int64             `json:"amount"`
	Currency         currency.Code     `json:"ccy,omitempty"`
	MerchantPaymInfo *MerchantPaymInfo `json:"merchantPaymInfo,omitempty"`
	// RedirectURL is the single return address used for both success
	// and failure when SuccessURL/FailURL are not set. When either of
	// those is present, it takes precedence for that outcome.
	RedirectURL string `json:"redirectUrl,omitempty"`
	// SuccessURL overrides RedirectURL for a successful payment. This
	// redirect is disabled by default; monobank support has to enable
	// it for the merchant.
	SuccessURL string `json:"successUrl,omitempty"`
	// FailURL overrides RedirectURL for a failed payment. Disabled by
	// default, like SuccessURL.
	FailURL         string        `json:"failUrl,omitempty"`
	WebHookURL      string        `json:"webHookUrl,omitempty"`
	Validity        int64         `json:"validity,omitempty"`
	PaymentType     PaymentType   `json:"paymentType,omitempty"`
	QrID            string        `json:"qrId,omitempty"`
	Code            string        `json:"code,omitempty"`
	SaveCardData    *SaveCardData `json:"saveCardData,omitempty"`
	AgentFeePercent float64       `json:"agentFeePercent,omitempty"`
	// TipsEmployeeID is described only in the older ReDoc spec
	// (api.monobank.ua/docs/acquiring.html); the invoice create pages
	// on monobank.ua/api-docs no longer list it.
	TipsEmployeeID string `json:"tipsEmployeeId,omitempty"`
	// DisplayType selects the checkout rendering (see [DisplayType]);
	// empty means the default full-page checkout.
	DisplayType DisplayType `json:"displayType,omitempty"`
	// WithAppURL asks the API to also return AppURL — a universal link
	// that opens the payment inside the monobank app. The docs say it
	// is not supported for QR payments or verification payments.
	WithAppURL bool `json:"withAppUrl,omitempty"`
}

// CreateInvoiceResponse is the response to POST /api/merchant/invoice/create.
type CreateInvoiceResponse struct {
	InvoiceID string `json:"invoiceId"`
	PageURL   string `json:"pageUrl"`
	// AppURL is a universal link ("https://mbnk.app/or/...") that opens
	// the payment inside the monobank app. (The official docs call it a
	// "monobank://" deep link, but both test and production tokens
	// return the mbnk.app form.) Only returned when the request set
	// WithAppURL.
	AppURL string `json:"appUrl,omitempty"`
}

// InvoiceStatus is the lifecycle state of an invoice.
type InvoiceStatus string

// Possible InvoiceStatus values.
const (
	InvoiceCreated    InvoiceStatus = "created"
	InvoiceProcessing InvoiceStatus = "processing"
	InvoiceHold       InvoiceStatus = "hold"
	InvoiceSuccess    InvoiceStatus = "success"
	InvoiceFailure    InvoiceStatus = "failure"
	InvoiceReversed   InvoiceStatus = "reversed"
	InvoiceExpired    InvoiceStatus = "expired"
)

// PaymentSystem — visa or mastercard.
type PaymentSystem string

// Possible PaymentSystem values.
const (
	Visa       PaymentSystem = "visa"
	Mastercard PaymentSystem = "mastercard"
)

// PaymentMethod is the channel used to pay an invoice.
type PaymentMethod string

// Possible PaymentMethod values.
const (
	MethodPAN      PaymentMethod = "pan"
	MethodApple    PaymentMethod = "apple"
	MethodGoogle   PaymentMethod = "google"
	MethodMonobank PaymentMethod = "monobank"
	MethodWallet   PaymentMethod = "wallet"
	MethodDirect   PaymentMethod = "direct"
)

// PaymentInfo is the card-side detail attached to a successful invoice.
// Fee and AgentFee are in minor units; the currency is inherited from
// the parent [InvoiceStatusResponse.Currency].
type PaymentInfo struct {
	MaskedPan     string        `json:"maskedPan"`
	ApprovalCode  string        `json:"approvalCode,omitempty"`
	RRN           string        `json:"rrn,omitempty"`
	TranID        string        `json:"tranId,omitempty"`
	Terminal      string        `json:"terminal"`
	Bank          string        `json:"bank,omitempty"`
	PaymentSystem PaymentSystem `json:"paymentSystem"`
	PaymentMethod PaymentMethod `json:"paymentMethod"`
	Fee           money.Money   `json:"fee,omitempty"`
	Country       string        `json:"country,omitempty"`
	AgentFee      money.Money   `json:"agentFee,omitempty"`
}

// WalletStatus is the tokenization state of a card.
type WalletStatus string

// Possible [WalletStatus] values.
const (
	WalletNew     WalletStatus = "new"
	WalletCreated WalletStatus = "created"
	WalletFailed  WalletStatus = "failed"
)

// WalletData is the tokenization state of a card following an invoice
// where SaveCardData.SaveCard was true.
type WalletData struct {
	CardToken string       `json:"cardToken"`
	WalletID  string       `json:"walletId"`
	Status    WalletStatus `json:"status"`
}

// TipsInfo is the tip amount/recipient on an invoice. Amount is in
// minor units of the invoice's currency (kopecks for UAH). int64
// avoids the 32-bit-platform overflow trap on the bare int type.
// Described only in the older ReDoc spec; the invoice status pages on
// monobank.ua/api-docs no longer list tipsInfo.
type TipsInfo struct {
	EmployeeID string `json:"employeeId"`
	Amount     int64  `json:"amount,omitempty"`
}

// ProcessingStatus is the state of an operation. One type covers
// several responses, but each documents its own subset:
//
//   - cancel: processing, success, failure;
//   - finalize: success only ("the finalize request was accepted");
//   - payment-direct and wallet/payment: processing, success, failure;
//   - statement rows: hold, processing, success, failure.
//
// sync-payment is not among them: it returns the invoice status shape
// ([InvoiceStatus]).
type ProcessingStatus string

// Possible [ProcessingStatus] values.
const (
	StatusProcessing ProcessingStatus = "processing"
	StatusSuccess    ProcessingStatus = "success"
	StatusFailure    ProcessingStatus = "failure"
	StatusHold       ProcessingStatus = "hold"
)

// CancelOp is one element in the cancel history of an invoice.
type CancelOp struct {
	Status       ProcessingStatus `json:"status"`
	Amount       money.Money      `json:"amount,omitempty"`
	Currency     currency.Code    `json:"ccy,omitempty"`
	CreatedDate  string           `json:"createdDate"`
	ModifiedDate string           `json:"modifiedDate"`
	ApprovalCode string           `json:"approvalCode,omitempty"`
	RRN          string           `json:"rrn,omitempty"`
	ExtRef       string           `json:"extRef,omitempty"`
}

// UnmarshalJSON attaches Currency to Amount.Code.
func (c *CancelOp) UnmarshalJSON(data []byte) error {
	type raw CancelOp
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*c = CancelOp(r)
	c.Amount.Code = c.Currency
	return nil
}

// InvoiceStatusResponse is the response to GET /api/merchant/invoice/status.
// The monetary fields are typed [money.Money]; Code is filled from
// Currency. CancelList[*].Amount and PaymentInfo.Fee/AgentFee also
// pick up Currency.
type InvoiceStatusResponse struct {
	InvoiceID     string        `json:"invoiceId"`
	Status        InvoiceStatus `json:"status"`
	FailureReason string        `json:"failureReason,omitempty"`
	ErrCode       string        `json:"errCode,omitempty"`
	Amount        money.Money   `json:"amount"`
	Currency      currency.Code `json:"ccy"`
	FinalAmount   money.Money   `json:"finalAmount,omitempty"`
	CreatedDate   string        `json:"createdDate,omitempty"`
	ModifiedDate  string        `json:"modifiedDate,omitempty"`
	Reference     string        `json:"reference,omitempty"`
	Destination   string        `json:"destination,omitempty"`
	CancelList    []CancelOp    `json:"cancelList,omitempty"`
	PaymentInfo   *PaymentInfo  `json:"paymentInfo,omitempty"`
	WalletData    *WalletData   `json:"walletData,omitempty"`
	TipsInfo      *TipsInfo     `json:"tipsInfo,omitempty"`
}

// UnmarshalJSON attaches Currency to every monetary field — Amount,
// FinalAmount, PaymentInfo.{Fee,AgentFee}. CancelList[i].Amount has
// its own Currency and is populated by its UnmarshalJSON.
func (i *InvoiceStatusResponse) UnmarshalJSON(data []byte) error {
	type raw InvoiceStatusResponse
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*i = InvoiceStatusResponse(r)
	c := i.Currency
	i.Amount.Code = c
	i.FinalAmount.Code = c
	if i.PaymentInfo != nil {
		i.PaymentInfo.Fee.Code = c
		i.PaymentInfo.AgentFee.Code = c
	}
	return nil
}

// CancelRequest is the body of POST /api/merchant/invoice/cancel.
type CancelRequest struct {
	InvoiceID string       `json:"invoiceId"`
	ExtRef    string       `json:"extRef,omitempty"`
	Amount    int64        `json:"amount,omitempty"`
	Items     []CancelItem `json:"items,omitempty"`
}

// CancelResponse is the response to POST /api/merchant/invoice/cancel.
type CancelResponse struct {
	Status       ProcessingStatus `json:"status"`
	CreatedDate  string           `json:"createdDate"`
	ModifiedDate string           `json:"modifiedDate"`
}

// FinalizeRequest is the body of POST /api/merchant/invoice/finalize.
type FinalizeRequest struct {
	InvoiceID string `json:"invoiceId"`
	// Amount is the amount to capture, in minor units. monobank.ua/
	// api-docs marks it required, while the older ReDoc spec makes it
	// optional (finalizing the full held amount). The SDK leaves a zero
	// Amount off the wire, which relies on the ReDoc behavior; set it
	// explicitly to follow the current docs.
	Amount int64        `json:"amount,omitempty"`
	Items  []CancelItem `json:"items,omitempty"`
}

// FinalizeResponse is the response to POST /api/merchant/invoice/finalize.
type FinalizeResponse struct {
	// Status is always StatusSuccess, which the docs define as "the
	// finalize request was accepted" — not that funds have settled.
	Status ProcessingStatus `json:"status"`
}

// RemoveRequest is the body of POST /api/merchant/invoice/remove.
type RemoveRequest struct {
	InvoiceID string `json:"invoiceId"`
}

// FiscalCheckType — sale or return.
type FiscalCheckType string

// Possible FiscalCheckType values.
const (
	FiscalSale   FiscalCheckType = "sale"
	FiscalReturn FiscalCheckType = "return"
)

// FiscalCheckStatus — lifecycle state of fiscalisation.
type FiscalCheckStatus string

// Possible FiscalCheckStatus values.
const (
	FiscalNew     FiscalCheckStatus = "new"
	FiscalProcess FiscalCheckStatus = "process"
	FiscalDone    FiscalCheckStatus = "done"
	FiscalFailed  FiscalCheckStatus = "failed"
)

// FiscalizationSource — which provider fiscalised the check.
type FiscalizationSource string

// Possible FiscalizationSource values.
const (
	SourceCheckbox    FiscalizationSource = "checkbox"
	SourceMonopay     FiscalizationSource = "monopay"
	SourceVchasnoKasa FiscalizationSource = "vchasnokasa"
)

// FiscalCheck is one fiscalised check tied to an invoice.
type FiscalCheck struct {
	ID                  string              `json:"id"`
	Type                FiscalCheckType     `json:"type"`
	Status              FiscalCheckStatus   `json:"status"`
	StatusDescription   string              `json:"statusDescription,omitempty"`
	TaxURL              string              `json:"taxUrl,omitempty"`
	File                string              `json:"file,omitempty"`
	FiscalizationSource FiscalizationSource `json:"fiscalizationSource"`
}

// FiscalChecksResponse is the response to GET /api/merchant/invoice/fiscal-checks.
type FiscalChecksResponse struct {
	Checks []FiscalCheck `json:"checks"`
}

// ReceiptResponse is the response to GET /api/merchant/invoice/receipt —
// File is a base64 PDF.
type ReceiptResponse struct {
	File string `json:"file"`
}

// CardData is the raw card details supplied with payment-direct.
type CardData struct {
	PAN string `json:"pan"`
	Exp string `json:"exp"`
	CVV string `json:"cvv"`
}

// InitiationKind — who initiated the payment (merchant or end-client).
type InitiationKind string

// Possible InitiationKind values.
const (
	InitMerchant InitiationKind = "merchant"
	InitClient   InitiationKind = "client"
)

// PaymentDirectRequest is the body of POST /api/merchant/invoice/payment-direct.
type PaymentDirectRequest struct {
	Amount           int64             `json:"amount"`
	Currency         currency.Code     `json:"ccy,omitempty"`
	CardData         CardData          `json:"cardData"`
	MerchantPaymInfo *MerchantPaymInfo `json:"merchantPaymInfo,omitempty"`
	WebHookURL       string            `json:"webHookUrl,omitempty"`
	PaymentType      PaymentType       `json:"paymentType,omitempty"`
	SaveCardData     *SaveCardData     `json:"saveCardData,omitempty"`
	RedirectURL      string            `json:"redirectUrl,omitempty"`
	InitiationKind   InitiationKind    `json:"initiationKind,omitempty"`
}

// PaymentDirectResponse is the response to /payment-direct.
type PaymentDirectResponse struct {
	InvoiceID     string           `json:"invoiceId"`
	TDSUrl        string           `json:"tdsUrl,omitempty"`
	Status        ProcessingStatus `json:"status"`
	FailureReason string           `json:"failureReason,omitempty"`
	Amount        money.Money      `json:"amount"`
	Currency      currency.Code    `json:"ccy"`
	CreatedDate   string           `json:"createdDate"`
	ModifiedDate  string           `json:"modifiedDate"`
}

// UnmarshalJSON attaches Currency to Amount.Code.
func (p *PaymentDirectResponse) UnmarshalJSON(data []byte) error {
	type raw PaymentDirectResponse
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*p = PaymentDirectResponse(r)
	p.Amount.Code = p.Currency
	return nil
}

// CardDataType is FPAN or DPAN.
type CardDataType string

// Possible CardDataType values.
const (
	FPAN CardDataType = "FPAN"
	DPAN CardDataType = "DPAN"
)

// SyncCardData is the wire card payload for /sync-payment.
type SyncCardData struct {
	PAN          string       `json:"pan"`
	Type         CardDataType `json:"type"`
	Exp          string       `json:"exp"`
	CVV          string       `json:"cvv,omitempty"`
	EciIndicator string       `json:"eciIndicator"`
	Cavv         string       `json:"cavv,omitempty"`
	Tavv         string       `json:"tavv,omitempty"`
	DsTranID     string       `json:"dsTranId,omitempty"`
	TReqID       string       `json:"tReqID,omitempty"`
	Mit          string       `json:"mit,omitempty"`
	Sst          float64      `json:"sst,omitempty"`
	Tid          string       `json:"tid,omitempty"`
}

// ApplePayPayload is the decrypted Apple Pay data for /sync-payment.
// The older ReDoc spec describes its fields as data taken from the
// Apple Pay crypto-container (DPAN token, ECI, cryptogram), i.e. after
// decryption — not the raw encrypted container. To pass the raw
// container instead, use [WalletPaymentRequest.AToken].
type ApplePayPayload struct {
	Token        string `json:"token"`
	Exp          string `json:"exp"`
	EciIndicator string `json:"eciIndicator"`
	Cryptogram   string `json:"cryptogram,omitempty"`
}

// GooglePayPayload is the decrypted Google Pay data for /sync-payment
// (DPAN token, ECI, cryptogram), as described in the older ReDoc spec.
// To pass the raw Google Pay token instead, use
// [WalletPaymentRequest.GToken].
type GooglePayPayload struct {
	Token        string `json:"token"`
	Exp          string `json:"exp"`
	EciIndicator string `json:"eciIndicator"`
	Cryptogram   string `json:"cryptogram,omitempty"`
}

// SyncPaymentRequest is the body of POST /api/merchant/invoice/sync-payment.
type SyncPaymentRequest struct {
	Amount           int64             `json:"amount"`
	Currency         currency.Code     `json:"ccy"`
	MerchantPaymInfo *MerchantPaymInfo `json:"merchantPaymInfo,omitempty"`
	CardData         *SyncCardData     `json:"cardData,omitempty"`
	ApplePay         *ApplePayPayload  `json:"applePay,omitempty"`
	GooglePay        *GooglePayPayload `json:"googlePay,omitempty"`
}

// SyncPaymentResponse mirrors InvoiceStatusResponse for the sync-payment case.
type SyncPaymentResponse = InvoiceStatusResponse

// ResetAmountRequest is the body of POST /api/merchant/qr/reset-amount.
type ResetAmountRequest struct {
	QrID string `json:"qrId"`
}

// PaymentScheme is how a paid invoice was funded — a single upfront
// payment or one of the "Pay in parts" (BNPL) schemes.
type PaymentScheme string

// Possible PaymentScheme values.
const (
	SchemeFull        PaymentScheme = "full"          // single upfront payment
	SchemeBNPLParts4  PaymentScheme = "bnpl_parts_4"  // split into 4 parts
	SchemeBNPLLater30 PaymentScheme = "bnpl_later_30" // deferred 30 days
)

// StatementInvoice is one row in /api/merchant/statement.
type StatementInvoice struct {
	InvoiceID     string           `json:"invoiceId"`
	Status        ProcessingStatus `json:"status"`
	MaskedPan     string           `json:"maskedPan"`
	Date          string           `json:"date"`
	PaymentScheme PaymentScheme    `json:"paymentScheme"`
	Amount        money.Money      `json:"amount"`
	ProfitAmount  money.Money      `json:"profitAmount,omitempty"`
	// AgentFee is the agent's commission, present when the invoice was
	// created with agentFeePercent.
	AgentFee     money.Money       `json:"agentFee,omitempty"`
	Currency     currency.Code     `json:"ccy"`
	ApprovalCode string            `json:"approvalCode,omitempty"`
	RRN          string            `json:"rrn,omitempty"`
	Reference    string            `json:"reference,omitempty"`
	ShortQrID    string            `json:"shortQrId,omitempty"`
	Destination  string            `json:"destination,omitempty"`
	CancelList   []StatementRefund `json:"cancelList,omitempty"`
	// SplitReceiverList is the per-receiver breakdown of a split
	// payment; empty outside the split scheme.
	SplitReceiverList []StatementSplitReceiver `json:"splitReceiverList,omitempty"`
	// Metadata echoes back the key→value map (string values) sent in
	// MerchantPaymInfo.Metadata when the invoice was created.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// UnmarshalJSON attaches Currency to every monetary field — Amount,
// ProfitAmount, AgentFee and the SplitReceiverList amounts.
func (s *StatementInvoice) UnmarshalJSON(data []byte) error {
	type raw StatementInvoice
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*s = StatementInvoice(r)
	c := s.Currency
	s.Amount.Code = c
	s.ProfitAmount.Code = c
	s.AgentFee.Code = c
	for i := range s.SplitReceiverList {
		sr := &s.SplitReceiverList[i]
		sr.Amount.Code = c
		sr.Profit.Code = c
		sr.AgentFee.Code = c
	}
	return nil
}

// StatementSplitReceiver is one receiver's share of a split payment in
// a statement row. All amounts are in minor units of the row's
// currency, which [StatementInvoice.UnmarshalJSON] attaches.
type StatementSplitReceiver struct {
	// SplitReceiverID is the split receiver's identifier (reference).
	SplitReceiverID string `json:"splitReceiverId"`
	// Amount is the part of the payment split to this receiver.
	Amount money.Money `json:"amount"`
	// Profit is what the receiver gets on its account after settlement
	// with the bank.
	Profit money.Money `json:"profit"`
	// AgentFee is the agent's commission on this part, present when the
	// invoice was created with agentFeePercent.
	AgentFee money.Money `json:"agentFee,omitempty"`
}

// StatementRefund is one cancel record nested in StatementInvoice.
type StatementRefund struct {
	Amount       money.Money   `json:"amount"`
	Currency     currency.Code `json:"ccy"`
	Date         string        `json:"date"`
	ApprovalCode string        `json:"approvalCode,omitempty"`
	RRN          string        `json:"rrn,omitempty"`
	MaskedPan    string        `json:"maskedPan"`
}

// UnmarshalJSON attaches Currency to Amount.Code.
func (s *StatementRefund) UnmarshalJSON(data []byte) error {
	type raw StatementRefund
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*s = StatementRefund(r)
	s.Amount.Code = s.Currency
	return nil
}

// StatementResponse is the wire shape of /api/merchant/statement.
type StatementResponse struct {
	List []StatementInvoice `json:"list"`
}

// WalletCard is one tokenised card in a wallet.
type WalletCard struct {
	CardToken string `json:"cardToken"`
	MaskedPan string `json:"maskedPan"`
	Country   string `json:"country,omitempty"`
}

// WalletResponse is the wire shape of GET /api/merchant/wallet.
type WalletResponse struct {
	Wallet []WalletCard `json:"wallet"`
}

// WalletPaymentRequest is the body of POST /api/merchant/wallet/payment.
//
// Besides a saved card (CardToken), the Google Pay and Apple Pay guides
// route their raw payment tokens through this endpoint (GToken,
// AToken). Set exactly one of CardToken, GToken, AToken.
type WalletPaymentRequest struct {
	// CardToken is the saved card's token. The endpoint schema marks it
	// required, but the Google Pay guide's example omits it, so it is
	// left off the wire when empty.
	CardToken   string        `json:"cardToken,omitempty"`
	Amount      int64         `json:"amount"`
	Currency    currency.Code `json:"ccy"`
	RedirectURL string        `json:"redirectUrl,omitempty"`
	WebHookURL  string        `json:"webHookUrl,omitempty"`
	// InitiationKind is required by /api/merchant/wallet/payment: it
	// tells the API whether the client is present (client) or the
	// charge is merchant-initiated (merchant). No omitempty — the API
	// rejects the request when it is absent.
	InitiationKind   InitiationKind    `json:"initiationKind"`
	MerchantPaymInfo *MerchantPaymInfo `json:"merchantPaymInfo,omitempty"`
	PaymentType      PaymentType       `json:"paymentType,omitempty"`
	// GToken is the Google Pay payment token, passed as the stringified
	// JSON Google returns (the guide's example: "gToken":
	// "{\"signature\":…}"). Documented only in the Google Pay guide,
	// not in the endpoint schema; the guide also says Google's test
	// tokens are not supported. The guide's example carries no
	// initiationKind, but the schema marks it required — unverified
	// which one the server enforces.
	GToken string `json:"gToken,omitempty"`
	// AToken is the encrypted Apple Pay payment data, passed as a
	// string. Documented only in the Apple Pay guide, which names the
	// parameter "aToken" in its text but writes "atoken" in its example;
	// the SDK sends "aToken". Unverified which spelling the server
	// accepts.
	AToken string `json:"aToken,omitempty"`
}

// WalletPaymentResponse is the response to wallet/payment.
type WalletPaymentResponse = PaymentDirectResponse
