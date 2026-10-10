//go:build unit

package domain

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"uuid"
)

// tooLong is the code of an expression over its length.
const tooLong = "TOO_LONG"

// listInputs is every list input of the package, built from the three
// expressions. TestEveryListInputIsListed fails when one is missing here.
func listInputs(sort, filter, fields string) map[string]interface{ Validate() error } {
	page := Paginator{Limit: 10}

	return map[string]interface{ Validate() error }{
		"SelectIDPTypesInput":        &SelectIDPTypesInput{Sort: sort, Filter: filter, Fields: fields, Paginator: page},
		"SelectIDPsInput":            &SelectIDPsInput{Sort: sort, Filter: filter, Fields: fields, Paginator: page},
		"SelectPoliciesInput":        &SelectPoliciesInput{Sort: sort, Filter: filter, Fields: fields, Paginator: page},
		"SelectProductsInput":        &SelectProductsInput{Sort: sort, Filter: filter, Fields: fields, Paginator: page},
		"SelectProjectsInput":        &SelectProjectsInput{Sort: sort, Filter: filter, Fields: fields, Paginator: page, UserID: uuid.NewV7()},
		"SelectRateLimitsInput":      &SelectRateLimitsInput{Sort: sort, Filter: filter, Fields: fields, Paginator: page},
		"SelectResourcesLimitsInput": &SelectResourcesLimitsInput{Sort: sort, Filter: filter, Fields: fields, Paginator: page},
		"SelectResourcesInput":       &SelectResourcesInput{Sort: sort, Filter: filter, Fields: fields, Paginator: page},
		"SelectRolesInput":           &SelectRolesInput{Sort: sort, Filter: filter, Fields: fields, Paginator: page},
		"SelectUsersInput":           &SelectUsersInput{Sort: sort, Filter: filter, Fields: fields, Paginator: page},
	}
}

// codes is the code of each of the three refusals, per list input. They
// differ for no reason but history, and a caller may match on them.
var codes = map[string][3]string{
	"SelectIDPTypesInput":        {"INVALID_SORT_FIELD", "INVALID_FILTER_FIELD", "INVALID_FIELD"},
	"SelectIDPsInput":            {"INVALID_SORT_FIELD", "INVALID_FILTER_FIELD", "INVALID_FIELD"},
	"SelectPoliciesInput":        {"INVALID_SORT_FIELD", "INVALID_FILTER_FIELD", "INVALID_FIELD"},
	"SelectProductsInput":        {"INVALID_SORT_FIELD", "INVALID_FILTER_FIELD", "INVALID_FIELD"},
	"SelectProjectsInput":        {"INVALID_SORT", "INVALID_FILTER", "INVALID_FIELDS"},
	"SelectRateLimitsInput":      {"INVALID_SORT_FIELD", "INVALID_FILTER_FIELD", "INVALID_FIELDS_FIELD"},
	"SelectResourcesLimitsInput": {"INVALID_SORT_FIELD", "INVALID_FILTER_FIELD", "INVALID_FIELDS_FIELD"},
	"SelectResourcesInput":       {"INVALID_SORT_FIELD", "INVALID_FILTER_FIELD", "INVALID_FIELD"},
	"SelectRolesInput":           {"INVALID_SORT_FIELD", "INVALID_FILTER_FIELD", "INVALID_FIELD"},
	"SelectUsersInput":           {"INVALID_SORT_FIELD", "INVALID_FILTER_FIELD", "INVALID_FIELD"},
}

var expressionFields = []string{FieldSort, FieldFilter, FieldFields}

// refusals returns what err says about one field, code by code.
func refusals(err error, field string) []string {
	verrs, ok := errors.AsType[*ValidationErrors](err)
	if !ok {
		return nil
	}

	var got []string

	for _, e := range verrs.Errors {
		if e.Field == field {
			got = append(got, e.Code)
		}
	}

	return got
}

// TestAnOverLongExpressionIsNotParsed: an expression over its length is
// refused as TOO_LONG, once, and no parser sees it. Six of the ten inputs
// recorded TOO_LONG and parsed the expression anyway, so the caller heard
// the length and then the parser's complaint about the same text; four had
// no length check, and an expression of any size went to the parser.
func TestAnOverLongExpressionIsNotParsed(t *testing.T) {
	t.Parallel()

	// Each is one character over its limit and malformed: a parser that saw
	// it would add its own refusal to the field.
	sort := strings.Repeat("(", MaxSortExpressionLength+1)
	filter := strings.Repeat("(", MaxFilterExpressionLength+1)
	fields := strings.Repeat("(", MaxFieldsExpressionLength+1)

	for name, input := range listInputs(sort, filter, fields) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := input.Validate()

			for _, field := range expressionFields {
				got := refusals(err, field)
				if len(got) != 1 || got[0] != tooLong {
					t.Errorf("%s was refused with %v, want %s alone: nothing parsed it", field, got, tooLong)
				}
			}
		})
	}
}

// TestTheLimitIsTheLastLengthParsed: an expression of exactly its maximum
// length is handed to the parser. It is malformed, so that the parser's
// refusal shows it was.
func TestTheLimitIsTheLastLengthParsed(t *testing.T) {
	t.Parallel()

	sort := strings.Repeat("(", MaxSortExpressionLength)
	filter := strings.Repeat("(", MaxFilterExpressionLength)
	fields := strings.Repeat("(", MaxFieldsExpressionLength)

	for name, input := range listInputs(sort, filter, fields) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := input.Validate()

			for i, field := range expressionFields {
				got := refusals(err, field)
				if len(got) != 1 || got[0] != codes[name][i] {
					t.Errorf("%s of exactly its limit was refused with %v, want the parser's %s alone", field, got, codes[name][i])
				}
			}
		})
	}

	// And a well-formed filter of exactly the limit is accepted.
	atLimit := "name = '" + strings.Repeat("a", MaxFilterExpressionLength-len("name = ''")) + "'"
	if len(atLimit) != MaxFilterExpressionLength {
		t.Fatalf("the test built %d characters, not the limit", len(atLimit))
	}

	if err := (&SelectProductsInput{Filter: atLimit, Paginator: Paginator{Limit: 10}}).Validate(); err != nil {
		t.Errorf("a well-formed filter of exactly %d characters: %v", MaxFilterExpressionLength, err)
	}
}

// TestAMalformedExpressionKeepsItsCode: inside the limit nothing changed. An
// expression the parser refuses is recorded once, under the code the input
// has always used for it, and a well-formed one is not refused.
func TestAMalformedExpressionKeepsItsCode(t *testing.T) {
	t.Parallel()

	for name, input := range listInputs("no_such_field asc", "no_such_field = 1", "no_such_field") {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := input.Validate()

			for i, field := range expressionFields {
				got := refusals(err, field)
				if len(got) != 1 || got[0] != codes[name][i] {
					t.Errorf("%s was refused with %v, want %s alone", field, got, codes[name][i])
				}
			}
		})
	}

	for name, input := range listInputs("id asc", "id = '0199a7a8-3bf2-7cc4-8d4b-0f6f2f0f3a11'", "id") {
		if err := input.Validate(); err != nil {
			t.Errorf("%s: a well-formed sort, filter and fields: %v", name, err)
		}
	}
}

// moduleRoot is the repository's root; the test runs in internal/core/domain.
func moduleRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	return root
}

// goFiles calls visit for every Go file of the module that is not a test.
func goFiles(t *testing.T, root string, visit func(rel string, body []byte)) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if name := d.Name(); path != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "build" || name == "dist") {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(root, path)
		visit(rel, body)

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

// TestListExpressionsAreParsedInOnePlace: the three functions of
// list_expressions.go are the only callers of the parsers. The length bound
// lives there, so a parse anywhere else is a parse with no bound.
//
// It does not look for the call, which has many spellings (an alias, a
// method value, a call split over two lines, a parser built inline). It
// looks for the things a call needs: the qfv package, and the parser
// variables. Outside the file that declares the variables and the file that
// calls them, the only thing a line may do with a parser is hand it to one
// of the three functions.
func TestListExpressionsAreParsedInOnePlace(t *testing.T) {
	t.Parallel()

	domainDir := filepath.Join("internal", "core", "domain")
	declares := filepath.Join(domainDir, "qfv_parsers.go")
	calls := filepath.Join(domainDir, "list_expressions.go")

	mentions := regexp.MustCompile(`\bqfv\.|\b\w*(Sort|Filter|Fields)Parser\b`)
	handsOver := regexp.MustCompile(`^\s*parse(Sort|Filter|Fields)Expression\(&\w+, \w+(Sort|Filter|Fields)Parser, ref\.(Sort|Filter|Fields), "[A-Z_]+"\)$`)

	var handed int

	goFiles(t, moduleRoot(t), func(rel string, body []byte) {
		if rel == declares || rel == calls {
			return
		}

		number := 0

		for line := range strings.SplitSeq(string(body), "\n") {
			number++

			if !mentions.MatchString(line) {
				continue
			}

			if handsOver.MatchString(line) {
				handed++

				continue
			}

			t.Errorf("%s:%d uses a list-expression parser outside list_expressions.go, where the length bound is: %s", rel, number, strings.TrimSpace(line))
		}
	})

	if want := 3 * len(codes); handed != want {
		t.Errorf("%d expressions are handed to the three functions, want %d: three for each list input", handed, want)
	}
}

// TestEveryListInputIsListed: a struct of this package with a Sort, a Filter
// and a Fields string is a list input, and the tests above run on the ones
// listInputs builds. A new one that is not there is not held to the bound by
// anything.
func TestEveryListInputIsListed(t *testing.T) {
	t.Parallel()

	listed := listInputs("", "", "")

	if len(listed) != len(codes) {
		t.Fatalf("%d list inputs and %d rows of codes", len(listed), len(codes))
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the package: %v", err)
	}

	found := 0

	for _, entry := range entries {
		if name := entry.Name(); !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}

			st, ok := spec.Type.(*ast.StructType)
			if !ok {
				return true
			}

			has := map[string]bool{}

			for _, field := range st.Fields.List {
				if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "string" {
					for _, name := range field.Names {
						has[name.Name] = true
					}
				}
			}

			if has["Sort"] || has["Filter"] || has["Fields"] {
				found++

				if _, ok := listed[spec.Name.Name]; !ok {
					t.Errorf("%s takes a list expression and is not in listInputs: nothing holds it to the length bound", spec.Name.Name)
				}
			}

			return true
		})
	}

	if found != len(listed) {
		t.Errorf("%d structs take a list expression, listInputs has %d", found, len(listed))
	}
}
