package business

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

func newErrorClient(t *testing.T) *Client {
	t.Helper()
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errCode":"INTERNAL","errText":"boom"}`))
	})
	return c
}

func assertAPIError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var apiErr *monobank.APIError
	require.True(t, errors.As(err, &apiErr), "want *monobank.APIError, got %T", err)
	assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
}

func TestBusiness_errorPaths(t *testing.T) {
	ctx := context.Background()
	idKey := "00000000-0000-0000-0000-000000000000"

	t.Run("Accounts", func(t *testing.T) {
		_, err := newErrorClient(t).Accounts(ctx)
		assertAPIError(t, err)
	})
	t.Run("Account", func(t *testing.T) {
		_, err := newErrorClient(t).Account(ctx, "UA1")
		assertAPIError(t, err)
	})
	t.Run("AccountBalances", func(t *testing.T) {
		_, err := newErrorClient(t).AccountBalances(ctx, "UA1", "2026-01-01", "2026-02-01")
		assertAPIError(t, err)
	})
	t.Run("Statement", func(t *testing.T) {
		_, err := newErrorClient(t).Statement(ctx, "UA1", time.Unix(1, 0), time.Time{}, StatementDown, 0)
		assertAPIError(t, err)
	})
	t.Run("Operation", func(t *testing.T) {
		_, err := newErrorClient(t).Operation(ctx, "op-1", "")
		assertAPIError(t, err)
	})
	t.Run("Contacts", func(t *testing.T) {
		_, err := newErrorClient(t).Contacts(ctx, 10, 0)
		assertAPIError(t, err)
	})
	t.Run("SearchContacts", func(t *testing.T) {
		_, err := newErrorClient(t).SearchContacts(ctx, "q", 10, 0)
		assertAPIError(t, err)
	})
	t.Run("ContactByID", func(t *testing.T) {
		_, err := newErrorClient(t).ContactByID(ctx, "c1")
		assertAPIError(t, err)
	})
	t.Run("CreateContact", func(t *testing.T) {
		err := newErrorClient(t).CreateContact(ctx, &CreateContactRequest{FirstName: "X"})
		assertAPIError(t, err)
	})
	t.Run("DeleteContact", func(t *testing.T) {
		err := newErrorClient(t).DeleteContact(ctx, "c1")
		assertAPIError(t, err)
	})
	t.Run("DeleteContactsBatch", func(t *testing.T) {
		err := newErrorClient(t).DeleteContactsBatch(ctx, []string{"c1", "c2"})
		assertAPIError(t, err)
	})
	t.Run("PreparePayment", func(t *testing.T) {
		_, err := newErrorClient(t).PreparePayment(ctx, idKey, &PaymentRequest{Amount: 100})
		assertAPIError(t, err)
	})
	t.Run("PaymentState", func(t *testing.T) {
		_, err := newErrorClient(t).PaymentState(ctx, "p1")
		assertAPIError(t, err)
	})
	t.Run("PaymentStateByReference", func(t *testing.T) {
		_, err := newErrorClient(t).PaymentStateByReference(ctx, "ref-1")
		assertAPIError(t, err)
	})
	t.Run("CreateSalaryRegistry", func(t *testing.T) {
		_, err := newErrorClient(t).CreateSalaryRegistry(ctx, idKey, &CreateSalaryRegistryRequest{})
		assertAPIError(t, err)
	})
	t.Run("SalaryRegistryTypes", func(t *testing.T) {
		_, err := newErrorClient(t).SalaryRegistryTypes(ctx)
		assertAPIError(t, err)
	})
	t.Run("SalaryRegistryStatus", func(t *testing.T) {
		_, err := newErrorClient(t).SalaryRegistryStatus(ctx, "reg-1")
		assertAPIError(t, err)
	})
	t.Run("UploadPayslips", func(t *testing.T) {
		_, err := newErrorClient(t).UploadPayslips(ctx, &BatchPayslipRequest{Period: "2026-01"})
		assertAPIError(t, err)
	})
	t.Run("DeletePayslips", func(t *testing.T) {
		err := newErrorClient(t).DeletePayslips(ctx, &DeletePayslipsRequest{Period: "2026-01"})
		assertAPIError(t, err)
	})
	t.Run("ImportStatus", func(t *testing.T) {
		_, err := newErrorClient(t).ImportStatus(ctx, "2026-01")
		assertAPIError(t, err)
	})
	t.Run("DeleteImport", func(t *testing.T) {
		err := newErrorClient(t).DeleteImport(ctx, "2026-01")
		assertAPIError(t, err)
	})
	t.Run("SendPayslipsToMobile", func(t *testing.T) {
		_, err := newErrorClient(t).SendPayslipsToMobile(ctx, "2026-01")
		assertAPIError(t, err)
	})
	t.Run("PayslipPDF", func(t *testing.T) {
		_, err := newErrorClient(t).PayslipPDF(ctx, "1234567890", "2026-01")
		assertAPIError(t, err)
	})
}
