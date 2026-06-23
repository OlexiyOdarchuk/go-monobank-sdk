package acquiring_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/acquiring"
)

func TestClassifySubscription(t *testing.T) {
	cancelled := &acquiring.SubscriptionStatusResponse{Status: acquiring.SubscriptionCancelled}
	assert.Equal(t, acquiring.HealthCancelled, acquiring.ClassifySubscription(cancelled))

	walletDead := &acquiring.SubscriptionStatusResponse{
		Status:     acquiring.SubscriptionActive,
		WalletData: acquiring.SubscriptionWalletData{Status: acquiring.WalletFailed},
	}
	assert.Equal(t, acquiring.HealthWalletDead, acquiring.ClassifySubscription(walletDead))

	active := &acquiring.SubscriptionStatusResponse{
		Status:     acquiring.SubscriptionActive,
		WalletData: acquiring.SubscriptionWalletData{Status: acquiring.WalletCreated},
	}
	assert.Equal(t, acquiring.HealthActive, acquiring.ClassifySubscription(active))

	assert.Equal(t, acquiring.HealthUnknown, acquiring.ClassifySubscription(nil))
}

func TestClassifySubscription_cancelledBeatsWallet(t *testing.T) {
	// Скасування має пріоритет навіть якщо токен теж failed.
	st := &acquiring.SubscriptionStatusResponse{
		Status:     acquiring.SubscriptionCancelled,
		WalletData: acquiring.SubscriptionWalletData{Status: acquiring.WalletFailed},
	}
	assert.Equal(t, acquiring.HealthCancelled, acquiring.ClassifySubscription(st))
}

func TestClassifyCharge(t *testing.T) {
	// Wallet failed домінує над статусом платежу.
	assert.Equal(t, acquiring.HealthWalletDead,
		acquiring.ClassifyCharge(acquiring.SubscriptionPaymentFailed, acquiring.WalletFailed))

	// Платіж не пройшов, але токен живий -> тимчасово, дати grace.
	assert.Equal(t, acquiring.HealthChargeFailed,
		acquiring.ClassifyCharge(acquiring.SubscriptionPaymentFailed, acquiring.WalletCreated))

	assert.Equal(t, acquiring.HealthActive,
		acquiring.ClassifyCharge(acquiring.SubscriptionPaymentSuccess, ""))
	assert.Equal(t, acquiring.HealthActive,
		acquiring.ClassifyCharge(acquiring.SubscriptionPaymentNew, ""))
}

func TestSubscriptionHealth_decisions(t *testing.T) {
	assert.True(t, acquiring.HealthChargeFailed.GraceEligible())
	assert.False(t, acquiring.HealthActive.GraceEligible())

	assert.True(t, acquiring.HealthCancelled.Terminal())
	assert.True(t, acquiring.HealthWalletDead.Terminal())
	assert.False(t, acquiring.HealthChargeFailed.Terminal())

	// Grace-логіка: не відрізаємо доступ доки стан відновлюваний.
	assert.True(t, acquiring.HealthActive.RetainAccess())
	assert.True(t, acquiring.HealthChargeFailed.RetainAccess())
	assert.True(t, acquiring.HealthUnknown.RetainAccess())
	assert.False(t, acquiring.HealthCancelled.RetainAccess())
	assert.False(t, acquiring.HealthWalletDead.RetainAccess())
}
