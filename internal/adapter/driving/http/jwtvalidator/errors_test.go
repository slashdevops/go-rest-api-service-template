package jwtvalidator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInvalidClaimsError_Error(t *testing.T) {
	t.Run("with_custom_message", func(t *testing.T) {
		err := &InvalidClaimsError{
			Message: "custom error message",
		}
		assert.Equal(t, "custom error message", err.Error())
	})

	t.Run("with_empty_message", func(t *testing.T) {
		err := &InvalidClaimsError{
			Message: "",
		}
		assert.Equal(t, "invalid claims", err.Error())
	})

	t.Run("with_no_message_field", func(t *testing.T) {
		err := &InvalidClaimsError{}
		assert.Equal(t, "invalid claims", err.Error())
	})
}

func TestInvalidTokenError_Error(t *testing.T) {
	t.Run("with_custom_message", func(t *testing.T) {
		err := &InvalidTokenError{
			Message: "custom token error message",
		}
		assert.Equal(t, "custom token error message", err.Error())
	})

	t.Run("with_empty_message", func(t *testing.T) {
		err := &InvalidTokenError{
			Message: "",
		}
		assert.Equal(t, "invalid token", err.Error())
	})

	t.Run("with_no_message_field", func(t *testing.T) {
		err := &InvalidTokenError{}
		assert.Equal(t, "invalid token", err.Error())
	})
}
