package jwtvalidator

import (
	"context"
)

type ValidatorType string

const (
	ValidatorTypeAccessToken        ValidatorType = "accessToken"
	ValidatorTypeRefreshToken       ValidatorType = "refreshToken"
	ValidatorTypePasswordResetToken ValidatorType = "passwordResetToken"
	ValidatorTypeVerificationToken  ValidatorType = "verificationToken"
)

// String returns the string representation of the ValidatorType.
func (vt ValidatorType) String() string {
	return string(vt)
}

// Verifier is the one capability a validator needs from outside: something that
// checks a token's signature and its registered claims and hands back what it
// carried.
//
// It is satisfied by the token.Signer adapter, which is what makes this package
// a policy layer rather than a second implementation of JWT verification. The
// two used to disagree — the middleware pinned the signing algorithm while the
// use-case path relied on a library behaviour, and neither validated iss or
// aud. The weaker of two verifiers is the one that decides, so there is now one.
type Verifier interface {
	Verify(ctx context.Context, token string) (claims map[string]any, err error)
}

// Validator is an interface for validating JWT tokens.
type Validator interface {
	// Validate validates a JWT token and returns the claims if the token is valid.
	Validate(ctx context.Context, token string) (claims map[string]any, err error)

	// GetClientID returns the client ID of the Validator.
	GetClientID() string
}
