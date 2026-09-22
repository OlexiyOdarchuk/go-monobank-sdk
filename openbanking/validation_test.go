package openbanking

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
)

// unreachableClient points at a port nothing listens on: any test
// here that accidentally performs I/O fails loudly instead of
// silently passing.
func unreachableClient(t *testing.T) *Client {
	t.Helper()
	cli := New(monobank.WithBaseURL("http://127.0.0.1:1"))
	t.Cleanup(func() { _ = cli.Close() })

	return cli
}

func TestValidation_noRoundTrip(t *testing.T) {
	ctx := context.Background()
	c := unreachableClient(t)
	opts := InitiationOptions{PSU: PSU{IPAddress: "8.8.8.8"}}
	acc := AccountOptions{ConsentID: "c1"}

	t.Run("CreateConsent/nil body", func(t *testing.T) {
		_, err := c.CreateConsent(ctx, nil, opts)
		assert.ErrorIs(t, err, ErrNilRequest)
	})
	t.Run("CreateConsent/no PSU IP", func(t *testing.T) {
		_, err := c.CreateConsent(ctx, &CreateConsentRequest{}, InitiationOptions{})
		assert.ErrorIs(t, err, ErrMissingPSUIPAddress)
	})
	t.Run("StartConsentAuthorisation/empty id", func(t *testing.T) {
		_, err := c.StartConsentAuthorisation(ctx, "")
		assert.ErrorIs(t, err, ErrEmptyID)
	})
	t.Run("Consent/empty id", func(t *testing.T) {
		_, err := c.Consent(ctx, "")
		assert.ErrorIs(t, err, ErrEmptyID)
	})
	t.Run("ConsentStatus/empty id", func(t *testing.T) {
		_, err := c.ConsentStatus(ctx, "")
		assert.ErrorIs(t, err, ErrEmptyID)
	})
	t.Run("RevokeConsent/empty id", func(t *testing.T) {
		assert.ErrorIs(t, c.RevokeConsent(ctx, ""), ErrEmptyID)
	})

	t.Run("Accounts/no consent", func(t *testing.T) {
		_, err := c.Accounts(ctx, AccountsOptions{})
		assert.ErrorIs(t, err, ErrMissingConsentID)
	})
	t.Run("Balances/empty account", func(t *testing.T) {
		_, err := c.Balances(ctx, "", acc)
		assert.ErrorIs(t, err, ErrEmptyID)
	})
	t.Run("Balances/no consent", func(t *testing.T) {
		_, err := c.Balances(ctx, "a1", AccountOptions{})
		assert.ErrorIs(t, err, ErrMissingConsentID)
	})
	t.Run("Transactions/empty account", func(t *testing.T) {
		_, err := c.Transactions(ctx, "", TransactionsOptions{AccountOptions: acc, BookingStatus: BookingBooked})
		assert.ErrorIs(t, err, ErrEmptyID)
	})
	t.Run("Transactions/no consent", func(t *testing.T) {
		_, err := c.Transactions(ctx, "a1", TransactionsOptions{BookingStatus: BookingBooked})
		assert.ErrorIs(t, err, ErrMissingConsentID)
	})
	t.Run("Transactions/no bookingStatus", func(t *testing.T) {
		_, err := c.Transactions(ctx, "a1", TransactionsOptions{AccountOptions: acc})
		assert.ErrorIs(t, err, ErrMissingBookingStatus)
	})

	t.Run("CreatePayment/empty product", func(t *testing.T) {
		_, err := c.CreatePayment(ctx, "", &CreatePaymentRequest{}, opts)
		assert.ErrorIs(t, err, ErrEmptyID)
	})
	t.Run("CreatePayment/nil body", func(t *testing.T) {
		_, err := c.CreatePayment(ctx, CreditTransfers, nil, opts)
		assert.ErrorIs(t, err, ErrNilRequest)
	})
	t.Run("CreatePayment/no PSU IP", func(t *testing.T) {
		_, err := c.CreatePayment(ctx, CreditTransfers, &CreatePaymentRequest{}, InitiationOptions{})
		assert.ErrorIs(t, err, ErrMissingPSUIPAddress)
	})
	t.Run("Payment/empty id", func(t *testing.T) {
		_, err := c.Payment(ctx, CreditTransfers, "")
		assert.ErrorIs(t, err, ErrEmptyID)
	})
	t.Run("PaymentStatus/empty product", func(t *testing.T) {
		_, err := c.PaymentStatus(ctx, "", "p1")
		assert.ErrorIs(t, err, ErrEmptyID)
	})
	t.Run("CancelPayment/empty id", func(t *testing.T) {
		assert.ErrorIs(t, c.CancelPayment(ctx, CreditTransfers, ""), ErrEmptyID)
	})
	t.Run("StartPaymentAuthorisation/empty id", func(t *testing.T) {
		_, err := c.StartPaymentAuthorisation(ctx, CreditTransfers, "", AuthorisationOptions{})
		assert.ErrorIs(t, err, ErrEmptyID)
	})
	t.Run("PaymentAuthorisations/empty id", func(t *testing.T) {
		_, err := c.PaymentAuthorisations(ctx, CreditTransfers, "")
		assert.ErrorIs(t, err, ErrEmptyID)
	})
	t.Run("PaymentAuthorisationStatus/empty authorisation", func(t *testing.T) {
		_, err := c.PaymentAuthorisationStatus(ctx, CreditTransfers, "p1", "")
		assert.ErrorIs(t, err, ErrEmptyID)
	})
	t.Run("PaymentAuthorisationStatus/empty payment", func(t *testing.T) {
		_, err := c.PaymentAuthorisationStatus(ctx, CreditTransfers, "", "a1")
		assert.ErrorIs(t, err, ErrEmptyID)
	})

	t.Run("RequestTestCertificate/nil body", func(t *testing.T) {
		_, err := c.RequestTestCertificate(ctx, nil)
		assert.ErrorIs(t, err, ErrNilRequest)
	})
	t.Run("TestCertificateStatus/empty CSR", func(t *testing.T) {
		_, err := c.TestCertificateStatus(ctx, "")
		assert.ErrorIs(t, err, ErrEmptyID)
	})
}

func TestMessages_parsesErrorEnvelope(t *testing.T) {
	cli, _ := newTestClient(t, jsonHandler(http.StatusBadRequest, `{"apiClientMessages":[
		{"category":"ERROR","code":"CURRENCY_MISMATCH","text":"Currency mismatch: expected UAH, got USD"}
	]}`))

	_, err := cli.Balances(context.Background(), "a1", AccountOptions{ConsentID: "c1"})
	require.Error(t, err)

	msgs := Messages(err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "ERROR", msgs[0].Category)
	assert.Equal(t, CodeCurrencyMismatch, msgs[0].Code)
	assert.Contains(t, msgs[0].Text, "expected UAH")

	// The shared APIError and its sentinels still work.
	var apiErr *monobank.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Empty(t, apiErr.ErrorDescription, "this API uses apiClientMessages, not errorDescription")
}

func TestMessages_unauthorizedSentinel(t *testing.T) {
	cli, _ := newTestClient(t, jsonHandler(http.StatusUnauthorized,
		`{"apiClientMessages":[{"category":"ERROR","code":"UNAUTHORIZED"}]}`))

	_, err := cli.Accounts(context.Background(), AccountsOptions{AccountOptions: AccountOptions{ConsentID: "c1"}})
	assert.ErrorIs(t, err, monobank.ErrUnauthorized)
	require.Len(t, Messages(err), 1)
}

func TestMessages_ignoresForeignErrors(t *testing.T) {
	assert.Nil(t, Messages(nil))
	assert.Nil(t, Messages(errors.New("boom")))
	assert.Nil(t, Messages(&monobank.APIError{Body: []byte("<html>gateway</html>")}))
	assert.Nil(t, Messages(&monobank.APIError{}))
}

func TestDate_roundTrip(t *testing.T) {
	var d Date
	require.NoError(t, json.Unmarshal([]byte(`"2024-12-31"`), &d))
	assert.Equal(t, "2024-12-31", d.String())
	assert.Equal(t, 2024, d.Time().Year())
	assert.False(t, d.IsZero())

	out, err := json.Marshal(d)
	require.NoError(t, err)
	assert.JSONEq(t, `"2024-12-31"`, string(out))
}

func TestDate_zeroAndInvalid(t *testing.T) {
	var zero Date
	assert.True(t, zero.IsZero())
	assert.Empty(t, zero.String())
	out, err := json.Marshal(zero)
	require.NoError(t, err)
	assert.JSONEq(t, `""`, string(out))

	// An empty string decodes to the zero Date, not an error.
	var d Date
	require.NoError(t, json.Unmarshal([]byte(`""`), &d))
	assert.True(t, d.IsZero())

	assert.Error(t, json.Unmarshal([]byte(`"31.12.2024"`), &d))
	assert.Error(t, json.Unmarshal([]byte(`42`), &d))
}

func TestNextPageID_edgeCases(t *testing.T) {
	_, ok := NextPageID(nil)
	assert.False(t, ok)

	_, ok = NextPageID(&TransactionsResponse{Transactions: &AccountReport{}})
	assert.False(t, ok, "no next link means the last page")

	_, ok = NextPageID(&TransactionsResponse{Transactions: &AccountReport{
		Links: AccountReportLinks{Next: &Href{Href: "https://ob/x?bookingStatus=booked"}},
	}})
	assert.False(t, ok, "a next link without pageId carries no cursor")

	id, ok := NextPageID(&TransactionsResponse{Transactions: &AccountReport{
		Links: AccountReportLinks{Next: &Href{Href: "https://ob/x?pageId=abc%20def"}},
	}})
	assert.True(t, ok)
	assert.Equal(t, "abc def", id)
}

func TestNew_defaultsToProduction(t *testing.T) {
	// The base URL is private to monobank.Client, so assert it the
	// only way a caller can: by where the request would go. A bogus
	// transport records the URL and fails the call.
	var seen string
	cli := New(monobank.WithHTTPDoer(doerFunc(func(r *http.Request) (*http.Response, error) {
		seen = r.URL.String()

		return nil, errors.New("blocked")
	})))
	t.Cleanup(func() { _ = cli.Close() })

	_, _ = cli.ConsentStatus(context.Background(), "c1")
	assert.Equal(t, BaseURLProduction+"/ext/v2/consents/account-access/c1/status", seen)
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestNewMTLSHTTPClient(t *testing.T) {
	certPEM, keyPEM := selfSignedPEM(t)

	t.Run("builds a client presenting the certificate", func(t *testing.T) {
		cli, err := NewMTLSHTTPClient(MTLSConfig{CertPEM: certPEM, KeyPEM: keyPEM})
		require.NoError(t, err)
		assert.Equal(t, DefaultTimeout, cli.Timeout)

		tr, ok := cli.Transport.(*http.Transport)
		require.True(t, ok)
		require.Len(t, tr.TLSClientConfig.Certificates, 1)
		assert.Equal(t, uint16(0x0303), tr.TLSClientConfig.MinVersion) // TLS 1.2
		assert.Nil(t, tr.TLSClientConfig.RootCAs, "no CA bundle means the system trust store")
	})

	t.Run("honors a custom CA bundle and timeout", func(t *testing.T) {
		cli, err := NewMTLSHTTPClient(MTLSConfig{
			CertPEM:    certPEM,
			KeyPEM:     keyPEM,
			RootCAsPEM: certPEM,
			Timeout:    5 * time.Second,
		})
		require.NoError(t, err)
		assert.Equal(t, 5*time.Second, cli.Timeout)
		tr, ok := cli.Transport.(*http.Transport)
		require.True(t, ok)
		assert.NotNil(t, tr.TLSClientConfig.RootCAs)
	})

	t.Run("rejects a missing certificate", func(t *testing.T) {
		_, err := NewMTLSHTTPClient(MTLSConfig{KeyPEM: keyPEM})
		assert.ErrorIs(t, err, ErrNoClientCert)
		_, err = NewMTLSHTTPClient(MTLSConfig{CertPEM: certPEM})
		assert.ErrorIs(t, err, ErrNoClientCert)
	})

	t.Run("rejects an unparsable CA bundle", func(t *testing.T) {
		_, err := NewMTLSHTTPClient(MTLSConfig{
			CertPEM: certPEM, KeyPEM: keyPEM, RootCAsPEM: []byte("not a PEM"),
		})
		assert.ErrorIs(t, err, ErrInvalidCA)
	})

	t.Run("rejects a mismatched key", func(t *testing.T) {
		_, otherKey := selfSignedPEM(t)
		_, err := NewMTLSHTTPClient(MTLSConfig{CertPEM: certPEM, KeyPEM: otherKey})
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrNoClientCert)
	})
}

func TestNewMTLSHTTPClientFromFiles(t *testing.T) {
	certPEM, keyPEM := selfSignedPEM(t)
	dir := t.TempDir()
	certFile := filepath.Join(dir, "qwac.pem")
	keyFile := filepath.Join(dir, "qwac.key")
	require.NoError(t, os.WriteFile(certFile, certPEM, 0o600))
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0o600))

	cli, err := NewMTLSHTTPClientFromFiles(certFile, keyFile, "")
	require.NoError(t, err)
	assert.NotNil(t, cli.Transport)

	cli, err = NewMTLSHTTPClientFromFiles(certFile, keyFile, certFile)
	require.NoError(t, err)
	assert.NotNil(t, cli.Transport)

	_, err = NewMTLSHTTPClientFromFiles(filepath.Join(dir, "missing.pem"), keyFile, "")
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = NewMTLSHTTPClientFromFiles(certFile, filepath.Join(dir, "missing.key"), "")
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = NewMTLSHTTPClientFromFiles(certFile, keyFile, filepath.Join(dir, "missing.ca"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// selfSignedPEM mints a throwaway ECDSA certificate and key. The test
// QWAC the bank issues is ECDSA-signed too, so this exercises the
// same code path without shipping a fixture.
func selfSignedPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "openbanking-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}
