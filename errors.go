package tesedi

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors. Match with errors.Is.
var (
	// ErrContractNotFound is returned by GetContractByNumber when the search
	// yields no exact match for the requested contract number.
	ErrContractNotFound = errors.New("tesedi: contract not found")

	// ErrAmbiguousContractNumber is returned by GetContractByNumber when the
	// search yields more than one exact match. This indicates upstream data
	// inconsistency.
	ErrAmbiguousContractNumber = errors.New("tesedi: ambiguous contract number")

	// ErrUnauthorized matches HTTP 401/403 responses via errors.Is.
	ErrUnauthorized = errors.New("tesedi: unauthorized")

	// ErrNotFound matches HTTP 404 responses via errors.Is.
	ErrNotFound = errors.New("tesedi: not found")
)

// APIError carries the HTTP status code and raw response body for an
// unsuccessful API call.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("tesedi: HTTP %d: %s", e.StatusCode, e.Body)
}

// Is allows callers to match ErrUnauthorized / ErrNotFound via errors.Is.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrUnauthorized:
		return e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden
	case ErrNotFound:
		return e.StatusCode == http.StatusNotFound
	}
	return false
}
