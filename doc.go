// Package tesedi is a Go client library for the Tesedi Asset Hub Partner
// API.
//
// The API is authenticated via a static partner API key sent on every
// request as the "x-api-key" header.
//
// All public methods accept a context.Context for cancellation and timeout
// control. Non-2xx responses surface as typed errors that wrap the upstream
// status code and body.
package tesedi
