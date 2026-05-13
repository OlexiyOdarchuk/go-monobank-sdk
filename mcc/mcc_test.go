package mcc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCode_Category covers each branch in Category():
// the specific-code overrides (which must take priority over the
// containing range), the broad ranges, and the unknown-fallback.
func TestCode_Category(t *testing.T) {
	tests := map[Code]Category{
		// --- specific-code overrides ---
		// 4829 is in 4000-4799… no, it's actually >4800 so falls in
		// a gap; the override saves it from CategoryUnknown.
		4829: CategoryTransfer,

		// 481x/482x/489x — telecom carve-outs from the 4xxx transport range.
		4812: CategoryTelecom,
		4813: CategoryTelecom,
		4814: CategoryTelecom,
		4816: CategoryTelecom,
		4821: CategoryTelecom,
		4899: CategoryTelecom,

		// 54xx — groceries carve-outs from the 5000-5599 retail range.
		5411: CategoryGroceries,
		5422: CategoryGroceries,
		5441: CategoryGroceries,
		5451: CategoryGroceries,
		5462: CategoryGroceries,
		5499: CategoryGroceries,

		// 55xx + 5983 — fuel carve-outs.
		5541: CategoryFuel,
		5542: CategoryFuel,
		5552: CategoryFuel,
		5983: CategoryFuel,

		// 5811-5814 — restaurants carve-out from 5700-5999 retail.
		5811: CategoryRestaurants,
		5812: CategoryRestaurants,
		5813: CategoryRestaurants,
		5814: CategoryRestaurants,

		// 8398 — charity carve-out from 8300-8999 professional.
		8398: CategoryCharity,

		// --- broad ranges ---
		// Agriculture (1-1499): boundary 1, mid, boundary 1499.
		1:    CategoryAgriculture,
		750:  CategoryAgriculture,
		1499: CategoryAgriculture,

		// Contracted services (1500-2999).
		1500: CategoryContracted,
		2000: CategoryContracted,
		2999: CategoryContracted,

		// Transport — airlines + car rental (3000-3999).
		3000: CategoryTransport,
		3500: CategoryTransport,
		3999: CategoryTransport,

		// Transport — local (4000-4799), excluding the telecom carve-outs above.
		4000: CategoryTransport,
		4111: CategoryTransport,
		4799: CategoryTransport,

		// Utilities (4900-4999).
		4900: CategoryUtilities,
		4950: CategoryUtilities,
		4999: CategoryUtilities,

		// Retail (5000-5599), excluding grocery/fuel carve-outs.
		5000: CategoryRetail,
		5300: CategoryRetail,
		5599: CategoryRetail,

		// Clothing (5600-5699).
		5600: CategoryClothing,
		5650: CategoryClothing,
		5699: CategoryClothing,

		// Retail again (5700-5999), excluding restaurant carve-outs.
		5700: CategoryRetail,
		5900: CategoryRetail,
		5999: CategoryRetail,

		// Financial (6000-6999).
		6000: CategoryFinancial,
		6500: CategoryFinancial,
		6999: CategoryFinancial,

		// Hotels (7000-7299).
		7000: CategoryHotels,
		7100: CategoryHotels,
		7299: CategoryHotels,

		// Entertainment (7800-7999).
		7800: CategoryEntertain,
		7900: CategoryEntertain,
		7999: CategoryEntertain,

		// Health (8000-8099).
		8000: CategoryHealth,
		8011: CategoryHealth,
		8099: CategoryHealth,

		// Education (8200-8299).
		8200: CategoryEducation,
		8211: CategoryEducation,
		8299: CategoryEducation,

		// Professional (8300-8999), excluding the 8398 charity carve-out.
		8300: CategoryProfessional,
		8500: CategoryProfessional,
		8999: CategoryProfessional,

		// Government (9000-9999).
		9000: CategoryGovernment,
		9311: CategoryGovernment,
		9999: CategoryGovernment,

		// --- unknown fallbacks ---
		0:     CategoryUnknown, // below all ranges
		7300:  CategoryUnknown, // gap between Hotels (7299) and Entertain (7800)
		7799:  CategoryUnknown,
		8100:  CategoryUnknown, // gap between Health (8099) and Education (8200)
		8199:  CategoryUnknown,
		10000: CategoryUnknown, // above all ranges
		-1:    CategoryUnknown, // negative
	}
	for code, want := range tests {
		assert.Equalf(t, want, code.Category(), "MCC %d", int(code))
	}
}

// TestCode_Category_specificOverridesRange verifies that the override
// cases for the carve-outs come first — e.g. 5411 is in 5000-5599
// (which would map to Retail) but the specific override puts it in
// Groceries. If someone re-orders Category() and breaks this, the
// regression shows up here.
func TestCode_Category_specificOverridesRange(t *testing.T) {
	// 5411 vs neighbour 5410 (not a carve-out)
	assert.Equal(t, CategoryGroceries, Code(5411).Category())
	assert.Equal(t, CategoryRetail, Code(5410).Category())

	// 5541 vs neighbour 5540
	assert.Equal(t, CategoryFuel, Code(5541).Category())
	assert.Equal(t, CategoryRetail, Code(5540).Category())

	// 8398 vs neighbour 8399
	assert.Equal(t, CategoryCharity, Code(8398).Category())
	assert.Equal(t, CategoryProfessional, Code(8399).Category())
}
