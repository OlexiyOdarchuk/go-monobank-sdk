package acquiring

import (
	"errors"
	"fmt"
	"math"
)

// Basket-validation errors. All are wrapped with the offending
// item's name/index for context; match the cause with errors.Is.
var (
	// ErrBasketName indicates a basket line with an empty Name.
	ErrBasketName = errors.New("acquiring: basket item name is required")
	// ErrBasketCode indicates a basket line with an empty Code. This
	// is the single most common acquiring mistake: fiscalization
	// silently fails downstream when code is missing, so it is
	// rejected up front.
	ErrBasketCode = errors.New("acquiring: basket item code is required")
	// ErrBasketQty indicates a non-positive Qty.
	ErrBasketQty = errors.New("acquiring: basket item qty must be > 0")
	// ErrBasketSum indicates a non-positive per-unit Sum.
	ErrBasketSum = errors.New("acquiring: basket item sum must be > 0")
	// ErrBasketTotal indicates Total != round(Qty * Sum).
	ErrBasketTotal = errors.New("acquiring: basket item total must equal qty*sum")
)

// expectedTotal returns round(qty * sum) in minor units, the value
// Mono expects in BasketItem.Total. Rounding is half-away-from-zero,
// matching [money.Money.Scale], so fractional quantities (1.5 kg at
// 2999/kg) land on a deterministic kopeck.
func expectedTotal(qty float64, sum int64) int64 {
	r := qty * float64(sum)
	if r >= 0 {
		return int64(r + 0.5)
	}
	return int64(r - 0.5)
}

// Validate checks a single [BasketItem] against Mono's invariants:
// Name and Code are present, Qty and Sum are positive, and — when
// Total is set — Total equals round(Qty*Sum). A zero Total is treated
// as "not specified" and is not cross-checked (use [BasketItem.Filled]
// to populate it). Returns nil when the item is acceptable.
func (it BasketItem) Validate() error {
	if it.Name == "" {
		return ErrBasketName
	}
	if it.Code == "" {
		return fmt.Errorf("%w (item %q)", ErrBasketCode, it.Name)
	}
	if it.Qty <= 0 || math.IsNaN(it.Qty) || math.IsInf(it.Qty, 0) {
		return fmt.Errorf("%w (item %q): %v", ErrBasketQty, it.Name, it.Qty)
	}
	if it.Sum <= 0 {
		return fmt.Errorf("%w (item %q): %d", ErrBasketSum, it.Name, it.Sum)
	}
	if it.Total != 0 {
		if want := expectedTotal(it.Qty, it.Sum); it.Total != want {
			return fmt.Errorf("%w (item %q): total=%d, qty*sum=%d", ErrBasketTotal, it.Name, it.Total, want)
		}
	}
	return nil
}

// Filled returns a copy of the item with Total set to round(Qty*Sum)
// when it was left at zero. Handy right before sending so the wire
// payload always carries an explicit, consistent Total.
func (it BasketItem) Filled() BasketItem {
	if it.Total == 0 {
		it.Total = expectedTotal(it.Qty, it.Sum)
	}
	return it
}

// NewBasketItem builds a validated [BasketItem] from the four fields
// Mono always requires — name, code, qty, and per-unit sum (in minor
// units) — computing Total as round(qty*sum). Use [BasketOption]
// values for the optional fiscalization metadata (unit, barcode,
// tax, ...). It returns an error instead of a half-built item when a
// required field is missing or non-positive.
//
//	it, err := acquiring.NewBasketItem("Кава", "SKU-1", 2, money.UAH(45).Minor,
//	    acquiring.WithUnit("шт."), acquiring.WithBasketTax(20))
func NewBasketItem(name, code string, qty float64, unitSum int64, opts ...BasketOption) (BasketItem, error) {
	it := BasketItem{Name: name, Code: code, Qty: qty, Sum: unitSum}
	for _, o := range opts {
		o(&it)
	}
	it = it.Filled()
	if err := it.Validate(); err != nil {
		return BasketItem{}, err
	}
	return it, nil
}

// BasketOption sets an optional field on a [BasketItem] built via
// [NewBasketItem] or [Basket.Add].
type BasketOption func(*BasketItem)

// WithUnit sets the unit label (for example "шт.", "кг").
func WithUnit(unit string) BasketOption { return func(it *BasketItem) { it.Unit = unit } }

// WithIcon sets the product image URL.
func WithIcon(url string) BasketOption { return func(it *BasketItem) { it.Icon = url } }

// WithBarcode sets the product barcode.
func WithBarcode(code string) BasketOption { return func(it *BasketItem) { it.Barcode = code } }

// WithUKTZED sets the UKT ZED classification code.
func WithUKTZED(code string) BasketOption { return func(it *BasketItem) { it.UKTZED = code } }

// WithBasketTax sets the fiscal tax-rate codes (from the Checkbox
// portal) for the line.
func WithBasketTax(tax ...int) BasketOption { return func(it *BasketItem) { it.Tax = tax } }

// WithBasketHeader sets the fiscalization header text shown before
// the product name.
func WithBasketHeader(s string) BasketOption { return func(it *BasketItem) { it.Header = s } }

// WithBasketFooter sets the fiscalization footer text shown after
// the product name.
func WithBasketFooter(s string) BasketOption { return func(it *BasketItem) { it.Footer = s } }

// WithBasketDiscounts attaches line-level discounts/surcharges.
func WithBasketDiscounts(d ...Adjustment) BasketOption {
	return func(it *BasketItem) { it.Discounts = d }
}

// Basket accumulates validated [BasketItem]s for a
// [MerchantPaymInfo.BasketOrder]. It collects items and surfaces the
// first validation failure, so call sites can chain Add and check
// once at the end. The zero value is ready to use; prefer
// [NewBasket] for clarity.
type Basket struct {
	items []BasketItem
	err   error
}

// NewBasket returns an empty [Basket].
func NewBasket() *Basket { return &Basket{} }

// Add appends a fully-formed item, validating it first. If the item
// is invalid, the error is retained and returned by [Basket.Items]
// / [Basket.Build]; subsequent Adds are ignored (first error wins),
// mirroring the strings.Builder / bufio.Scanner error idiom.
func (b *Basket) Add(it BasketItem) *Basket {
	if b.err != nil {
		return b
	}
	it = it.Filled()
	if err := it.Validate(); err != nil {
		b.err = err
		return b
	}
	b.items = append(b.items, it)
	return b
}

// AddItem is the shorthand for Add(NewBasketItem(...)). It applies
// the same required-field rules and option set.
func (b *Basket) AddItem(name, code string, qty float64, unitSum int64, opts ...BasketOption) *Basket {
	if b.err != nil {
		return b
	}
	it, err := NewBasketItem(name, code, qty, unitSum, opts...)
	if err != nil {
		b.err = err
		return b
	}
	b.items = append(b.items, it)
	return b
}

// Total returns the sum of every line's Total in minor units. It is
// meaningful only once the basket is error-free; on a basket that
// already holds an error it returns the partial total accumulated so
// far.
func (b *Basket) Total() int64 {
	var sum int64
	for _, it := range b.items {
		sum += it.Total
	}
	return sum
}

// Len returns the number of accepted items.
func (b *Basket) Len() int { return len(b.items) }

// Err returns the first validation error encountered, or nil.
func (b *Basket) Err() error { return b.err }

// Items returns the accumulated items, or the first validation
// error. The returned slice is a copy — callers may mutate it freely.
func (b *Basket) Items() ([]BasketItem, error) {
	if b.err != nil {
		return nil, b.err
	}
	out := make([]BasketItem, len(b.items))
	copy(out, b.items)
	return out, nil
}

// Build validates that the basket's line totals add up to wantTotal
// (the invoice Amount) and returns the items. Pass wantTotal = 0 to
// skip the cross-check and just return the items. A mismatch is the
// classic "basket doesn't equal the charge" rejection from Mono;
// catching it locally saves a round-trip.
func (b *Basket) Build(wantTotal int64) ([]BasketItem, error) {
	if b.err != nil {
		return nil, b.err
	}
	if wantTotal != 0 {
		if got := b.Total(); got != wantTotal {
			return nil, fmt.Errorf("%w: basket total=%d, invoice amount=%d", ErrBasketTotal, got, wantTotal)
		}
	}
	return b.Items()
}
