package webhook

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// ErrUnknownType повертається з [Parse], коли поле "type" на верхньому
// рівні payload-у не входить у відомий список Type*-констант. Це не
// помилка верифікації — payload автентичний, просто його тип ще не
// представлений у SDK.
var ErrUnknownType = errors.New("unknown webhook type")

// Відомі типи webhook-подій.
const (
	// TypeStatementItem — один запис виписки по рахунку (єдиний тип,
	// який Mono документує станом на сьогодні).
	TypeStatementItem = "StatementItem"
)

// Response — payload вебхука Mono верхнього рівня.
type Response struct {
	Type string `json:"type"` // див. Type*-константи
	Data Data   `json:"data"`
}

// Data — корисне навантаження вебхука. Для TypeStatementItem
// Transaction містить повну транзакцію (як з /personal/statement), а
// AccountID — id рахунку, на якому вона сталась.
type Data struct {
	AccountID   string           `json:"account"`
	Transaction bank.Transaction `json:"statementItem"`
}

// Parse декодує сирий body вебхука у [Response]. Якщо "type" не входить
// у відомі Type*-константи — Response усе одно повертається (із
// розпарсеним type), але разом із [ErrUnknownType]. Так викликач може
// або проігнорувати такі події, або обробити їх загальним кодом.
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
