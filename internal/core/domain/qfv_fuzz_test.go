package domain

import (
	"testing"
	"unicode/utf8"
)

// The three query-expression parsers take strings straight from the query
// string of every list endpoint. Each seed is a shape a real client sends;
// the fuzzer mutates from there. The property is narrow on purpose: neither
// the validator nor the parser may panic, and a string the validator accepts
// must be one the parser can parse without panicking.
func FuzzFilterExpression(f *testing.F) {
	for _, seed := range []string{
		"name='admin'", "name='a' AND description='b'", "created_at>'2026-01-01'",
		"name LIKE 'x%'", "(name='a' OR name='b') AND system=true", "name='x'' OR 1=1--",
		"", " ", "'", "((", "name=", "\x00", "name='" + string(make([]byte, 3000)) + "'",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		if !utf8.ValidString(in) {
			return
		}

		if err := ValidateFilterExpression(in, FieldFilter); err != nil {
			return
		}

		_, _ = RolesFilterParser.Parse(in)
		_, _ = UsersFilterParser.Parse(in)
	})
}

func FuzzSortExpression(f *testing.F) {
	for _, seed := range []string{"name ASC", "name DESC, created_at ASC", "", ",", "name", "name asc desc", "\x00 ASC"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		if !utf8.ValidString(in) {
			return
		}

		if err := ValidateSortExpression(in, FieldSort); err != nil {
			return
		}

		_, _ = RolesSortParser.Parse(in)
	})
}

func FuzzFieldsExpression(f *testing.F) {
	for _, seed := range []string{"id,name", "id, name ,description", "", ",", "id,,name", "*", "\x00"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		if !utf8.ValidString(in) {
			return
		}

		if err := ValidateFieldsExpression(in, FieldFields); err != nil {
			return
		}

		_, _ = RolesFieldsParser.Parse(in)
	})
}
