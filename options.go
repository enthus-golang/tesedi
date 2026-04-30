package tesedi

import (
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets a custom HTTP client. The default is a new client with
// a 30s timeout. nil is ignored.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// WithRateLimit caps requests per minute and per hour. A non-positive value
// disables that limit. Both limits are evaluated; a request blocks (respecting
// context) when either bucket is empty.
func WithRateLimit(perMinute, perHour int) Option {
	return func(c *Client) {
		if perMinute > 0 {
			c.minLimiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(perMinute)), perMinute)
		}
		if perHour > 0 {
			c.hourLimiter = rate.NewLimiter(rate.Every(time.Hour/time.Duration(perHour)), perHour)
		}
	}
}

// WithRetry configures retry behavior for transient failures (network errors,
// HTTP 5xx, HTTP 429). maxAttempts is the total number of attempts including
// the first; pass 1 to disable retries. baseBackoff is the initial delay
// between attempts; subsequent delays double up to a 30s cap with ±25% jitter.
// HTTP 429 honors the Retry-After header when present.
func WithRetry(maxAttempts int, baseBackoff time.Duration) Option {
	return func(c *Client) {
		if maxAttempts < 1 {
			maxAttempts = 1
		}
		c.maxAttempts = maxAttempts
		c.baseBackoff = baseBackoff
	}
}

// WithLogger sets a logger for transient retry events. nil disables logging.
func WithLogger(logger Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}
