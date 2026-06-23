package bank

import (
	"errors"
	"fmt"

	"github.com/vtopc/epoch"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
)

// Rate is one row from /bank/currency: an exchange rate between two
// currencies. CurrencyCodeA and CurrencyCodeB are ISO 4217 numeric
// codes.
//
// RateSell and RateBuy are the sell and buy rates (two-way exchange).
// RateCross is the cross rate (used when no direct quote is
// available). When a pair only has RateCross, RateSell and RateBuy
// are 0.
//
// The name Rate (rather than Currency) was chosen to avoid confusion
// with [currency.Code] (the typed ISO 4217 numeric).
type Rate struct {
	CurrencyCodeA int           `json:"currencyCodeA"`
	CurrencyCodeB int           `json:"currencyCodeB"`
	Date          epoch.Seconds `json:"date"`
	RateSell      float64       `json:"rateSell"`
	RateBuy       float64       `json:"rateBuy"`
	RateCross     float64       `json:"rateCross"`
}

// Rates is a slice of [Rate].
type Rates []Rate

// ErrNoRate indicates that the list has no rate for the given pair
// (neither direct nor reverse).
var ErrNoRate = errors.New("bank: no rate for the requested currency pair")

// Convert converts amount into the to currency using the rates in
// this snapshot. If amount.Code == to, returns amount unchanged.
//
// Rate selection rules:
//
//   - Find a row that contains the pair (amount.Code, to) in either
//     direction.
//   - If amount.Code == CurrencyCodeA and to == CurrencyCodeB:
//     you are selling A for B → use RateBuy (the bank buys A); the
//     amount is multiplied.
//   - If amount.Code == CurrencyCodeB and to == CurrencyCodeA:
//     you are buying A for B → use RateSell (the bank sells A); the
//     amount is divided.
//   - If RateBuy/RateSell is zero (happens for less liquid pairs),
//     fall back to RateCross.
//
// Rounded to the nearest minor unit (via [money.Money.Scale]). If the
// pair is absent, returns [ErrNoRate].
//
// Limitation: the formula assumes the same number of decimal places
// in both currencies (2). For pairs with JPY/KRW (0 decimals) the
// result is off by a factor of 100 — handle that manually.
func (rs Rates) Convert(amount money.Money, to currency.Code) (money.Money, error) {
	if amount.Code == to {
		return amount, nil
	}

	for _, r := range rs {
		a := currency.Code(r.CurrencyCodeA)
		b := currency.Code(r.CurrencyCodeB)

		switch {
		case amount.Code == a && to == b:
			// Conversion A → B: multiply.
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
			// Conversion B → A: divide.
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
