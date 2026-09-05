package jwtvalidator

import (
	"context"
	"log/slog"

	"uuid"
)

// validateToken verifies a token and applies the per-class claim policy.
//
// The signature, the algorithm, iss, aud and exp are not checked here: they are
// checked once, by the token.Signer adapter behind [Verifier], which is the
// only implementation of any of it. This package used to carry a second one
// that disagreed with it — see the package comment.
//
// Everything it can refuse becomes the same opaque [InvalidTokenError]. The
// reason goes to the log; what reaches the caller is decided by the middleware,
// deliberately in one place.
func validateToken(ctx context.Context, verifier Verifier, tokenString string, requireJTI bool) (map[string]any, error) {
	// A validator with no verifier cannot check anything, so it must refuse
	// everything. Wiring this wrong should cost every request, loudly, rather
	// than quietly admit tokens nobody verified.
	if verifier == nil {
		slog.Error("jwtvalidator: no verifier is configured, so every token is refused")

		return nil, &InvalidTokenError{}
	}

	claims, err := verifier.Verify(ctx, tokenString)
	if err != nil {
		slog.Debug("jwtvalidator: token refused", "error", err)

		return nil, &InvalidTokenError{}
	}

	if len(claims) == 0 {
		slog.Debug("jwtvalidator: token carried no claims")

		return nil, &InvalidClaimsError{}
	}

	if requireJTI {
		if err := validateJTI(claims); err != nil {
			return nil, err
		}
	}

	return claims, nil
}

// validateJTI requires a jti that is a v7-shaped, non-nil UUID.
//
// Only the classes that can be revoked need it: a refresh token and a
// password-reset token are named on the denylist by this claim, so one that
// cannot be named cannot be revoked.
func validateJTI(claims map[string]any) error {
	jtiStr, ok := claims["jti"].(string)
	if !ok {
		slog.Debug("jwtvalidator: the token has no usable jti claim")

		return &InvalidClaimsError{}
	}

	jti, err := uuid.Parse(jtiStr)
	if err != nil {
		slog.Debug("jwtvalidator: the jti claim is not a uuid", "error", err)

		return &InvalidClaimsError{}
	}

	if jti == uuid.Nil() {
		slog.Debug("jwtvalidator: the jti claim is the nil uuid")

		return &InvalidClaimsError{}
	}

	return nil
}
