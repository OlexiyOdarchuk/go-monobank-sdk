package openbanking

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
)

// Identifiers come from the bank, but they end up in a URL path, so a
// value containing "/" or "?" must not be able to reshape the request
// — least of all climb out of the resource it addresses.
func TestPathSegmentsAreEscaped(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   string
		// wantPath is the path the server decodes, i.e. the id must
		// arrive as one segment however it was spelled.
		wantPath string
		// wantRaw is what has to appear escaped on the wire.
		wantRaw string
	}{
		{"slash", "a/b", "/ext/v2/consents/account-access/a/b/status", "%2F"},
		// Dots inside a longer segment are harmless — the slashes that
		// would make them a traversal are escaped. The dangerous case
		// is a segment that IS a dot; it has its own test below.
		{"dots with slashes", "../../admin", "/ext/v2/consents/account-access/../../admin/status", "%2F"},
		{"question mark", "a?b", "/ext/v2/consents/account-access/a?b/status", "%3F"},
		{"cyrillic", "згода", "/ext/v2/consents/account-access/згода/status", "%D0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotRawURI string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotRawURI = r.URL.Path, r.RequestURI
				_, _ = w.Write([]byte(`{"consentStatus":"valid"}`))
			}))
			t.Cleanup(srv.Close)

			cli := New(monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
			t.Cleanup(func() { _ = cli.Close() })

			_, err := cli.ConsentStatus(context.Background(), tc.id)
			require.NoError(t, err)

			// The id stays inside its own segment: no traversal, no
			// stray query string.
			assert.Equal(t, tc.wantPath, gotPath)
			assert.Contains(t, gotRawURI, tc.wantRaw, "the separator must travel percent-encoded")
			assert.True(t, strings.HasSuffix(gotRawURI, "/status"),
				"the trailing segment must survive: %s", gotRawURI)
		})
	}
}

// url.PathEscape does not touch a dot, and the path is later resolved
// against the base URL — where Go collapses dot-segments. A bare ".."
// would therefore address a different resource than the caller named,
// so it has to be rejected before the request is built.
func TestDotSegmentsRejected(t *testing.T) {
	for _, id := range []string{"..", "."} {
		t.Run(id, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Errorf("a request went out for id %q", id)
			}))
			t.Cleanup(srv.Close)

			cli := New(monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
			t.Cleanup(func() { _ = cli.Close() })
			ctx := context.Background()

			_, err := cli.ConsentStatus(ctx, id)
			assert.ErrorIs(t, err, ErrEmptyID)
			_, err = cli.Consent(ctx, id)
			assert.ErrorIs(t, err, ErrEmptyID)
			assert.ErrorIs(t, cli.RevokeConsent(ctx, id), ErrEmptyID)
			_, err = cli.StartConsentAuthorisation(ctx, id)
			assert.ErrorIs(t, err, ErrEmptyID)
			_, err = cli.Balances(ctx, id, AccountOptions{ConsentID: "c", PSUIPAddress: "127.0.0.1"})
			assert.ErrorIs(t, err, ErrEmptyID)
			_, err = cli.Payment(ctx, CreditTransfers, id)
			assert.ErrorIs(t, err, ErrEmptyID)
			assert.ErrorIs(t, cli.CancelPayment(ctx, CreditTransfers, id), ErrEmptyID)
		})
	}
}
