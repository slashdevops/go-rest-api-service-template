package domain

import (
	"testing"

	"uuid"

	"github.com/stretchr/testify/assert"
)

// TestUUIDVersion pins the version derivation to fixed literals rather than to
// a generator, so it keeps testing the same thing no matter which uuid package
// backs the domain. The standard library exposes no Version method, so this
// nibble read is the only thing enforcing the v7-everywhere rule.
func TestUUIDVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"v1", "f81d4fae-7dec-11d0-a765-00a0c91e6bf6", 1},
		{"v4", "9c5b94b1-35ad-49bb-b118-8e8fc24abf80", 4},
		{"v6", "1ec9414c-232a-6b00-b3c8-9f6bdeced846", 6},
		{"v7", "01982303-f0f9-7e82-a25a-b0671dc354c2", 7},
		{"nil", "00000000-0000-0000-0000-000000000000", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, UUIDVersion(uuid.MustParse(tc.in)))
		})
	}
}

func TestIsUUIDV7(t *testing.T) {
	t.Run("v7", func(t *testing.T) {
		assert.True(t, IsUUIDV7(uuid.MustParse("01982303-f0f9-7e82-a25a-b0671dc354c2")))
	})

	t.Run("v4_is_rejected", func(t *testing.T) {
		assert.False(t, IsUUIDV7(uuid.MustParse("9c5b94b1-35ad-49bb-b118-8e8fc24abf80")))
	})

	t.Run("nil_is_rejected", func(t *testing.T) {
		assert.False(t, IsUUIDV7(uuid.UUID{}))
	})
}

func TestValidateUUIDVersion(t *testing.T) {
	v7 := uuid.MustParse("01982303-f0f9-7e82-a25a-b0671dc354c2")

	t.Run("v7_passes_v7_requirement", func(t *testing.T) {
		assert.NoError(t, ValidateUUID(v7, 7, "id"))
	})

	t.Run("v7_fails_v4_requirement", func(t *testing.T) {
		assert.Error(t, ValidateUUID(v7, 4, "id"))
	})

	t.Run("nil_is_rejected", func(t *testing.T) {
		assert.Error(t, ValidateUUID(uuid.UUID{}, 0, "id"))
	})

	t.Run("zero_required_version_skips_the_check", func(t *testing.T) {
		assert.NoError(t, ValidateUUID(uuid.MustParse("9c5b94b1-35ad-49bb-b118-8e8fc24abf80"), 0, "id"))
	})
}
