package repositorypg

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const rateLimitsMigration = "../../../../database/migrations/00015_rate_limits.sql"

func readRateLimitsMigration(t *testing.T) string {
	t.Helper()

	b, err := os.ReadFile(rateLimitsMigration)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	return string(b)
}

// A CONSTRAINT NAME REFERENCED FROM GO IS A CONTRACT.
//
// Renaming one in a later migration breaks no build and fails no happy-path
// test -- it silently turns a documented 409 or 403 back into a 500, found by a
// user. This has already happened once in this codebase, which is why the
// mapping is by name at all rather than by message substring.
func TestRateLimitConstraintNamesExistInTheMigration(t *testing.T) {
	t.Parallel()

	sql := readRateLimitsMigration(t)

	referenced := []string{
		constraintRateLimitName,
		constraintRateLimitWindowPeriod,
		constraintRateLimitStrategyCheck,
		constraintRateLimitScopeCheck,
		constraintRateLimitAudienceCheck,
		constraintRateLimitKindCheck,
		constraintRateLimitTargetCheck,
		constraintRateLimitMethodsCheck,
	}

	for _, name := range referenced {
		if !strings.Contains(sql, name) {
			t.Fatalf("handlePgError matches on %q but the migration does not define it; "+
				"a renamed constraint turns a documented 409/403 back into a 500", name)
		}
	}
}

// The reverse direction: a constraint the migration defines but nothing maps
// surfaces to the caller as a 500 with a Postgres message in it.
func TestEveryRateLimitConstraintIsMapped(t *testing.T) {
	t.Parallel()

	sql := readRateLimitsMigration(t)

	mapped := []string{
		constraintRateLimitName,
		constraintRateLimitWindowPeriod,
		constraintRateLimitStrategyCheck,
		constraintRateLimitScopeCheck,
		constraintRateLimitAudienceCheck,
		constraintRateLimitKindCheck,
		constraintRateLimitTargetCheck,
		constraintRateLimitMethodsCheck,
	}

	// The window-table CHECKs are deliberately absent from handlePgError:
	// validation rejects those values before a write, and if one ever did reach
	// Postgres the generic path reports it. They are listed here so the test
	// says "known and excluded" rather than silently passing.
	knownUnmapped := []string{
		"chk_rate_limit_windows_requests",
		"chk_rate_limit_windows_period",
		"chk_rate_limit_windows_burst",
	}

	declared := regexp.MustCompile(`CONSTRAINT (\w+)`).FindAllStringSubmatch(sql, -1)
	if len(declared) == 0 {
		t.Fatal("no named constraints found in the migration; the regex or the file changed shape")
	}

	for _, m := range declared {
		name := m[1]
		if slices.Contains(mapped, name) || slices.Contains(knownUnmapped, name) {
			continue
		}

		t.Fatalf("the migration declares %q but handlePgError does not map it, and it is not in knownUnmapped; "+
			"it would reach the caller as a 500 carrying a Postgres message", name)
	}
}

// The default branch of buildScanFields must list every column the query
// selects, in the same order. It is a []any, so a mismatch is a run-time scan
// arity error on a live request -- exactly the class of bug that hides until
// someone lists rate limits in production.
func TestRateLimitScanFieldsCoverEveryColumn(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("rate_limits.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	body := string(src)

	fieldsArray := regexp.MustCompile(`(?s)fieldsArray := \[\]string\{(.*?)\n\t\}`).FindStringSubmatch(body)
	if fieldsArray == nil {
		t.Fatal("fieldsArray literal not found; the Select query changed shape")
	}

	columns := regexp.MustCompile(`"(\w+)"`).FindAllStringSubmatch(fieldsArray[1], -1)

	defaultBranch := regexp.MustCompile(`(?s)if requestedFields == "" \{\n\t\treturn \[\]any\{(.*?)\n\t\t\}`).FindStringSubmatch(body)
	if defaultBranch == nil {
		t.Fatal("the default scan branch was not found; buildScanFields changed shape")
	}

	scans := regexp.MustCompile(`&(?:item|raw)\.(\w+)`).FindAllStringSubmatch(defaultBranch[1], -1)

	if len(columns) != len(scans) {
		t.Fatalf("the query selects %d columns but the default scan branch has %d targets; "+
			"pgx reports this as a scan-arity error on a live request, not at build time",
			len(columns), len(scans))
	}
}

// The partial-fields list is a []string, so a column missing from it is not a
// compile error -- it is a field the API silently refuses to return.
func TestRateLimitsPartialFieldsCoverEveryColumn(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("rate_limits.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	m := regexp.MustCompile(`(?s)fieldsArray := \[\]string\{(.*?)\n\t\}`).FindStringSubmatch(string(src))
	if m == nil {
		t.Fatal("fieldsArray literal not found")
	}

	for _, c := range regexp.MustCompile(`"(\w+)"`).FindAllStringSubmatch(m[1], -1) {
		column := c[1]

		// serial_id backs the paginator and is never a requestable field.
		if column == "serial_id" {
			continue
		}

		if !slices.Contains(rateLimitsPartialFieldNames(t), column) {
			t.Fatalf("the Select query returns %q but RateLimitsPartialFields does not list it; "+
				"the API would refuse to return a column it can read", column)
		}
	}
}

func rateLimitsPartialFieldNames(t *testing.T) []string {
	t.Helper()

	// Resolve each Field* constant to its real value in fields.go rather than
	// deriving the spelling from the identifier. Deriving it is wrong for every
	// acronym -- FieldID would become "i_d" -- and a helper that is wrong about
	// half the names makes this test assert nothing useful.
	fields, err := os.ReadFile("../../../core/domain/fields.go")
	if err != nil {
		t.Fatalf("read fields.go: %v", err)
	}

	values := make(map[string]string)
	for _, m := range regexp.MustCompile(`(Field\w+)\s+= "(\w+)"`).FindAllStringSubmatch(string(fields), -1) {
		values[m[1]] = m[2]
	}

	b, err := os.ReadFile("../../../core/domain/rate_limits.go")
	if err != nil {
		t.Fatalf("read rate_limits.go: %v", err)
	}

	m := regexp.MustCompile(`(?s)RateLimitsPartialFields = \[\]string\{(.*?)\n\t\}`).FindSubmatch(b)
	if m == nil {
		t.Fatal("RateLimitsPartialFields literal not found")
	}

	out := make([]string, 0)

	for _, f := range regexp.MustCompile(`(Field\w+)`).FindAllStringSubmatch(string(m[1]), -1) {
		v, ok := values[f[1]]
		if !ok {
			t.Fatalf("RateLimitsPartialFields names %s but fields.go does not declare it", f[1])
		}

		out = append(out, v)
	}

	if len(out) == 0 {
		t.Fatal("resolved no field names; the literal or the regex changed shape")
	}

	return out
}

// Values go in as $n placeholders, always. The one place this repo got it wrong
// -- roles.UpdateByID -- is a documented injection bug, and the way it got there
// is somebody copying a nearby file.
func TestRateLimitQueriesUsePlaceholdersOnly(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("rate_limits.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	// Match the WHOLE call, not just up to the first SQL keyword: the previous
	// version's match ended at "WHERE", so the allow-list below could never
	// recognise the one legitimate call and the test failed on correct code.
	suspicious := regexp.MustCompile(`fmt\.Sprintf\("[^"]*(?i:SELECT|INSERT|UPDATE|DELETE|WHERE|VALUES)[^"]*"[^)]*\)`).FindAllString(string(src), -1)

	for _, call := range suspicious {
		// The filter clause is the one interpolation, and it is built by the
		// shared qfv helpers, which validate every field name against an
		// allow-list before it reaches the query.
		if strings.Contains(call, `"WHERE (%s)", filterSentence`) {
			continue
		}

		t.Fatalf("a query is being built with fmt.Sprintf rather than $n placeholders: %s", call)
	}
}
