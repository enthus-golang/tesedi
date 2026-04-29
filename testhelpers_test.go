package tesedi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// makeJWT builds an unsigned JWT carrying only an exp claim. The signature
// is bogus — these tokens are for client-side parsing tests, not for any
// real authorization.
func makeJWT(t *testing.T, exp time.Time) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]int64{"exp": exp.Unix()})
	require.NoError(t, err)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	sig := base64.RawURLEncoding.EncodeToString([]byte("not-a-real-sig"))
	return header + "." + payloadB64 + "." + sig
}

// validAuthHandler responds to the /auth endpoint with a 1-hour JWT when
// the apiKey header matches the expected value. authCalls, if non-nil, is
// incremented on every invocation (handy for asserting auth-call counts).
func validAuthHandler(t *testing.T, apiKey string, authCalls *int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if authCalls != nil {
			*authCalls++
		}
		if r.Header.Get("apiKey") != apiKey {
			http.Error(w, "bad key", http.StatusUnauthorized)
			return
		}
		token := makeJWT(t, time.Now().Add(time.Hour))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
	}
}

// startServer wires the given mux onto an httptest.Server with cleanup
// registered against t.
func startServer(t *testing.T, mux *http.ServeMux) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}
