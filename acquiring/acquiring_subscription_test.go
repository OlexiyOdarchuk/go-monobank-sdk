package acquiring

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonoPayKeyImport(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/merchant/monopay/pubkey-import", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var got MonoPayKeyImportRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "BASE64PEM", got.KeyValue)
		assert.Equal(t, "prod-key", got.KeyName)
		_, _ = w.Write([]byte(`{"result":{"keyId":"k-1"}}`))
	})

	id, err := c.MonoPayKeyImport(context.Background(), &MonoPayKeyImportRequest{
		KeyValue: "BASE64PEM",
		KeyName:  "prod-key",
	})
	require.NoError(t, err)
	assert.Equal(t, "k-1", id)
}

func TestMonoPayKeyImport_nil(t *testing.T) {
	c := New("x")
	_, err := c.MonoPayKeyImport(context.Background(), nil)
	assert.ErrorIs(t, err, ErrNilRequest)
}

func TestMonoPayKeyDelete(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/merchant/monopay/pubkey-delete", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var got MonoPayKeyDeleteRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "k-9", got.KeyID)
		_, _ = w.Write([]byte(`{}`))
	})

	require.NoError(t, c.MonoPayKeyDelete(context.Background(), "k-9"))
}

func TestMonoPayKeyList(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/merchant/monopay/pubkey-list", r.URL.Path)
		_, _ = w.Write([]byte(`{"result":[{"keyId":"k-1","keyName":"prod","expiresAt":"2027-01-01"},{"keyId":"k-2","keyName":"stage"}]}`))
	})

	out, err := c.MonoPayKeyList(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, "k-1", out[0].KeyID)
	assert.Equal(t, "stage", out[1].KeyName)
}

func TestSplitReceivers(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/merchant/split-receiver/list", r.URL.Path)
		_, _ = w.Write([]byte(`{"list":[{"splitReceiverId":"sr1","okpo":"12345678","name":"Sub LLC"}]}`))
	})

	out, err := c.SplitReceivers(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "sr1", out[0].SplitReceiverID)
	assert.Equal(t, "Sub LLC", out[0].Name)
}

func TestTerminals(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/merchant/t2p/terminal/list", r.URL.Path)
		_, _ = w.Write([]byte(`{"list":[{"code":"t1","name":"Shop A","terminal":"T1A"}]}`))
	})

	out, err := c.Terminals(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "T1A", out[0].Terminal)
}

func TestSubscriptionCreate(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/merchant/subscription/create", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var got SubscriptionCreateRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, int64(9900), got.Amount)
		assert.Equal(t, "1m", got.Interval)
		require.NotNil(t, got.WebHookURLs)
		assert.Equal(t, "https://example.com/charge", got.WebHookURLs.ChargeURL)
		_, _ = w.Write([]byte(`{"subscriptionId":"sub-1","pageUrl":"https://pay.mono/sub-1"}`))
	})

	out, err := c.SubscriptionCreate(context.Background(), &SubscriptionCreateRequest{
		Amount:   9900,
		Currency:      980,
		Interval: "1m",
		WebHookURLs: &SubscriptionWebHookURLs{
			ChargeURL: "https://example.com/charge",
			StatusURL: "https://example.com/status",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "sub-1", out.SubscriptionID)
	assert.Equal(t, "https://pay.mono/sub-1", out.PageURL)
}

func TestSubscriptionCreate_nil(t *testing.T) {
	c := New("x")
	_, err := c.SubscriptionCreate(context.Background(), nil)
	assert.ErrorIs(t, err, ErrNilRequest)
}

func TestSubscriptionEdit(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/merchant/subscription/edit", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var got SubscriptionEditRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "sub-1", got.SubscriptionID)
		assert.Equal(t, SubscriptionCancel, got.Action)
		assert.Equal(t, int64(500), got.RefundAmount)
		_, _ = w.Write([]byte(`{}`))
	})

	require.NoError(t, c.SubscriptionEdit(context.Background(), &SubscriptionEditRequest{
		SubscriptionID: "sub-1",
		Action:         SubscriptionCancel,
		RefundAmount:   500,
	}))
}

func TestSubscriptionEdit_nil(t *testing.T) {
	c := New("x")
	err := c.SubscriptionEdit(context.Background(), nil)
	assert.ErrorIs(t, err, ErrNilRequest)
}

func TestSubscriptionRemove(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/merchant/subscription/remove", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		var got SubscriptionRemoveRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, "sub-7", got.SubscriptionID)
		_, _ = w.Write([]byte(`{}`))
	})

	require.NoError(t, c.SubscriptionRemove(context.Background(), "sub-7"))
}

func TestSubscriptionStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/merchant/subscription/status", r.URL.Path)
		assert.Equal(t, "sub-1", r.URL.Query().Get("subscriptionId"))
		_, _ = w.Write([]byte(`{
			"subscriptionId":"sub-1",
			"status":"active",
			"startDate":"2026-01-01",
			"amount":9900,
			"ccy":980,
			"interval":"1m",
			"nextChargeDate":"2026-02-01",
			"summary":{"totalPaid":3,"totalFailed":0},
			"walletData":{"cardToken":"tok","walletId":"w","status":"created"}
		}`))
	})

	out, err := c.SubscriptionStatus(context.Background(), "sub-1")
	require.NoError(t, err)
	assert.Equal(t, SubscriptionActive, out.Status)
	assert.Equal(t, 3, out.Summary.TotalPaid)
	assert.Equal(t, "tok", out.WalletData.CardToken)
}

func TestSubscriptionList(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/merchant/subscription/list", r.URL.Path)
		q := r.URL.Query()
		assert.Equal(t, from.Format(time.RFC3339), q.Get("dateFrom"))
		assert.Equal(t, to.Format(time.RFC3339), q.Get("dateTo"))
		assert.Equal(t, "active", q.Get("status"))
		assert.Equal(t, "50", q.Get("limit"))
		assert.Equal(t, "2", q.Get("page"))
		_, _ = w.Write([]byte(`{
			"list":[{"subscriptionId":"sub-1","amount":9900,"interval":"1m","startDate":"2026-01-01","created":"2026-01-01","status":"active"}],
			"pagination":{"totalItems":1,"itemsPerPage":50,"currentPage":2,"totalPages":1}
		}`))
	})

	out, err := c.SubscriptionList(context.Background(), SubscriptionListOptions{
		Status:   SubscriptionActive,
		Limit:    50,
		Page:     2,
		DateFrom: from,
		DateTo:   to,
	})
	require.NoError(t, err)
	require.Len(t, out.List, 1)
	assert.Equal(t, SubscriptionActive, out.List[0].Status)
	assert.Equal(t, 2, out.Pagination.CurrentPage)
}

func TestSubscriptionList_requiresDateFrom(t *testing.T) {
	c := New("x")
	_, err := c.SubscriptionList(context.Background(), SubscriptionListOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DateFrom is required")
}

func TestSubscriptionList_omitsOptionalQueryParams(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		assert.Empty(t, q.Get("dateTo"))
		assert.Empty(t, q.Get("status"))
		assert.Empty(t, q.Get("limit"))
		assert.Empty(t, q.Get("page"))
		_, _ = w.Write([]byte(`{"pagination":{}}`))
	})

	_, err := c.SubscriptionList(context.Background(), SubscriptionListOptions{DateFrom: from})
	require.NoError(t, err)
}

func TestSubscriptionPayments(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/merchant/subscription/payments", r.URL.Path)
		q := r.URL.Query()
		assert.Equal(t, "sub-1", q.Get("subscriptionId"))
		assert.Equal(t, from.Format(time.RFC3339), q.Get("dateFrom"))
		assert.Equal(t, "10", q.Get("limit"))
		_, _ = w.Write([]byte(`{
			"payments":[{"amount":9900,"ccy":980,"status":"success","chargedAt":"2026-02-01T00:00:00Z"}],
			"pagination":{"totalItems":1,"itemsPerPage":10,"currentPage":1,"totalPages":1}
		}`))
	})

	out, err := c.SubscriptionPayments(context.Background(), SubscriptionPaymentsOptions{
		SubscriptionID: "sub-1",
		DateFrom:       from,
		Limit:          10,
	})
	require.NoError(t, err)
	require.Len(t, out.Payments, 1)
	assert.Equal(t, SubscriptionPaymentSuccess, out.Payments[0].Status)
}

func TestSubscriptionPayments_requiresFields(t *testing.T) {
	c := New("x")

	_, err := c.SubscriptionPayments(context.Background(), SubscriptionPaymentsOptions{
		DateFrom: time.Now(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SubscriptionID is required")

	_, err = c.SubscriptionPayments(context.Background(), SubscriptionPaymentsOptions{
		SubscriptionID: "sub-1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DateFrom is required")
}
