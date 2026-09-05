//go:build unit

package repositorypg

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// TestPaginationJoinerFollowsTheQueryItIsSpliceIntoGuards the argument that
// decides WHERE vs AND.
//
// buildPaginationCriteria emits the second half of a query's predicate, so it
// has to know whether the template it lands in already opened one. It infers
// that from the filter text and, when the filter is empty, from whereInQuery.
// Six call sites passed false into templates that already said WHERE, so with
// no filter the rendered SQL was
//
//	WHERE vp.user_id = $1  WHERE (vp.serial_id < …)
//
// which Postgres rejects with `syntax error at or near "WHERE"`. Page one was
// fine — it has no token, so no criteria — and page two returned 500. It
// reached users as a 503 on the Next button of /projects and /pa_tokens.
func TestPaginationJoinerFollowsTheQueryItIsSplicedInto(t *testing.T) {
	id := uuid.NewV7()

	t.Run("a template that already opened a predicate gets AND", func(t *testing.T) {
		clause, _ := buildPaginationCriteria("vp", domain.TokenDirectionNext, id, 42, "", true)

		if !strings.Contains(string(clause), "AND") {
			t.Errorf("expected AND, got %q", clause)
		}

		if strings.Contains(string(clause), "WHERE") {
			t.Errorf("a second WHERE is a syntax error; got %q", clause)
		}
	})

	t.Run("a template with no predicate gets WHERE", func(t *testing.T) {
		clause, _ := buildPaginationCriteria("vp", domain.TokenDirectionNext, id, 42, "", false)

		if !strings.Contains(string(clause), "WHERE") {
			t.Errorf("expected WHERE, got %q", clause)
		}
	})

	t.Run("a non-empty filter always continues it", func(t *testing.T) {
		// The filter already carries its own joiner, so the criteria must
		// continue rather than open. This holds whichever way whereInQuery is
		// set, which is why the bug only showed up with an empty filter.
		for _, whereInQuery := range []bool{true, false} {
			clause, _ := buildPaginationCriteria("vp", domain.TokenDirectionNext, id, 42, "AND (name = 'x')", whereInQuery)
			if strings.Contains(string(clause), "WHERE") {
				t.Errorf("whereInQuery=%v: expected no WHERE after a filter, got %q", whereInQuery, clause)
			}
		}
	})
}

// TestEveryPaginationCallSiteMatchesItsTemplate is the check that would have
// caught the bug above, which the helper test cannot.
//
// The helper was always correct; six CALL SITES lied to it. So the thing worth
// testing is the relationship between a query template and the argument passed
// beside it: if the template already contains WHERE, the call must pass
// whereInQuery = true.
//
// Source-scanned rather than executed, because reaching these functions needs a
// database and the mistake is visible in the text. The same reasoning as
// handler.TestEverySwaggerStatusIsDeclared.
func TestEveryPaginationCallSiteMatchesItsTemplate(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the repository package: %v", err)
	}

	call := regexp.MustCompile(`buildPaginationCriteria\(\s*"[^"]+",[^)]*?,\s*(true|false)\s*\)`)
	tmpl := regexp.MustCompile(`\bWHERE\b`)
	checked := 0

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}

		source := string(src)

		for _, m := range call.FindAllStringSubmatchIndex(source, -1) {
			checked++

			whereInQuery := source[m[2]:m[3]] == "true"

			// The query template this call renders into is the one declared
			// above it in the same function.
			start := strings.LastIndex(source[:m[0]], "queryTemplate := `")
			if start == -1 {
				continue
			}

			end := strings.Index(source[start+len("queryTemplate := `"):], "`")
			if end == -1 {
				continue
			}

			template := source[start : start+len("queryTemplate := `")+end]

			if tmpl.MatchString(template) && !whereInQuery {
				t.Errorf(
					"%s: a buildPaginationCriteria call passes whereInQuery=false, but the "+
						"template above it already contains WHERE. With an empty filter the "+
						"criteria will open a second WHERE and Postgres will reject page two "+
						"with a syntax error, while page one keeps working",
					name)
			}
		}
	}

	if checked == 0 {
		t.Fatal("no buildPaginationCriteria call sites found; this test would pass by looking at nothing")
	}

	t.Logf("checked %d pagination call sites", checked)
}
