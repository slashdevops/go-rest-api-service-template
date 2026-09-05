package app

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestEveryHandlerIsRegistered pins that every handler the composition root
// builds is also mounted on the router.
//
// # Why this is worth a test
//
// products -- the worked example the whole template tells you to copy -- had
// a domain, both ports, a use-case, a repository, a handler, a migration and
// an integration suite, and answered 404 on every route: nothing in
// internal/app constructed it or called its RegisterRoutes. Every gate was
// green, because no gate looks at the composition root, and the integration
// suite that would have caught it is the one CI never runs.
//
// The composition root is the one place a new entity can be forgotten without
// a compile error, so this test reads it: a field on [Handlers] with no
// matching `a.handlers.<Field>.RegisterRoutes(` in server.go is a handler that
// exists and serves nothing.
func TestEveryHandlerIsRegistered(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("reading server.go: %v", err)
	}

	handlers := reflect.TypeFor[Handlers]()

	for i := range handlers.NumField() {
		field := handlers.Field(i).Name

		if !strings.Contains(string(src), "a.handlers."+field+".RegisterRoutes(") {
			t.Errorf(
				"Handlers.%s is built but never registered: add "+
					"`a.handlers.%s.RegisterRoutes(...)` to registerRoutes in server.go, "+
					"or the entity answers 404 on every route",
				field, field,
			)
		}
	}
}
