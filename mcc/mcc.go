// Package mcc — типізовані хелпери для ISO 18245 Merchant Category Code,
// що приходять у payload-ах виписки. Mono віддає просто int; [Code.Category]
// групує його у бакет, який реально хочеться бачити у звітах (Groceries
// проти Restaurants, Fuel проти Transport тощо).
package mcc

// Code — ISO 18245 Merchant Category Code. Транзакції в виписці несуть
// його у полях Transaction.MCC і Transaction.OriginalMCC.
type Code int

// Category — група верхнього рівня, до якої належить MCC. Підбір
// гранулярності — практичний для PFM-звітів: окремо Groceries / Fuel /
// Restaurants замість одного загального «Retail».
type Category string

// Відомі категорії. Список не вичерпний — додавай за потребою.
const (
	CategoryUnknown      Category = "Unknown"
	CategoryAgriculture  Category = "Agriculture"
	CategoryContracted   Category = "ContractedServices"
	CategoryTransport    Category = "Transport"
	CategoryFuel         Category = "Fuel"
	CategoryUtilities    Category = "Utilities"
	CategoryRetail       Category = "Retail"
	CategoryGroceries    Category = "Groceries"
	CategoryClothing     Category = "Clothing"
	CategoryEntertain    Category = "Entertainment"
	CategoryRestaurants  Category = "Restaurants"
	CategoryHotels       Category = "Hotels"
	CategoryHealth       Category = "Health"
	CategoryEducation    Category = "Education"
	CategoryProfessional Category = "ProfessionalServices"
	CategoryFinancial    Category = "Financial"
	CategoryTransfer     Category = "MoneyTransfer"
	CategoryGovernment   Category = "Government"
	CategoryTelecom      Category = "Telecom"
	CategoryCharity      Category = "Charity"
)

// Category повертає високорівневий бакет для MCC згідно з таблицями
// діапазонів ISO 18245. Невідомі коди повертають [CategoryUnknown].
//
// Конкретні коди (наприклад 5411 — продуктові магазини) матчаться раніше
// за діапазон, що їх містить (first-match wins) — так grocery не
// перетворюється на загальний Retail.
func (c Code) Category() Category {
	switch {
	// --- specific codes that override their containing range ---
	case c == 4829:
		return CategoryTransfer
	case c == 4812, c == 4813, c == 4814, c == 4816, c == 4821, c == 4899:
		return CategoryTelecom
	case c == 5411, c == 5422, c == 5441, c == 5451, c == 5462, c == 5499:
		return CategoryGroceries
	case c == 5541, c == 5542, c == 5552, c == 5983:
		return CategoryFuel
	case c >= 5811 && c <= 5814:
		return CategoryRestaurants
	case c == 8398:
		return CategoryCharity

	// --- broad ranges ---
	case c >= 1 && c <= 1499:
		return CategoryAgriculture
	case c >= 1500 && c <= 2999:
		return CategoryContracted
	case c >= 3000 && c <= 3999:
		return CategoryTransport // airlines + car-rental
	case c >= 4000 && c <= 4799:
		return CategoryTransport
	case c >= 4900 && c <= 4999:
		return CategoryUtilities
	case c >= 5000 && c <= 5599:
		return CategoryRetail
	case c >= 5600 && c <= 5699:
		return CategoryClothing
	case c >= 5700 && c <= 5999:
		return CategoryRetail
	case c >= 6000 && c <= 6999:
		return CategoryFinancial
	case c >= 7000 && c <= 7299:
		return CategoryHotels
	case c >= 7800 && c <= 7999:
		return CategoryEntertain
	case c >= 8000 && c <= 8099:
		return CategoryHealth
	case c >= 8200 && c <= 8299:
		return CategoryEducation
	case c >= 8300 && c <= 8999:
		return CategoryProfessional
	case c >= 9000 && c <= 9999:
		return CategoryGovernment
	}
	return CategoryUnknown
}
