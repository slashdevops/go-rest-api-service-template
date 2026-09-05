package domain

import (
	"testing"

	"uuid"

	"github.com/stretchr/testify/assert"
)

func TestMakePointer(t *testing.T) {
	t.Run("string_pointer", func(t *testing.T) {
		value := "test string"
		pointer := new(value)

		assert.NotNil(t, pointer, "Pointer should not be nil")
		assert.Equal(t, value, *pointer, "Dereferenced pointer should equal original value")
	})

	t.Run("int_pointer", func(t *testing.T) {
		value := 42
		pointer := new(value)

		assert.NotNil(t, pointer, "Pointer should not be nil")
		assert.Equal(t, value, *pointer, "Dereferenced pointer should equal original value")
	})

	t.Run("bool_pointer_true", func(t *testing.T) {
		value := true
		pointer := new(value)

		assert.NotNil(t, pointer, "Pointer should not be nil")
		assert.Equal(t, value, *pointer, "Dereferenced pointer should equal original value")
	})

	t.Run("bool_pointer_false", func(t *testing.T) {
		value := false
		pointer := new(value)

		assert.NotNil(t, pointer, "Pointer should not be nil")
		assert.Equal(t, value, *pointer, "Dereferenced pointer should equal original value")
	})

	t.Run("float64_pointer", func(t *testing.T) {
		value := 3.14159
		pointer := new(value)

		assert.NotNil(t, pointer, "Pointer should not be nil")
		assert.Equal(t, value, *pointer, "Dereferenced pointer should equal original value")
	})

	t.Run("float32_pointer", func(t *testing.T) {
		value := float32(2.718)
		pointer := new(value)

		assert.NotNil(t, pointer, "Pointer should not be nil")
		assert.Equal(t, value, *pointer, "Dereferenced pointer should equal original value")
	})

	t.Run("uuid_pointer", func(t *testing.T) {
		value := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
		pointer := new(value)

		assert.NotNil(t, pointer, "Pointer should not be nil")
		assert.Equal(t, value, *pointer, "Dereferenced pointer should equal original value")
	})

	t.Run("integer_types", func(t *testing.T) {
		// Test various integer types
		testCases := []struct {
			name  string
			value any
		}{
			{"int8", int8(8)},
			{"int16", int16(16)},
			{"int32", int32(32)},
			{"int64", int64(64)},
			{"uint", uint(1)},
			{"uint8", uint8(8)},
			{"uint16", uint16(16)},
			{"uint32", uint32(32)},
			{"uint64", uint64(64)},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				switch v := tc.value.(type) {
				case int8:
					pointer := new(v)
					assert.NotNil(t, pointer, "Pointer should not be nil")
					assert.Equal(t, v, *pointer, "Dereferenced pointer should equal original value")
				case int16:
					pointer := new(v)
					assert.NotNil(t, pointer, "Pointer should not be nil")
					assert.Equal(t, v, *pointer, "Dereferenced pointer should equal original value")
				case int32:
					pointer := new(v)
					assert.NotNil(t, pointer, "Pointer should not be nil")
					assert.Equal(t, v, *pointer, "Dereferenced pointer should equal original value")
				case int64:
					pointer := new(v)
					assert.NotNil(t, pointer, "Pointer should not be nil")
					assert.Equal(t, v, *pointer, "Dereferenced pointer should equal original value")
				case uint:
					pointer := new(v)
					assert.NotNil(t, pointer, "Pointer should not be nil")
					assert.Equal(t, v, *pointer, "Dereferenced pointer should equal original value")
				case uint8:
					pointer := new(v)
					assert.NotNil(t, pointer, "Pointer should not be nil")
					assert.Equal(t, v, *pointer, "Dereferenced pointer should equal original value")
				case uint16:
					pointer := new(v)
					assert.NotNil(t, pointer, "Pointer should not be nil")
					assert.Equal(t, v, *pointer, "Dereferenced pointer should equal original value")
				case uint32:
					pointer := new(v)
					assert.NotNil(t, pointer, "Pointer should not be nil")
					assert.Equal(t, v, *pointer, "Dereferenced pointer should equal original value")
				case uint64:
					pointer := new(v)
					assert.NotNil(t, pointer, "Pointer should not be nil")
					assert.Equal(t, v, *pointer, "Dereferenced pointer should equal original value")
				}
			})
		}
	})

	t.Run("slice_pointer", func(t *testing.T) {
		value := []string{"a", "b", "c"}
		pointer := new(value)

		assert.NotNil(t, pointer, "Pointer should not be nil")
		assert.Equal(t, value, *pointer, "Dereferenced pointer should equal original value")
		assert.Equal(t, len(value), len(*pointer), "Slice lengths should match")
	})

	t.Run("map_pointer", func(t *testing.T) {
		value := map[string]int{"a": 1, "b": 2}
		pointer := new(value)

		assert.NotNil(t, pointer, "Pointer should not be nil")
		assert.Equal(t, value, *pointer, "Dereferenced pointer should equal original value")
		assert.Equal(t, len(value), len(*pointer), "Map lengths should match")
	})

	t.Run("struct_pointer", func(t *testing.T) {
		type TestStruct struct {
			Name string
			Age  int
		}

		value := TestStruct{Name: "John", Age: 30}
		pointer := new(value)

		assert.NotNil(t, pointer, "Pointer should not be nil")
		assert.Equal(t, value, *pointer, "Dereferenced pointer should equal original value")
		assert.Equal(t, value.Name, pointer.Name, "Struct field should match")
		assert.Equal(t, value.Age, pointer.Age, "Struct field should match")
	})

	t.Run("nil_value", func(t *testing.T) {
		// Test with nil interface
		var value any = nil
		pointer := new(value)

		assert.NotNil(t, pointer, "Pointer should not be nil")
		assert.Nil(t, *pointer, "Dereferenced pointer should be nil")
	})

	t.Run("zero_values", func(t *testing.T) {
		// Test with zero values of different types
		testCases := []struct {
			name  string
			value any
		}{
			{"empty_string", ""},
			{"zero_int", 0},
			{"zero_float", 0.0},
			{"false_bool", false},
			{"nil_uuid", uuid.Nil()},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				switch v := tc.value.(type) {
				case string:
					pointer := new(v)
					assert.NotNil(t, pointer, "Pointer should not be nil")
					assert.Equal(t, v, *pointer, "Dereferenced pointer should equal original value")
				case int:
					pointer := new(v)
					assert.NotNil(t, pointer, "Pointer should not be nil")
					assert.Equal(t, v, *pointer, "Dereferenced pointer should equal original value")
				case float64:
					pointer := new(v)
					assert.NotNil(t, pointer, "Pointer should not be nil")
					assert.Equal(t, v, *pointer, "Dereferenced pointer should equal original value")
				case bool:
					pointer := new(v)
					assert.NotNil(t, pointer, "Pointer should not be nil")
					assert.Equal(t, v, *pointer, "Dereferenced pointer should equal original value")
				case uuid.UUID:
					pointer := new(v)
					assert.NotNil(t, pointer, "Pointer should not be nil")
					assert.Equal(t, v, *pointer, "Dereferenced pointer should equal original value")
				}
			})
		}
	})

	t.Run("pointer_type_verification", func(t *testing.T) {
		// Test that the returned pointer has the correct type
		stringVal := "test"
		stringPtr := new(stringVal)
		assert.IsType(t, (*string)(nil), stringPtr, "Should return *string type")

		intVal := 42
		intPtr := new(intVal)
		assert.IsType(t, (*int)(nil), intPtr, "Should return *int type")

		boolVal := true
		boolPtr := new(boolVal)
		assert.IsType(t, (*bool)(nil), boolPtr, "Should return *bool type")
	})

	t.Run("memory_independence", func(t *testing.T) {
		// Test that modifying the original value doesn't affect the pointed value
		// This verifies that MakePointer creates a new copy
		originalString := "original"
		pointer := new(originalString)

		// Modify the original variable (this won't affect the pointer because Go copies the value)
		originalString = "modified"

		assert.Equal(t, "original", *pointer, "Pointer should still point to original value")
		assert.NotEqual(t, originalString, *pointer, "Pointer should not reflect changes to original variable")
	})

	t.Run("multiple_pointers_same_value", func(t *testing.T) {
		// Test creating multiple pointers from the same value
		value := "shared"
		ptr1 := new(value)
		ptr2 := new(value)

		assert.Equal(t, *ptr1, *ptr2, "Both pointers should have the same dereferenced value")
		// Note: We don't assert ptr1 != ptr2 because Go may optimize string literals
		// and point to the same memory location for identical strings
		assert.Equal(t, value, *ptr1, "First pointer should equal original value")
		assert.Equal(t, value, *ptr2, "Second pointer should equal original value")
	})

	t.Run("generics_functionality", func(t *testing.T) {
		// Test that the function works with Go generics properly
		// by verifying it can handle custom types
		type CustomInt int
		type CustomString string

		customInt := CustomInt(42)
		customString := CustomString("test")

		intPtr := new(customInt)
		stringPtr := new(customString)

		assert.Equal(t, customInt, *intPtr, "Custom int type should work")
		assert.Equal(t, customString, *stringPtr, "Custom string type should work")
		assert.IsType(t, (*CustomInt)(nil), intPtr, "Should preserve custom int type")
		assert.IsType(t, (*CustomString)(nil), stringPtr, "Should preserve custom string type")
	})
}

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
