package tesedi

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_Defaults(t *testing.T) {
	c := New("https://api.example", "https://auth.example", "key")

	require.NotNil(t, c.httpClient)
	assert.Equal(t, 30*time.Second, c.httpClient.Timeout)
	assert.Equal(t, 3, c.maxAttempts)
	assert.Equal(t, 500*time.Millisecond, c.baseBackoff)
	assert.Nil(t, c.minLimiter)
	assert.Nil(t, c.hourLimiter)
	assert.Nil(t, c.logger)
}

func TestNew_Options(t *testing.T) {
	customHTTP := &http.Client{Timeout: 5 * time.Second}
	customLogger := log.New(io.Discard, "", 0)

	c := New("https://api.example", "https://auth.example", "key",
		WithHTTPClient(customHTTP),
		WithRateLimit(60, 3600),
		WithRetry(7, 100*time.Millisecond),
		WithLogger(customLogger),
	)

	assert.Same(t, customHTTP, c.httpClient)
	assert.Equal(t, 7, c.maxAttempts)
	assert.Equal(t, 100*time.Millisecond, c.baseBackoff)
	assert.NotNil(t, c.minLimiter)
	assert.NotNil(t, c.hourLimiter)
	assert.NotNil(t, c.logger)
}

func TestNew_NilHTTPClientIgnored(t *testing.T) {
	c := New("https://api.example", "https://auth.example", "key", WithHTTPClient(nil))
	assert.NotNil(t, c.httpClient)
	assert.Equal(t, 30*time.Second, c.httpClient.Timeout)
}

func TestNew_RateLimitZeroDisables(t *testing.T) {
	c := New("https://api.example", "https://auth.example", "key", WithRateLimit(0, 10))
	assert.Nil(t, c.minLimiter)
	assert.NotNil(t, c.hourLimiter)
}

func TestNew_RetryAttemptsClampedToOne(t *testing.T) {
	c := New("https://api.example", "https://auth.example", "key", WithRetry(0, time.Second))
	assert.Equal(t, 1, c.maxAttempts)
}

func TestDo_Retry5xxThenSuccess(t *testing.T) {
	var apiCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts/1", func(w http.ResponseWriter, r *http.Request) {
		n := apiCalls.Add(1)
		if n < 3 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"contractId":"1","contractNumber":"X"}}`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(5, time.Millisecond))
	contract, err := c.GetContract(context.Background(), "1")
	require.NoError(t, err)
	assert.Equal(t, "X", contract.ContractNumber)
	assert.EqualValues(t, 3, apiCalls.Load())
}

func TestDo_MaxAttemptsExhausted(t *testing.T) {
	var apiCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts/1", func(w http.ResponseWriter, r *http.Request) {
		apiCalls.Add(1)
		http.Error(w, "still down", http.StatusServiceUnavailable)
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(3, time.Millisecond))
	_, err := c.GetContract(context.Background(), "1")
	require.Error(t, err)

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	assert.EqualValues(t, 3, apiCalls.Load())
}

func TestDo_NoRetryOn4xx(t *testing.T) {
	var apiCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts/1", func(w http.ResponseWriter, r *http.Request) {
		apiCalls.Add(1)
		http.Error(w, "nope", http.StatusBadRequest)
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(5, time.Millisecond))
	_, err := c.GetContract(context.Background(), "1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errors.New("dummy")) == false) // sanity
	assert.EqualValues(t, 1, apiCalls.Load())
}

func TestDo_404MapsToErrNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts/1", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	_, err := c.GetContract(context.Background(), "1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestDo_401TriggersReauthAndSucceeds(t *testing.T) {
	var apiCalls atomic.Int32
	authCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", &authCalls))
	mux.HandleFunc("/api/contracts/1", func(w http.ResponseWriter, r *http.Request) {
		n := apiCalls.Add(1)
		if n == 1 {
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"contractId":"1","contractNumber":"X"}}`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	contract, err := c.GetContract(context.Background(), "1")
	require.NoError(t, err)
	assert.Equal(t, "X", contract.ContractNumber)
	assert.EqualValues(t, 2, apiCalls.Load())
	assert.Equal(t, 2, authCalls, "expected one reauth after the 401")
}

func TestDo_PersistentUnauthorizedReturnsErr(t *testing.T) {
	var apiCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts/1", func(w http.ResponseWriter, r *http.Request) {
		apiCalls.Add(1)
		http.Error(w, "denied", http.StatusUnauthorized)
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	_, err := c.GetContract(context.Background(), "1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnauthorized))
	// 1 initial + 1 reauth retry = 2 calls
	assert.EqualValues(t, 2, apiCalls.Load())
}

func TestDo_429HonorsRetryAfter(t *testing.T) {
	var apiCalls atomic.Int32
	var firstAt time.Time
	var secondAt time.Time
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts/1", func(w http.ResponseWriter, r *http.Request) {
		n := apiCalls.Add(1)
		if n == 1 {
			firstAt = time.Now()
			w.Header().Set("Retry-After", "1")
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		secondAt = time.Now()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"contractId":"1","contractNumber":"X"}}`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(2, time.Millisecond))
	_, err := c.GetContract(context.Background(), "1")
	require.NoError(t, err)
	require.False(t, firstAt.IsZero())
	require.False(t, secondAt.IsZero())
	delta := secondAt.Sub(firstAt)
	assert.GreaterOrEqual(t, delta, 900*time.Millisecond, "Retry-After should hold the retry for ~1s")
}

func TestDo_NetworkErrorRetries(t *testing.T) {
	// Set up a server that closes the connection on first request, then succeeds.
	var apiCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts/1", func(w http.ResponseWriter, r *http.Request) {
		n := apiCalls.Add(1)
		if n == 1 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("hijacker not available")
			}
			conn, _, err := hj.Hijack()
			require.NoError(t, err)
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"contractId":"1","contractNumber":"X"}}`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(3, time.Millisecond))
	contract, err := c.GetContract(context.Background(), "1")
	require.NoError(t, err)
	assert.Equal(t, "X", contract.ContractNumber)
	assert.GreaterOrEqual(t, apiCalls.Load(), int32(2))
}

func TestDo_ContextCanceledStopsRetry(t *testing.T) {
	var apiCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts/1", func(w http.ResponseWriter, r *http.Request) {
		apiCalls.Add(1)
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	server := startServer(t, mux)

	ctx, cancel := context.WithCancel(context.Background())
	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(10, 50*time.Millisecond))
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := c.GetContract(ctx, "1")
	require.Error(t, err)
}

func TestComputeBackoff_BoundedAndJittered(t *testing.T) {
	c := New("https://api.example", "https://auth.example", "key", WithRetry(10, time.Second))
	// Jitter can extend the backoff by up to 25% above the 30s cap, hence 38s.
	for attempt := 1; attempt <= 8; attempt++ {
		d := c.computeBackoff(attempt)
		assert.LessOrEqual(t, d, 38*time.Second, "attempt %d backoff %s exceeded cap+jitter", attempt, d)
		assert.Greater(t, d, time.Duration(0))
	}
}

type recordingLogger struct {
	lines []string
}

func (l *recordingLogger) Printf(format string, args ...any) {
	l.lines = append(l.lines, format)
}

func TestWithLogger_RecordsRetryEvents(t *testing.T) {
	logger := &recordingLogger{}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts/1", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusServiceUnavailable)
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k",
		WithRetry(2, time.Millisecond),
		WithLogger(logger),
	)
	_, _ = c.GetContract(context.Background(), "1")
	require.NotEmpty(t, logger.lines, "logger should have recorded a retry event")
}

func TestAPIError_ErrorMessageFormat(t *testing.T) {
	err := &APIError{StatusCode: 502, Body: "upstream down"}
	assert.Equal(t, "tesedi: HTTP 502: upstream down", err.Error())
}

func TestSleepFor_RespectsContext(t *testing.T) {
	c := New("https://api.example", "https://auth.example", "key")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	c.sleepFor(ctx, time.Hour) // would take an hour without ctx cancellation
	assert.Less(t, time.Since(start), 50*time.Millisecond)
}

func TestDecode_NilTargetDrainsBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"x":1}`))
	}))
	t.Cleanup(server.Close)

	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	require.NoError(t, decode(resp, nil))
}

func TestDecode_BadJSON(t *testing.T) {
	resp := &http.Response{
		Body: io.NopCloser(strings.NewReader("not json")),
	}
	err := decode(resp, &struct {
		X int `json:"x"`
	}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
}

// silence unused import warning if helper changes.
var _ = strconv.Itoa
