package jwtvalidator

import (
	"context"
)

// RefreshTokenValidator validates tokens issued by THIS service.
//
// The doc comment used to say "issued by Google Identity Platform", which it
// has never done: these are our own ES256 tokens, and an identity provider's
// tokens are handled by the oauthidp adapter.
type RefreshTokenValidator struct {
	Verifier Verifier
	ClientID string
}

// Validate validates a JWT token and returns the claims if the token is valid.
func (ref *RefreshTokenValidator) Validate(ctx context.Context, token string) (claims map[string]any, err error) {
	return validateToken(ctx, ref.Verifier, token, true)
}

// GetClientID returns the client ID of the RefreshTokenValidator.
func (ref *RefreshTokenValidator) GetClientID() string {
	return ref.ClientID
}
