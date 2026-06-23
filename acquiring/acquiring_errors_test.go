package acquiring

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

// errorClient повертає 500 на будь-який запит — універсальний спосіб
// проштовхнути всі endpoint-и через error-гілку c.c.Do(...).
func errorClient(t *testing.T) *Client {
	t.Helper()
	return newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errCode":"INTERNAL","errText":"boom"}`))
	})
}

func assertAPIError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var apiErr *monobank.APIError
	require.True(t, errors.As(err, &apiErr), "want *monobank.APIError, got %T", err)
	assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
}

func TestAcquiring_errorPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("MerchantDetails", func(t *testing.T) {
		_, err := errorClient(t).MerchantDetails(ctx)
		assertAPIError(t, err)
	})
	t.Run("Employees", func(t *testing.T) {
		_, err := errorClient(t).Employees(ctx)
		assertAPIError(t, err)
	})
	t.Run("PubKey", func(t *testing.T) {
		_, err := errorClient(t).PubKey(ctx)
		assertAPIError(t, err)
	})
	t.Run("Submerchants", func(t *testing.T) {
		_, err := errorClient(t).Submerchants(ctx)
		assertAPIError(t, err)
	})
	t.Run("Statement", func(t *testing.T) {
		_, err := errorClient(t).Statement(ctx, time.Unix(1, 0), time.Time{}, "")
		assertAPIError(t, err)
	})
	t.Run("CreateInvoice", func(t *testing.T) {
		_, err := errorClient(t).CreateInvoice(ctx, &CreateInvoiceRequest{Amount: 100, Currency: 980})
		assertAPIError(t, err)
	})
	t.Run("InvoiceStatus", func(t *testing.T) {
		_, err := errorClient(t).InvoiceStatus(ctx, "i1")
		assertAPIError(t, err)
	})
	t.Run("CancelInvoice", func(t *testing.T) {
		_, err := errorClient(t).CancelInvoice(ctx, &CancelRequest{InvoiceID: "i1"})
		assertAPIError(t, err)
	})
	t.Run("FinalizeInvoice", func(t *testing.T) {
		_, err := errorClient(t).FinalizeInvoice(ctx, &FinalizeRequest{InvoiceID: "i1"})
		assertAPIError(t, err)
	})
	t.Run("RemoveInvoice", func(t *testing.T) {
		err := errorClient(t).RemoveInvoice(ctx, "i1")
		assertAPIError(t, err)
	})
	t.Run("FiscalChecks", func(t *testing.T) {
		_, err := errorClient(t).FiscalChecks(ctx, "i1")
		assertAPIError(t, err)
	})
	t.Run("QRList", func(t *testing.T) {
		_, err := errorClient(t).QRList(ctx)
		assertAPIError(t, err)
	})
	t.Run("QRDetails", func(t *testing.T) {
		_, err := errorClient(t).QRDetails(ctx, "Q1")
		assertAPIError(t, err)
	})
	t.Run("QRResetAmount", func(t *testing.T) {
		err := errorClient(t).QRResetAmount(ctx, "Q1")
		assertAPIError(t, err)
	})
	t.Run("Wallet", func(t *testing.T) {
		_, err := errorClient(t).Wallet(ctx, "w1")
		assertAPIError(t, err)
	})
	t.Run("DeleteCard", func(t *testing.T) {
		err := errorClient(t).DeleteCard(ctx, "tok")
		assertAPIError(t, err)
	})
	t.Run("WalletPayment", func(t *testing.T) {
		_, err := errorClient(t).WalletPayment(ctx, &WalletPaymentRequest{CardToken: "tok"})
		assertAPIError(t, err)
	})
	t.Run("MonoPayKeyImport", func(t *testing.T) {
		_, err := errorClient(t).MonoPayKeyImport(ctx, &MonoPayKeyImportRequest{KeyValue: "x"})
		assertAPIError(t, err)
	})
	t.Run("MonoPayKeyDelete", func(t *testing.T) {
		err := errorClient(t).MonoPayKeyDelete(ctx, "k1")
		assertAPIError(t, err)
	})
	t.Run("MonoPayKeyList", func(t *testing.T) {
		_, err := errorClient(t).MonoPayKeyList(ctx)
		assertAPIError(t, err)
	})
	t.Run("SplitReceivers", func(t *testing.T) {
		_, err := errorClient(t).SplitReceivers(ctx)
		assertAPIError(t, err)
	})
	t.Run("Terminals", func(t *testing.T) {
		_, err := errorClient(t).Terminals(ctx)
		assertAPIError(t, err)
	})
	t.Run("SubscriptionCreate", func(t *testing.T) {
		_, err := errorClient(t).SubscriptionCreate(ctx, &SubscriptionCreateRequest{Interval: "1m"})
		assertAPIError(t, err)
	})
	t.Run("SubscriptionEdit", func(t *testing.T) {
		err := errorClient(t).SubscriptionEdit(ctx, &SubscriptionEditRequest{SubscriptionID: "s"})
		assertAPIError(t, err)
	})
	t.Run("SubscriptionRemove", func(t *testing.T) {
		err := errorClient(t).SubscriptionRemove(ctx, "s")
		assertAPIError(t, err)
	})
	t.Run("SubscriptionStatus", func(t *testing.T) {
		_, err := errorClient(t).SubscriptionStatus(ctx, "s")
		assertAPIError(t, err)
	})
	t.Run("SubscriptionList", func(t *testing.T) {
		_, err := errorClient(t).SubscriptionList(ctx, SubscriptionListOptions{DateFrom: time.Now()})
		assertAPIError(t, err)
	})
	t.Run("SubscriptionPayments", func(t *testing.T) {
		_, err := errorClient(t).SubscriptionPayments(ctx, SubscriptionPaymentsOptions{
			SubscriptionID: "s", DateFrom: time.Now(),
		})
		assertAPIError(t, err)
	})
	t.Run("Receipt", func(t *testing.T) {
		_, err := errorClient(t).Receipt(ctx, "i1", "")
		assertAPIError(t, err)
	})
	t.Run("PaymentDirect", func(t *testing.T) {
		_, err := errorClient(t).PaymentDirect(ctx, &PaymentDirectRequest{CardData: CardData{PAN: "4111", Exp: "0130", CVV: "123"}})
		assertAPIError(t, err)
	})
}
