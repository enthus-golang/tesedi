package tesedi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticate_Success(t *testing.T) {
	authCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", &authCalls))
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k")
	require.NoError(t, c.authenticate(context.Background()))
	assert.Equal(t, 1, authCalls)
	assert.NotEmpty(t, c.token)
	assert.True(t, c.tokenExpiry.After(time.Now()))
}

func TestAuthenticate_BadKeyReturnsAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "different-key", nil))
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "wrong")
	err := c.authenticate(context.Background())
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	assert.True(t, errors.Is(err, ErrUnauthorized))
}

func TestAuthenticate_TokenReuse(t *testing.T) {
	authCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", &authCalls))
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k")
	require.NoError(t, c.authenticate(context.Background()))
	require.NoError(t, c.authenticate(context.Background()))
	require.NoError(t, c.authenticate(context.Background()))
	assert.Equal(t, 1, authCalls, "token should be cached across calls")
}

func TestAuthenticate_RefreshOnExpiry(t *testing.T) {
	authCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		authCalls++
		token := makeJWT(t, time.Now().Add(2*time.Second))
		_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k")
	require.NoError(t, c.authenticate(context.Background()))
	require.Equal(t, 1, authCalls)

	// Force the cached expiry into the leeway window so the next call refreshes.
	c.tokenMu.Lock()
	c.tokenExpiry = time.Now().Add(10 * time.Second) // less than tokenRefreshLeeway in the future
	c.tokenMu.Unlock()

	require.NoError(t, c.authenticate(context.Background()))
	assert.Equal(t, 2, authCalls, "stale token should trigger refresh")
}

func TestAuthenticate_MalformedJWT(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "not.a.jwt.too.many.parts"})
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k")
	err := c.authenticate(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse token expiry")
}

func TestAuthenticate_MissingTokenInResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"hello": "world"})
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k")
	err := c.authenticate(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing token")
}

func TestInvalidateToken_ClearsCache(t *testing.T) {
	c := New("https://api.example", "https://auth.example", "k")
	c.token = "x"
	c.tokenExpiry = time.Now().Add(time.Hour)

	c.invalidateToken()

	assert.Empty(t, c.token)
	assert.True(t, c.tokenExpiry.IsZero())
}

func TestJWTExpiry_ValidToken(t *testing.T) {
	exp := time.Now().Add(time.Hour).Truncate(time.Second)
	token := makeJWT(t, exp)

	got, err := jwtExpiry(token)
	require.NoError(t, err)
	assert.True(t, got.Equal(exp), "expected %s, got %s", exp, got)
}

func TestJWTExpiry_BadFormat(t *testing.T) {
	cases := map[string]string{
		"only one part":    "abc",
		"two parts":        "abc.def",
		"non-base64":       "abc.!!!.xyz",
		"missing exp":      "abc." + base64.RawURLEncoding.EncodeToString([]byte(`{"foo":"bar"}`)) + ".xyz",
		"non-json payload": "abc." + base64.RawURLEncoding.EncodeToString([]byte("not json")) + ".xyz",
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := jwtExpiry(token)
			require.Error(t, err)
		})
	}
}

func TestJWTExpiry_PaddedBase64(t *testing.T) {
	// Some tokens use base64.URLEncoding (with padding) instead of RawURLEncoding.
	header := base64.URLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, err := json.Marshal(map[string]int64{"exp": time.Now().Add(time.Hour).Unix()})
	require.NoError(t, err)
	payloadB64 := base64.URLEncoding.EncodeToString(payload)
	token := strings.Join([]string{header, payloadB64, "sig"}, ".")

	_, err = jwtExpiry(token)
	require.NoError(t, err)
}
