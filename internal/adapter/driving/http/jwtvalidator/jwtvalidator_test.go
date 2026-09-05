package jwtvalidator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidatorType_String(t *testing.T) {
	tests := []struct {
		name     string
		vt       ValidatorType
		expected string
	}{
		{
			name:     "access_token",
			vt:       ValidatorTypeAccessToken,
			expected: "accessToken",
		},
		{
			name:     "refresh_token",
			vt:       ValidatorTypeRefreshToken,
			expected: "refreshToken",
		},
		{
			name:     "password_reset_token",
			vt:       ValidatorTypePasswordResetToken,
			expected: "passwordResetToken",
		},
		{
			name:     "custom_validator_type",
			vt:       ValidatorType("customType"),
			expected: "customType",
		},
		{
			name:     "empty_validator_type",
			vt:       ValidatorType(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.vt.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}
