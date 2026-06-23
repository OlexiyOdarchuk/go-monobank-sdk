package monobanktest_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/monobanktest"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/personal"
)

func TestServer_URL_andClose(t *testing.T) {
	srv := monobanktest.NewServer(t)
	url := srv.URL()
	assert.True(t, strings.HasPrefix(url, "http://"), "URL has scheme: %s", url)

	srv.WithWebHookSubscription()
	cli := personal.New("token", srv.Option())
	require.NoError(t, cli.SetWebHook(context.Background(), "https://example.com/wh"))

	srv.Close()
	// Після Close нові запити мають падати з мережевою помилкою.
	err := cli.SetWebHook(context.Background(), "https://example.com/wh")
	require.Error(t, err)
}

func TestServer_WithServerKey(t *testing.T) {
	srv := monobanktest.NewServer(t)
	srv.WithServerKey(map[string]any{
		"serverKeyId":    "k1",
		"serverPubKey":   "BNDZP+AGo===",
		"serverTimeMsec": 1700000000000,
	})

	resp, err := http.Get(srv.URL() + "/bank/sync")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServer_WithWebHookSubscription_returns200(t *testing.T) {
	srv := monobanktest.NewServer(t)
	srv.WithWebHookSubscription()

	cli := personal.New("token", srv.Option())
	require.NoError(t, cli.SetWebHook(context.Background(), "https://example.com/wh"))
}
