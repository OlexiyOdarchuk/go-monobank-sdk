package bank

import "context"

// API — інтерфейс публічних bank-endpoint-ів (без авторизації). Існує
// окремо від *[Client], щоб користувачі могли мокувати його через
// mockgen/testify-mock у власних тестах. Сам [Client] цей інтерфейс
// реалізує (перевіряється compile-time assert-ом нижче).
type API interface {
	Rates(ctx context.Context) (Rates, error)
	ServerKey(ctx context.Context) (*ServerKey, error)
}

// Compile-time assert: *Client задовольняє [API].
var _ API = (*Client)(nil)
