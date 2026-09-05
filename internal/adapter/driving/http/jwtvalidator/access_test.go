package jwtvalidator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccessTokenValidator_GetClientID(t *testing.T) {
	validator := AccessTokenValidator{ClientID: "test-client-id"}

	assert.Equal(t, "test-client-id", validator.GetClientID())
}

func TestAccessTokenValidator_Validate(t *testing.T) {
	ctx := context.Background()

	t.Run("verifies_through_the_signer_and_returns_the_claims", func(t *testing.T) {
		verifier := &fakeVerifier{claims: validatorClaims()}
		validator := AccessTokenValidator{Verifier: verifier, ClientID: "test-client-id"}

		claims, err := validator.Validate(ctx, "the-token")
		require.NoError(t, err)

		assert.Equal(t, 1, verifier.calls)
		assert.Equal(t, "01a02b03-0000-7000-8000-000000000009", claims["sub"])
	})

	t.Run("refuses_what_the_signer_refuses", func(t *testing.T) {
		validator := AccessTokenValidator{
			Verifier: &fakeVerifier{err: errors.New("crypto/ecdsa: verification error")},
			ClientID: "test-client-id",
		}

		_, err := validator.Validate(ctx, "the-token")
		require.Error(t, err)

		assert.NotContains(t, err.Error(), "crypto/ecdsa", "the library's wording must not reach a caller")
	})

	t.Run("refuses_everything_when_no_verifier_is_wired", func(t *testing.T) {
		validator := AccessTokenValidator{ClientID: "test-client-id"}

		_, err := validator.Validate(ctx, "the-token")
		assert.Error(t, err)
	})

	t.Run("needs_no_jti_because_an_access_token_is_not_revocable", func(t *testing.T) {
		claims := validatorClaims()
		delete(claims, "jti")

		validator := AccessTokenValidator{Verifier: &fakeVerifier{claims: claims}, ClientID: "test-client-id"}

		_, err := validator.Validate(ctx, "the-token")
		assert.NoError(t, err, "access tokens are not denylisted, so they carry no revocation identity")
	})
}
