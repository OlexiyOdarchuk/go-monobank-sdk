package acquiring

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
)

// ErrCode is the machine-readable error identifier the acquiring API
// returns in the `errCode` field of a 4xx/5xx body, alongside a
// human-readable `errText`. Mono documents a small, stable set of
// these; branch on the [Code*] constants rather than on errText
// (which is free-form and localized).
type ErrCode string

// Documented acquiring error codes, grouped by the HTTP status the
// docs list them under (every endpoint page of
// monobank.ua/api-docs/acquiring carries the same table). The list is
// intentionally open — an unknown code is preserved verbatim in
// [APIError.Code] rather than being dropped.
const (
	// 400 — the request is invalid.
	CodeBadRequest ErrCode = "BAD_REQUEST"
	// CodeBadRequest1001 is documented with the same meaning as
	// BAD_REQUEST. On /subscription/remove the docs also use it for
	// "a payment has already been made for this subscriptionId".
	CodeBadRequest1001 ErrCode = "1001"
	// CodeInvalidMerchantPaymInfo: an invalid value inside
	// merchantPaymInfo (invoice create, token payments, payment
	// by card details).
	CodeInvalidMerchantPaymInfo ErrCode = "INVALID_MERCHANT_PAYM_INFO"
	// CodeOrderInProgress: the amount set on a QR cash desk is already
	// being paid, so it cannot be reset.
	CodeOrderInProgress ErrCode = "ORDER_IN_PROGRESS"
	// CodeHoldInvoiceNotFinalized: the invoice was created with
	// paymentType=hold and paid but not finalized yet; returned when
	// asking for extended details of a successful payment.
	CodeHoldInvoiceNotFinalized ErrCode = "HOLD_INVOICE_NOT_FINALIZED"
	// CodeWrongCancelAmount: a cancel carried a wrong amount, or the
	// payment has already been cancelled.
	CodeWrongCancelAmount ErrCode = "WRONG_CANCEL_AMOUNT"
	// CodeTokenNotFound: the card token given in the request was not
	// found.
	CodeTokenNotFound ErrCode = "TOKEN_NOT_FOUND"

	// 403 — invalid X-Token.
	CodeForbidden ErrCode = "FORBIDDEN"

	// 404 — a lookup by one of the request parameters found nothing.
	CodeNotFound ErrCode = "NOT_FOUND"
	// CodeNotFound1004 is documented with the same meaning as
	// NOT_FOUND.
	CodeNotFound1004 ErrCode = "1004"

	// 405 — wrong HTTP method.
	CodeMethodNotAllowed ErrCode = "METHOD_NOT_ALLOWED"

	// 429 — too many requests.
	CodeTooManyRequests ErrCode = "TMR"

	// 500 — unexpected server-side failure.
	CodeInternalError ErrCode = "INTERNAL_ERROR"
	// CodeCancelNotAvailable: the payment cannot be cancelled;
	// returned by cancel requests.
	CodeCancelNotAvailable ErrCode = "CANCEL_NOT_AVAILABLE"

	// CodeExternalError appears only in the Google Pay guide's error
	// table (sample errText "mdes.decrypt 500"), without an HTTP
	// status, so no Is* predicate covers it.
	CodeExternalError ErrCode = "EXTERNAL_ERROR"
)

// APIError is the typed view of an acquiring error response. The
// base [monobank.Client] surfaces every non-2xx as
// *[monobank.APIError] (it only parses the personal/corporate
// `errorDescription` shape); acquiring instead ships
// {"errCode": "...", "errText": "..."}. [AsAPIError] bridges the two:
// it pulls the underlying *monobank.APIError out of an error chain
// and parses its body into this richer type.
//
// APIError wraps the underlying transport error, so
// errors.Is(err, monobank.ErrNotFound) and errors.As(err,
// &monobank.APIError{...}) keep working on a value returned from
// [AsAPIError].
type APIError struct {
	// StatusCode is the HTTP status of the response.
	StatusCode int
	// Code is the parsed `errCode`. Empty when the body did not carry
	// one (for example a bare gateway 502 with an HTML body).
	Code ErrCode
	// Text is the parsed `errText` (human-readable, possibly
	// localized). Empty when absent.
	Text string

	// base is the underlying transport error this was derived from.
	// Preserved for Unwrap so the monobank sentinels still match.
	base *monobank.APIError
}

func (e *APIError) Error() string {
	switch {
	case e.Code != "" && e.Text != "":
		return fmt.Sprintf("acquiring: HTTP %d %s: %s", e.StatusCode, e.Code, e.Text)
	case e.Code != "":
		return fmt.Sprintf("acquiring: HTTP %d %s", e.StatusCode, e.Code)
	case e.base != nil:
		return e.base.Error()
	default:
		return fmt.Sprintf("acquiring: HTTP %d", e.StatusCode)
	}
}

// Unwrap exposes the underlying *[monobank.APIError] so the standard
// sentinels (monobank.ErrUnauthorized, ErrNotFound,
// ErrTooManyRequests, ...) still match via errors.Is on a value
// returned by [AsAPIError].
func (e *APIError) Unwrap() error {
	if e.base == nil {
		return nil
	}
	return e.base
}

// errCodeBody is the acquiring error shape.
type errCodeBody struct {
	ErrCode string `json:"errCode"`
	ErrText string `json:"errText"`
}

// AsAPIError extracts a typed acquiring [APIError] from an error
// chain. It returns ok=false for nil and for errors that are not a
// monobank transport error (for example a context cancellation or a
// local marshal failure), so callers can cleanly distinguish a
// server-side rejection from a client-side problem:
//
//	if apiErr, ok := acquiring.AsAPIError(err); ok {
//	    switch apiErr.Code {
//	    case acquiring.CodeNotFound:     // invoice gone
//	    case acquiring.CodeTooManyRequests: // back off
//	    }
//	}
//
// When the underlying body has no parseable errCode (an empty body,
// HTML, or the personal-style errorDescription), ok is still true —
// Code/Text are simply empty and StatusCode carries the signal.
func AsAPIError(err error) (*APIError, bool) {
	if err == nil {
		return nil, false
	}
	var base *monobank.APIError
	if !errors.As(err, &base) {
		return nil, false
	}
	out := &APIError{StatusCode: base.StatusCode, base: base}
	var body errCodeBody
	if json.Unmarshal(base.Body, &body) == nil {
		out.Code = ErrCode(body.ErrCode)
		out.Text = body.ErrText
	}
	// The base parser may already have lifted errText into
	// ErrorDescription for personal-shaped bodies; fall back to it so
	// Text is populated either way.
	if out.Text == "" {
		out.Text = base.ErrorDescription
	}
	return out, true
}

// Code returns the acquiring [ErrCode] carried by err, or "" when
// err is not an acquiring API error or carried no code. A small
// convenience over [AsAPIError] for switch statements.
func Code(err error) ErrCode {
	if e, ok := AsAPIError(err); ok {
		return e.Code
	}
	return ""
}

// Documented codes per HTTP status, used by the Is* predicates.
var (
	badRequestCodes = []ErrCode{
		CodeBadRequest, CodeBadRequest1001, CodeInvalidMerchantPaymInfo,
		CodeOrderInProgress, CodeHoldInvoiceNotFinalized,
		CodeWrongCancelAmount, CodeTokenNotFound,
	}
	notFoundCodes      = []ErrCode{CodeNotFound, CodeNotFound1004}
	internalErrorCodes = []ErrCode{CodeInternalError, CodeCancelNotAvailable}
)

// is reports whether err is an acquiring API error with the given
// HTTP status OR one of the codes the docs list under that status.
// The status alone is enough: each documented status carries several
// codes (a 404 may say NOT_FOUND or 1004), so matching a single code
// would miss documented responses. The code side catches a documented
// code on an unexpected status.
func is(err error, status int, codes ...ErrCode) bool {
	e, ok := AsAPIError(err)
	if !ok {
		return false
	}
	if e.StatusCode == status {
		return true
	}
	for _, c := range codes {
		if e.Code == c {
			return true
		}
	}

	return false
}

// IsBadRequest reports whether err is a 400 from the acquiring API, or
// carries one of the codes documented for 400: BAD_REQUEST, 1001,
// INVALID_MERCHANT_PAYM_INFO, ORDER_IN_PROGRESS,
// HOLD_INVOICE_NOT_FINALIZED, WRONG_CANCEL_AMOUNT, TOKEN_NOT_FOUND.
// Use [Code] to tell them apart.
func IsBadRequest(err error) bool {
	return is(err, http.StatusBadRequest, badRequestCodes...)
}

// IsForbidden reports whether err is a 403 / FORBIDDEN (the docs
// describe it as an invalid X-Token).
func IsForbidden(err error) bool {
	return is(err, http.StatusForbidden, CodeForbidden)
}

// IsNotFound reports whether err is a 404, or carries NOT_FOUND or
// 1004 (unknown invoiceId, qrId, subscriptionId, ...).
func IsNotFound(err error) bool {
	return is(err, http.StatusNotFound, notFoundCodes...)
}

// IsTooManyRequests reports whether err is a 429 / TMR. The caller
// should back off and retry later (the SDK already retries transient
// statuses with backoff, so seeing this means the budget is
// genuinely exhausted).
func IsTooManyRequests(err error) bool {
	return is(err, http.StatusTooManyRequests, CodeTooManyRequests)
}

// IsInternalError reports whether err is a 500, or carries
// INTERNAL_ERROR or CANCEL_NOT_AVAILABLE. A 500 with INTERNAL_ERROR is
// generally safe to retry for an idempotent read; CANCEL_NOT_AVAILABLE
// means the cancel itself is not possible, so retrying will not help.
func IsInternalError(err error) bool {
	return is(err, http.StatusInternalServerError, internalErrorCodes...)
}
