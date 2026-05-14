// Package mcc provides typed helpers for ISO 18245 Merchant Category
// Codes that arrive in statement payloads. Mono ships a bare int;
// [Code.Category] groups it into a bucket you actually want to see
// in reports (Groceries vs Restaurants, Fuel vs Transport, etc.).
package mcc

// Code is an ISO 18245 Merchant Category Code. Statement transactions
// carry it in the Transaction.MCC and Transaction.OriginalMCC fields.
type Code int

// Category is the top-level group an MCC belongs to. The granularity
// is chosen pragmatically for PFM reports: separate Groceries / Fuel
// / Restaurants instead of a single generic "Retail".
type Category string

// Known categories. The list is non-exhaustive — extend as needed.
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

// Category returns the high-level bucket for an MCC per the ISO 18245
// range tables. Unknown codes return [CategoryUnknown].
//
// Specific codes (for example 5411 — grocery stores) are matched
// before the range that contains them (first-match wins), so
// grocery does not get bucketed as generic Retail.
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
