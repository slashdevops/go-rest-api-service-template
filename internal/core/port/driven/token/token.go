// Package token defines the driven port that use-cases consume to
// sign and verify their own JWTs (access tokens, refresh tokens,
// account-verification tokens, password-reset tokens, IDP state
// tokens, personal-access tokens). The implementation lives in
// internal/adapter/driven/tokenjwt which wraps the
// github.com/golang-jwt/jwt/v5 library.
//
// The Verify method maps every concrete jwt.Err* sentinel to a
// domain.InvalidJWTError so use-cases never need to import the jwt
// package or pattern-match on its error types.
package token

import (
	"context"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/token.go -source=token.go Signer

// Signer is the driven port consumed by use-cases.
type Signer interface {
	// Sign produces a signed JWT string from the given domain claims.
	Sign(ctx context.Context, claims domain.JWTClaims) (string, error)

	// Verify parses and validates a signed JWT and returns its claims
	// as an opaque map. On any verification failure (expired, bad
	// signature, malformed, etc.) the error is a *domain.InvalidJWTError.
	Verify(ctx context.Context, token string) (map[string]any, error)
}
