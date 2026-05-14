package bank

import (
	"context"
	"encoding/base64"
	"fmt"
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

// Регресія M2: MITM-стиль атаки — підмінити /bank/sync на uncompressed
// точку (X, Y), яка проходить базовий length/prefix-чек, але НЕ лежить
// на кривій secp256k1. До фіксу asServerKey приймала такі ключі, і всі
// наступні webhook-верифікації провалювалися 401-ми, що тригерило
// нескінченний refresh-storm на /bank/sync.
func TestServerKey_rejectsOffCurvePoint(t *testing.T) {
	// 1+32+32 = 65 byte uncompressed point with prefix 0x04, але координати
	// фіктивні (X=Y=1) — точно не на secp256k1.
	point := make([]byte, 65)
	point[0] = 0x04
	point[32] = 1 // X low byte
	point[64] = 1 // Y low byte
	body := fmt.Sprintf(`{"serverKeyId":"k","serverPubKey":%q,"serverTimeMsec":0}`,
		base64.StdEncoding.EncodeToString(point))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New(monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
	_, err := c.ServerKey(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPubKey, "off-curve point must be rejected")
}

func TestServerKey_rejectsWrongLength(t *testing.T) {
	short := make([]byte, 32) // не 65 байтів
	body := fmt.Sprintf(`{"serverKeyId":"k","serverPubKey":%q,"serverTimeMsec":0}`,
		base64.StdEncoding.EncodeToString(short))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New(monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
	_, err := c.ServerKey(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPubKey)
}

func TestServerKey_rejectsWrongPrefix(t *testing.T) {
	// Compressed point (0x02 or 0x03) — секцію не приймаємо, бо очікуємо 0x04.
	point := make([]byte, 65)
	point[0] = 0x02
	body := fmt.Sprintf(`{"serverKeyId":"k","serverPubKey":%q,"serverTimeMsec":0}`,
		base64.StdEncoding.EncodeToString(point))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New(monobank.WithBaseURL(srv.URL), monobank.WithHTTPClient(srv.Client()))
	_, err := c.ServerKey(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPubKey)
}
