package bank

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Real /bank/sync response captured against mono — exposes the bank's
// secp256k1 public point in uncompressed form (1+32+32 = 65 bytes,
// base64-encoded).
const validServerKeyResponse = `{
	"serverKeyId":"2626ff34473bb66260b930af946fa9641a06bcd4",
	"serverPubKey":"BNDZP+AGoRC+ER1plDSUCHOw2/aBNIocmD2gS/v34/b0iQ1HBo+oS3/f402e3OXA5uCxakSjuxGMP6X0XP9VIUk=",
	"serverTimeMsec":1700000000000
}`

func TestServerKey_happyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/bank/sync", r.URL.Path)
		_, _ = w.Write([]byte(validServerKeyResponse))
	}))
	defer srv.Close()

	c := New(monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
	sk, err := c.ServerKey(context.Background())
	require.NoError(t, err)
	require.NotNil(t, sk)

	assert.Equal(t, "2626ff34473bb66260b930af946fa9641a06bcd4", sk.ID)
	require.NotNil(t, sk.PubKey)
	// secp256k1 point coordinates are positive 256-bit ints.
	assert.NotNil(t, sk.PubKey.X)
	assert.NotNil(t, sk.PubKey.Y)
	assert.Equal(t, time.UnixMilli(1700000000000), sk.ServerTime)
}

func TestServerKey_badBase64(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"serverKeyId":"k","serverPubKey":"!!!not base64!!!","serverTimeMsec":0}`))
	}))
	defer srv.Close()

	c := New(monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
	_, err := c.ServerKey(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode serverPubKey")
}
