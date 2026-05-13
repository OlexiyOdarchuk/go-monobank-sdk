package business

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New("test-token", monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
	return c, srv
}

func TestTokenAuth_setsHeaders(t *testing.T) {
	var seenToken, seenAccept string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenToken = r.Header.Get("X-Token")
		seenAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	})

	_, err := c.Accounts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-token", seenToken)
	assert.Equal(t, "application/json", seenAccept)
}

func TestAccounts(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ext/v1/accounts", r.URL.Path)
		_, _ = w.Write([]byte(`[{"iban":"UA1","currency":980,"balance":42.5}]`))
	})

	got, err := c.Accounts(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "UA1", got[0].IBAN)
	assert.Equal(t, 980, got[0].Currency)
	assert.InDelta(t, 42.5, got[0].Balance, 1e-9)
}

func TestAccountBalances(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ext/v1/accounts/UA293220010000026/balances", r.URL.Path)
		assert.Equal(t, "2026-01-01", r.URL.Query().Get("dateFrom"))
		assert.Equal(t, "2026-01-31", r.URL.Query().Get("dateTo"))
		_, _ = w.Write([]byte(`[{"date":"2026-01-15","balance":10,"isFinal":true}]`))
	})

	out, err := c.AccountBalances(context.Background(), "UA293220010000026", "2026-01-01", "2026-01-31")
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.True(t, out[0].IsFinal)
}

func TestContacts_pagination(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ext/v1/salary-contacts", r.URL.Path)
		assert.Equal(t, "50", r.URL.Query().Get("limit"))
		assert.Equal(t, "100", r.URL.Query().Get("offset"))
		_, _ = w.Write([]byte(`{"hasMore":true,"contacts":[{"id":"a","fullName":"X"}]}`))
	})

	out, err := c.Contacts(context.Background(), 50, 100)
	require.NoError(t, err)
	assert.True(t, out.HasMore)
	require.Len(t, out.Contacts, 1)
	assert.Equal(t, "X", out.Contacts[0].FullName)
}

func TestCreateContact(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		body, _ := io.ReadAll(r.Body)
		var got CreateContactRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "Петро", got.FirstName)
		assert.Equal(t, IDCard, got.DocumentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	err := c.CreateContact(context.Background(), &CreateContactRequest{
		FirstName:    "Петро",
		LastName:     "Петренко",
		DocumentType: IDCard,
	})
	require.NoError(t, err)
}

func TestCreateContact_nil(t *testing.T) {
	c := New("x")
	err := c.CreateContact(context.Background(), nil)
	assert.ErrorIs(t, err, ErrNilRequest)
}

func TestDeleteContactsBatch(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		body, _ := io.ReadAll(r.Body)
		var got struct {
			IDs []string `json:"ids"`
		}
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, []string{"a", "b"}, got.IDs)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	require.NoError(t, c.DeleteContactsBatch(context.Background(), []string{"a", "b"}))
}

func TestPreparePayment_setsIdempotencyKey(t *testing.T) {
	var seenIdem string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenIdem = r.Header.Get("Idempotency-Key")
		body, _ := io.ReadAll(r.Body)
		var got PaymentRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "UA-from", got.SenderIBAN)
		_, _ = w.Write([]byte(`{"id":"pmt-1"}`))
	})

	out, err := c.PreparePayment(context.Background(), "test-uuid",
		&PaymentRequest{
			SenderIBAN:  "UA-from",
			Receiver:    PaymentReceiver{IBAN: "UA-to", EDRPOU: "123", Name: "X"},
			Destination: "test",
			Amount:      100,
			Currency:    "UAH",
		})
	require.NoError(t, err)
	assert.Equal(t, "test-uuid", seenIdem)
	assert.Equal(t, "pmt-1", out.ID)
}

func TestPaymentStateByReference(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "ref-42", r.URL.Query().Get("externalReference"))
		_, _ = w.Write([]byte(`{"id":"pmt-1","state":"IN_STATEMENT"}`))
	})

	out, err := c.PaymentStateByReference(context.Background(), "ref-42")
	require.NoError(t, err)
	assert.Equal(t, PaymentInStatement, out.State)
}

func TestStatement_omitsTo(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// /ext/v1/statement/UA1/1700000000 — five path segments, no `to`
		segs := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		require.Equal(t, 5, len(segs), "to should be omitted when zero")
		assert.Equal(t, "UA1", segs[3])
		assert.Equal(t, "1700000000", segs[4])
		assert.Equal(t, "DOWN", r.URL.Query().Get("direction"))
		_, _ = w.Write([]byte(`[]`))
	})

	from := time.Unix(1_700_000_000, 0)
	_, err := c.Statement(context.Background(), "UA1", from, time.Time{}, StatementDown, 0)
	require.NoError(t, err)
}

func TestUploadPayslips_unwrapsResult(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ext/v1/payslips/batch", r.URL.Path)
		_, _ = w.Write([]byte(`{"result":{"period":"2026-01","status":"LOADED","batchStats":{"employeesInBatch":3,"successInBatch":3,"failedInBatch":0},"overallStats":{"totalEmployees":3,"totalSuccessEmployees":3,"totalFailedEmployees":0},"failedEmployees":[],"createdAt":"2026-01-15T00:00:00Z","updatedAt":"2026-01-15T00:00:00Z"}}`))
	})

	out, err := c.UploadPayslips(context.Background(), &BatchPayslipRequest{
		Period:    "2026-01",
		Employees: []BatchEmployee{{Identification: "1", Attributes: []BatchAttribute{{AttributeName: "x", Value: "1", SortOrder: 1}}}},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, out.BatchStats.EmployeesInBatch)
	assert.Equal(t, "LOADED", out.Status)
}

func TestPayslipPDF_returnsRawBytes(t *testing.T) {
	pdfBytes := []byte("%PDF-1.4 fake")
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/pdf", r.Header.Get("Accept"))
		assert.Equal(t, "ID123", r.URL.Query().Get("identification"))
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdfBytes)
	})

	got, err := c.PayslipPDF(context.Background(), "ID123", "2026-01")
	require.NoError(t, err)
	assert.Equal(t, pdfBytes, got)
}

func TestSalaryRegistryTypes(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"alias":"SALARY_ADVANCE","description":"Аванс"}]`))
	})

	out, err := c.SalaryRegistryTypes(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "SALARY_ADVANCE", out[0].Alias)
}
