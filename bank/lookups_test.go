package bank_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/bank"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/currency"
)

func sampleClient() bank.ClientInfo {
	return bank.ClientInfo{
		Accounts: bank.Accounts{
			{AccountID: "acc-uah", Currency: currency.UAH, IBAN: "UA111"},
			{AccountID: "acc-usd", Currency: currency.USD, IBAN: "UA222"},
			{AccountID: "acc-fop", Currency: currency.UAH, IBAN: "UA333"},
		},
		Jars: bank.Jars{
			{ID: "jar-1", Title: "Відпустка"},
			{ID: "jar-2", Title: "Подушка"},
		},
	}
}

func TestAccounts_ByID_ByIBAN(t *testing.T) {
	c := sampleClient()

	a, ok := c.Account("acc-usd")
	require.True(t, ok)
	assert.Equal(t, currency.USD, a.Currency)

	a, ok = c.Accounts.ByIBAN("UA333")
	require.True(t, ok)
	assert.Equal(t, "acc-fop", a.AccountID)

	_, ok = c.Account("missing")
	assert.False(t, ok)
}

func TestAccounts_ByCurrency(t *testing.T) {
	c := sampleClient()
	uah := c.Accounts.ByCurrency(currency.UAH)
	assert.Len(t, uah, 2) // black + fop
	assert.Empty(t, c.Accounts.ByCurrency(currency.EUR))
}

func TestClientInfo_Jar(t *testing.T) {
	c := sampleClient()
	j, ok := c.Jar("jar-2")
	require.True(t, ok)
	assert.Equal(t, "Подушка", j.Title)

	_, ok = c.Jar("nope")
	assert.False(t, ok)
}

// Вказівник аліасить елемент зрізу — правка через нього видима в зрізі.
func TestAccounts_ByID_aliasesSlice(t *testing.T) {
	c := sampleClient()
	a, ok := c.Account("acc-uah")
	require.True(t, ok)
	a.SendID = "changed"
	assert.Equal(t, "changed", c.Accounts[0].SendID)
}
