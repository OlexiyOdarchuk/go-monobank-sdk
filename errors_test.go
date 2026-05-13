package monobank

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseErrorDescription(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"valid", `{"errorDescription":"Unknown 'X-Token'"}`, "Unknown 'X-Token'"},
		{"with extra fields", `{"errorDescription":"oops","traceId":"abc"}`, "oops"},
		{"missing field", `{"foo":"bar"}`, ""},
		{"not json", `<html>error</html>`, ""},
		{"empty", ``, ""},
		{"empty errorDescription", `{"errorDescription":""}`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseErrorDescription([]byte(tc.body)))
		})
	}
}

func TestAPIError_PopulatesErrorDescription(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errorDescription":"Unknown 'X-Token'"}`))
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/x", http.NoBody)
	err := c.Do(req, nil)

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "Unknown 'X-Token'", apiErr.ErrorDescription)
	// Raw body remains accessible.
	assert.Contains(t, string(apiErr.Body), "errorDescription")
}

func TestAPIError_ErrorPrefersDescription(t *testing.T) {
	e := &APIError{
		Method:           http.MethodGet,
		URL:              "https://api.monobank.ua/personal/client-info",
		StatusCode:       http.StatusForbidden,
		ErrorDescription: "Unknown 'X-Token'",
		Body:             []byte(`{"errorDescription":"Unknown 'X-Token'"}`),
	}
	s := e.Error()
	assert.Contains(t, s, "Unknown 'X-Token'")
	// JSON wrapper should NOT appear when description is set — message stays clean.
	assert.NotContains(t, s, `{"errorDescription"`)
}

func TestAPIError_ErrorFallsBackToBodyWhenNoDescription(t *testing.T) {
	e := &APIError{
		Method:     http.MethodGet,
		URL:        "https://api.monobank.ua/x",
		StatusCode: http.StatusBadGateway,
		Body:       []byte(`<html>upstream timeout</html>`),
	}
	assert.Contains(t, e.Error(), "upstream timeout")
}

func TestAPIError_NonJSONBodyLeavesDescriptionEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`<!doctype html><body>nginx</body>`))
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/x", http.NoBody)
	var apiErr *APIError
	require.ErrorAs(t, c.Do(req, nil), &apiErr)
	assert.Empty(t, apiErr.ErrorDescription)
	assert.Contains(t, string(apiErr.Body), "nginx")
}
