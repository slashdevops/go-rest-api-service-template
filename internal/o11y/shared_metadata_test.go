package o11y_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoSharedMetadataActionWrite fails the build if anything assigns to the
// Action field of a metadata value held on a receiver.
//
// # What it is guarding
//
// A handler, service or repository builds one o11y.Metadata at construction and
// keeps it on the struct. That struct is shared by every request the instance
// serves, and the instances are long-lived singletons, so
//
//	ref.metricsMetadata.Action = "GetByID"
//
// is two concurrent requests writing the same word. It is a data race by the
// letter of the memory model, and its visible symptom is worse than a crash:
// whichever request loses has its span and its metric filed under the other
// one's action, so the traces are quietly wrong rather than absent.
//
// It was at 447 call sites, in every layer, because the package documentation
// showed it as the pattern. Nothing caught it for years: no unit test drove a
// single instance concurrently under -race, and the integration suite runs a
// binary built without the detector. It surfaced only when two endpoints of one
// handler were driven at once in a test.
//
// o11y.Metadata is passed by value, so [o11y.SetupTrace] and its siblings take
// the action as a parameter and set it on their own copy. There is no longer a
// reason to write the stored field, which is why this test can be absolute
// about it.
//
// # Why AST and not grep
//
// A regexp over the source would match the pattern in a comment, in a string,
// and in this file's own documentation — and would miss a receiver named
// anything other than `ref`. Matching the assignment structurally is exact.
func TestNoSharedMetadataActionWrite(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var offenders []string

	for _, dir := range []string{"internal", "cmd"} {
		walk := filepath.Join(root, dir)

		err := filepath.Walk(walk, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}

			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}

			rel, _ := filepath.Rel(root, path)

			ast.Inspect(file, func(n ast.Node) bool {
				assign, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}

				for _, lhs := range assign.Lhs {
					if name := sharedActionTarget(lhs); name != "" {
						offenders = append(offenders, rel+": "+name+" = ...")
					}
				}

				return true
			})

			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", walk, err)
		}
	}

	if len(offenders) > 0 {
		t.Errorf(
			"assignment to the Action field of a metadata value held on a receiver, in %d place(s):\n  %s\n\n"+
				"That field is shared by every request the instance serves. Pass the action to "+
				"o11y.SetupTrace / SetupTraceHTTP / SetupTraceWithTimeout instead — they set it on a copy.",
			len(offenders), strings.Join(offenders, "\n  "),
		)
	}
}

// sharedActionTarget returns the printed expression when lhs is a write to
// `<something>.<field>.Action`, and "" otherwise.
//
// The middle selector is not pinned to `metricsMetadata`: a future field with a
// different name is the same bug, and naming it here would let a rename walk
// straight past the guard.
func sharedActionTarget(lhs ast.Expr) string {
	outer, ok := lhs.(*ast.SelectorExpr)
	if !ok || outer.Sel.Name != "Action" {
		return ""
	}

	inner, ok := outer.X.(*ast.SelectorExpr)
	if !ok {
		// A plain `meta.Action = ...` on a local variable is fine: a local is
		// not shared with anybody.
		return ""
	}

	receiver, ok := inner.X.(*ast.Ident)
	if !ok {
		return ""
	}

	return receiver.Name + "." + inner.Sel.Name + ".Action"
}

func repoRoot(t *testing.T) string {
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
			t.Fatal("could not find the repository root (no go.mod above the test)")
		}

		dir = parent
	}
}
