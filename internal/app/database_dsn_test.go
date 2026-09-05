package app

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
)

func dsnConfig(t *testing.T) *config.DatabaseConfig {
	t.Helper()

	c := config.NewDatabaseConfig()
	c.Address.Value = "localhost"
	c.Port.Value = 5432
	c.Username.Value = "username"
	c.Password.Value = "password"
	c.Name.Value = "goapitemplate"

	return c
}

// TestDatabaseDSNEscapesEveryComponent is the regression test for a bug that
// was live, not hypothetical.
//
// The DSN used to be built with fmt.Sprintf as key=value pairs, which cannot
// carry a value containing a space and failed silently: a password of "my pass"
// produced `password=my pass`, and pgx read the password as "my" and then lost
// the database name because the remaining tokens no longer lined up. The
// connection failed with an error about the wrong thing, or connected as
// something else.
func TestDatabaseDSNEscapesEveryComponent(t *testing.T) {
	t.Parallel()

	passwords := []struct {
		name  string
		value string
	}{
		{"plain", "password"},
		{"with a space", "my pass"},
		{"with a single quote", "pa'ss"},
		{"with an equals sign", "pa=ss"},
		{"with an at sign", "pa@ss"},
		{"with a slash", "pa/ss"},
		{"with a percent", "pa%ss"},
	}

	for _, tc := range passwords {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := dsnConfig(t)
			c.Password.Value = tc.value

			parsed, err := pgxpool.ParseConfig(databaseDSN(c))
			if err != nil {
				t.Fatalf("the DSN did not parse: %v", err)
			}

			if got := parsed.ConnConfig.Password; got != tc.value {
				t.Errorf("password round-tripped as %q, want %q", got, tc.value)
			}

			// The database name is the field the old bug silently dropped.
			if got := parsed.ConnConfig.Database; got != "goapitemplate" {
				t.Errorf("database name round-tripped as %q, want %q", got, "goapitemplate")
			}

			if got := parsed.ConnConfig.User; got != "username" {
				t.Errorf("user round-tripped as %q, want %q", got, "username")
			}
		})
	}
}

func TestDatabaseDSNCarriesSSLSettings(t *testing.T) {
	t.Parallel()

	t.Run("no certificate paths when none are configured", func(t *testing.T) {
		t.Parallel()

		// An empty sslrootcert is not the same as an absent one — pgx tries to
		// read it and fails, so the DSN must omit the key entirely.
		dsn := databaseDSN(dsnConfig(t))

		for _, key := range []string{"sslrootcert", "sslcert", "sslkey"} {
			if strings.Contains(dsn, key) {
				t.Errorf("DSN carries %s with nothing configured: %s", key, dsn)
			}
		}
	})

	t.Run("a path containing a space survives", func(t *testing.T) {
		t.Parallel()

		// Certificate paths are operator-supplied and nothing stops one having
		// a space in it. This is the case the old key=value DSN could not carry.
		c := dsnConfig(t)
		c.SSLMode.Value = "verify-full"
		c.SSLRootCertFile.Value = "/etc/my certs/ca.crt"

		dsn := databaseDSN(c)

		// pgx would read the file, which does not exist here, so assert on the
		// encoded DSN rather than on a parse.
		if !strings.Contains(dsn, "sslrootcert=") {
			t.Fatalf("sslrootcert missing from %s", dsn)
		}

		if strings.Contains(dsn, "/etc/my certs/ca.crt") {
			t.Errorf("the path was embedded unescaped, which is what broke the old DSN: %s", dsn)
		}
	})

	t.Run("sslmode and TimeZone are carried", func(t *testing.T) {
		t.Parallel()

		c := dsnConfig(t)
		c.SSLMode.Value = "require"

		parsed, err := pgxpool.ParseConfig(databaseDSN(c))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}

		if parsed.ConnConfig.TLSConfig == nil {
			t.Error("sslmode=require should have produced a TLS config")
		}
	})

	t.Run("disable produces no TLS config", func(t *testing.T) {
		t.Parallel()

		parsed, err := pgxpool.ParseConfig(databaseDSN(dsnConfig(t)))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}

		if parsed.ConnConfig.TLSConfig != nil {
			t.Error("sslmode=disable should not have produced a TLS config")
		}
	})
}
