package acquiring_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
)

func TestNewBasketItem_ok(t *testing.T) {
	it, err := acquiring.NewBasketItem("Кава", "SKU-1", 2, 4500,
		acquiring.WithUnit("шт."), acquiring.WithBasketTax(20))
	require.NoError(t, err)
	assert.Equal(t, int64(9000), it.Total) // total = qty*sum
	assert.Equal(t, "шт.", it.Unit)
	assert.Equal(t, []int{20}, it.Tax)
}

func TestNewBasketItem_fractionalQty(t *testing.T) {
	// 1.5 кг по 29.99 грн = 44.985 -> округлення до 4499 копійок.
	it, err := acquiring.NewBasketItem("Яблука", "FRUIT", 1.5, 2999)
	require.NoError(t, err)
	assert.Equal(t, int64(4499), it.Total)
}

func TestNewBasketItem_validation(t *testing.T) {
	_, err := acquiring.NewBasketItem("", "C", 1, 100)
	require.ErrorIs(t, err, acquiring.ErrBasketName)

	_, err = acquiring.NewBasketItem("Name", "", 1, 100)
	require.ErrorIs(t, err, acquiring.ErrBasketCode)

	_, err = acquiring.NewBasketItem("Name", "C", 0, 100)
	require.ErrorIs(t, err, acquiring.ErrBasketQty)

	_, err = acquiring.NewBasketItem("Name", "C", 1, 0)
	require.ErrorIs(t, err, acquiring.ErrBasketSum)
}

func TestBasketItem_Validate_totalMismatch(t *testing.T) {
	it := acquiring.BasketItem{Name: "X", Code: "C", Qty: 2, Sum: 100, Total: 250}
	require.ErrorIs(t, it.Validate(), acquiring.ErrBasketTotal)

	// Коректний total проходить.
	it.Total = 200
	require.NoError(t, it.Validate())
}

func TestBasket_Build_ok(t *testing.T) {
	b := acquiring.NewBasket().
		AddItem("Кава", "SKU-1", 2, 4500).
		AddItem("Чай", "SKU-2", 1, 3000)

	assert.Equal(t, 2, b.Len())
	assert.Equal(t, int64(12000), b.Total())

	items, err := b.Build(12000) // total збігається з amount інвойсу
	require.NoError(t, err)
	require.Len(t, items, 2)
}

func TestBasket_Build_amountMismatch(t *testing.T) {
	b := acquiring.NewBasket().AddItem("Кава", "SKU-1", 1, 4500)
	_, err := b.Build(9999)
	require.ErrorIs(t, err, acquiring.ErrBasketTotal)
}

func TestBasket_firstErrorWins(t *testing.T) {
	b := acquiring.NewBasket().
		AddItem("ok", "C1", 1, 100).
		AddItem("bad", "", 1, 100). // no code
		AddItem("ok2", "C2", 1, 100)
	require.ErrorIs(t, b.Err(), acquiring.ErrBasketCode)
	_, err := b.Items()
	require.ErrorIs(t, err, acquiring.ErrBasketCode)
}

func TestBasket_withMerchantPaymInfo(t *testing.T) {
	// Інтеграція з реальною структурою запиту.
	items, err := acquiring.NewBasket().
		AddItem("Товар", "SKU", 1, money.UAH(100).Minor). // 10000 копійок
		Build(0)
	require.NoError(t, err)
	info := &acquiring.MerchantPaymInfo{BasketOrder: items}
	assert.Equal(t, int64(10000), info.BasketOrder[0].Total)
}
