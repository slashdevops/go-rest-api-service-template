package app

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestRunScriptAndAirAgree pins that the two ways of starting the service
// against the development stack start the SAME service.
//
// # Why this is worth a test
//
// run.sh and .air.toml are both "run this against the dev environment". When
// they drift, the one you did not use is the one that hides a bug — and they
// had drifted: run.sh ran the IP rate limiter at 200 req/s burst 400 while
// .air.toml ran the shipped 100/300. A limit that held under `air` could pass
// under run.sh and vice versa, and neither matched what ships.
//
// It is the same rule that removed the old 24h access-token override from both
// files: a dev stack that disagrees with production hides exactly the bugs
// production will have. This test extends that from "dev vs production" to
// "dev vs dev", which is the gap the override fix did not close.
func TestRunScriptAndAirAgree(t *testing.T) {
	t.Parallel()

	root := repoRootFrom(t)

	air := airArgs(t, filepath.Join(root, ".air.toml"))
	run := runScriptArgs(t, filepath.Join(root, "run.sh"))

	if len(air) == 0 {
		t.Fatal("parsed no arguments out of .air.toml; the parser has lost track of the format")
	}

	onlyInRun := difference(run, air)
	onlyInAir := difference(air, run)

	if len(onlyInRun) > 0 || len(onlyInAir) > 0 {
		t.Errorf(
			"run.sh and .air.toml do not start the same service.\n"+
				"  only in run.sh:    %v\n"+
				"  only in .air.toml: %v\n\n"+
				"Both are 'run this against the dev environment'. Whichever one you are not "+
				"using is the one that will hide a bug — and where a value has a shipped "+
				"default, dev should run the shipped default.",
			onlyInRun, onlyInAir,
		)
	}
}

// airArgs pulls the args_bin array out of .air.toml. Comments inside the array
// are ignored, which is why the quoted strings are matched rather than the
// lines.
func airArgs(t *testing.T, path string) []string {
	t.Helper()

	body := readFile(t, path)

	block := regexp.MustCompile(`(?s)args_bin\s*=\s*\[(.*?)\n\]`).FindStringSubmatch(body)
	if block == nil {
		t.Fatalf("%s: could not find the args_bin array", path)
	}

	var args []string

	for _, m := range regexp.MustCompile(`"(-[^"]*)"`).FindAllStringSubmatch(block[1], -1) {
		args = append(args, normaliseArg(m[1]))
	}

	slices.Sort(args)

	return args
}

// runScriptArgs pulls the flags out of run.sh: every continued line that starts
// with a dash.
func runScriptArgs(t *testing.T, path string) []string {
	t.Helper()

	var args []string

	for line := range strings.SplitSeq(readFile(t, path), "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), `\`))
		if !strings.HasPrefix(trimmed, "-") {
			continue
		}

		args = append(args, normaliseArg(trimmed))
	}

	slices.Sort(args)

	return args
}

// normaliseArg strips the quoting each format needs, so that
// `-x.y.z="*"` in a shell script and `-x.y.z=*` in TOML compare equal.
func normaliseArg(arg string) string {
	return strings.ReplaceAll(strings.TrimSpace(arg), `"`, "")
}

func difference(a, b []string) []string {
	var only []string

	for _, x := range a {
		if !slices.Contains(b, x) {
			only = append(only, x)
		}
	}

	return only
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	return string(body)
}

func repoRootFrom(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find the repository root")
		}

		dir = parent
	}
}
