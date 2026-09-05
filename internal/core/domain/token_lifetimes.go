package domain

import (
	"fmt"
	"time"

	"uuid"
)

// The bounds on the two session lifetimes, and the values a fresh database is
// seeded with.
//
// These are the ONLY place the numbers live in Go. The migration that creates
// authn_token_lifetimes repeats them as CHECK constraints and as the seed row,
// and TestSeedTokenLifetimesMatchDomainDefaults fails if the two drift -- one
// number in two places is the trap the rate limiter removed, so it is guarded
// rather than trusted. The API returns them in every GET so the frontend never
// has to hardcode one either.
//
// # Why these numbers
//
//   - 2m access minimum: comfortably above the frontend's 5s cookie buffer and
//     the 10s revocation reload interval, so a token is never issued that the
//     machinery around it cannot keep up with.
//   - 48h access maximum: past two days an access token is a session in all
//     but name, and the residual-access window after a logout with revocation
//     switched off is the whole lifetime.
//   - 12h refresh minimum: below half a day a refresh token buys nothing over
//     a long access token; the point of the pair is that one is short-lived and
//     the other is not.
//   - 168h (7d) refresh maximum: a session may be renewed for a week and not
//     more, because rotation carries the ORIGINAL expiry across every refresh
//     and never renews it -- a longer ceiling is a product decision about
//     immortal sessions, not a tuning knob.
//   - refresh STRICTLY greater than access: an equal pair makes the refresh
//     token expire in the same instant as the access token it would renew, so
//     there is never a moment at which refreshing is both possible and useful.
const (
	ValidAuthnAccessTokenMinDuration  = 2 * time.Minute
	ValidAuthnAccessTokenMaxDuration  = 48 * time.Hour
	ValidAuthnRefreshTokenMinDuration = 12 * time.Hour
	ValidAuthnRefreshTokenMaxDuration = 168 * time.Hour

	// DefaultAuthnAccessTokenDuration and DefaultAuthnRefreshTokenDuration are
	// what a fresh database is seeded with. They are not a fallback: a replica
	// that cannot read the row does not serve these, it refuses to start.
	DefaultAuthnAccessTokenDuration  = 5 * time.Minute
	DefaultAuthnRefreshTokenDuration = 24 * time.Hour
)

// TokenLifetimes is the one row of authn_token_lifetimes: how long an access
// token and a refresh token issued from now on will live.
//
// It is a SINGLETON, not a collection. There is no create and no delete: the
// row is seeded by migration and a service with no lifetimes cannot issue a
// token, which is the same invariant the rate limiter keeps -- if the service
// is serving, it has lifetimes.
//
// A change here never touches a token already issued. Every token carries its
// own exp, and the verifier reads that, so the access lifetime applies at the
// next login or refresh and the refresh lifetime at the next login only --
// rotation carries the expiry the session started with.
type TokenLifetimes struct {
	UpdatedAt            time.Time
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration

	// UpdatedBy is the user who last changed the row, or uuid.Nil() for the
	// seeded values. Kept as a plain id, deliberately not a foreign key:
	// deleting the admin must not touch this row.
	UpdatedBy uuid.UUID
}

// Validate applies the bounds and the ordering rule.
//
// Every failure is reported at once, each naming its field, so an operator who
// got both numbers wrong is told so in one answer rather than one per attempt.
func (ref *TokenLifetimes) Validate() error {
	var errs ValidationErrors

	validateTokenLifetimes(&errs, ref.AccessTokenDuration, ref.RefreshTokenDuration)

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// UpdateTokenLifetimesInput is what a PUT carries.
type UpdateTokenLifetimesInput struct {
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration

	// UpdatedBy is the caller, taken from the verified token -- never from
	// the body.
	UpdatedBy uuid.UUID
}

// Validate applies the same rules as [TokenLifetimes.Validate], plus the
// requirement that the change is attributed to somebody.
func (ref *UpdateTokenLifetimesInput) Validate() error {
	var errs ValidationErrors

	validateTokenLifetimes(&errs, ref.AccessTokenDuration, ref.RefreshTokenDuration)

	if ref.UpdatedBy == uuid.Nil() {
		errs.AddError(FieldUpdatedBy, "the caller performing the change is required", "REQUIRED")
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

func validateTokenLifetimes(errs *ValidationErrors, access, refresh time.Duration) {
	if access < ValidAuthnAccessTokenMinDuration || access > ValidAuthnAccessTokenMaxDuration {
		errs.AddError(FieldAccessTokenDuration,
			fmt.Sprintf("must be between %s and %s, got %s",
				ValidAuthnAccessTokenMinDuration, ValidAuthnAccessTokenMaxDuration, access),
			"OUT_OF_RANGE")
	}

	if refresh < ValidAuthnRefreshTokenMinDuration || refresh > ValidAuthnRefreshTokenMaxDuration {
		errs.AddError(FieldRefreshTokenDuration,
			fmt.Sprintf("must be between %s and %s, got %s",
				ValidAuthnRefreshTokenMinDuration, ValidAuthnRefreshTokenMaxDuration, refresh),
			"OUT_OF_RANGE")
	}

	// Strictly greater. Reported on the refresh field because that is the one
	// an operator raises to fix it; lowering the access token is the other way
	// out, but the message has to pick one.
	if refresh <= access {
		errs.AddError(FieldRefreshTokenDuration,
			fmt.Sprintf("must be longer than %s (%s); an equal pair leaves no moment at which refreshing is both possible and useful, got %s",
				FieldAccessTokenDuration, access, refresh),
			"ORDERING")
	}
}

// TokenLifetimeBounds is what a GET returns beside the values, so the client
// validates against the same numbers the server does.
type TokenLifetimeBounds struct {
	AccessTokenMin  time.Duration
	AccessTokenMax  time.Duration
	RefreshTokenMin time.Duration
	RefreshTokenMax time.Duration
}

// TokenLifetimesBounds returns the bounds every write is checked against.
func TokenLifetimesBounds() TokenLifetimeBounds {
	return TokenLifetimeBounds{
		AccessTokenMin:  ValidAuthnAccessTokenMinDuration,
		AccessTokenMax:  ValidAuthnAccessTokenMaxDuration,
		RefreshTokenMin: ValidAuthnRefreshTokenMinDuration,
		RefreshTokenMax: ValidAuthnRefreshTokenMaxDuration,
	}
}

// DefaultTokenLifetimes returns the values a fresh database is seeded with.
// "Reset to defaults" in a client is a PUT of exactly this.
func DefaultTokenLifetimes() TokenLifetimes {
	return TokenLifetimes{
		AccessTokenDuration:  DefaultAuthnAccessTokenDuration,
		RefreshTokenDuration: DefaultAuthnRefreshTokenDuration,
	}
}

// TokenLifetimesNotFoundError means the singleton row is missing.
//
// This is a 500, never a 404: the row is created by migration and the service
// refuses to start without it, so reaching this at request time means the
// table was emptied underneath a running process.
type TokenLifetimesNotFoundError struct {
	Message string
}

func (e *TokenLifetimesNotFoundError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = "the authn_token_lifetimes row is missing; it is seeded by migration and must never be deleted"
	}

	return msg
}
