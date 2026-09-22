package openbanking

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
)

// capture records what the fake bank saw, so a test can assert on the
// request after the call returned.
type capture struct {
	Method string
	Path   string
	Query  map[string][]string
	Header http.Header
	Body   []byte
}

// newTestClient points a Client at an httptest server. Real calls
// need a QWAC in the TLS handshake; over loopback plain HTTP the
// handshake is out of the picture, which is exactly what lets these
// tests exercise the paths, headers and bodies.
func newTestClient(t *testing.T, h http.HandlerFunc) (*Client, *capture) {
	t.Helper()
	got := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got.Method = r.Method
		got.Path = r.URL.Path
		got.Query = r.URL.Query()
		got.Header = r.Header.Clone()
		got.Body = body
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	cli := New(monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
	t.Cleanup(func() { _ = cli.Close() })

	return cli, got
}

// jsonHandler answers every request with status and the raw body.
func jsonHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != "" {
			_, _ = io.WriteString(w, body)
		}
	}
}

func testConsentOptions() InitiationOptions {
	return InitiationOptions{
		PSU: PSU{
			ID:              "UA413220010000026206347644036",
			IDType:          PSUIDTypeIBAN,
			CorporateID:     "UA173220010000026002700000121",
			CorporateIDType: PSUIDTypeIBAN,
			IPAddress:       "8.8.8.8",
		},
		TPPRedirectURI: "https://example.com/redirect",
	}
}

func TestRequestIDAuth_setsHeaders(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusOK, `{"consentStatus":"valid"}`))

	_, err := cli.ConsentStatus(context.Background(), "c1")
	require.NoError(t, err)

	assert.NotEmpty(t, got.Header.Get(HeaderRequestID))
	assert.Len(t, got.Header.Get(HeaderRequestID), 36, "X-Request-ID must be a UUID")
	assert.Equal(t, "application/json", got.Header.Get("Accept"))
}

func TestContextWithRequestID_overridesGeneratedID(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusOK, `{"consentStatus":"valid"}`))

	ctx := ContextWithRequestID(context.Background(), "99391c7e-ad88-49ec-a2ad-99ddcb1f7721")
	_, err := cli.ConsentStatus(ctx, "c1")
	require.NoError(t, err)

	assert.Equal(t, "99391c7e-ad88-49ec-a2ad-99ddcb1f7721", got.Header.Get(HeaderRequestID))
}

func TestRequestIDAuth_customGenerator(t *testing.T) {
	a := RequestIDAuth{NewRequestID: func() string { return "fixed" }}
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	require.NoError(t, a.SetAuth(req))
	assert.Equal(t, "fixed", req.Header.Get(HeaderRequestID))

	// A nil request is a no-op, and LogValue stays terse.
	require.NoError(t, a.SetAuth(nil))
	assert.Equal(t, "openbanking.RequestIDAuth{}", a.LogValue().String())
}

func TestCreateConsent(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusCreated, `{
		"consentId": "2409105jHHToBoHe3Lqw",
		"consentStatus": "received",
		"_links": {"startAuthorisation": {"href": "https://ob/authorisations"}}
	}`))

	in := &CreateConsentRequest{
		Access: AccountAccess{
			Cards: []AccountAccessRights{{
				Account: Account{IBAN: "UA413220010000026206347644036", Currency: "UAH"},
				Rights:  []AccessRight{AccessAccountDetails, AccessBalances},
			}},
			Payments: []AccountAccessRights{{
				Account: Account{IBAN: "UA173220010000026002700000121", Currency: "UAH"},
				Rights:  []AccessRight{AccessTransactions},
			}},
		},
		ConsentType:        ConsentDetailed,
		RecurringIndicator: true,
		ValidTo:            NewDate(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)),
		FrequencyPerDay:    4,
	}
	out, err := cli.CreateConsent(context.Background(), in, testConsentOptions())
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, got.Method)
	assert.Equal(t, "/ext/v2/consents/account-access", got.Path)
	assert.Equal(t, "application/json", got.Header.Get("Content-Type"))
	assert.Equal(t, "8.8.8.8", got.Header.Get(HeaderPSUIPAddress))
	assert.Equal(t, "UA413220010000026206347644036", got.Header.Get(HeaderPSUID))
	assert.Equal(t, "IBAN", got.Header.Get(HeaderPSUIDType))
	assert.Equal(t, "UA173220010000026002700000121", got.Header.Get(HeaderPSUCorporateID))
	assert.Equal(t, "IBAN", got.Header.Get(HeaderPSUCorporateIDType))
	assert.Equal(t, "https://example.com/redirect", got.Header.Get(HeaderTPPRedirectURI))

	var sent map[string]any
	require.NoError(t, json.Unmarshal(got.Body, &sent))
	assert.Equal(t, "detailed", sent["consentType"])
	assert.Equal(t, true, sent["recurringIndicator"])
	assert.InDelta(t, 4.0, sent["frequencyPerDay"], 0)
	assert.Equal(t, "2024-12-31", sent["validTo"], "validTo must be an ISO date, not RFC 3339")

	assert.Equal(t, "2409105jHHToBoHe3Lqw", out.ConsentID)
	assert.Equal(t, ConsentReceived, out.ConsentStatus)
	require.NotNil(t, out.Links.StartAuthorisation)
	assert.Equal(t, "https://ob/authorisations", out.Links.StartAuthorisation.Href)
}

func TestStartConsentAuthorisation(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusCreated, `{
		"authorisationId": "a1",
		"scaStatus": "started",
		"_links": {"scaRedirect": {"href": "https://ob/sca"}}
	}`))

	out, err := cli.StartConsentAuthorisation(context.Background(), "2409105jHHToBoHe3Lqw")
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, got.Method)
	assert.Equal(t, "/ext/v2/consents/account-access/2409105jHHToBoHe3Lqw/authorisations", got.Path)
	assert.Equal(t, "a1", out.AuthorisationID)
	assert.Equal(t, SCAStarted, out.SCAStatus)
	require.NotNil(t, out.Links.SCARedirect)
	assert.Equal(t, "https://ob/sca", out.Links.SCARedirect.Href)
	assert.Nil(t, out.Links.SCAStatus, "the consent flow returns no scaStatus link")
}

func TestConsent(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusOK, `{
		"access": {"cards": [{"account": {"iban": "UA41", "currency": "UAH"}, "rights": ["balances"]}]},
		"consentType": "detailed",
		"recurringIndicator": true,
		"validTo": "2024-12-31",
		"frequencyPerDay": 4,
		"consentStatus": "valid"
	}`))

	out, err := cli.Consent(context.Background(), "c1")
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, got.Method)
	assert.Equal(t, "/ext/v2/consents/account-access/c1", got.Path)
	assert.Equal(t, ConsentValid, out.ConsentStatus)
	assert.Equal(t, ConsentDetailed, out.ConsentType)
	assert.Equal(t, "2024-12-31", out.ValidTo.String())
	require.Len(t, out.Access.Cards, 1)
	assert.Equal(t, []AccessRight{AccessBalances}, out.Access.Cards[0].Rights)
}

func TestConsentStatus(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusOK, `{"consentStatus":"revokedByPsu"}`))

	out, err := cli.ConsentStatus(context.Background(), "c1")
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, got.Method)
	assert.Equal(t, "/ext/v2/consents/account-access/c1/status", got.Path)
	assert.Equal(t, ConsentRevokedByPSU, out.ConsentStatus)
}

func TestRevokeConsent_expects204(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusNoContent, ""))

	require.NoError(t, cli.RevokeConsent(context.Background(), "c1"))
	assert.Equal(t, http.MethodDelete, got.Method)
	assert.Equal(t, "/ext/v2/consents/account-access/c1", got.Path)
}

func TestRevokeConsent_rejects200(t *testing.T) {
	// The spec documents 204 only; a 200 means something else answered.
	cli, _ := newTestClient(t, jsonHandler(http.StatusOK, `{}`))

	err := cli.RevokeConsent(context.Background(), "c1")
	assertStatusError(t, err, http.StatusOK)
}

func TestAccounts(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusOK, `{"accounts": [
		{"resourceId": "hH1eKFyoP", "iban": "UA41", "currency": "UAH", "product": "platinum",
		 "balances": [{"balanceAmount": {"currency": "UAH", "amount": "100.02"},
		               "balanceType": "interimAvailable", "creditLimitIncluded": true}]}
	]}`))

	out, err := cli.Accounts(context.Background(), AccountsOptions{
		AccountOptions: AccountOptions{ConsentID: "c1", PSUIPAddress: "8.8.8.8"},
		WithBalance:    true,
	})
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, got.Method)
	assert.Equal(t, "/ext/v2/accounts", got.Path)
	assert.Equal(t, []string{"true"}, got.Query["withBalance"])
	assert.Equal(t, "c1", got.Header.Get(HeaderConsentID))
	assert.Equal(t, "8.8.8.8", got.Header.Get(HeaderPSUIPAddress))

	require.Len(t, out, 1)
	assert.Equal(t, "hH1eKFyoP", out[0].ResourceID)
	require.Len(t, out[0].Balances, 1)
	assert.Equal(t, "100.02", out[0].Balances[0].BalanceAmount.Value)
	assert.Equal(t, BalanceInterimAvailable, out[0].Balances[0].BalanceType)
	assert.True(t, out[0].Balances[0].CreditLimitIncluded)
}

func TestAccounts_withoutBalanceOmitsQuery(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusOK, `{"accounts": []}`))

	_, err := cli.Accounts(context.Background(), AccountsOptions{
		AccountOptions: AccountOptions{ConsentID: "c1"},
	})
	require.NoError(t, err)

	assert.Empty(t, got.Query)
	assert.Empty(t, got.Header.Get(HeaderPSUIPAddress), "an empty optional header must not be sent")
}

func TestBalances(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusOK, `{
		"account": {"iban": "UA41", "currency": "UAH", "product": "platinum"},
		"balances": [{"balanceAmount": {"currency": "UAH", "amount": "-7.5"}, "balanceType": "interimAvailable"}]
	}`))

	out, err := cli.Balances(context.Background(), "hH1eKFyoP", AccountOptions{ConsentID: "c1"})
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, got.Method)
	assert.Equal(t, "/ext/v2/accounts/hH1eKFyoP/balances", got.Path)
	assert.Equal(t, "UA41", out.Account.IBAN)
	require.Len(t, out.Balances, 1)
	assert.Equal(t, "-7.5", out.Balances[0].BalanceAmount.Value)
}

func TestTransactions(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusOK, `{
		"account": {"iban": "UA41", "currency": "UAH"},
		"transactions": {
			"booked": [{
				"transactionId": "t1",
				"bookingDate": "2024-01-02",
				"transactionAmount": {"currency": "UAH", "amount": "-100.02"},
				"creditor": {"name": "ACME"},
				"cardTransaction": {
					"transactionDateTime": "2024-01-02T15:04:05.123456789Z",
					"tradeName": "ACME Store",
					"merchantCategoryCode": "5411"
				},
				"remittanceInformationUnstructured": ["Оплата за послуги"]
			}],
			"_links": {
				"first": {"href": "https://ob/ext/v2/accounts/a/transactions?bookingStatus=booked"},
				"next": {"href": "https://ob/ext/v2/accounts/a/transactions?bookingStatus=booked&pageId=CURSOR"}
			}
		}
	}`))

	out, err := cli.Transactions(context.Background(), "hH1eKFyoP", TransactionsOptions{
		AccountOptions: AccountOptions{ConsentID: "c1"},
		BookingStatus:  BookingBoth,
		DateFrom:       NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
		DateTo:         NewDate(time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)),
		PageID:         "PREV",
	})
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, got.Method)
	assert.Equal(t, "/ext/v2/accounts/hH1eKFyoP/transactions", got.Path)
	assert.Equal(t, []string{"both"}, got.Query["bookingStatus"])
	assert.Equal(t, []string{"2024-01-01"}, got.Query["dateFrom"])
	assert.Equal(t, []string{"2024-01-31"}, got.Query["dateTo"])
	assert.Equal(t, []string{"PREV"}, got.Query["pageId"])

	require.NotNil(t, out.Transactions)
	require.Len(t, out.Transactions.Booked, 1)
	tx := out.Transactions.Booked[0]
	assert.Equal(t, "t1", tx.TransactionID)
	assert.Equal(t, "2024-01-02", tx.BookingDate.String())
	assert.Equal(t, "-100.02", tx.TransactionAmount.Value)
	require.NotNil(t, tx.Creditor)
	assert.Equal(t, "ACME", tx.Creditor.Name)
	require.NotNil(t, tx.CardTransaction)
	assert.Equal(t, "5411", tx.CardTransaction.MerchantCategoryCode)
	assert.Equal(t, 123456789, tx.CardTransaction.TransactionDateTime.Nanosecond())
	assert.Equal(t, []string{"Оплата за послуги"}, tx.RemittanceInformationUnstructured)

	next, ok := NextPageID(out)
	assert.True(t, ok)
	assert.Equal(t, "CURSOR", next)
}

func TestTransactions_omitsUnsetDates(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusOK, `{"account": {"iban": "UA41"}}`))

	out, err := cli.Transactions(context.Background(), "a1", TransactionsOptions{
		AccountOptions: AccountOptions{ConsentID: "c1"},
		BookingStatus:  BookingBooked,
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"booked"}, got.Query["bookingStatus"])
	assert.NotContains(t, got.Query, "dateFrom")
	assert.NotContains(t, got.Query, "dateTo")
	assert.NotContains(t, got.Query, "pageId")
	assert.Nil(t, out.Transactions)

	_, ok := NextPageID(out)
	assert.False(t, ok, "a page without transactions has no cursor")
}

func TestCreatePayment(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusCreated, `{
		"transactionStatus": "RCVD",
		"paymentId": "241212472HToBoHe3Lq1",
		"_links": {"startAuthorisation": {"href": "https://ob/auth"}}
	}`))

	in := &CreatePaymentRequest{
		InstructedAmount: Amount{Currency: "UAH", Value: "100.02"},
		CreditorAccount:  PaymentAccountReference{IBAN: "UA17", Currency: "UAH"},
		Creditor: Creditor{
			Name:           "ТОВ Ромашка",
			CreditorID:     "12345678",
			CreditorIDType: CreditorUSRC,
		},
		DebtorAccount:                     PaymentAccountReference{IBAN: "UA41", Currency: "UAH"},
		RemittanceInformationUnstructured: []string{"Оплата за послуги згідно рахунку 314415"},
	}
	out, err := cli.CreatePayment(context.Background(), CreditTransfers, in, testConsentOptions())
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, got.Method)
	assert.Equal(t, "/ext/v2/payments/credit-transfers", got.Path)
	assert.Equal(t, "application/json", got.Header.Get("Content-Type"))
	assert.Equal(t, "8.8.8.8", got.Header.Get(HeaderPSUIPAddress))

	var sent CreatePaymentRequest
	require.NoError(t, json.Unmarshal(got.Body, &sent))
	assert.Equal(t, *in, sent)
	assert.Contains(t, string(got.Body), `"amount":"100.02"`)

	assert.Equal(t, StatusReceived, out.TransactionStatus)
	assert.Equal(t, "241212472HToBoHe3Lq1", out.PaymentID)
	require.NotNil(t, out.Links.StartAuthorisation)
}

func TestCreatePayment_instantProduct(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusCreated, `{"transactionStatus":"RCVD","paymentId":"p1"}`))

	_, err := cli.CreatePayment(context.Background(), InstantCreditTransfers,
		&CreatePaymentRequest{}, testConsentOptions())
	require.NoError(t, err)
	assert.Equal(t, "/ext/v2/payments/instant-credit-transfers", got.Path)
}

func TestPayment(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusOK, `{
		"transactionStatus": "ACCC",
		"paymentId": "p1",
		"instructedAmount": {"currency": "UAH", "amount": "1.5"},
		"creditorAccount": {"iban": "UA17", "currency": "UAH"},
		"creditor": {"name": "ACME", "creditorId": "12345678", "creditorIdType": "USRC"},
		"debtorAccount": {"iban": "UA41", "currency": "UAH"}
	}`))

	out, err := cli.Payment(context.Background(), CreditTransfers, "p1")
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, got.Method)
	assert.Equal(t, "/ext/v2/payments/credit-transfers/p1", got.Path)
	assert.Equal(t, StatusAcceptedCreditSettlementCompleted, out.TransactionStatus)
	assert.Equal(t, "1.5", out.InstructedAmount.Value)
	assert.Equal(t, CreditorUSRC, out.Creditor.CreditorIDType)
	assert.Equal(t, "UA41", out.DebtorAccount.IBAN)
}

func TestPaymentStatus(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusOK, `{"transactionStatus":"RJCT"}`))

	out, err := cli.PaymentStatus(context.Background(), CreditTransfers, "p1")
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, got.Method)
	assert.Equal(t, "/ext/v2/payments/credit-transfers/p1/status", got.Path)
	assert.Equal(t, StatusRejected, out.TransactionStatus)
}

func TestCancelPayment_expects204(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusNoContent, ""))

	require.NoError(t, cli.CancelPayment(context.Background(), InstantCreditTransfers, "p1"))
	assert.Equal(t, http.MethodDelete, got.Method)
	assert.Equal(t, "/ext/v2/payments/instant-credit-transfers/p1", got.Path)
}

func TestStartPaymentAuthorisation(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusCreated, `{
		"authorisationId": "a1",
		"scaStatus": "started",
		"_links": {"scaStatus": {"href": "https://ob/status"}}
	}`))

	out, err := cli.StartPaymentAuthorisation(context.Background(), CreditTransfers, "p1",
		AuthorisationOptions{SCAApproachPreference: SCADecoupled})
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, got.Method)
	assert.Equal(t, "/ext/v2/payments/credit-transfers/p1/authorisations", got.Path)
	assert.Equal(t, "DECOUPLED", got.Header.Get(HeaderTPPSCAApproachPreference))
	assert.Nil(t, out.Links.SCARedirect, "a DECOUPLED authorisation has no redirect link")
	require.NotNil(t, out.Links.SCAStatus)
	assert.Equal(t, "https://ob/status", out.Links.SCAStatus.Href)
}

func TestStartPaymentAuthorisation_omitsEmptyPreference(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusCreated, `{"authorisationId":"a1","scaStatus":"started"}`))

	_, err := cli.StartPaymentAuthorisation(context.Background(), CreditTransfers, "p1", AuthorisationOptions{})
	require.NoError(t, err)
	assert.Empty(t, got.Header.Get(HeaderTPPSCAApproachPreference))
}

func TestPaymentAuthorisations(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusOK, `{"authorisationIds":["a1","a2"]}`))

	out, err := cli.PaymentAuthorisations(context.Background(), CreditTransfers, "p1")
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, got.Method)
	assert.Equal(t, "/ext/v2/payments/credit-transfers/p1/authorisations", got.Path)
	assert.Equal(t, []string{"a1", "a2"}, out)
}

func TestPaymentAuthorisationStatus(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusOK, `{"scaStatus":"finalised"}`))

	out, err := cli.PaymentAuthorisationStatus(context.Background(), CreditTransfers, "p1", "a1")
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, got.Method)
	assert.Equal(t, "/ext/v2/payments/credit-transfers/p1/authorisations/a1", got.Path)
	assert.Equal(t, SCAFinalised, out.SCAStatus)
}

func TestRequestTestCertificate(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusCreated, `{"status":"pending"}`))

	in := &CertificateRequest{
		Organization:  `ТОВ "Фінтех Сервіс"`,
		ContactPerson: "Іваненко Іван Іванович",
		Phone:         "+380501234567",
		Email:         "test@example.com",
		Description:   "Інтеграція з Open Banking API",
		EDRPOU:        "12345678",
		CSRPEM:        "LS0tLS1CRUdJTi",
	}
	out, err := cli.RequestTestCertificate(context.Background(), in)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, got.Method)
	assert.Equal(t, "/ext/test-integration/cert-request", got.Path)
	var sent CertificateRequest
	require.NoError(t, json.Unmarshal(got.Body, &sent))
	assert.Equal(t, *in, sent)
	assert.Equal(t, CertPending, out.Status)
}

func TestTestCertificateStatus(t *testing.T) {
	cli, got := newTestClient(t, jsonHandler(http.StatusOK, `{"status":"approved","certificate":"LS0tLS1CRUd"}`))

	out, err := cli.TestCertificateStatus(context.Background(), "LS0tLS1CRUdJTi")
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, got.Method)
	assert.Equal(t, "/ext/test-integration/cert-request/status", got.Path)
	assert.JSONEq(t, `{"csrPem":"LS0tLS1CRUdJTi"}`, string(got.Body))
	assert.Equal(t, CertApproved, out.Status)
	assert.Equal(t, "LS0tLS1CRUd", out.Certificate)
	assert.Empty(t, out.RejectReason)
}

// assertStatusError checks that err is the SDK's shared APIError with
// the given status.
func assertStatusError(t *testing.T, err error, status int) {
	t.Helper()
	require.Error(t, err)
	var apiErr *monobank.APIError
	require.ErrorAs(t, err, &apiErr, "want *monobank.APIError, got %T", err)
	assert.Equal(t, status, apiErr.StatusCode)
}
