//go:build integration

// Package openbanking integration tests hit a real monobank Open
// Banking host. They are excluded from the default `go test ./...`
// run; enable with:
//
//	OB_CERT=qwac.pem OB_KEY=qwac.key \
//	    go test -tags=integration -run Integration ./openbanking/...
//
// The tests skip when their inputs are absent, so the suite is a green
// no-op until someone supplies a certificate. CI runs it weekly — see
// .github/workflows/integration.yaml.
//
// This is the package with the widest gap between "matches the spec"
// and "known to work": none of its 17 operations has ever completed
// against the bank, and the mTLS handshake has never happened. Every
// test here exists to close part of that gap.
//
// Default host is [openbanking.BaseURLStage]; override with OB_BASE_URL.
package openbanking_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/openbanking"
)

// certOptIn gates the sandbox certificate desk: it files an
// application a human then processes, and the endpoint is the only one
// in the spec that documents 429.
const certOptIn = "yes-request-certificate"

func envOrSkip(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("%s is not set", key)
	}
	return v
}

func baseURL() string {
	if v := os.Getenv("OB_BASE_URL"); v != "" {
		return v
	}
	return openbanking.BaseURLStage
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// liveClient builds a client with the caller's QWAC. Skips unless both
// halves of the certificate are on disk.
func liveClient(t *testing.T) *openbanking.Client {
	t.Helper()
	httpCli, err := openbanking.NewMTLSHTTPClientFromFiles(
		envOrSkip(t, "OB_CERT"), envOrSkip(t, "OB_KEY"), os.Getenv("OB_CA"))
	require.NoError(t, err, "the QWAC must load")

	cli := openbanking.New(
		monobank.WithBaseURL(baseURL()),
		monobank.WithHTTPClient(httpCli),
	)
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

// The package doc claims that without a QWAC the bank drops the
// connection in the handshake, so the failure arrives as a transport
// error rather than as 401. That claim has never been observed. This
// test needs no certificate — it deliberately calls without one.
//
// Gated anyway: it reaches the bank, and nothing should do that
// unasked.
func TestIntegration_NoClientCertIsTransportError(t *testing.T) {
	envOrSkip(t, "OB_PROBE_HANDSHAKE")

	cli := openbanking.New(monobank.WithBaseURL(baseURL()))
	t.Cleanup(func() { _ = cli.Close() })

	_, err := cli.Accounts(liveContext(t), openbanking.AccountsOptions{
		AccountOptions: openbanking.AccountOptions{
			ConsentID: "probe", PSUIPAddress: "127.0.0.1",
		},
	})
	require.Error(t, err, "a call without a QWAC must not succeed")

	var apiErr *monobank.APIError
	if errors.As(err, &apiErr) {
		t.Fatalf("expected a transport failure, got HTTP %d — "+
			"the package doc says the handshake dies first", apiErr.StatusCode)
	}
	var urlErr *url.Error
	assert.True(t, errors.As(err, &urlErr),
		"expected a *url.Error from the TLS layer, got %T: %v", err, err)
	t.Logf("handshake without a QWAC failed as documented: %v", err)
}

// The first call that proves a QWAC is accepted end to end. Creating a
// consent is the entry point of the whole AIS flow, so nothing else
// can be tested until this one passes.
//
// OB_IBAN must be an account the PSU owns.
func TestIntegration_CreateConsent(t *testing.T) {
	iban := envOrSkip(t, "OB_IBAN")
	psuIP := os.Getenv("OB_PSU_IP")
	if psuIP == "" {
		psuIP = "127.0.0.1"
	}
	cli, ctx := liveClient(t), liveContext(t)

	out, err := cli.CreateConsent(ctx, &openbanking.CreateConsentRequest{
		Access: openbanking.AccountAccess{
			Payments: []openbanking.AccountAccessRights{{
				Account: openbanking.Account{IBAN: iban},
				Rights: []openbanking.AccessRight{
					openbanking.AccessBalances,
					openbanking.AccessTransactions,
				},
			}},
		},
		ConsentType:        openbanking.ConsentDetailed,
		RecurringIndicator: true,
		ValidTo:            openbanking.NewDate(time.Now().AddDate(0, 1, 0)),
		FrequencyPerDay:    4,
	}, openbanking.InitiationOptions{
		PSU:            openbanking.PSU{IPAddress: psuIP},
		TPPRedirectURI: "https://example.com/psd2/return",
	})
	require.NoError(t, err, "creating a consent must succeed (201)")
	require.NotEmpty(t, out.ConsentID, "the consent id is what everything else needs")
	t.Logf("consent %s created with status %q", out.ConsentID, out.ConsentStatus)

	// Leaving a live consent behind is exactly the data-protection
	// problem the SDK warns about elsewhere.
	t.Cleanup(func() {
		if err := cli.RevokeConsent(context.Background(), out.ConsentID); err != nil {
			t.Logf("could not revoke consent %s: %v", out.ConsentID, err)
		}
	})

	// The spec does not say which state a revoked consent lands in and
	// the godoc says so. This is where we find out.
	status, err := cli.ConsentStatus(ctx, out.ConsentID)
	require.NoError(t, err)
	t.Logf("consent status right after creation: %q", status.ConsentStatus)
}

// Starting the authorisation is where the spec's silence about
// _links.scaRedirect for consents (as opposed to payments) gets
// resolved. Needs a consent id from a run of the test above.
func TestIntegration_StartConsentAuthorisation(t *testing.T) {
	consentID := envOrSkip(t, "OB_CONSENT_ID")

	out, err := liveClient(t).StartConsentAuthorisation(liveContext(t), consentID)
	require.NoError(t, err, "starting an authorisation must succeed (201)")
	t.Logf("authorisationId=%q scaStatus=%q scaRedirect=%v scaStatusLink=%v",
		out.AuthorisationID, out.SCAStatus,
		out.Links.SCARedirect != nil, out.Links.SCAStatus != nil)
}

// Reading accounts is the point of a consent; it only works once the
// PSU has approved one, so it takes the id rather than making one.
func TestIntegration_Accounts(t *testing.T) {
	consentID := envOrSkip(t, "OB_CONSENT_ID")
	psuIP := os.Getenv("OB_PSU_IP")
	if psuIP == "" {
		psuIP = "127.0.0.1"
	}

	accounts, err := liveClient(t).Accounts(liveContext(t), openbanking.AccountsOptions{
		AccountOptions: openbanking.AccountOptions{ConsentID: consentID, PSUIPAddress: psuIP},
		WithBalance:    true,
	})
	require.NoError(t, err, "listing accounts must succeed")
	for _, a := range accounts {
		// WithBalance is documented as a request, not a promise — the
		// godoc says so, and this is the check behind that wording.
		t.Logf("account %s: %d embedded balances", a.ResourceID, len(a.Balances))
	}
}

// TestIntegration_RequestTestCertificate files a sandbox certificate
// application. A human processes it, so it is gated: run it once when
// onboarding, not on a schedule.
func TestIntegration_RequestTestCertificate(t *testing.T) {
	if os.Getenv("OB_ALLOW_CERT_REQUEST") != certOptIn {
		t.Skipf("this files an application a human processes; set OB_ALLOW_CERT_REQUEST=%s to run", certOptIn)
	}
	csr := envOrSkip(t, "OB_CSR_PEM")

	cli := openbanking.New(monobank.WithBaseURL(openbanking.BaseURLSandbox))
	t.Cleanup(func() { _ = cli.Close() })

	out, err := cli.RequestTestCertificate(liveContext(t), &openbanking.CertificateRequest{
		Organization:  envOrSkip(t, "OB_ORG"),
		ContactPerson: envOrSkip(t, "OB_CONTACT"),
		Phone:         envOrSkip(t, "OB_PHONE"),
		Email:         envOrSkip(t, "OB_EMAIL"),
		Description:   "go-monobank-sdk integration onboarding",
		EDRPOU:        envOrSkip(t, "OB_EDRPOU"),
		CSRPEM:        csr,
	})
	require.NoError(t, err, "filing a certificate request must succeed (201)")
	t.Logf("certificate request accepted: %+v", out)
}
