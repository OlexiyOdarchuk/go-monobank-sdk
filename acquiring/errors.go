package acquiring

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

// ErrCode is the machine-readable error identifier the acquiring API
// returns in the `errCode` field of a 4xx/5xx body, alongside a
// human-readable `errText`. Mono documents a small, stable set of
// these; branch on the [Code*] constants rather than on errText
// (which is free-form and localized).
type ErrCode string

// Documented acquiring error codes (see the "Common Error Responses"
// section of the acquiring API docs). The list is intentionally
// open — an unknown code is preserved verbatim in [APIError.Code]
// rather than being dropped.
const (
	CodeBadRequest       ErrCode = "BAD_REQUEST"        // 400
	CodeForbidden        ErrCode = "FORBIDDEN"          // 403
	CodeNotFound         ErrCode = "NOT_FOUND"          // 404
	CodeMethodNotAllowed ErrCode = "METHOD_NOT_ALLOWED" // 405
	CodeTooManyRequests  ErrCode = "TMR"                // 429
	CodeInternalError    ErrCode = "INTERNAL_ERROR"     // 500
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

// is reports whether err is an acquiring API error matching the
// given code OR HTTP status. Matching on either side makes the
// predicates robust: Mono is consistent about the status code even
// on the rare response where the body lacks an errCode.
func is(err error, code ErrCode, status int) bool {
	e, ok := AsAPIError(err)
	if !ok {
		return false
	}
	if e.Code != "" {
		return e.Code == code
	}
	return e.StatusCode == status
}

// IsBadRequest reports whether err is a 400 / BAD_REQUEST from the
// acquiring API (malformed request, validation failure).
func IsBadRequest(err error) bool {
	return is(err, CodeBadRequest, http.StatusBadRequest)
}

// IsForbidden reports whether err is a 403 / FORBIDDEN (the token
// lacks rights for this endpoint or merchant).
func IsForbidden(err error) bool {
	return is(err, CodeForbidden, http.StatusForbidden)
}

// IsNotFound reports whether err is a 404 / NOT_FOUND (unknown
// invoiceId, qrId, subscriptionId, ...).
func IsNotFound(err error) bool {
	return is(err, CodeNotFound, http.StatusNotFound)
}

// IsTooManyRequests reports whether err is a 429 / TMR. The caller
// should back off and retry later (the SDK already retries transient
// statuses with backoff, so seeing this means the budget is
// genuinely exhausted).
func IsTooManyRequests(err error) bool {
	return is(err, CodeTooManyRequests, http.StatusTooManyRequests)
}

// IsInternalError reports whether err is a 500 / INTERNAL_ERROR on
// Mono's side — generally safe to retry an idempotent read.
func IsInternalError(err error) bool {
	return is(err, CodeInternalError, http.StatusInternalServerError)
}
