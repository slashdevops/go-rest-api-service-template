package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"uuid"
)

func lifetimes(access, refresh time.Duration) *TokenLifetimes {
	return &TokenLifetimes{AccessTokenDuration: access, RefreshTokenDuration: refresh}
}

// The boundaries are inclusive on both ends, and the seeded defaults sit
// inside them. Each case names the field it expects the failure on, because a
// validator that refused everything would pass a test that only checks "it
// failed".
func TestTokenLifetimesValidateBounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		access  time.Duration
		refresh time.Duration
		field   string // "" means valid
	}{
		{"seeded_defaults", DefaultAuthnAccessTokenDuration, DefaultAuthnRefreshTokenDuration, ""},
		{"access_at_min", 2 * time.Minute, 24 * time.Hour, ""},
		{"access_just_under_min", 2*time.Minute - time.Second, 24 * time.Hour, FieldAccessTokenDuration},
		{"access_at_max", 48 * time.Hour, 168 * time.Hour, ""},
		{"access_just_over_max", 48*time.Hour + time.Second, 168 * time.Hour, FieldAccessTokenDuration},
		{"refresh_at_min", 5 * time.Minute, 12 * time.Hour, ""},
		{"refresh_just_under_min", 5 * time.Minute, 12*time.Hour - time.Second, FieldRefreshTokenDuration},
		{"refresh_at_max", 5 * time.Minute, 168 * time.Hour, ""},
		{"refresh_just_over_max", 5 * time.Minute, 168*time.Hour + time.Second, FieldRefreshTokenDuration},
		{"refresh_equal_to_access", 24 * time.Hour, 24 * time.Hour, FieldRefreshTokenDuration},
		{"refresh_shorter_than_access", 48 * time.Hour, 12 * time.Hour, FieldRefreshTokenDuration},
		{"zero_values", 0, 0, FieldAccessTokenDuration},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := lifetimes(tc.access, tc.refresh).Validate()

			if tc.field == "" {
				if err != nil {
					t.Fatalf("expected valid, got %v", err)
				}

				return
			}

			verrs, ok := errors.AsType[*ValidationErrors](err)
			if !ok {
				t.Fatalf("expected *ValidationErrors, got %T: %v", err, err)
			}

			found := false

			for _, e := range verrs.Errors {
				if e.Field == tc.field {
					found = true
				}
			}

			if !found {
				t.Fatalf("expected an error on field %q, got %v", tc.field, err)
			}
		})
	}
}

// Both numbers wrong must be reported together. One error per attempt would
// have the operator fix the access token, resubmit, and only then learn the
// refresh token was wrong too.
func TestTokenLifetimesValidateReportsEveryFailureAtOnce(t *testing.T) {
	t.Parallel()

	err := lifetimes(time.Second, time.Second).Validate()

	verrs, ok := errors.AsType[*ValidationErrors](err)
	if !ok {
		t.Fatalf("expected *ValidationErrors, got %T", err)
	}

	fields := map[string]bool{}
	for _, e := range verrs.Errors {
		fields[e.Field] = true
	}

	if !fields[FieldAccessTokenDuration] || !fields[FieldRefreshTokenDuration] {
		t.Fatalf("expected both fields reported, got %v", err)
	}
}

// The ordering message has to say which field to change and why; "invalid"
// alone sends the operator back to the docs.
func TestTokenLifetimesOrderingMessageNamesBothFields(t *testing.T) {
	t.Parallel()

	err := lifetimes(24*time.Hour, 24*time.Hour).Validate()
	if err == nil {
		t.Fatal("an equal pair must be refused")
	}

	if !strings.Contains(err.Error(), FieldAccessTokenDuration) || !strings.Contains(err.Error(), "longer than") {
		t.Fatalf("message should explain the ordering rule, got: %v", err)
	}
}

// The update input adds one rule: the change is attributed. The body never
// carries the caller; the handler takes it from the verified token.
func TestUpdateTokenLifetimesInputRequiresACaller(t *testing.T) {
	t.Parallel()

	in := &UpdateTokenLifetimesInput{
		AccessTokenDuration:  DefaultAuthnAccessTokenDuration,
		RefreshTokenDuration: DefaultAuthnRefreshTokenDuration,
	}

	err := in.Validate()

	verrs, ok := errors.AsType[*ValidationErrors](err)
	if !ok || len(verrs.Errors) != 1 || verrs.Errors[0].Field != FieldUpdatedBy {
		t.Fatalf("expected exactly one error on %s, got %v", FieldUpdatedBy, err)
	}

	in.UpdatedBy = uuid.NewV7()

	if err := in.Validate(); err != nil {
		t.Fatalf("expected valid once attributed, got %v", err)
	}
}

// The defaults must satisfy the bounds, or a fresh database would be seeded
// with a row the validator refuses on first load and every replica would
// refuse to start.
func TestDefaultTokenLifetimesAreWithinBounds(t *testing.T) {
	t.Parallel()

	d := DefaultTokenLifetimes()
	if err := d.Validate(); err != nil {
		t.Fatalf("the seeded defaults must validate: %v", err)
	}

	b := TokenLifetimesBounds()
	if b.AccessTokenMin >= b.AccessTokenMax || b.RefreshTokenMin >= b.RefreshTokenMax {
		t.Fatalf("bounds are inverted: %+v", b)
	}
}
