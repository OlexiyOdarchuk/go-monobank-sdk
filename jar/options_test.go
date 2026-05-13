package jar_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/jar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithHTTPClient_isApplied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jarId":"x","title":"T","ownerName":"O","amount":1,"goal":2,"currency":980}`))
	}))
	defer srv.Close()

	custom := &http.Client{Timeout: 5 * time.Second}
	c := jar.New(
		jar.WithHTTPClient(custom),
		jar.WithAPIBaseURL(srv.URL),
	)
	info, err := c.ByLongID(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, "x", info.JarID)
}
