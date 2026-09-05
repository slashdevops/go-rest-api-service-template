package jwtvalidator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeVerifier stands in for the token signer.
//
// Signature, algorithm, iss, aud and exp are not this package's job any more —
// they are checked once, in the tokenjwt adapter, and tested there. What is
// left here is policy: which classes require a jti, and what a refused caller
// is told. A fake keeps those two concerns from being tested through each other.
type fakeVerifier struct {
	claims map[string]any
	err    error
	calls  int
	token  string
}

func (f *fakeVerifier) Verify(_ context.Context, token string) (map[string]any, error) {
	f.calls++
	f.token = token

	return f.claims, f.err
}

func validatorClaims() map[string]any {
	return map[string]any{
		"sub":        "01a02b03-0000-7000-8000-000000000009",
		"token_type": "refresh",
		"jti":        "01a02b03-0000-7000-8000-000000000001",
	}
}

func TestValidateToken(t *testing.T) {
	ctx := context.Background()

	t.Run("delegates_verification_and_returns_the_claims", func(t *testing.T) {
		verifier := &fakeVerifier{claims: validatorClaims()}

		claims, err := validateToken(ctx, verifier, "the-token", false)
		require.NoError(t, err)

		assert.Equal(t, 1, verifier.calls, "the token must be verified, not merely decoded")
		assert.Equal(t, "the-token", verifier.token)
		assert.Equal(t, validatorClaims(), claims)
	})

	t.Run("a_nil_verifier_refuses_every_token", func(t *testing.T) {
		// Wiring this wrong must cost every request rather than quietly admit
		// tokens nobody verified.
		_, err := validateToken(ctx, nil, "the-token", false)
		assert.Error(t, err)
	})

	t.Run("a_refusal_says_nothing_the_verifier_said", func(t *testing.T) {
		// The middleware used to write this straight to the client, which is
		// how "crypto/ecdsa: verification error" reached API consumers.
		verifier := &fakeVerifier{err: errors.New("crypto/ecdsa: verification error")}

		_, err := validateToken(ctx, verifier, "the-token", false)
		require.Error(t, err)

		assert.NotContains(t, err.Error(), "crypto/ecdsa")
		assert.NotContains(t, err.Error(), "the-token")
	})

	t.Run("empty_claims_are_refused", func(t *testing.T) {
		verifier := &fakeVerifier{claims: map[string]any{}}

		_, err := validateToken(ctx, verifier, "the-token", false)
		assert.Error(t, err)
	})

	t.Run("jti_is_required_only_when_asked_for", func(t *testing.T) {
		claims := validatorClaims()
		delete(claims, "jti")

		_, err := validateToken(ctx, &fakeVerifier{claims: claims}, "the-token", false)
		assert.NoError(t, err, "an access token is not on the denylist, so it needs no jti")

		_, err = validateToken(ctx, &fakeVerifier{claims: claims}, "the-token", true)
		assert.Error(t, err, "a revocable token that cannot be named cannot be revoked")
	})
}

func TestValidateJTI(t *testing.T) {
	t.Run("accepts_a_uuid", func(t *testing.T) {
		assert.NoError(t, validateJTI(validatorClaims()))
	})

	for name, jti := range map[string]any{
		"missing":      nil,
		"not_a_string": 12345,
		"not_a_uuid":   "definitely-not-a-uuid",
		"the_nil_uuid": "00000000-0000-0000-0000-000000000000",
	} {
		t.Run("rejects_a_jti_that_is_"+name, func(t *testing.T) {
			claims := validatorClaims()
			if jti == nil {
				delete(claims, "jti")
			} else {
				claims["jti"] = jti
			}

			assert.Error(t, validateJTI(claims))
		})
	}
}
