package domain

import (
	"github.com/slashdevops/qfv"
)

// The three functions below are the only callers of the qfv parsers
// (TestListExpressionsAreParsedInOnePlace). A list input hands them its
// sort, its filter and its fields, and each records what is wrong with the
// expression, under the code that input has always used.
//
// # An expression over its length is not parsed
//
// A list's sort, filter and fields arrive in the query string, which the
// body limit does not bound: a request line may be as long as the header
// limit allows, 1 MiB by default. The length check and the parse were two
// independent steps, and the first was each input's to remember: six list
// inputs recorded TOO_LONG and handed the expression to the parser anyway,
// and four had no length check at all. The filter parser is recursive, and
// until qfv v1.0.3 the input chose how deep it recursed: a deeply nested
// filter overflowed the stack, which is a fatal error of the runtime that
// Recovery does not catch.
//
// The parser bounds its own depth now. The length bound is still enforced
// here, in front of it, where the parsers are called: it is the cheaper
// refusal, it covers the sort and the fields, and it does not depend on
// what a library does with a megabyte of input. An expression over its
// length is refused as TOO_LONG, once, and no parser sees it. No input
// checks the length for itself any more: there is one place to get it
// right.

func parseSortExpression(errs *ValidationErrors, parser *qfv.SortParser, expression, code string) {
	expression = boundedExpression(errs, expression, FieldSort, ValidateSortExpression)
	if expression == "" {
		return
	}

	if _, err := parser.Parse(expression); err != nil {
		errs.AddError(FieldSort, err.Error(), code)
	}
}

func parseFilterExpression(errs *ValidationErrors, parser *qfv.FilterParser, expression, code string) {
	expression = boundedExpression(errs, expression, FieldFilter, ValidateFilterExpression)
	if expression == "" {
		return
	}

	if _, err := parser.Parse(expression); err != nil {
		errs.AddError(FieldFilter, err.Error(), code)
	}
}

func parseFieldsExpression(errs *ValidationErrors, parser *qfv.FieldsParser, expression, code string) {
	expression = boundedExpression(errs, expression, FieldFields, ValidateFieldsExpression)
	if expression == "" {
		return
	}

	if _, err := parser.Parse(expression); err != nil {
		errs.AddError(FieldFields, err.Error(), code)
	}
}

// boundedExpression returns the expression a parser may be given: the
// expression itself, or nothing when it is empty or over its length, which
// it records.
func boundedExpression(errs *ValidationErrors, expression, field string, check func(expression, field string) error) string {
	if err := check(expression, field); err != nil {
		errs.Add(err)

		return ""
	}

	return expression
}
