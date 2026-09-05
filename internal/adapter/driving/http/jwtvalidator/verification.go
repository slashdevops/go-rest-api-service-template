package jwtvalidator

import (
	"context"
)

// VerificationTokenValidator validates account-verification tokens issued by
// THIS service.
//
// It requires no jti, for the same reason an access token does not: a
// verification token is not on the denylist, so there is nothing to name it by.
// Making it single-use is a separate piece of work — until then, the token
// works until it expires, however many times it is presented.
type VerificationTokenValidator struct {
	Verifier Verifier
	ClientID string
}

// Validate validates a JWT token and returns the claims if the token is valid.
func (ref *VerificationTokenValidator) Validate(ctx context.Context, token string) (claims map[string]any, err error) {
	return validateToken(ctx, ref.Verifier, token, false)
}

// GetClientID returns the client ID of the VerificationTokenValidator.
func (ref *VerificationTokenValidator) GetClientID() string {
	return ref.ClientID
}
