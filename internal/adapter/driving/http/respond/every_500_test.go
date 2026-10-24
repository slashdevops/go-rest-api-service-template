package respond

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writes500Itself are the functions that may write a 500 without asking
// WriteCallerGone, each with why.
var writes500Itself = map[string]string{
	// A panic is not a cancellation: whatever the request's context says,
	// the service failed.
	"Recovery": "a panic",
}

// TestEvery500AsksWhetherTheCallerWentAway: a request its caller abandoned
// is answered 499 with no body, and counted refused at every layer.
// A site that answers an error with a 500 of its own -- a sentence the
// contract names, not WriteInternalError's -- and does not ask
// WriteCallerGone first answers that request 500 at ERROR while the layers
// below count it refused: CheckAuthz and the two health checks did. Every
// function of the HTTP adapter that writes a 500 itself asks it.
func TestEvery500AsksWhetherTheCallerWentAway(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	checked := 0

	for _, dir := range []string{"../handler", "../middleware"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}

		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}

			file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
			if err != nil {
				t.Fatal(err)
			}

			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}

				var writes500, asks bool

				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}

					switch callee := selector(call.Fun); callee {
					case "respond.WriteCallerGone":
						asks = true
					case "respond.WriteJSONMessage", "respond.WriteJSONMessageWithCode":
						if len(call.Args) > 2 && selector(call.Args[2]) == "http.StatusInternalServerError" {
							writes500 = true
						}
					}

					return true
				})

				if !writes500 {
					continue
				}

				checked++

				if _, exempt := writes500Itself[fn.Name.Name]; exempt {
					continue
				}

				if !asks {
					t.Errorf("%s: %s writes a 500 of its own and does not ask respond.WriteCallerGone first: "+
						"a request its caller abandoned is answered 500 at ERROR and counted refused below", fset.Position(fn.Pos()), fn.Name.Name)
				}
			}
		}
	}

	if checked < 4 {
		t.Fatalf("found %d functions that write a 500 themselves, want at least CheckAuthz, the two health checks and Recovery: this would pass by reading nothing", checked)
	}
}

// selector is "pkg.Name" for a selector expression, and "" otherwise.
func selector(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}

	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}

	return pkg.Name + "." + sel.Sel.Name
}
