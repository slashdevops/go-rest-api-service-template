//go:build unit

package domain

import (
	"strings"
	"testing"
)

// TestADeeplyNestedFilterIsRefused: the filter parser is recursive, and until
// qfv v1.0.3 the input chose how deep it recursed: a deeply nested filter
// overflowed the stack, a fatal error that Recovery does not catch. v1.0.3
// refuses nesting past 100 levels.
//
// The filters here are inside the length limit, which is refused first
// (list_expressions.go): it is the depth that is tested. On the older parser
// the last case is accepted.
func TestADeeplyNestedFilterIsRefused(t *testing.T) {
	t.Parallel()

	nested := func(levels int) error {
		filter := strings.Repeat("(", levels) + "name = 'a'" + strings.Repeat(")", levels)
		if len(filter) > MaxFilterExpressionLength {
			t.Fatalf("%d levels are %d characters, over the length limit: this would test the length", levels, len(filter))
		}

		return (&SelectProductsInput{Filter: filter, Paginator: Paginator{Limit: 10}}).Validate()
	}

	if err := nested(100); err != nil {
		t.Errorf("a hundred levels of parentheses: %v", err)
	}

	err := nested(101)
	if got := refusals(err, FieldFilter); len(got) != 1 || got[0] != "INVALID_FILTER_FIELD" {
		t.Errorf("a hundred and one levels were refused with %v (%v), want the parser's refusal alone", got, err)
	}
}
