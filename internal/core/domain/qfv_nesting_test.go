//go:build unit

package domain

import (
	"strings"
	"testing"
)

// TestADeeplyNestedFilterIsRefused: the filter parser is recursive, and until
// qfv v1.0.3 the input chose how deep it recursed. A filter of a few hundred
// kilobytes of opening parentheses -- a query string may be as long as the
// header limit, 1 MiB -- ended the process with a stack overflow, a fatal
// error that Recovery does not catch. v1.0.3 refuses nesting past 100 levels.
//
// On the older parser this test does not fail: it kills the test binary.
func TestADeeplyNestedFilterIsRefused(t *testing.T) {
	t.Parallel()

	deep := strings.Repeat("(", 1<<19)

	for name, input := range map[string]interface{ Validate() error }{
		"users":    &SelectUsersInput{Filter: deep, Paginator: Paginator{Limit: 10}},
		"products": &SelectProductsInput{Filter: deep, Paginator: Paginator{Limit: 10}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if err := input.Validate(); err == nil {
				t.Fatal("half a megabyte of parentheses was accepted as a filter")
			}
		})
	}

	// Depth, not length: a hundred levels still parse.
	nested := strings.Repeat("(", 100) + "name = 'a'" + strings.Repeat(")", 100)
	if _, err := ProductsFilterParser.Parse(nested); err != nil {
		t.Errorf("a hundred levels of parentheses: %v", err)
	}
}
