package repositorypg

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// The constraint names are a contract with the migration: renaming one there
// without changing the constant here does not break the build, it turns a
// documented 400 into a 500 discovered by a user.
func TestTokenLifetimesConstraintNamesExistInTheMigration(t *testing.T) {
	t.Parallel()

	_, this, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve this file's path")
	}

	migration := filepath.Join(filepath.Dir(this), "..", "..", "..", "..", "database", "migrations", "00016_authn_token_lifetimes.sql")

	body, err := os.ReadFile(migration)
	if err != nil {
		t.Fatalf("read %s: %v", migration, err)
	}

	for _, name := range []string{
		constraintTokenLifetimesAccess,
		constraintTokenLifetimesRefresh,
		constraintTokenLifetimesOrder,
	} {
		if !strings.Contains(string(body), "CONSTRAINT "+name+" ") {
			t.Errorf("constraint %q is referenced from Go but not declared in %s", name, filepath.Base(migration))
		}
	}
}

// The seed row and the Go defaults are the same two numbers written in two
// places -- the migration in seconds, the domain in time.Duration -- and
// nothing connects them. This is what does.
func TestSeedTokenLifetimesMatchDomainDefaults(t *testing.T) {
	t.Parallel()

	_, this, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve this file's path")
	}

	migration := filepath.Join(filepath.Dir(this), "..", "..", "..", "..", "database", "migrations", "00016_authn_token_lifetimes.sql")

	body, err := os.ReadFile(migration)
	if err != nil {
		t.Fatalf("read %s: %v", migration, err)
	}

	seed := regexp.MustCompile(`INSERT INTO authn_token_lifetimes \(id, access_token_seconds, refresh_token_seconds\)\s*VALUES \('[0-9a-f-]+', (\d+), (\d+)\)`)

	m := seed.FindStringSubmatch(string(body))
	if m == nil {
		t.Fatalf("could not find the seed INSERT in %s", filepath.Base(migration))
	}

	access, _ := strconv.Atoi(m[1])
	refresh, _ := strconv.Atoi(m[2])

	defaults := domain.DefaultTokenLifetimes()

	if time.Duration(access)*time.Second != defaults.AccessTokenDuration {
		t.Errorf("seed access_token_seconds = %d, domain default = %s", access, defaults.AccessTokenDuration)
	}

	if time.Duration(refresh)*time.Second != defaults.RefreshTokenDuration {
		t.Errorf("seed refresh_token_seconds = %d, domain default = %s", refresh, defaults.RefreshTokenDuration)
	}

	// The CHECK bounds too: they are the domain bounds in seconds.
	bounds := domain.TokenLifetimesBounds()

	for _, want := range []string{
		"access_token_seconds BETWEEN " + strconv.Itoa(int(bounds.AccessTokenMin.Seconds())) + " AND " + strconv.Itoa(int(bounds.AccessTokenMax.Seconds())),
		"refresh_token_seconds BETWEEN " + strconv.Itoa(int(bounds.RefreshTokenMin.Seconds())) + " AND " + strconv.Itoa(int(bounds.RefreshTokenMax.Seconds())),
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("migration CHECK does not match the domain bounds; expected %q", want)
		}
	}
}
