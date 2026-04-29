// Package tesedi is a Go client library for the Tesedi partner API.
//
// The API is authenticated via a static API key that is exchanged for a
// short-lived bearer token at the auth endpoint. The client caches the token
// internally and refreshes it proactively before expiry.
//
// All public methods accept a context.Context for cancellation and timeout
// control. Non-2xx responses surface as typed errors that wrap the upstream
// status code and body.
package tesedi
