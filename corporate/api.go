package corporate

import (
	"context"
	"iter"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// API is the interface of the Corporate Open API client (including
// monoKEP). It exists separately from *[Client] so callers can mock
// it via mockgen/testify-mock in their own tests. [Client]
// implements this interface (verified by the compile-time assert
// below).
type API interface {
	// Authorization and company settings.
	Auth(ctx context.Context, callbackURL string, permissions ...auth.Permission) (*TokenRequest, error)
	CheckAuth(ctx context.Context, requestID string) error
	Register(ctx context.Context, in *RegistrationRequest) (*RegistrationResponse, error)
	RegistrationStatus(ctx context.Context, pubkeyPEM []byte) (*RegistrationStatusResponse, error)
	GetSettings(ctx context.Context) (*Settings, error)
	SetWebHook(ctx context.Context, uri string) error

	// Client data.
	ClientInfo(ctx context.Context, requestID string) (*bank.ClientInfo, error)
	Transactions(ctx context.Context, requestID, accountID string, from, to time.Time) (bank.Transactions, error)
	TransactionsRange(ctx context.Context, requestID, accountID string, from, to time.Time) (bank.Transactions, error)
	TransactionsRangeIter(ctx context.Context, requestID, accountID string, from, to time.Time) iter.Seq2[bank.Transaction, error]

	// monoKEP.
	SignatureCreate(ctx context.Context, in *SignatureCreateRequest) (*SignatureCreateResponse, error)
	SignatureStatus(ctx context.Context, requestID string) (*SignatureStatusResponse, error)
	SignatureCancel(ctx context.Context, requestID string) error
}

// Compile-time assert: *Client satisfies [API].
var _ API = (*Client)(nil)
