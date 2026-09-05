// Package respond contains helpers for writing JSON HTTP responses in a
// consistent format.
//
// It provides two response entry points:
//   - WriteJSONData: writes any JSON-serializable payload with a status code.
//   - WriteJSONMessage: writes a structured payload.HTTPMessage containing
//     timestamp, HTTP status, request method, and request path.
//
// WriteJSONMessage is used by handlers and middleware for user-facing API
// messages and centralizes structured logging fields for request diagnostics.
//
// The package also uses an internal sync.Pool for HTTPMessage instances to
// reduce allocations under high request throughput.
//
// All writers set Content-Type to "application/json" before emitting the
// response body.
package respond
