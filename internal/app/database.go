package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/slashdevops/go-rest-api-service-template/database"
)

// databaseDSN builds the connection string as a URL rather than as
// key=value pairs.
//
// The key=value form this used to build with fmt.Sprintf could not carry a
// value containing a space, and did so silently. A password of "my pass"
// produced `password=my pass`, which pgx parses as password "my" followed by a
// bare token — taking the *database name* with it, because the token count no
// longer lines up. The connection then fails with an error about the wrong
// thing entirely, or succeeds as a different user.
//
// A URL escapes every component, which fixes that and is also what makes the
// certificate paths below safe: they are operator-supplied filesystem paths and
// nothing stops one containing a space.
func databaseDSN(cfg *config.DatabaseConfig) string {
	params := url.Values{}
	params.Set("sslmode", cfg.SSLMode.Value)
	params.Set("TimeZone", cfg.TimeZone.Value)

	// Only send certificate paths that are actually configured. An empty
	// sslrootcert is not the same as an absent one: pgx tries to read it.
	for key, value := range map[string]string{
		"sslrootcert": cfg.SSLRootCertFile.Value,
		"sslcert":     cfg.SSLCertFile.Value,
		"sslkey":      cfg.SSLKeyFile.Value,
	} {
		if value != "" {
			params.Set(key, value)
		}
	}

	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.Username.Value, cfg.Password.Value),
		Host:     net.JoinHostPort(cfg.Address.Value, strconv.Itoa(cfg.Port.Value)),
		Path:     "/" + cfg.Name.Value,
		RawQuery: params.Encode(),
	}

	return dsn.String()
}

// warnOnUnprotectedDatabaseConnection reports a connection that is not
// protected, once, at startup.
//
// "disable" is plainly cleartext. "allow" and "prefer" are the ones worth
// naming separately: they read as though they do something, and they silently
// fall back to cleartext when the server does not offer TLS, with nothing
// reporting that it happened. "require" encrypts to whoever answered but does
// not authenticate them, so it does not stop an interception that terminates
// TLS.
func (a *App) warnOnUnprotectedDatabaseConnection() {
	switch a.configs.Database.SSLMode.Value {
	case "disable":
		slog.Warn("database connection is not encrypted; credentials, password hashes and embeddings cross the network in the clear",
			"ssl_mode", "disable",
			"enable_with", "database.ssl.mode=verify-full",
		)
	case "allow", "prefer":
		slog.Warn("database SSL mode silently falls back to cleartext when the server does not offer TLS",
			"ssl_mode", a.configs.Database.SSLMode.Value,
			"use_instead", "verify-full",
		)
	case "require":
		slog.Warn("database connection is encrypted but the server is not authenticated; this does not stop an interception",
			"ssl_mode", "require",
			"use_instead", "verify-full",
		)
	}
}

// initDatabase sets up the database connection and runs migrations if enabled
func (a *App) initDatabase(ctx context.Context) error {
	dbDSN := databaseDSN(a.configs.Database)

	a.warnOnUnprotectedDatabaseConnection()

	// Parse config. ParseConfig also reads and validates any certificate files
	// named in the DSN, so a wrong path or an unreadable CA fails here rather
	// than at the first connection.
	dbCfg, err := pgxpool.ParseConfig(dbDSN)
	if err != nil {
		return fmt.Errorf("error parsing pgx pool config: %w", err)
	}

	// Set connection pool parameters
	dbCfg.MaxConns = int32(a.configs.Database.MaxConns.Value) //nolint:gosec // bounded by DatabaseConfig.Validate
	dbCfg.MinConns = int32(a.configs.Database.MinConns.Value) //nolint:gosec // bounded by DatabaseConfig.Validate
	dbCfg.MaxConnLifetime = a.configs.Database.ConnMaxLifetime.Value
	dbCfg.MaxConnIdleTime = a.configs.Database.ConnMaxIdleTime.Value

	slog.Info("initializing database connection pool")

	// One pool. The service used to build two -- one to run migrations, then a
	// second whose AfterConnect registered the pgvector types, because those
	// types cannot be registered until the extension the migrations create
	// exists. With no vector column left in the schema there is nothing to
	// register, so the second pool bought nothing but a reconnect.
	a.dbPool, err = pgxpool.NewWithConfig(ctx, dbCfg)
	if err != nil {
		return fmt.Errorf("database connection error: %w", err)
	}

	slog.Debug("database connection pool established",
		"kind", a.configs.Database.Kind.Value,
		"address", a.configs.Database.Address.Value,
		"port", a.configs.Database.Port.Value,
		"username", a.configs.Database.Username.Value,
		"database", a.configs.Database.Name.Value,
		"ssl_mode", a.configs.Database.SSLMode.Value,
		"max_conns", a.configs.Database.MaxConns.Value,
		"min_conns", a.configs.Database.MinConns.Value,
		"conn_max_lifetime", a.configs.Database.ConnMaxLifetime.Value,
		"conn_max_idle_time", a.configs.Database.ConnMaxIdleTime.Value,
	)

	// Test database connection
	dbPingCtx, cancel := context.WithTimeout(ctx, a.configs.Database.MaxPingTimeout.Value)
	defer cancel()

	slog.Info("testing database connection")

	if err := a.dbPool.Ping(dbPingCtx); err != nil {
		return fmt.Errorf("database ping error: %w", err)
	}

	slog.Info("database connection test successful")

	// Run migrations if enabled
	if a.configs.Database.MigrationEnable.Value {
		slog.Info("running database migrations")

		db := stdlib.OpenDBFromPool(a.dbPool)
		if err := database.Migrate(ctx, "pgx", db); err != nil {
			return fmt.Errorf("database migration error: %w", err)
		}

		slog.Info("database migrations completed successfully")
	}

	slog.Info("database initialized successfully")

	return nil
}
