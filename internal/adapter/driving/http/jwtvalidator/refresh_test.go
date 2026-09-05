package jwtvalidator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshTokenValidator_GetClientID(t *testing.T) {
	validator := RefreshTokenValidator{ClientID: "test-client-id"}

	assert.Equal(t, "test-client-id", validator.GetClientID())
}

func TestRefreshTokenValidator_Validate(t *testing.T) {
	ctx := context.Background()

	t.Run("verifies_through_the_signer_and_returns_the_claims", func(t *testing.T) {
		verifier := &fakeVerifier{claims: validatorClaims()}
		validator := RefreshTokenValidator{Verifier: verifier, ClientID: "test-client-id"}

		claims, err := validator.Validate(ctx, "the-token")
		require.NoError(t, err)

		assert.Equal(t, 1, verifier.calls)
		assert.Equal(t, "01a02b03-0000-7000-8000-000000000009", claims["sub"])
	})

	t.Run("refuses_what_the_signer_refuses", func(t *testing.T) {
		validator := RefreshTokenValidator{
			Verifier: &fakeVerifier{err: errors.New("crypto/ecdsa: verification error")},
			ClientID: "test-client-id",
		}

		_, err := validator.Validate(ctx, "the-token")
		require.Error(t, err)

		assert.NotContains(t, err.Error(), "crypto/ecdsa", "the library's wording must not reach a caller")
	})

	t.Run("refuses_everything_when_no_verifier_is_wired", func(t *testing.T) {
		validator := RefreshTokenValidator{ClientID: "test-client-id"}

		_, err := validator.Validate(ctx, "the-token")
		assert.Error(t, err)
	})

	t.Run("requires_a_jti_because_a_refresh_token_can_be_revoked", func(t *testing.T) {
		claims := validatorClaims()
		delete(claims, "jti")

		validator := RefreshTokenValidator{Verifier: &fakeVerifier{claims: claims}, ClientID: "test-client-id"}

		_, err := validator.Validate(ctx, "the-token")
		assert.Error(t, err, "a refresh token that cannot be named cannot be revoked")
	})
}
