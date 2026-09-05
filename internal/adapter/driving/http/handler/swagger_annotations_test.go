package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// statusNames maps the net/http constant names this service writes to their
// codes. Listing them rather than reflecting over net/http keeps the test
// honest about what it understands: an unrecognised constant is reported
// instead of silently ignored.
var statusNames = map[string]int{
	"StatusOK": 200, "StatusCreated": 201, "StatusAccepted": 202, "StatusNoContent": 204,
	"StatusPartialContent": 206, "StatusFound": 302, "StatusSeeOther": 303,
	"StatusNotModified": 304, "StatusBadRequest": 400, "StatusUnauthorized": 401,
	"StatusForbidden": 403, "StatusNotFound": 404, "StatusMethodNotAllowed": 405,
	"StatusConflict": 409, "StatusGone": 410, "StatusUnprocessableEntity": 422,
	"StatusTooManyRequests": 429, "StatusInternalServerError": 500,
	"StatusNotImplemented": 501, "StatusBadGateway": 502,
	"StatusServiceUnavailable": 503, "StatusGatewayTimeout": 504,
}

// middlewareSupplied are answered before a handler runs, so they are declared
// from the middleware chain and never appear in a body.
var middlewareSupplied = []int{401, 403, 429}

var declaredStatus = regexp.MustCompile(`@(?:Success|Failure)\s+(\d{3})`)

// TestEverySwaggerStatusIsDeclared checks each handler's @Success/@Failure
// annotations against the status codes it can actually write.
//
// # Why this is a test and not a checklist
//
// Everything under docs/api/ is generated FROM these annotations, and nothing
// validates them against the code. That inverts the usual risk: the generated
// spec is always "in sync" -- with whatever the comments happen to say -- so a
// wrong @Failure ships as the published contract and no gate fails. The
// guidelines describe auditing this by hand after a batch of handler work; this
// does it on every run instead. It found one: /health/detailed answers 206 when
// a component is degraded and said nothing about it.
//
// Two directions, and they fail differently:
//
//   - written but not declared: a real response no generated client expects
//   - declared but not written: a branch invented for every generated client,
//     which the guidelines call worse than silence
//
// # False positives this avoids by walking the syntax tree
//
// A hand-rolled scan of the source finds three things that are not responses,
// and all three are present in this package: a status assigned to a STRUCT
// FIELD (`RedirectCode: http.StatusFound` is a value in a body, not a response
// code), a status inside COMMENTED-OUT code (registerUser carries a disabled
// `// http.Redirect(...)`), and a status reached only through a VARIABLE or a
// HELPER, which a naive scan misses in the other direction. The first two are
// excluded by construction here; the last two are followed explicitly.
func TestEverySwaggerStatusIsDeclared(t *testing.T) {
	files := parseHandlerPackage(t)

	// Every function in the package -- methods and plain -- so a status
	// produced by a helper can be followed to where it is written.
	bodies := map[string]*ast.BlockStmt{}

	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				bodies[fn.Name.Name] = fn.Body
			}
		}
	}

	audited := 0

	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Doc == nil || fn.Body == nil || !isHTTPHandler(fn) {
				continue
			}

			doc := fn.Doc.Text()
			if !strings.Contains(doc, "@Router") {
				continue
			}

			audited++

			declared := map[int]bool{}
			for _, m := range declaredStatus.FindAllStringSubmatch(doc, -1) {
				code, _ := strconv.Atoi(m[1])
				declared[code] = true
			}

			written := writtenStatuses(fn.Body, bodies, 0)

			for code := range written {
				if !declared[code] {
					t.Errorf(
						"%s writes %d but does not declare it: add an @Success/@Failure, "+
							"or every generated client will treat a real response as unexpected",
						fn.Name.Name, code)
				}
			}

			for code := range declared {
				if !written[code] && !slices.Contains(middlewareSupplied, code) {
					t.Errorf(
						"%s declares %d but cannot write it: remove the annotation, "+
							"or every generated client gets a branch that never happens",
						fn.Name.Name, code)
				}
			}
		}
	}

	if audited == 0 {
		t.Fatal("no annotated handlers found; this test would pass by looking at nothing")
	}

	t.Logf("audited %d annotated handlers", audited)
}

// isHTTPHandler reports whether fn takes (http.ResponseWriter, *http.Request).
func isHTTPHandler(fn *ast.FuncDecl) bool {
	params := fn.Type.Params
	if params == nil || len(params.List) != 2 {
		return false
	}

	sel, ok := params.List[0].Type.(*ast.SelectorExpr)

	return ok && sel.Sel.Name == "ResponseWriter"
}

// writtenStatuses returns every status code this body can send: written
// literally, held in a variable first, or written by a helper it calls.
func writtenStatuses(body *ast.BlockStmt, bodies map[string]*ast.BlockStmt, depth int) map[int]bool {
	out := map[int]bool{}

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		pkg, isIdent := sel.X.(*ast.Ident)

		switch {
		// respond.WriteJSONMessage(w, r, http.StatusX, ...) and friends, plus
		// http.Redirect(w, r, url, http.StatusX).
		case isIdent && (pkg.Name == "respond" || (pkg.Name == "http" && sel.Sel.Name == "Redirect")):
			for _, arg := range call.Args {
				if code, ok := statusFromExpr(arg); ok {
					out[code] = true
				}

				// A status chosen earlier and passed in -- the shape
				// getDetailedHealth uses to pick 200/206/503.
				if ident, ok := arg.(*ast.Ident); ok {
					for code := range assignedStatuses(body, ident.Name, bodies) {
						out[code] = true
					}
				}
			}

		// ref.someHelper(...) — the helper may be the thing that writes.
		case isIdent && pkg.Name == "ref" && depth < 2:
			if helper, found := bodies[sel.Sel.Name]; found && helper != body {
				for code := range writtenStatuses(helper, bodies, depth+1) {
					out[code] = true
				}
			}
		}

		return true
	})

	return out
}

// assignedStatuses finds the status codes that can reach name within body,
// whether written as a constant or returned by a helper it was assigned from.
func assignedStatuses(body *ast.BlockStmt, name string, bodies map[string]*ast.BlockStmt) map[int]bool {
	out := map[int]bool{}

	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		for i, lhs := range assign.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if !ok || ident.Name != name || i >= len(assign.Rhs) {
				continue
			}

			rhs := assign.Rhs[i]

			if code, ok := statusFromExpr(rhs); ok {
				out[code] = true

				continue
			}

			// statusCode := httpStatusForHealth(...) -- the codes are the ones
			// that function can return.
			if call, ok := rhs.(*ast.CallExpr); ok {
				if fn, ok := call.Fun.(*ast.Ident); ok {
					if helper, found := bodies[fn.Name]; found {
						for code := range returnedStatuses(helper) {
							out[code] = true
						}
					}
				}
			}
		}

		return true
	})

	return out
}

// returnedStatuses finds the status constants a function returns.
func returnedStatuses(body *ast.BlockStmt) map[int]bool {
	out := map[int]bool{}

	ast.Inspect(body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}

		for _, result := range ret.Results {
			if code, ok := statusFromExpr(result); ok {
				out[code] = true
			}
		}

		return true
	})

	return out
}

// statusFromExpr reads http.StatusX. A KeyValueExpr never reaches here, so a
// status used as a struct field is not mistaken for a response code.
func statusFromExpr(e ast.Expr) (int, bool) {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return 0, false
	}

	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "http" {
		return 0, false
	}

	code, found := statusNames[sel.Sel.Name]

	return code, found
}

// parseHandlerPackage parses every non-test file of this package.
//
// parser.ParseDir is deprecated as of Go 1.25 and x/tools/go/packages is a
// dependency this does not need: the handlers are one flat directory.
func parseHandlerPackage(t *testing.T) []*ast.File {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the handler package: %v", err)
	}

	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}

		files = append(files, file)
	}

	return files
}

var securityScheme = regexp.MustCompile(`@Security\s+(\w+)`)

// TestEveryAuthenticatedHandlerDeclaresItsMiddlewareCodes requires 401, 403 and
// 429 on every handler that sits behind a token.
//
// # Why this is separate from TestEverySwaggerStatusIsDeclared
//
// That test compares annotations against the codes a handler BODY writes, and
// these three are never in a body -- they are written by the chain in
// internal/app/server.go before the handler runs. So it can only permit them,
// which is what middlewareSupplied does. Nothing required them, and the drift
// that followed was total: 108 of 136 authenticated operations did not declare
// the 401 their own chain returns, 109 omitted 403 and 129 omitted 429. Every
// generated client modelled those endpoints as unable to fail authentication.
//
// The chain, from internal/app/server.go:
//
//	CheckAccessToken -> CheckPATokenActive -> CheckAuthz    (inside the limiter)
//
// # The one exception, and why it is not a widening
//
// verificationTokenMiddlewares is the only chain WITHOUT CheckAuthz: the caller
// is proving an email address, holds no roles yet, and their account stays
// disabled until the call succeeds, so an authorization check would refuse
// every legitimate verification. A 403 there is a branch that never happens,
// which the guidelines call worse than silence. It is keyed on the declared
// scheme rather than on the endpoint's name, so a second verification-style
// route is covered without being told about it.
//
// 429 has no exception. Two chains skip the POST-auth limiter deliberately
// (password reset and verification), but the PRE-auth stage is part of
// apiCommonMdws and so wraps the whole API router -- only the /health and
// /version bypass prefixes escape it, and neither is authenticated.
func TestEveryAuthenticatedHandlerDeclaresItsMiddlewareCodes(t *testing.T) {
	files := parseHandlerPackage(t)
	audited := 0

	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Doc == nil || !isHTTPHandler(fn) {
				continue
			}

			doc := fn.Doc.Text()
			if !strings.Contains(doc, "@Router") {
				continue
			}

			schemes := map[string]bool{}
			for _, m := range securityScheme.FindAllStringSubmatch(doc, -1) {
				schemes[m[1]] = true
			}

			if len(schemes) == 0 {
				continue // a public endpoint: none of the three apply
			}

			audited++

			declared := map[int]bool{}
			for _, m := range declaredStatus.FindAllStringSubmatch(doc, -1) {
				code, _ := strconv.Atoi(m[1])
				declared[code] = true
			}

			for _, code := range middlewareSupplied {
				if code == 403 && len(schemes) == 1 && schemes["VerificationToken"] {
					continue
				}

				if !declared[code] {
					t.Errorf(
						"%s is authenticated but does not declare %d: its middleware chain "+
							"can return it, and nothing in the handler body will ever show that",
						fn.Name.Name, code)
				}
			}
		}
	}

	if audited == 0 {
		t.Fatal("no authenticated handlers found; this test would pass by looking at nothing")
	}

	t.Logf("audited %d authenticated handlers", audited)
}
