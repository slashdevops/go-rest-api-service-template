package handler

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The bucket_key string is part of the published contract -- it is the answer to
// "10 per minute per WHAT", it ships in every /rate_limits/effective response,
// and it is the swagger example every generated client carries.
//
// Nothing connects it to the code that builds the key, so it drifted: it said
// "(rule_id, scope_key) -- one budget per rule" long after several windows per
// rule were added, which put the window id in the key and made a two-window rule
// two buckets. A sentence that is confidently wrong about the thing it exists to
// explain is worse than no sentence.
//
// This reads the middleware source and fails if the key gains or loses a
// component the published string does not mention. It is a coarse check on
// purpose: the alternative is exporting the key builder solely to be asserted
// on, which would be a worse trade than parsing one line.
func TestBucketKeyStringNamesEveryComponentOfTheRealKey(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("../middleware/ratelimit.go")
	if err != nil {
		t.Fatalf("read middleware source: %v", err)
	}

	// key: m.Rule.ID.String() + ":" + windowKey(wdw) + ":" + scopeValue,
	line := regexp.MustCompile(`(?m)^\s*key:\s*(.+),$`).FindSubmatch(src)
	if line == nil {
		t.Fatal("could not find the bucket key expression in middleware/ratelimit.go; " +
			"if it moved, point this test at it rather than deleting it")
	}

	expr := string(line[1])

	// Each component the key actually concatenates, and the word the published
	// string must use for it.
	for _, c := range []struct{ inKey, inString string }{
		{"m.Rule.ID", "rule_id"},
		{"windowKey(wdw)", "window parameters"},
		{"scopeValue", "scope_key"},
	} {
		if !strings.Contains(expr, c.inKey) {
			t.Fatalf("the bucket key no longer includes %s. If that is deliberate, remove %q "+
				"from the published bucket_key string in the same change:\n  %s",
				c.inKey, c.inString, expr)
		}

		if !strings.Contains(bucketKeyDescription, c.inString) {
			t.Fatalf("the bucket key includes %s but the published bucket_key string does not say %q. "+
				"Clients are told a budget is keyed on something narrower than it is:\n  %s",
				c.inKey, c.inString, bucketKeyDescription)
		}
	}

	// The verb is NOT in the key, and the string has said so from the start.
	// That half is the most likely misreading and the reason the field exists.
	if strings.Contains(expr, "r.Method") {
		t.Fatal("the bucket key now varies by verb, which doubles what a methods={GET,POST} rule allows. " +
			"If deliberate, the published string must stop saying the budget is shared across verbs")
	}
}
