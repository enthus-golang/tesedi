package tesedi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// startServer wires the given mux onto an httptest.Server with cleanup
// registered against t.
func startServer(t *testing.T, mux *http.ServeMux) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// requireAPIKey returns true if the request carries the expected x-api-key
// header. Tests use this to assert that the client always sends auth.
func requireAPIKey(r *http.Request, expected string) bool {
	return r.Header.Get("x-api-key") == expected
}
