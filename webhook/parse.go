package webhook

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// ErrUnknownType is returned from [Parse] when the top-level "type"
// field of the payload is not in the known list of Type* constants.
// This is not a verification error — the payload is authentic, its
// type just is not represented in the SDK yet.
var ErrUnknownType = errors.New("unknown webhook type")

// Known webhook event types.
const (
	// TypeStatementItem is a single account-statement entry (the
	// only type Mono documents as of today).
	TypeStatementItem = "StatementItem"
)

// Response is the top-level Mono webhook payload.
type Response struct {
	Type string `json:"type"` // see the Type* constants
	Data Data   `json:"data"`
}

// Data is the webhook payload. For TypeStatementItem, Transaction
// holds the full transaction (as in /personal/statement) and
// AccountID is the id of the account it happened on.
type Data struct {
	AccountID   string           `json:"account"`
	Transaction bank.Transaction `json:"statementItem"`
}

// Parse decodes the raw webhook body into a [Response]. If "type" is
// not in the known Type* constants, Response is still returned (with
// the parsed type) along with [ErrUnknownType]. The caller can then
// either ignore such events or handle them with generic code.
func Parse(body []byte) (*Response, error) {
	var v Response
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, fmt.Errorf("decode webhook: %w", err)
	}
	switch v.Type {
	case TypeStatementItem:
		return &v, nil
	default:
		return &v, fmt.Errorf("%w: %q", ErrUnknownType, v.Type)
	}
}
