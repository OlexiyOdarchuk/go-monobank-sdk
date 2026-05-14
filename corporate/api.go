package corporate

import (
	"context"
	"iter"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// API — інтерфейс Corporate Open API клієнта (включно з monoКЕП).
// Існує окремо від *[Client], щоб користувачі могли мокувати його
// через mockgen/testify-mock у власних тестах. Сам [Client] цей
// інтерфейс реалізує (перевіряється compile-time assert-ом нижче).
type API interface {
	// Авторизація і налаштування компанії.
	Auth(ctx context.Context, callbackURL string, permissions ...auth.Permission) (*TokenRequest, error)
	CheckAuth(ctx context.Context, requestID string) error
	Register(ctx context.Context, in *RegistrationRequest) (*RegistrationResponse, error)
	RegistrationStatus(ctx context.Context, pubkeyPEM []byte) (*RegistrationStatusResponse, error)
	GetSettings(ctx context.Context) (*Settings, error)
	SetWebHook(ctx context.Context, uri string) error

	// Дані клієнта.
	ClientInfo(ctx context.Context, requestID string) (*bank.ClientInfo, error)
	Transactions(ctx context.Context, requestID, accountID string, from, to time.Time) (bank.Transactions, error)
	TransactionsRange(ctx context.Context, requestID, accountID string, from, to time.Time) (bank.Transactions, error)
	TransactionsRangeIter(ctx context.Context, requestID, accountID string, from, to time.Time) iter.Seq2[bank.Transaction, error]

	// monoКЕП.
	SignatureCreate(ctx context.Context, in *SignatureCreateRequest) (*SignatureCreateResponse, error)
	SignatureStatus(ctx context.Context, requestID string) (*SignatureStatusResponse, error)
	SignatureCancel(ctx context.Context, requestID string) error
}

// Compile-time assert: *Client задовольняє [API].
var _ API = (*Client)(nil)
