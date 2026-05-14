package acquiring

import (
	"encoding/json"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
)

// MerchantDetails is the response to GET /api/merchant/details.
type MerchantDetails struct {
	MerchantID   string `json:"merchantId"`
	MerchantName string `json:"merchantName"`
	EDRPOU       string `json:"edrpou"`
}

// Employee is one row from /api/merchant/employee/list.
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

// UnmarshalJSON прив’язує Currency → Amount.Code.
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
}

// MerchantPaymInfo is the marketing/checkout metadata attached to an invoice.
type MerchantPaymInfo struct {
	Reference      string       `json:"reference,omitempty"`
	Destination    string       `json:"destination,omitempty"`
	Comment        string       `json:"comment,omitempty"`
	CustomerEmails []string     `json:"customerEmails,omitempty"`
	Discounts      []Adjustment `json:"discounts,omitempty"`
	BasketOrder    []BasketItem `json:"basketOrder,omitempty"`
}

// PaymentType — debit (capture immediately) or hold (capture later via finalize).
type PaymentType string

// Possible PaymentType values.
const (
	PaymentDebit PaymentType = "debit"
	PaymentHold  PaymentType = "hold"
)

// SaveCardData asks the bank to tokenize the card for future wallet payments.
type SaveCardData struct {
	SaveCard bool   `json:"saveCard"`
	WalletID string `json:"walletId,omitempty"`
}

// CreateInvoiceRequest is the body of POST /api/merchant/invoice/create.
type CreateInvoiceRequest struct {
	Amount           int64             `json:"amount"`
	Currency         currency.Code     `json:"ccy,omitempty"`
	MerchantPaymInfo *MerchantPaymInfo `json:"merchantPaymInfo,omitempty"`
	RedirectURL      string            `json:"redirectUrl,omitempty"`
	WebHookURL       string            `json:"webHookUrl,omitempty"`
	Validity         int64             `json:"validity,omitempty"`
	PaymentType      PaymentType       `json:"paymentType,omitempty"`
	QrID             string            `json:"qrId,omitempty"`
	Code             string            `json:"code,omitempty"`
	SaveCardData     *SaveCardData     `json:"saveCardData,omitempty"`
	AgentFeePercent  float64           `json:"agentFeePercent,omitempty"`
	TipsEmployeeID   string            `json:"tipsEmployeeId,omitempty"`
}

// CreateInvoiceResponse is the response to POST /api/merchant/invoice/create.
type CreateInvoiceResponse struct {
	InvoiceID string `json:"invoiceId"`
	PageURL   string `json:"pageUrl"`
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
// Fee та AgentFee — у мінорних одиницях; валюта успадковується з
// батьківського [InvoiceStatusResponse.Currency].
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

// WalletStatus — стан токенізації картки.
type WalletStatus string

// Можливі значення [WalletStatus].
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

// TipsInfo is the tip amount/recipient on an invoice.
type TipsInfo struct {
	EmployeeID string `json:"employeeId"`
	Amount     int    `json:"amount,omitempty"`
}

// ProcessingStatus — стан асинхронної операції (cancel/finalize/
// payment-direct/wallet-payment/sync-payment/statement). Один тип на
// всі ці місця, бо wire-енумерація однакова.
type ProcessingStatus string

// Можливі значення [ProcessingStatus].
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

// UnmarshalJSON прив’язує Currency → Amount.Code.
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
// Грошові поля — типізовані [money.Money]; Code заповнюється з Currency.
// CancelList[*].Amount, PaymentInfo.Fee/AgentFee теж отримують Currency.
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

// UnmarshalJSON прив’язує Currency до всіх грошових полів — Amount, FinalAmount,
// PaymentInfo.{Fee,AgentFee}. CancelList[i].Amount має власний Currency і
// заповнюється його UnmarshalJSON-ом.
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
	InvoiceID string       `json:"invoiceId"`
	Amount    int64        `json:"amount,omitempty"`
	Items     []CancelItem `json:"items,omitempty"`
}

// FinalizeResponse is the response to POST /api/merchant/invoice/finalize.
type FinalizeResponse struct {
	Status ProcessingStatus `json:"status"` // always StatusSuccess
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
	SourceCheckbox FiscalizationSource = "checkbox"
	SourceMonopay  FiscalizationSource = "monopay"
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

// UnmarshalJSON прив’язує Currency → Amount.Code.
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

// ApplePayPayload is the wire shape for an Apple Pay token.
type ApplePayPayload struct {
	Token        string `json:"token"`
	Exp          string `json:"exp"`
	EciIndicator string `json:"eciIndicator"`
	Cryptogram   string `json:"cryptogram,omitempty"`
}

// GooglePayPayload is the wire shape for a Google Pay token.
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

// StatementInvoice is one row in /api/merchant/statement.
type StatementInvoice struct {
	InvoiceID     string            `json:"invoiceId"`
	Status        ProcessingStatus  `json:"status"`
	MaskedPan     string            `json:"maskedPan"`
	Date          string            `json:"date"`
	PaymentScheme string            `json:"paymentScheme"`
	Amount        money.Money       `json:"amount"`
	ProfitAmount  money.Money       `json:"profitAmount,omitempty"`
	Currency      currency.Code     `json:"ccy"`
	ApprovalCode  string            `json:"approvalCode,omitempty"`
	RRN           string            `json:"rrn,omitempty"`
	Reference     string            `json:"reference,omitempty"`
	ShortQrID     string            `json:"shortQrId,omitempty"`
	Destination   string            `json:"destination,omitempty"`
	CancelList    []StatementRefund `json:"cancelList,omitempty"`
}

// UnmarshalJSON прив’язує Currency → Amount/ProfitAmount.Code.
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
	return nil
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

// UnmarshalJSON прив’язує Currency → Amount.Code.
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
type WalletPaymentRequest struct {
	CardToken        string            `json:"cardToken"`
	Amount           int64             `json:"amount"`
	Currency         currency.Code     `json:"ccy"`
	RedirectURL      string            `json:"redirectUrl,omitempty"`
	WebHookURL       string            `json:"webHookUrl,omitempty"`
	InitiationKind   InitiationKind    `json:"initiationKind"`
	MerchantPaymInfo *MerchantPaymInfo `json:"merchantPaymInfo,omitempty"`
	PaymentType      PaymentType       `json:"paymentType,omitempty"`
}

// WalletPaymentResponse is the response to wallet/payment.
type WalletPaymentResponse = PaymentDirectResponse
