package o11y_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// logCallees are the functions whose odd string arguments are attribute KEYS.
var logCallees = map[string]bool{
	"Debug": true, "Info": true, "Warn": true, "Error": true,
	"DebugContext": true, "InfoContext": true, "WarnContext": true, "ErrorContext": true,
	"Trace": true, "Fatal": true, "Log": true,
}

// sensitiveLogKeys must never appear as an attribute key above TRACE.
//
// # What this is guarding
//
// TRACE is the only level that never leaves the process ([o11y.ExportedLogFloor]),
// and it is the only level allowed to carry SQL with its arguments, request and
// response bodies, and anything else unbounded. Everything above it reaches a
// searchable, retained log store.
//
// Each key below named a real line before this test existed:
//
//   - "email" on three WARN lines in the re-verification path, so every address
//     that asked to be re-verified was written to the store.
//   - "query" on seven DEBUG lines in the repository. DEBUG is above the export
//     floor, so those shipped -- and repository.Users.Insert interpolated its
//     arguments, which on that statement are the user's e-mail and their bcrypt
//     password hash. 161 other SQL logs in the same package already used
//     cslog.Trace; these seven were the outliers, and being outliers is exactly
//     why nobody noticed.
//
// The rule is a blocklist rather than a review habit because the failure is
// silent and permanent: nothing breaks, the line simply exists in a store that
// is backed up and searchable by everyone who can read it.
var sensitiveLogKeys = map[string]string{
	"email":         "log user_id; an address identifies a person and the store is searchable",
	"user_email":    "log user_id",
	"mail":          "log user_id",
	"password":      "never log password material at any level",
	"password_hash": "never log password material at any level",
	"secret":        "never log a secret at any level",
	"client_secret": "never log a secret at any level",
	"token":         "log the jti, never the token",
	"access_token":  "log the jti, never the token",
	"refresh_token": "log the jti, never the token",
	"api_key":       "never log a credential at any level",
	"api_token":     "never log a credential at any level",
	"query":         "SQL belongs on cslog.Trace, which never leaves the process",
	"sql":           "SQL belongs on cslog.Trace, which never leaves the process",
	"prompt":        "a prompt is the tenant's data; TRACE only",
	"completion":    "a completion is the tenant's data; TRACE only",
	"body":          "a body is unbounded and may hold anything; TRACE only",
}

// exportedLogCall reports whether a call is one whose records can be exported.
// cslog.Trace and cslog.Fatal are excluded for opposite reasons: Trace is below
// the floor and never leaves, and Fatal is the last thing a process says.
func exportedLogCall(pkg, fn string) bool {
	if pkg == "cslog" {
		return false
	}

	return pkg == "slog" && logCallees[fn]
}

// TestNoSensitiveKeysAboveTrace fails the build when a log line that can be
// exported carries a key that must not leave the process.
func TestNoSensitiveKeysAboveTrace(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var offenders []string

	for _, dir := range []string{"internal", "cmd", "pkg"} {
		err := filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fset := token.NewFileSet()

			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return parseErr
			}

			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				relative = path
			}

			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				pkg, ok := sel.X.(*ast.Ident)
				if !ok || !exportedLogCall(pkg.Name, sel.Sel.Name) {
					return true
				}

				for _, arg := range call.Args {
					lit, ok := arg.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}

					key, unquoteErr := strconv.Unquote(lit.Value)
					if unquoteErr != nil {
						continue
					}

					if why, bad := sensitiveLogKeys[key]; bad {
						offenders = append(offenders,
							relative+":"+strconv.Itoa(fset.Position(lit.Pos()).Line)+
								" key "+lit.Value+" -- "+why)
					}
				}

				return true
			})

			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}

	if len(offenders) > 0 {
		t.Errorf("%d log call(s) carry a key that must not leave the process.\n"+
			"Everything above TRACE reaches a searchable, retained log store; only "+
			"cslog.Trace stays local.\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// TestLogKeysAreSnakeCase pins the spelling of attribute keys.
//
// A key is how a value is found in the log store, and a store that holds both
// userID and user_id cannot be queried for either: a filter on one silently
// misses the other's lines, and nothing reports the miss. Both spellings were
// present -- userID three times against user_id nine -- along with scopeID,
// languageID, softLimit and five more.
//
// snake_case, because that is what Loki shows: it rewrites dots to underscores
// when it promotes an attribute, so app.layer is queried as app_layer and a
// camelCase key would be the only thing on a line spelled differently from
// every other.
func TestLogKeysAreSnakeCase(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var offenders []string

	for _, dir := range []string{"internal", "cmd", "pkg"} {
		err := filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fset := token.NewFileSet()

			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return parseErr
			}

			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				relative = path
			}

			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				pkg, ok := sel.X.(*ast.Ident)
				if !ok || (pkg.Name != "slog" && pkg.Name != "cslog") || !logCallees[sel.Sel.Name] {
					return true
				}

				// Odd arguments after the message are keys. The message itself
				// is prose and is not checked.
				for i, arg := range call.Args {
					if !isKeyPosition(pkg.Name, sel.Sel.Name, i) {
						continue
					}

					lit, ok := arg.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}

					key, unquoteErr := strconv.Unquote(lit.Value)
					if unquoteErr != nil || key == "" {
						continue
					}

					if strings.ToLower(key) != key {
						offenders = append(offenders,
							relative+":"+strconv.Itoa(fset.Position(lit.Pos()).Line)+" key "+lit.Value)
					}
				}

				return true
			})

			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}

	if len(offenders) > 0 {
		t.Errorf("%d log attribute key(s) are not lower_snake_case. A store holding "+
			"both userID and user_id can be queried for neither.\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// isKeyPosition reports whether argument i of a log call is an attribute key.
//
// The shapes differ: slog.Info(msg, k, v, ...) puts the message first, while
// slog.InfoContext(ctx, msg, k, v, ...) and cslog.Trace(ctx, msg, k, v, ...)
// put a context before it. Getting this wrong would check the MESSAGE, which is
// prose and legitimately contains capitals.
func isKeyPosition(pkg, fn string, i int) bool {
	first := 1 // index of the first key, after the message

	switch {
	case pkg == "cslog", strings.HasSuffix(fn, "Context"):
		first = 2 // ctx, msg, then keys
	case fn == "Log":
		first = 3 // ctx, level, msg, then keys
	}

	return i >= first && (i-first)%2 == 0
}
