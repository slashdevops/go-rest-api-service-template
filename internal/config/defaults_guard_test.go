package config

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// configDir is this package's source directory, resolved from this file rather
// than from the process working directory.
//
// A sibling test in this package chdirs, so filepath.Glob("*.go") found nothing
// when the suite ran as a whole -- the check silently examined zero files while
// passing on its own. The guard-the-guard assertion below is what turned that
// into a failure instead of a false green.
func configDir(t *testing.T) string {
	t.Helper()

	_, this, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve this file's path")
	}

	return filepath.Dir(this)
}

// No two settings in a group may share one default constant.
//
// The default for a setting is written TWICE -- once in NewField, the
// environment-variable path, and once in app.setupFlags, the flag path. Nothing
// connects them, so borrowing a neighbouring field's constant in one of the two
// makes the same setting answer differently depending on how it was set.
//
// That had already happened: MaxIdleConnsPerHost was declared with
// DefaultHTTPClientMaxIdleConns while its flag used
// DefaultHTTPClientMaxIdleConnsPerHost. It was invisible because both constants
// are 100 -- change either and the two paths silently disagree.
//
// The rule is "no sharing" rather than "the constant is named after its field":
// the naming rule sounds stronger but is not, because several groups spell the
// pair in a different word order (EntitiesHardTTL / DefaultCacheHardEntitiesTTL)
// and that is style, not a defect. Sharing is the thing that is always wrong.
func TestNoTwoSettingsShareADefaultConstant(t *testing.T) {
	t.Parallel()

	// One NewField call, matched without crossing into the next: the body may
	// not contain another "NewField(".
	newField := regexp.MustCompile(`(\w+):\s*NewField\(\s*"([^"]+)"\s*,\s*"[^"]+"\s*,((?:[^()]|\([^()]*\))*?),\s*(Default\w+)\s*\)`)
	ctor := regexp.MustCompile(`(?s)func New(\w+Config)\(\)[^{]*\{(.*?)\n\}`)

	dir := configDir(t)

	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	checked := 0

	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}

		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}

		for _, c := range ctor.FindAllSubmatch(src, -1) {
			group := string(c[1])
			owner := map[string]string{} // constant -> the flag that already claimed it

			for _, m := range newField.FindAllSubmatch(c[2], -1) {
				flag, constant := string(m[2]), string(m[4])
				checked++

				if first, taken := owner[constant]; taken {
					t.Errorf("%s (%s): %s and %s both default from %s.\n"+
						"    A default shared between two settings is how one of them ends up "+
						"disagreeing with its own flag registration; give each its own constant.",
						f, group, first, flag, constant)

					continue
				}

				owner[constant] = flag
			}
		}
	}

	// Guard the guard: a pattern that matched nothing would pass silently.
	if checked < 100 {
		t.Fatalf("only %d NewField declarations matched; the pattern has drifted "+
			"and this test is no longer checking anything", checked)
	}
}
