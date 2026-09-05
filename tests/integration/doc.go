//go:build integration

// Package integration is the project's integration test suite. It
// exercises the HTTP API end-to-end against a real Postgres + Valkey +
// SMTP stack brought up by `make start-dev-env` + `air`.
//
// # Build tag
//
// Every file in this package is gated on `//go:build integration`, so a
// plain `go test ./...` will not pick the suite up. Run it with:
//
//	go test -race -tags=integration ./tests/integration
//	go test -race -tags=integration ./tests/integration -run TestHealthStatus
//
// # Environment requirements
//
// TestMain (see suite_test.go) probes the database, the API health
// endpoint, and the mail server with a 5-second timeout and exits
// with a clear message if any of them is unreachable, so a missing
// dev env fails fast with one line instead of a cascade of opaque
// dial errors.
//
// # Helper landscape
//
// All helpers live in helper_functions_test.go. The most-used groups:
//
// HTTP plumbing
//
//   - newAPIEndpoint(method, path)        – build an apiEndpoint for the API server
//   - newMailAPIEndpoint(method, path)    – build an apiEndpoint for the mail server
//   - apiEndpoint.RewriteSlugs(slugs...)  – fill `{id}`-style path placeholders
//   - apiEndpoint.SetQueryParam(k, v)     – mutate query string
//   - apiEndpoint.SetQueryParams(map)     – mutate multiple query params
//   - sendHTTPRequest(t, ctx, ep, body, headers...) – send a request and log it
//   - parserResponseBody[T](t, resp)      – decode JSON response into T
//   - readResponseBody(t, resp)           – read raw body (resets it for re-reads)
//
// Auth / user fixtures
//
//   - generateUserData(t)                 – unique email/firstName/lastName
//   - generatePassword(t)                 – random password meeting policy
//   - getAdminUserTokens(t)               – insert an admin user, log in, return tokens
//
// External-system readiness
//
//   - requireOllamaAvailable(t)           – t.Skipf when Ollama isn't running
//   - waitFor(t, timeout, msg, probeFn)   – bounded poll for external eventual
//     consistency (replaces fixed time.Sleep waits)
//
// Email harness
//
//   - getEmailsForRecipient(t, address)   – fetch emails delivered to an address
//   - getVerifyLinkFromEmail(t, message)  – extract verification URL
//   - deleteAllEmails()                   – wipe the mail server (teardown)
//
// DB seeding / inspection
//
//   - createUserInDB / deleteUserByEmailFromDB / enableUserByEmailFromDB
//
// # Conventions (per CLAUDE.md §Testing)
//
//   - Subtest names use snake_case: t.Run("not_found", ...), t.Run("missing_authorization", ...).
//   - Each test prefers t.Context() over context.Background() so the context
//     cancels on test cleanup.
//   - Use require.* for preconditions that must hold for the remainder of the
//     test to make sense (status code, JSON decode). Use assert.* for parallel
//     independent observations. Inconsistent past usage is being migrated.
package integration
