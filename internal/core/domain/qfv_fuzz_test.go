package domain

import (
	"testing"
	"unicode/utf8"
)

// The three list expressions are strings taken straight from the query
// string of every list endpoint. Each seed is a shape a real client sends;
// the fuzzer mutates from there. The property is narrow on purpose: a list
// input's Validate, which bounds the expression and then parses it, must not
// panic, whatever it is given.
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

		page := Paginator{Limit: 10}

		_ = (&SelectRolesInput{Filter: in, Paginator: page}).Validate()
		_ = (&SelectUsersInput{Filter: in, Paginator: page}).Validate()
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

		_ = (&SelectRolesInput{Sort: in, Paginator: Paginator{Limit: 10}}).Validate()
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

		_ = (&SelectRolesInput{Fields: in, Paginator: Paginator{Limit: 10}}).Validate()
	})
}
