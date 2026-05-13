// Package money надає типізоване представлення грошових сум разом із
// валютою — щоб не плутати «10 копійок» і «10 центів» у тому самому
// int64.
//
// Реалізація — пара (Minor, Code), де Minor — мінорна одиниця валюти
// (копійки для UAH, центи для USD/EUR), а Code — ISO 4217 числовий
// код. У JSON [Money] серіалізується як гола кількість мінорних
// одиниць (для wire-сумісності з Mono-форматом, де amount і currency
// йдуть окремими полями) — батьківські структури проставляють Code з
// сусіднього CurrencyCode-поля у своєму [json.Unmarshaler].
package money

import (
	"encoding/json"
	"fmt"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
)

// Money — сума у мінорних одиницях (копійки, центи) з прив'язкою до
// валюти. Нульове значення — 0 у невідомій валюті.
type Money struct {
	Minor int64
	Code  currency.Code
}

// New створює Money з кількістю мінорних одиниць і валютою.
func New(minor int64, code currency.Code) Money {
	return Money{Minor: minor, Code: code}
}

// IsZero повертає true, якщо сума — 0 (валюта ігнорується).
func (m Money) IsZero() bool { return m.Minor == 0 }

// Equal повідомляє, чи два Money рівні і за сумою, і за валютою.
func (m Money) Equal(other Money) bool {
	return m.Minor == other.Minor && m.Code == other.Code
}

// Add додає інше Money тієї ж валюти. Повертає помилку, якщо валюти
// відрізняються (для крос-валютної арифметики використовуй
// [bank.Rates.Convert]).
func (m Money) Add(other Money) (Money, error) {
	if m.Code != other.Code {
		return Money{}, fmt.Errorf("money: cannot add %s + %s — different currencies", m.Code, other.Code)
	}
	return Money{Minor: m.Minor + other.Minor, Code: m.Code}, nil
}

// Sub віднімає інше Money тієї ж валюти. Помилка — як в [Money.Add].
func (m Money) Sub(other Money) (Money, error) {
	if m.Code != other.Code {
		return Money{}, fmt.Errorf("money: cannot sub %s - %s — different currencies", m.Code, other.Code)
	}
	return Money{Minor: m.Minor - other.Minor, Code: m.Code}, nil
}

// Neg повертає Money з протилежним знаком (валюта не змінюється).
func (m Money) Neg() Money { return Money{Minor: -m.Minor, Code: m.Code} }

// Mul множить суму на цілий скаляр. Корисно для агрегатів «10 одиниць
// по такій-то ціні». Для дробових множників (комісії, відсотки)
// використовуй [Money.Scale].
func (m Money) Mul(n int64) Money { return Money{Minor: m.Minor * n, Code: m.Code} }

// Scale множить суму на дробовий множник (наприклад 0.05 для 5%
// комісії), округлюючи до найближчої мінорної одиниці (banker's rounding
// не застосовується — звичайне «round half away from zero»).
func (m Money) Scale(factor float64) Money {
	r := float64(m.Minor) * factor
	if r >= 0 {
		return Money{Minor: int64(r + 0.5), Code: m.Code}
	}
	return Money{Minor: int64(r - 0.5), Code: m.Code}
}

// Major повертає суму у мажорних одиницях (грн/USD/EUR), припускаючи
// 2 знаки після коми (виключення на кшталт JPY/KRW поки не
// підтримуються — обробляй вручну через .Minor).
func (m Money) Major() float64 { return float64(m.Minor) / 100 }

// String форматує гроші у вигляді «42.50 UAH» (2 знаки після коми + ISO
// alpha-3 код). Корисно для логів і дебагу.
func (m Money) String() string {
	return fmt.Sprintf("%.2f %s", m.Major(), m.Code)
}

// MarshalJSON серіалізує Money як голу кількість мінорних одиниць —
// сумісно з wire-форматом Mono (де сума і валюта йдуть окремими полями).
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.Minor)
}

// UnmarshalJSON читає голу int64 у Minor. Code лишається нульовим — його
// проставляє батьківська структура у власному UnmarshalJSON, копіюючи
// зі сусіднього поля CurrencyCode.
func (m *Money) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &m.Minor)
}
