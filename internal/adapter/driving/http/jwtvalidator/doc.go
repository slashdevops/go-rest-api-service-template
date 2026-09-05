// Package jwtvalidator provides JWT validation primitives used by HTTP
// middleware and services.
//
// The package defines a common Validator interface and concrete validators for
// the token classes used by this service:
//   - AccessTokenValidator
//   - RefreshTokenValidator
//   - PasswordResetTokenValidator
//
// The validators do not verify signatures themselves. Each holds a [Verifier] —
// satisfied by the token.Signer adapter — which checks the signature, the
// signing algorithm, iss, aud and exp in one place. This package adds only the
// per-class claim policy on top.
//
// It used to carry its own verification routine, and the two disagreed: this
// one pinned the signing method and checked no kid, the other checked kid and
// pinned no method, and neither checked iss or aud though every token carries
// both. Two verifiers that disagree are not defence in depth, because the
// weaker one is the one that decides.
//
// ValidatorType identifies validator purpose when wiring middleware and
// dependency maps:
//   - ValidatorTypeAccessToken
//   - ValidatorTypeRefreshToken
//   - ValidatorTypePasswordResetToken
//
// Validation behavior:
//   - Access token validation does not require a JTI claim.
//   - Refresh and password-reset token validation require a valid non-nil JTI
//     claim parsable as UUID.
//
// Error model:
//   - InvalidTokenError is returned when verification fails.
//   - InvalidClaimsError is returned when claims are malformed or required
//     claims (such as JTI) are missing/invalid.
//
// Both are deliberately opaque. The reason a token was refused goes to the log;
// what a caller is told is decided by the middleware, in one place. Callers used
// to receive the jwt library's own text — "crypto/ecdsa: verification error" —
// which made a dependency's internals part of this API's contract.
//
// The package intentionally focuses on token integrity and claim-shape checks;
// route-level authorization and token_type policy checks are applied by HTTP
// middleware in internal/http/middleware.
package jwtvalidator
