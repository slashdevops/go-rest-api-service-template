package domain

import (
	"strings"
	"testing"
)

// ValidatePassword is the rule for CHOOSING a password. It used to accept six
// characters and a password bcrypt refuses; the list of forbidden passwords
// had ten entries.
func TestValidatePassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		code     string // "" means accepted
	}{
		{name: "three_classes", password: "Meadow7Lark", code: ""},
		{name: "two_classes_and_long", password: "correcthorse1", code: ""},
		{name: "four_classes", password: "SecureP@ssw0rd123", code: ""},
		{name: "exactly_the_minimum", password: "Ab1!Ab1!", code: ""},
		{name: "exactly_bcrypts_limit", password: "Ab1!" + strings.Repeat("x", ValidUserPasswordMaxLength-4), code: ""},
		{name: "empty", password: "", code: "REQUIRED"},
		{name: "seven_characters_was_accepted", password: "Ab1!Ab1", code: "TOO_SHORT"},
		{name: "longer_than_bcrypt_hashes", password: "Ab1!" + strings.Repeat("x", ValidUserPasswordMaxLength-3), code: "TOO_LONG"},
		{name: "bytes_not_runes", password: strings.Repeat("ñ", ValidUserPasswordMaxLength/2+1) + "A1!", code: "TOO_LONG"},
		{name: "common", password: "password123", code: "WEAK_PASSWORD"},
		{name: "a_suffix_is_not_on_the_list", password: "LetMeIn1", code: ""},
		{name: "common_upper_cased", password: "PASSWORD1", code: "WEAK_PASSWORD"},
		{name: "common_with_a_number_is_still_common", password: "Password1234", code: "WEAK_PASSWORD"},
		{name: "common_from_the_longer_list", password: "trustno1", code: "WEAK_PASSWORD"},
		{name: "two_classes_short", password: "wrong-pw", code: "WEAK_PASSWORD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidatePassword(tt.password, FieldPassword)
			if tt.code == "" {
				if err != nil {
					t.Fatalf("expected %q to be accepted, got %v", tt.password, err)
				}
				return
			}

			verr, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("expected a *ValidationError for %q, got %T %v", tt.password, err, err)
			}
			if verr.Code != tt.code {
				t.Fatalf("expected code %s for %q, got %s (%s)", tt.code, tt.password, verr.Code, verr.Message)
			}
		})
	}
}

// ValidateLoginPassword bounds a password that is being CHECKED against a
// hash. It must not apply the choosing rule: an account whose password was
// fine when it was set must still be able to sign in after the rule tightens,
// and a wrong password must answer 401, not a 400 naming the policy.
func TestValidateLoginPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		code     string
	}{
		{name: "legacy_six_characters", password: "abc123", code: ""},
		{name: "weak_by_policy_is_still_a_login", password: "password", code: ""},
		{name: "empty", password: "", code: "REQUIRED"},
		{name: "longer_than_bcrypt_compares", password: strings.Repeat("x", ValidUserPasswordMaxLength+1), code: "TOO_LONG"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateLoginPassword(tt.password, FieldPassword)
			if tt.code == "" {
				if err != nil {
					t.Fatalf("expected %q to be accepted, got %v", tt.password, err)
				}
				return
			}

			verr, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("expected a *ValidationError, got %T %v", err, err)
			}
			if verr.Code != tt.code {
				t.Fatalf("expected code %s, got %s", tt.code, verr.Code)
			}
		})
	}
}

// The bounds are the ones the rest of the service relies on: bcrypt refuses
// anything over 72 bytes, and the email column is sized to the validator.
func TestUserValidationBoundsMatchTheirConsumers(t *testing.T) {
	t.Parallel()

	if ValidUserPasswordMaxLength != 72 {
		t.Fatalf("bcrypt hashes at most 72 bytes; the cap is %d", ValidUserPasswordMaxLength)
	}
	if ValidUserPasswordMinLength < 8 {
		t.Fatalf("eight is the floor for a user-chosen password; the minimum is %d", ValidUserPasswordMinLength)
	}
	if ValidUserEmailMaxLength != MaxEmailLength {
		t.Fatalf("ValidateEmail allows %d but the user cap is %d; users.email must match ValidateEmail", MaxEmailLength, ValidUserEmailMaxLength)
	}
}
