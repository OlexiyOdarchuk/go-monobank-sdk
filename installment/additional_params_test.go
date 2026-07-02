package installment_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/installment"
)

// Раніше NDS/ExtInitialSum були значеннями Money (структура), тож
// omitempty не діяв і в payload завжди летіло "nds":"0.00" —
// хибне ПДВ, якого продавець не задавав. Тепер вони вказівники.
func TestCreateAdditionalParams_omitsUnsetMoney(t *testing.T) {
	b, err := json.Marshal(installment.CreateAdditionalParams{SellerPhone: "+380501234567"})
	require.NoError(t, err)
	assert.NotContains(t, string(b), "nds")
	assert.NotContains(t, string(b), "ext_initial_sum")
}

func TestCreateAdditionalParams_includesSetMoney(t *testing.T) {
	nds := installment.NewMoney(0, 50) // 0.50
	init := installment.NewMoney(100, 0)
	b, err := json.Marshal(installment.CreateAdditionalParams{
		NDS:           &nds,
		ExtInitialSum: &init,
	})
	require.NoError(t, err)
	assert.Contains(t, string(b), `"nds":0.50`)
	assert.Contains(t, string(b), `"ext_initial_sum":100.00`)
}

func TestReturnAdditionalParams_omitsUnsetNDS(t *testing.T) {
	b, err := json.Marshal(installment.ReturnAdditionalParams{})
	require.NoError(t, err)
	assert.Equal(t, "{}", string(b))
}

func TestOrderState_IsTerminal(t *testing.T) {
	assert.True(t, installment.StateSuccess.IsTerminal())
	assert.True(t, installment.StateFail.IsTerminal())
	assert.False(t, installment.StateInProcess.IsTerminal())
	assert.False(t, installment.OrderState("SOME_FUTURE_STATE").IsTerminal())

	assert.True(t, installment.StateSuccess.IsSuccess())
	assert.False(t, installment.StateFail.IsSuccess())
}

func TestParseCallback(t *testing.T) {
	body := []byte(`{"order_id":"ord-1","state":"SUCCESS","order_sub_state":"DONE"}`)
	info, err := installment.ParseCallback(body)
	require.NoError(t, err)
	assert.Equal(t, "ord-1", info.OrderID)
	assert.Equal(t, installment.StateSuccess, info.State)
	assert.Equal(t, installment.SubDone, info.OrderSubState)
	assert.True(t, info.State.IsTerminal())
}
