// Architecture test: enforces the hexagonal invariant that nothing
// under internal/core/... imports infrastructure. Runs as part of the
// regular `go test ./...` suite — any new violation breaks CI.
//
// If you genuinely need to reach for infrastructure from a use-case,
// add a port under internal/core/port/driven/ and an adapter under
// internal/adapter/driven/ instead of relaxing this test.

package core_test

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// forbiddenInCore lists import path fragments that must never appear
// in any package under internal/core/...
//
// Anything that touches the network, a database, a filesystem, a
// telemetry exporter, or a third-party SDK belongs in an adapter, not
// in the core.
var forbiddenInCore = []string{
	// adapters and the composition root
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/",
	"github.com/slashdevops/go-rest-api-service-template/internal/app",
	// HTTP / network
	"net/http",
	// databases
	"database/sql",
	"github.com/jackc/pgx",
	// cache backends
	"github.com/valkey-io/valkey-go",
	// mail
	"github.com/slashdevops/mailer",
	// policy engine
	"github.com/open-policy-agent/opa",
}

func TestCoreHasNoInfraImports(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedImports | packages.NeedName}
	pkgs, err := packages.Load(cfg, "github.com/slashdevops/go-rest-api-service-template/internal/core/...")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("no packages found under internal/core/...; expected at least the root core package")
	}

	var violations int
	for _, p := range pkgs {
		for imp := range p.Imports {
			for _, bad := range forbiddenInCore {
				if strings.Contains(imp, bad) {
					t.Errorf("%s imports forbidden %q (matched %q)", p.PkgPath, imp, bad)
					violations++
				}
			}
		}
	}
	if violations > 0 {
		t.Logf("hexagonal invariant violated: %d forbidden imports under internal/core/", violations)
	}
}
