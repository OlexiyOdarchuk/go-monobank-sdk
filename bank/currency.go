package bank

import (
	"errors"
	"fmt"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
	"github.com/vtopc/epoch"
)

// Rate — один рядок із /bank/currency: курс обміну між двома валютами.
// CurrencyCodeA та CurrencyCodeB — числові коди за ISO 4217.
//
// RateSell і RateBuy — курси продажу та купівлі (двосторонній обмін).
// RateCross — кросс-курс (вживається, коли пряма котировка відсутня).
// Якщо для пари є тільки RateCross — RateSell і RateBuy будуть 0.
//
// Назву обрано Rate (а не Currency), щоб не плутати з [currency.Code]
// (типізований ISO 4217 numeric).
type Rate struct {
	CurrencyCodeA int           `json:"currencyCodeA"`
	CurrencyCodeB int           `json:"currencyCodeB"`
	Date          epoch.Seconds `json:"date"`
	RateSell      float64       `json:"rateSell"`
	RateBuy       float64       `json:"rateBuy"`
	RateCross     float64       `json:"rateCross"`
}

// Rates — slice типу [Rate].
type Rates []Rate

// ErrNoRate — у списку немає курсу між зазначеною парою валют (ні
// прямого, ні зворотного).
var ErrNoRate = errors.New("bank: no rate for the requested currency pair")

// Convert конвертує amount у валюту to за курсами з цієї виписки.
// Якщо amount.Code == to — повертає amount без змін.
//
// Правила підбору курсу:
//
//   - Шукаємо рядок, що містить пару (amount.Code, to) у будь-якому
//     напрямку.
//   - Якщо amount.Code == CurrencyCodeA, а to == CurrencyCodeB:
//     ти продаєш A за B → користуємо RateBuy (банк купує A); сума множиться.
//   - Якщо amount.Code == CurrencyCodeB, а to == CurrencyCodeA:
//     ти купуєш A за B → користуємо RateSell (банк продає A); сума ділиться.
//   - Якщо RateBuy/RateSell нульовий (буває для менш активних пар) —
//     fallback на RateCross.
//
// Округлення — до найближчої мінорної одиниці (через [money.Money.Scale]).
// Якщо пари немає — повертає [ErrNoRate].
//
// Обмеження: формула припускає однакову кількість десяткових місць у
// обох валютах (2). Для пар із JPY/KRW (0 десяткових) результат буде
// зі зсувом на множник 100 — обробляй вручну.
func (rs Rates) Convert(amount money.Money, to currency.Code) (money.Money, error) {
	if amount.Code == to {
		return amount, nil
	}

	for _, r := range rs {
		a := currency.Code(r.CurrencyCodeA)
		b := currency.Code(r.CurrencyCodeB)

		switch {
		case amount.Code == a && to == b:
			// Конвертація A → B: множимо.
			rate := r.RateBuy
			if rate == 0 {
				rate = r.RateCross
			}
			if rate == 0 {
				continue
			}
			out := amount.Scale(rate)
			out.Code = to
			return out, nil

		case amount.Code == b && to == a:
			// Конвертація B → A: ділимо.
			rate := r.RateSell
			if rate == 0 {
				rate = r.RateCross
			}
			if rate == 0 {
				continue
			}
			out := amount.Scale(1.0 / rate)
			out.Code = to
			return out, nil
		}
	}
	return money.Money{}, fmt.Errorf("%w: %s → %s", ErrNoRate, amount.Code, to)
}
