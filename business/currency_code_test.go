package business_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/business"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/currency"
)

// corp-api непослідовно шле currencyCode: у дикому вигляді бачили
// alpha-3 ("UAH"), а специфікація документує числовий код ("980").
// UnmarshalJSON має привʼязати Code до Amount в обох випадках.
func TestStatementItem_currencyCode_bothFormats(t *testing.T) {
	for _, wire := range []string{`"UAH"`, `"980"`} {
		var it business.StatementItem
		err := json.Unmarshal([]byte(`{"id":"1","amount":12345,"currencyCode":`+wire+`}`), &it)
		require.NoError(t, err, "wire=%s", wire)
		assert.Equal(t, currency.UAH, it.Amount.Code, "wire=%s", wire)
		assert.Equal(t, int64(12345), it.Amount.Minor, "wire=%s", wire)
	}
}

func TestStatementItem_currencyCode_unknownStaysZero(t *testing.T) {
	var it business.StatementItem
	err := json.Unmarshal([]byte(`{"id":"1","amount":100,"currencyCode":"XYZ"}`), &it)
	require.NoError(t, err)
	assert.Equal(t, currency.Code(0), it.Amount.Code)
	assert.Equal(t, int64(100), it.Amount.Minor) // сума все одно розпарсена
}
