package tesedi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

// Client is a Tesedi API client. It is safe for concurrent use by multiple
// goroutines.
type Client struct {
	baseURL string
	apiKey  string

	httpClient  *http.Client
	minLimiter  *rate.Limiter
	hourLimiter *rate.Limiter

	maxAttempts int
	baseBackoff time.Duration
	logger      Logger
}

// New creates a Tesedi API client.
//
// baseURL is the API root, e.g. "https://example.tesedi.com/api".
// apiKey is the static partner key sent on every request as the
// "x-api-key" header (per the Asset Hub Partner API spec).
//
// Both arguments are required; this constructor does not validate them
// — instead, the first call needing them surfaces any errors.
func New(baseURL, apiKey string, opts ...Option) *Client {
	c := &Client{
		baseURL:     baseURL,
		apiKey:      apiKey,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		maxAttempts: 3,
		baseBackoff: 500 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// do performs an HTTP request, applying rate limits and retries. path is
// appended to baseURL. query may be nil. On success, the caller owns the
// response body and must close it. Non-2xx responses are surfaced as
// *APIError after the body is read and closed.
func (c *Client) do(ctx context.Context, method, path string, query url.Values) (*http.Response, error) {
	if c.minLimiter != nil {
		if err := c.minLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("tesedi: rate limit wait: %w", err)
		}
	}
	if c.hourLimiter != nil {
		if err := c.hourLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("tesedi: rate limit wait: %w", err)
		}
	}

	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var lastErr error
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
		if err != nil {
			return nil, fmt.Errorf("tesedi: build request: %w", err)
		}
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("Accept", "application/json")

		resp, doErr := c.httpClient.Do(req)
		if doErr == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		if doErr != nil {
			lastErr = fmt.Errorf("tesedi: request: %w", doErr)
			if !c.shouldRetry(attempt, 0, true) {
				return nil, lastErr
			}
			c.sleepBeforeRetry(ctx, attempt, nil)
			continue
		}

		// Non-2xx: drain and close body so the connection can be reused.
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		apiErr := &APIError{StatusCode: resp.StatusCode, Body: string(body)}
		lastErr = apiErr

		if !c.shouldRetry(attempt, resp.StatusCode, false) {
			return nil, apiErr
		}
		c.sleepBeforeRetry(ctx, attempt, resp)
	}

	return nil, lastErr
}

func (c *Client) shouldRetry(attempt, statusCode int, networkErr bool) bool {
	if attempt >= c.maxAttempts {
		return false
	}
	if networkErr {
		return true
	}
	if statusCode == http.StatusTooManyRequests {
		return true
	}
	if statusCode >= 500 && statusCode <= 599 {
		return true
	}
	return false
}

func (c *Client) sleepBeforeRetry(ctx context.Context, attempt int, resp *http.Response) {
	d := c.computeBackoff(attempt)
	if resp != nil {
		if v := resp.Header.Get("Retry-After"); v != "" {
			if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
				d = time.Duration(secs) * time.Second
			}
		}
	}
	if c.logger != nil {
		c.logger.Printf("tesedi: retry attempt %d after %s", attempt, d)
	}
	c.sleepFor(ctx, d)
}

func (c *Client) computeBackoff(attempt int) time.Duration {
	const cap = 30 * time.Second
	if c.baseBackoff <= 0 {
		return 0
	}
	d := c.baseBackoff << uint(attempt-1)
	if d <= 0 || d > cap {
		d = cap
	}
	jitter := time.Duration(rand.Int64N(int64(d)/2+1)) - d/4
	return d + jitter
}

func (c *Client) sleepFor(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// decode reads and JSON-decodes a successful response into v, then closes the body.
func decode(resp *http.Response, v any) error {
	defer func() { _ = resp.Body.Close() }()
	if v == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("tesedi: decode response: %w", err)
	}
	return nil
}
