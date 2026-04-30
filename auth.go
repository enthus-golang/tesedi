package tesedi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// tokenRefreshLeeway is how far before the JWT exp claim we proactively
// refresh the cached token.
const tokenRefreshLeeway = 60 * time.Second

// authenticate ensures c.token is valid for at least tokenRefreshLeeway from
// now. It issues an /auth call when the cached token is missing or near
// expiry. Concurrent callers serialize on c.tokenMu — only one HTTP exchange
// happens at a time.
func (c *Client) authenticate(ctx context.Context) error {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.token != "" && time.Now().Add(tokenRefreshLeeway).Before(c.tokenExpiry) {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.authURL, nil)
	if err != nil {
		return fmt.Errorf("tesedi: build auth request: %w", err)
	}
	req.Header.Set("apiKey", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("tesedi: auth request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var authResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return fmt.Errorf("tesedi: decode auth response: %w", err)
	}
	if authResp.Token == "" {
		return fmt.Errorf("tesedi: auth response missing token")
	}

	expiry, err := jwtExpiry(authResp.Token)
	if err != nil {
		return fmt.Errorf("tesedi: parse token expiry: %w", err)
	}

	c.token = authResp.Token
	c.tokenExpiry = expiry
	return nil
}

func (c *Client) invalidateToken() {
	c.tokenMu.Lock()
	c.token = ""
	c.tokenExpiry = time.Time{}
	c.tokenMu.Unlock()
}

// jwtExpiry decodes the unverified payload of a JWT and returns the time
// indicated by its exp claim. The signature is not validated — that is the
// auth server's responsibility.
func jwtExpiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("malformed JWT (need 3 parts, got %d)", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some JWTs include trailing '=' padding.
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("base64 decode: %w", err)
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.NewDecoder(bytes.NewReader(payload)).Decode(&claims); err != nil {
		return time.Time{}, fmt.Errorf("decode payload: %w", err)
	}
	if claims.Exp == 0 {
		return time.Time{}, fmt.Errorf("missing exp claim")
	}
	return time.Unix(claims.Exp, 0), nil
}
