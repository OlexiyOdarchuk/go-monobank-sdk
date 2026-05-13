package business

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- salary contacts: missing endpoints ---

func TestSearchContacts(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ext/v1/salary-contacts/search", r.URL.Path)
		assert.Equal(t, "Петренко", r.URL.Query().Get("query"))
		assert.Equal(t, "20", r.URL.Query().Get("limit"))
		_, _ = w.Write([]byte(`{"hasMore":false,"contacts":[{"id":"c1","fullName":"Петренко П."}]}`))
	})

	out, err := c.SearchContacts(context.Background(), "Петренко", 20, 0)
	require.NoError(t, err)
	assert.False(t, out.HasMore)
	require.Len(t, out.Contacts, 1)
	assert.Equal(t, "Петренко П.", out.Contacts[0].FullName)
}

func TestContactByID(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ext/v1/salary-contacts/abc-123", r.URL.Path)
		_, _ = w.Write([]byte(`{"id":"abc-123","fullName":"X","iban":"UA1","documentType":"ID_CARD"}`))
	})

	out, err := c.ContactByID(context.Background(), "abc-123")
	require.NoError(t, err)
	assert.Equal(t, "abc-123", out.ID)
	assert.Equal(t, IDCard, out.DocumentType)
}

func TestDeleteContact(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/ext/v1/salary-contacts/abc-123", r.URL.Path)
		_, _ = w.Write([]byte(`{}`))
	})

	require.NoError(t, c.DeleteContact(context.Background(), "abc-123"))
}

// --- accounts: single by IBAN ---

func TestAccount_single(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ext/v1/accounts/UA293220010000026", r.URL.Path)
		_, _ = w.Write([]byte(`{"iban":"UA293220010000026","currency":980,"balance":1000.50}`))
	})

	out, err := c.Account(context.Background(), "UA293220010000026")
	require.NoError(t, err)
	assert.Equal(t, "UA293220010000026", out.IBAN)
	assert.InDelta(t, 1000.50, out.Balance, 1e-9)
}

// --- salary registries: 2 of 3 untested endpoints ---

func TestCreateSalaryRegistry_setsIdempotencyKey(t *testing.T) {
	var seenKey string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenKey = r.Header.Get("Idempotency-Key")
		body, _ := io.ReadAll(r.Body)
		var got CreateSalaryRegistryRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "August 2026", got.RegistryName)
		require.Len(t, got.Recipients, 1)
		assert.Equal(t, int64(50000), got.Recipients[0].Amount)
		_, _ = w.Write([]byte(`{"id":"reg-1","state":"SAVED","createTimestamp":"2026-08-15T10:30:00Z"}`))
	})

	out, err := c.CreateSalaryRegistry(context.Background(), "uniq-key-42",
		&CreateSalaryRegistryRequest{
			RegistryName:       "August 2026",
			SenderIBAN:         "UA1",
			SalaryRegistryType: "SALARY",
			From:               "2026-08-01",
			To:                 "2026-08-31",
			Recipients: []SalaryRecipient{
				{FullName: "Петренко П.", IBAN: "UA2", Amount: 50000},
			},
		})
	require.NoError(t, err)
	assert.Equal(t, "uniq-key-42", seenKey)
	assert.Equal(t, "reg-1", out.ID)
}

func TestCreateSalaryRegistry_nil(t *testing.T) {
	c := New("x")
	_, err := c.CreateSalaryRegistry(context.Background(), "k", nil)
	assert.ErrorIs(t, err, ErrNilRequest)
}

func TestSalaryRegistryStatus(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ext/v1/payments/salary/registries/reg-9/status", r.URL.Path)
		_, _ = w.Write([]byte(`{"status":"FAIL","updatedAt":"2026-08-15T11:45:00Z","declineReason":"insufficient funds"}`))
	})

	out, err := c.SalaryRegistryStatus(context.Background(), "reg-9")
	require.NoError(t, err)
	assert.Equal(t, RegistryFail, out.Status)
	assert.Equal(t, "insufficient funds", out.DeclineReason)
}

// --- statement: single operation lookup ---

func TestOperation(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ext/v1/statement", r.URL.Path)
		assert.Equal(t, "op-1", r.URL.Query().Get("id"))
		assert.Equal(t, "ext-42", r.URL.Query().Get("externalReference"))
		_, _ = w.Write([]byte(`{"id":"op-1","externalReference":"ext-42","amount":-12345,"status":"DONE","currencyCode":"UAH"}`))
	})

	out, err := c.Operation(context.Background(), "op-1", "ext-42")
	require.NoError(t, err)
	assert.Equal(t, "op-1", out.ID)
	assert.Equal(t, OperationDone, out.Status)
	assert.Equal(t, int64(-12345), out.Amount.Minor)
}

// --- payments: single PaymentState by id ---

func TestPaymentState_byID(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ext/v1/payment/pmt-7/state", r.URL.Path)
		_, _ = w.Write([]byte(`{"id":"pmt-7","state":"DRAFT"}`))
	})

	out, err := c.PaymentState(context.Background(), "pmt-7")
	require.NoError(t, err)
	assert.Equal(t, PaymentDraft, out.State)
}

// --- payslips: 4 missing endpoints ---

func TestDeletePayslips(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/ext/v1/payslips/batch", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var got DeletePayslipsRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "2026-01", got.Period)
		assert.Equal(t, []string{"1234567890", "0987654321"}, got.Identifications)
		_, _ = w.Write([]byte(`{}`))
	})

	require.NoError(t, c.DeletePayslips(context.Background(), &DeletePayslipsRequest{
		Period:          "2026-01",
		Identifications: []string{"1234567890", "0987654321"},
	}))
}

func TestImportStatus_unwrapsResult(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ext/v1/payslip-imports/status", r.URL.Path)
		assert.Equal(t, "2026-01", r.URL.Query().Get("period"))
		_, _ = w.Write([]byte(`{"result":{"period":"2026-01","status":"LOADED","totalEmployees":10,"totalSuccessEmployees":9,"totalFailedEmployees":1,"failedEmployees":[{"identification":"X","reason":"CONTACT_NOT_FOUND"}],"createdAt":"a","updatedAt":"b"}}`))
	})

	out, err := c.ImportStatus(context.Background(), "2026-01")
	require.NoError(t, err)
	assert.Equal(t, ImportLoaded, out.Status)
	assert.Equal(t, 10, out.TotalEmployees)
	require.Len(t, out.FailedEmployees, 1)
	assert.Equal(t, ContactNotFound, out.FailedEmployees[0].Reason)
}

func TestDeleteImport(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/ext/v1/payslip-imports", r.URL.Path)
		assert.Equal(t, "2026-01", r.URL.Query().Get("period"))
		_, _ = w.Write([]byte(`{}`))
	})

	require.NoError(t, c.DeleteImport(context.Background(), "2026-01"))
}

func TestSendPayslipsToMobile_unwrapsResult(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/ext/v1/payslip-imports/send", r.URL.Path)
		assert.Equal(t, "2026-01", r.URL.Query().Get("period"))
		_, _ = w.Write([]byte(`{"result":{"period":"2026-01","status":"SENT","employeesSent":42}}`))
	})

	out, err := c.SendPayslipsToMobile(context.Background(), "2026-01")
	require.NoError(t, err)
	assert.Equal(t, "SENT", out.Status)
	assert.Equal(t, 42, out.EmployeesSent)
}
