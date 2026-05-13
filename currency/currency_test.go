package currency

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCode_String(t *testing.T) {
	tests := map[Code]string{
		UAH:        "UAH",
		USD:        "USD",
		EUR:        "EUR",
		GBP:        "GBP",
		PLN:        "PLN",
		Code(7777): "7777", // unknown — falls back to decimal
	}
	for code, want := range tests {
		assert.Equalf(t, want, code.String(), "String() for %d", int(code))
	}
}
