package openbanking

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/mcc"
)

// Berlin Group header names. They are exported because two of them
// only ever arrive on the response and the base [monobank.Client]
// decodes bodies, not headers — see the package documentation for the
// [monobank.WithResponseHook] recipe that reads
// [HeaderASPSPSCAApproach] and [HeaderLocation].
const (
	HeaderRequestID                = "X-Request-ID"
	HeaderConsentID                = "Consent-ID"
	HeaderPSUID                    = "PSU-ID"
	HeaderPSUIDType                = "PSU-ID-Type"
	HeaderPSUCorporateID           = "PSU-Corporate-ID"
	HeaderPSUCorporateIDType       = "PSU-Corporate-ID-Type"
	HeaderPSUIPAddress             = "PSU-IP-Address"
	HeaderTPPRedirectURI           = "TPP-Redirect-URI"
	HeaderTPPSCAApproachPreference = "TPP-SCA-Approach-Preference"

	// HeaderASPSPSCAApproach is response-only: it tells which SCA
	// approach the bank actually picked for an authorisation.
	HeaderASPSPSCAApproach = "ASPSP-SCA-Approach"
	// HeaderLocation is response-only: the URI of the resource a 201
	// created.
	HeaderLocation = "Location"
)

// dateLayout is the ISO-8601 calendar-date layout the API uses for
// validTo, bookingDate and the dateFrom/dateTo query parameters.
const dateLayout = "2006-01-02"

// Date is a calendar date without a time zone, as the API encodes it
// ("2024-12-31"). It exists because time.Time marshals to RFC 3339 —
// sending that where the bank expects a plain date is a 400.
//
// The zero Date marshals to an empty string and is skipped by the
// query builders, which is how optional date filters are expressed.
type Date time.Time

// NewDate converts a time.Time to a [Date], dropping the clock part.
func NewDate(t time.Time) Date { return Date(t) }

// Time returns the underlying time.Time (midnight in t's location).
func (d Date) Time() time.Time { return time.Time(d) }

// IsZero reports whether the date is unset.
func (d Date) IsZero() bool { return time.Time(d).IsZero() }

// String renders the date as the API spells it, or "" when unset.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}

	return time.Time(d).Format(dateLayout)
}

// MarshalJSON implements json.Marshaler.
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// UnmarshalJSON implements json.Unmarshaler. An empty string decodes
// to the zero Date rather than an error, so an unset optional date
// cannot fail the decoding of the whole response.
func (d *Date) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == "" {
		*d = Date(time.Time{})

		return nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return err
	}
	*d = Date(t)

	return nil
}

// PaymentProduct is the {paymentProduct} path segment of the payment
// endpoints. It selects the rail the transfer travels on.
type PaymentProduct string

// Possible PaymentProduct values.
const (
	// CreditTransfers — an ordinary SEP credit transfer.
	CreditTransfers PaymentProduct = "credit-transfers"
	// InstantCreditTransfers — SEP-instant (round the clock,
	// settled within seconds).
	InstantCreditTransfers PaymentProduct = "instant-credit-transfers"
)

// ConsentType is the granularity of an account-access consent.
type ConsentType string

// Possible ConsentType values.
const (
	// ConsentDetailed — the only type [Client.CreateConsent] accepts:
	// the TPP lists the exact accounts and rights it asks for.
	ConsentDetailed ConsentType = "detailed"
	// ConsentAccountList — may come back from [Client.Consent] for a
	// consent created outside this SDK; it cannot be requested.
	ConsentAccountList ConsentType = "accountList"
)

// ConsentStatus is the lifecycle state of an account-access consent.
type ConsentStatus string

// Possible ConsentStatus values.
const (
	// ConsentReceived — created, not authorised by the PSU yet.
	ConsentReceived ConsentStatus = "received"
	// ConsentRejected — the PSU declined the authorisation.
	ConsentRejected ConsentStatus = "rejected"
	// ConsentValid — authorised and usable for data calls.
	ConsentValid ConsentStatus = "valid"
	// ConsentExpired — validTo has passed.
	ConsentExpired ConsentStatus = "expired"
	// ConsentRevokedByPSU — the PSU withdrew it in their bank app.
	ConsentRevokedByPSU ConsentStatus = "revokedByPsu"
	// ConsentTerminatedByTPP — withdrawn via [Client.RevokeConsent].
	ConsentTerminatedByTPP ConsentStatus = "terminatedByTpp"
	// ConsentReplacedByTPP — superseded by a newer consent.
	ConsentReplacedByTPP ConsentStatus = "replacedByTpp"
)

// AccessRight is a single permission granted on one account inside a
// consent.
type AccessRight string

// Possible AccessRight values.
const (
	// AccessAccountDetails unlocks the account in [Client.Accounts].
	AccessAccountDetails AccessRight = "accountDetails"
	// AccessBalances unlocks [Client.Balances].
	AccessBalances AccessRight = "balances"
	// AccessTransactions unlocks [Client.Transactions].
	AccessTransactions AccessRight = "transactions"
)

// BalanceType is the kind of balance a [Balance] carries. The spec
// declares exactly one value today.
type BalanceType string

// Possible BalanceType values.
const (
	// BalanceInterimAvailable — the amount available to spend right
	// now, intraday and not yet booked.
	BalanceInterimAvailable BalanceType = "interimAvailable"
)

// BookingStatus filters the statement by how settled its entries are.
// It is a mandatory query parameter of [Client.Transactions].
type BookingStatus string

// Possible BookingStatus values.
const (
	// BookingBooked — settled entries only.
	BookingBooked BookingStatus = "booked"
	// BookingPending — authorised but not yet settled.
	BookingPending BookingStatus = "pending"
	// BookingBoth — booked and pending together.
	BookingBoth BookingStatus = "both"
)

// SCAStatus is the state of one strong-customer-authentication
// process (a consent authorisation or a payment authorisation).
type SCAStatus string

// Possible SCAStatus values.
const (
	// SCAStarted — the PSU has not finished authenticating yet.
	SCAStarted SCAStatus = "started"
	// SCAFinalised — authentication succeeded; the resource moves on.
	SCAFinalised SCAStatus = "finalised"
	// SCAFailed — authentication was declined or timed out.
	SCAFailed SCAStatus = "failed"
)

// SCAApproach is how the PSU is taken through authentication.
//
// The bank picks it from the PSU's own type and only treats
// [InitiationOptions] / [AuthorisationOptions] preferences as a hint:
// natural persons and sole proprietors always get [SCARedirect],
// legal entities always get [SCADecoupled].
type SCAApproach string

// Possible SCAApproach values.
const (
	// SCARedirect — the TPP sends the PSU to _links.scaRedirect.
	SCARedirect SCAApproach = "REDIRECT"
	// SCADecoupled — the PSU confirms in their own banking app; no
	// redirect link is returned and the TPP polls the SCA status.
	SCADecoupled SCAApproach = "DECOUPLED"
)

// TransactionStatus is the ISO 20022 state of an initiated payment.
type TransactionStatus string

// Possible TransactionStatus values.
const (
	// StatusReceived (RCVD) — accepted for processing.
	StatusReceived TransactionStatus = "RCVD"
	// StatusRejected (RJCT) — refused; the money never moved.
	StatusRejected TransactionStatus = "RJCT"
	// StatusAcceptedSettlementCompleted (ACSC) — debited from the
	// payer.
	StatusAcceptedSettlementCompleted TransactionStatus = "ACSC"
	// StatusAcceptedCreditSettlementCompleted (ACCC) — credited to
	// the beneficiary; the terminal success state.
	StatusAcceptedCreditSettlementCompleted TransactionStatus = "ACCC"
	// StatusCancelled (CANC) — cancelled, e.g. via
	// [Client.CancelPayment].
	StatusCancelled TransactionStatus = "CANC"
)

// PSUIDType is the type of identifier carried in the PSU-ID /
// PSU-Corporate-ID headers. The spec declares a single value.
type PSUIDType string

// PSUIDTypeIBAN — the PSU is identified by one of their IBANs.
const PSUIDTypeIBAN PSUIDType = "IBAN"

// CreditorIDType classifies [Creditor.CreditorID].
type CreditorIDType string

// Possible CreditorIDType values. The spec lists the codes without
// expanding them; the comments below reflect the Ukrainian payment
// vocabulary they come from.
const (
	// CreditorRNRCT — РНОКПП, an individual taxpayer number.
	CreditorRNRCT CreditorIDType = "RNRCT"
	// CreditorPassport — a passport.
	CreditorPassport CreditorIDType = "PSPT"
	// CreditorOther — some other identifier.
	CreditorOther CreditorIDType = "OT"
	// CreditorUnknown — the identifier type is not known.
	CreditorUnknown CreditorIDType = "UNKN"
	// CreditorUSRC — ЄДРПОУ, a legal entity registration code.
	CreditorUSRC CreditorIDType = "USRC"
	// CreditorTransit — a transit account identifier.
	CreditorTransit CreditorIDType = "TRAN"
	// CreditorNotApplicable — no identifier applies.
	CreditorNotApplicable CreditorIDType = "NA"
)

// Href is the Berlin Group HAL link object: a bare {"href": "..."}.
type Href struct {
	Href string `json:"href"`
}

// Amount is a monetary value in the currency of the account.
//
// The bank encodes the number as a decimal string with a dot
// separator (up to 14 integer and 3 fractional digits), so the SDK
// keeps it a string: a float64 would silently round, and the repo's
// money.Money is a minor-unit integer with no room for the third
// decimal this API allows. The field is named Value to avoid the
// stutter of Amount.Amount; the wire tag is the spec's "amount".
type Amount struct {
	// Currency is the ISO 4217 alpha-3 code, e.g. "UAH".
	Currency string `json:"currency"`
	Value    string `json:"amount"`
}

// Account identifies an account inside a consent request.
type Account struct {
	IBAN     string `json:"iban,omitempty"`
	Currency string `json:"currency,omitempty"`
}

// AccountAccessRights asks for (or reports) a set of rights on one
// account.
type AccountAccessRights struct {
	Account Account       `json:"account"`
	Rights  []AccessRight `json:"rights"`
}

// AccountAccess groups the requested rights by product family. Every
// group is optional; leaving one empty asks for nothing on it.
type AccountAccess struct {
	// Cards — card accounts.
	Cards []AccountAccessRights `json:"cards,omitempty"`
	// Payments — current accounts.
	Payments []AccountAccessRights `json:"payments,omitempty"`
	// Savings — savings accounts ("banky"/deposits).
	Savings []AccountAccessRights `json:"savings,omitempty"`
	// Loans — credit accounts.
	Loans []AccountAccessRights `json:"loans,omitempty"`
}

// AccountDetails is one account as [Client.Accounts] returns it.
// Balances is filled only when the call asked for it and the consent
// grants [AccessBalances].
type AccountDetails struct {
	// ResourceID is the {accountId} used by [Client.Balances] and
	// [Client.Transactions]. It is not the IBAN. The spec says
	// nothing about how long it stays valid or whether it is shared
	// across consents — re-read it from [Client.Accounts] rather
	// than storing it indefinitely.
	ResourceID string    `json:"resourceId"`
	IBAN       string    `json:"iban,omitempty"`
	Currency   string    `json:"currency"`
	Product    string    `json:"product,omitempty"`
	Balances   []Balance `json:"balances,omitempty"`
}

// AccountReference echoes which account a balance or statement
// response belongs to.
type AccountReference struct {
	IBAN     string `json:"iban,omitempty"`
	Currency string `json:"currency,omitempty"`
	Product  string `json:"product,omitempty"`
}

// PaymentAccountReference names the payer or beneficiary account of a
// payment. Unlike [AccountReference] it carries no product name.
type PaymentAccountReference struct {
	IBAN     string `json:"iban,omitempty"`
	Currency string `json:"currency,omitempty"`
}

// Balance is a single balance of an account.
type Balance struct {
	BalanceAmount Amount      `json:"balanceAmount"`
	BalanceType   BalanceType `json:"balanceType"`
	// CreditLimitIncluded reports whether the credit limit is part of
	// BalanceAmount — decisive for card accounts, where the available
	// amount otherwise looks larger than the customer's own money.
	CreditLimitIncluded bool `json:"creditLimitIncluded,omitempty"`
}

// Party is a counterparty of a statement entry.
type Party struct {
	Name string `json:"name,omitempty"`
}

// Creditor is the beneficiary of a payment. All three fields are
// mandatory when initiating one.
type Creditor struct {
	Name           string         `json:"name"`
	CreditorID     string         `json:"creditorId"`
	CreditorIDType CreditorIDType `json:"creditorIdType"`
}

// CardTransactionDetails carries the card-specific attributes of a
// statement entry.
type CardTransactionDetails struct {
	// TransactionDateTime is RFC 3339 with nanoseconds.
	TransactionDateTime time.Time `json:"transactionDateTime,omitzero"`
	// TradeName is the merchant's trading name.
	TradeName string `json:"tradeName,omitempty"`
	// MerchantCategoryCode is the four-digit MCC; see the repo's mcc
	// package for the lookup table.
	MerchantCategoryCode string `json:"merchantCategoryCode,omitempty"`
}

// Transaction is one statement entry.
type Transaction struct {
	TransactionID     string `json:"transactionId,omitempty"`
	BookingDate       Date   `json:"bookingDate,omitzero"`
	TransactionAmount Amount `json:"transactionAmount"`
	Creditor          *Party `json:"creditor,omitempty"`
	Debtor            *Party `json:"debtor,omitempty"`
	// CardTransaction is present for card entries only.
	CardTransaction *CardTransactionDetails `json:"cardTransaction,omitempty"`
	// RemittanceInformationUnstructured is the payment purpose. The
	// spec pins it to exactly one element, but keeps it an array —
	// the SDK does not flatten it, so the wire shape round-trips.
	RemittanceInformationUnstructured []string `json:"remittanceInformationUnstructured,omitempty"`
}

// AccountReportLinks paginates a statement.
type AccountReportLinks struct {
	// First links to the first page of the statement.
	First *Href `json:"first,omitempty"`
	// Next is present only while further pages exist; its href
	// carries the pageId to feed back into
	// [TransactionsOptions.PageID].
	Next *Href `json:"next,omitempty"`
}

// AccountReport is one page of a statement.
type AccountReport struct {
	Booked TransactionList    `json:"booked,omitempty"`
	Links  AccountReportLinks `json:"_links,omitzero"`
}

// ClientMessage is one entry of the apiClientMessages array the bank
// returns with 4xx/5xx responses.
type ClientMessage struct {
	// Category is the severity, e.g. "ERROR".
	Category string `json:"category"`
	// Code is the machine-readable reason, e.g. "BAD_REQUEST" or
	// [CodeCurrencyMismatch].
	Code string `json:"code"`
	// Text is the human-readable explanation; for
	// [CodeCurrencyMismatch] it names both currencies.
	Text string `json:"text,omitempty"`
}

// CodeCurrencyMismatch is the only error code the spec spells out: the
// currency of the account or card does not match the one the request
// asked for. [ClientMessage.Text] then carries both currencies.
const CodeCurrencyMismatch = "CURRENCY_MISMATCH"

// errorBody is the error envelope of this API. It is *not* the
// {"errorDescription": ...} shape the rest of Mono uses, which is why
// [monobank.APIError.ErrorDescription] stays empty here and
// [Messages] parses the raw body instead.
type errorBody struct {
	APIClientMessages []ClientMessage `json:"apiClientMessages"`
}

// Messages extracts the apiClientMessages of a failed call. It
// returns nil when err is not a [monobank.APIError] or its body is
// not the Open Banking error envelope, so it is safe to call on any
// error:
//
//	if msgs := openbanking.Messages(err); len(msgs) > 0 {
//		log.Println(msgs[0].Code, msgs[0].Text)
//	}
//
// The typed [monobank.APIError] (status code, raw body) remains
// reachable through errors.As, and the usual sentinels —
// [monobank.ErrUnauthorized], [monobank.ErrNotFound],
// [monobank.ErrTooManyRequests] — still match via errors.Is.
func Messages(err error) []ClientMessage {
	var apiErr *monobank.APIError
	if !errors.As(err, &apiErr) || len(apiErr.Body) == 0 {
		return nil
	}
	var body errorBody
	if jsonErr := json.Unmarshal(apiErr.Body, &body); jsonErr != nil {
		return nil
	}

	return body.APIClientMessages
}

// PSU describes the payment-service user the TPP acts for. The fields
// become the Berlin Group PSU-* request headers.
//
// Which of them the bank needs depends on the customer: a natural
// person or sole proprietor is identified by ID/IDType, a legal
// entity by CorporateID/CorporateIDType. IPAddress is the address of
// the PSU's own device and is mandatory when creating a consent or a
// payment. The spec does not say what the bank does with it, but
// forwarding your own server's address instead of the customer's
// makes the field meaningless.
type PSU struct {
	ID              string
	IDType          PSUIDType
	CorporateID     string
	CorporateIDType PSUIDType
	IPAddress       string
}

// apply writes the non-empty PSU fields onto an outgoing request.
func (p PSU) apply(h http.Header) {
	setIfNotEmpty(h, HeaderPSUID, p.ID)
	setIfNotEmpty(h, HeaderPSUIDType, string(p.IDType))
	setIfNotEmpty(h, HeaderPSUCorporateID, p.CorporateID)
	setIfNotEmpty(h, HeaderPSUCorporateIDType, string(p.CorporateIDType))
	setIfNotEmpty(h, HeaderPSUIPAddress, p.IPAddress)
}

// InitiationOptions carries the headers shared by the two requests
// that start an SCA flow — [Client.CreateConsent] and
// [Client.CreatePayment].
type InitiationOptions struct {
	// PSU identifies the customer; PSU.IPAddress is mandatory for
	// both requests and its absence is reported as
	// [ErrMissingPSUIPAddress].
	PSU PSU
	// TPPRedirectURI is where the bank returns the PSU after a
	// REDIRECT authorisation. It is optional in the spec, but a
	// REDIRECT flow without it strands the customer in the bank's UI.
	TPPRedirectURI string
}

// AccountOptions carries the headers every account-information call
// needs.
type AccountOptions struct {
	// ConsentID is the id from [CreateConsentResponse]. It is
	// mandatory; an empty value is reported as
	// [ErrMissingConsentID] before the request leaves the process.
	ConsentID string
	// PSUIPAddress is optional here and should be set only when the
	// call is driven by a PSU sitting in front of the TPP's UI
	// (as opposed to a background refresh).
	PSUIPAddress string
}

// apply writes the account-information headers onto a request.
func (o AccountOptions) apply(h http.Header) {
	h.Set(HeaderConsentID, o.ConsentID)
	setIfNotEmpty(h, HeaderPSUIPAddress, o.PSUIPAddress)
}

// AccountsOptions parameterises [Client.Accounts].
type AccountsOptions struct {
	AccountOptions
	// WithBalance asks the bank to embed [AccountDetails.Balances].
	// The spec does not state how it interacts with the rights the
	// consent granted, so do not assume every listed account comes
	// back with a balance.
	WithBalance bool
}

// TransactionsOptions parameterises [Client.Transactions].
type TransactionsOptions struct {
	AccountOptions
	// BookingStatus is mandatory — see [ErrMissingBookingStatus].
	BookingStatus BookingStatus
	// DateFrom and DateTo bound the period; the zero [Date] omits the
	// bound and lets the bank apply its own default window.
	DateFrom Date
	DateTo   Date
	// PageID continues a previous page. Take it from the pageId query
	// parameter of [AccountReportLinks.Next] rather than building it
	// by hand — it is an opaque cursor.
	PageID string
}

// AuthorisationOptions parameterises
// [Client.StartPaymentAuthorisation].
type AuthorisationOptions struct {
	// SCAApproachPreference is a hint, not an instruction: the bank
	// overrides it with the approach that fits the customer type (see
	// [SCAApproach]). Leave it empty to accept the default.
	SCAApproachPreference SCAApproach
}

// setIfNotEmpty avoids sending empty optional headers, which some
// gateways treat differently from an absent one.
func setIfNotEmpty(h http.Header, name, value string) {
	if value != "" {
		h.Set(name, value)
	}
}

// CurrencyCode returns the amount's currency as the SDK's typed
// [currency.Code]. Open Banking spells currencies alpha-3 ("UAH")
// while the rest of the bank's APIs send the ISO 4217 number, so the
// field stays a string on the wire and the conversion is offered
// here. ok=false for a code the SDK does not know.
func (a Amount) CurrencyCode() (currency.Code, bool) {
	return currency.FromAlpha3(a.Currency)
}

// MCCCode returns the typed MCC of a card entry, for chaining
// .Category() the way [bank.Transaction.MCCCode] does. The wire field
// is a string here, so anything that is not four digits in the valid
// 1..9999 range comes back as Code(0), which Category() folds into
// [mcc.CategoryUnknown].
func (d CardTransactionDetails) MCCCode() mcc.Code {
	n, err := strconv.Atoi(d.MerchantCategoryCode)
	if err != nil || n < 1 || n > 9999 {
		return mcc.Code(0)
	}
	return mcc.Code(n)
}
