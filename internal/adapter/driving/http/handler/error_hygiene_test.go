package handler

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoHandlerForwardsALibraryErrorString reads the package source and
// fails on the two shapes that used to ship a dependency's error text as
// part of this API's contract: a 500 whose body is err.Error(), and a
// request-body decode failure answered with the decoder's own words.
//
// Measured 2026-09-06 before the sweep: 218 sites of the first kind and 49
// of the second. "unexpected EOF" and "invalid cursor: not base64" were both
// visible from curl. The same rule the stdlib-uuid migration and the JWT
// middleware wrote down, now enforced for the whole package.
func TestNoHandlerForwardsALibraryErrorString(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	internalWithText := regexp.MustCompile(`StatusInternalServerError,\s*\w+\.Error\(\)`)
	decodeThenText := regexp.MustCompile(`Decode\(&[^\n]*\n(?:[^\n]*\n){0,4}?[ \t]*respond\.WriteJSONMessage\([^\n]*\.Error\(\)`)

	var offenders []string

	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}

		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}

		for _, m := range internalWithText.FindAllIndex(src, -1) {
			offenders = append(offenders, f+": a 500 body is err.Error(); use respond.WriteInternalError: "+string(src[m[0]:m[1]]))
		}

		for range decodeThenText.FindAllIndex(src, -1) {
			offenders = append(offenders, f+": a decode failure forwards the decoder's text; use respond.WriteDecodeError")
		}
	}

	if len(offenders) > 0 {
		t.Errorf("%d handler sites forward a library's error string to the client:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}
