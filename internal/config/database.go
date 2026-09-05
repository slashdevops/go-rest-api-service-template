package config

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

const (
	DefaultDatabaseKind     = "pgxpool"
	DefaultDatabaseAddress  = "localhost"
	DefaultDatabasePort     = 5432
	DefaultDatabaseUsername = "username"
	DefaultDatabasePassword = "password"
	DefaultDatabaseName     = "go-rest-api-service-template"
	// DefaultDatabaseSSLMode stays "disable" so the shipped default keeps
	// working against a server that does not speak TLS — including this repo's
	// dev environment. The service warns at startup when the connection is
	// unencrypted or downgradeable; see docs/certificates/postgres-tls.md for
	// what each mode actually buys.
	DefaultDatabaseSSLMode         = "disable"
	DefaultDatabaseTimeZone        = "UTC"
	DefaultDatabaseMaxPingTimeout  = 5 * time.Second
	DefaultDatabaseMaxQueryTimeout = 5 * time.Second
	DefaultDatabaseMaxConns        = 20
	DefaultDatabaseMinConns        = 5
	DefaultDatabaseConnMaxIdleTime = 30 * time.Minute
	DefaultDatabaseConnMaxLifetime = 5 * time.Minute
	DefaultDatabaseMigrationEnable = true
)

type DatabaseConfig struct {
	Kind     Field[string]
	Address  Field[string]
	Username Field[string]
	Password Field[string]
	Name     Field[string]
	SSLMode  Field[string]

	// Certificate paths for the verifying SSL modes. Without them verify-ca and
	// verify-full can only validate against the host trust store, which a
	// privately signed server will never match, and mutual TLS is impossible.
	SSLRootCertFile Field[string]
	SSLCertFile     Field[string]
	SSLKeyFile      Field[string]
	Port            Field[int]
	TimeZone        Field[string]

	MaxConns Field[int]
	MinConns Field[int]

	MaxQueryTimeout Field[time.Duration]
	MaxPingTimeout  Field[time.Duration]

	ConnMaxIdleTime Field[time.Duration]
	ConnMaxLifetime Field[time.Duration]

	MigrationEnable Field[bool]
}

func NewDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{
		Kind:            NewField("database.kind", "DATABASE_KIND", "Database Kind. Possible values ["+domain.ValidDatabaseKind+"]", DefaultDatabaseKind),
		Address:         NewField("database.address", "DATABASE_ADDRESS", "Database IP Address or Hostname", DefaultDatabaseAddress),
		Port:            NewField("database.port", "DATABASE_PORT", "Database Port", DefaultDatabasePort),
		Username:        NewField("database.username", "DATABASE_USERNAME", "Database Username", DefaultDatabaseUsername),
		Password:        NewField("database.password", "DATABASE_PASSWORD", "Database Password", DefaultDatabasePassword),
		Name:            NewField("database.name", "DATABASE_NAME", "Database Name", DefaultDatabaseName),
		SSLMode:         NewField("database.ssl.mode", "DATABASE_SSL_MODE", "Database SSL Mode. Possible values ["+domain.ValidDatabaseSSLModes+"]", DefaultDatabaseSSLMode),
		SSLRootCertFile: NewField("database.ssl.root.cert.file", "DATABASE_SSL_ROOT_CERT_FILE", "PEM file holding the CA that signed the database server certificate. Required by verify-ca and verify-full unless the server certificate is publicly trusted", ""),
		SSLCertFile:     NewField("database.ssl.cert.file", "DATABASE_SSL_CERT_FILE", "Client certificate for mutual TLS to the database. Requires database.ssl.key.file", ""),
		SSLKeyFile:      NewField("database.ssl.key.file", "DATABASE_SSL_KEY_FILE", "Private key for database.ssl.cert.file", ""),
		TimeZone:        NewField("database.time.zone", "DATABASE_TIME_ZONE", "Database Time Zone", DefaultDatabaseTimeZone),

		MaxPingTimeout:  NewField("database.max.ping.timeout", "DATABASE_MAX_PING_TIMEOUT", "Database Max Ping Timeout", DefaultDatabaseMaxPingTimeout),
		MaxQueryTimeout: NewField("database.max.query.timeout", "DATABASE_MAX_QUERY_TIMEOUT", "Database Max Query Timeout", DefaultDatabaseMaxQueryTimeout),

		MaxConns: NewField("database.max.conns", "DATABASE_MAX_CONNS", "Database Max Idle Connections", DefaultDatabaseMaxConns),
		MinConns: NewField("database.min.conns", "DATABASE_MIN_CONNS", "Database Max Open Connections", DefaultDatabaseMinConns),

		ConnMaxIdleTime: NewField("database.conn.max.idle.time", "DATABASE_CONN_MAX_IDLE_TIME", "Database Connection Max Idle Time", DefaultDatabaseConnMaxIdleTime),
		ConnMaxLifetime: NewField("database.conn.max.lifetime", "DATABASE_CONN_MAX_LIFETIME", "Database Connection Max Lifetime", DefaultDatabaseConnMaxLifetime),

		MigrationEnable: NewField("database.migration.enabled", "DATABASE_MIGRATION_ENABLED", "Database migration is enables?", DefaultDatabaseMigrationEnable),
	}
}

// ParseEnvVars reads the database configuration from environment variables
// and sets the values in the configuration
func (c *DatabaseConfig) ParseEnvVars() {
	c.Kind.Value = GetEnv(c.Kind.EnVarName, c.Kind.Value)
	c.Address.Value = GetEnv(c.Address.EnVarName, c.Address.Value)
	c.Port.Value = GetEnv(c.Port.EnVarName, c.Port.Value)
	c.Username.Value = GetEnv(c.Username.EnVarName, c.Username.Value)
	c.Password.Value = GetEnv(c.Password.EnVarName, c.Password.Value)
	c.Name.Value = GetEnv(c.Name.EnVarName, c.Name.Value)
	c.SSLMode.Value = GetEnv(c.SSLMode.EnVarName, c.SSLMode.Value)
	c.SSLRootCertFile.Value = GetEnv(c.SSLRootCertFile.EnVarName, c.SSLRootCertFile.Value)
	c.SSLCertFile.Value = GetEnv(c.SSLCertFile.EnVarName, c.SSLCertFile.Value)
	c.SSLKeyFile.Value = GetEnv(c.SSLKeyFile.EnVarName, c.SSLKeyFile.Value)
	c.TimeZone.Value = GetEnv(c.TimeZone.EnVarName, c.TimeZone.Value)

	c.MaxPingTimeout.Value = GetEnv(c.MaxPingTimeout.EnVarName, c.MaxPingTimeout.Value)
	c.MaxQueryTimeout.Value = GetEnv(c.MaxQueryTimeout.EnVarName, c.MaxQueryTimeout.Value)

	c.MaxConns.Value = GetEnv(c.MaxConns.EnVarName, c.MaxConns.Value)
	c.MinConns.Value = GetEnv(c.MinConns.EnVarName, c.MinConns.Value)

	c.ConnMaxIdleTime.Value = GetEnv(c.ConnMaxIdleTime.EnVarName, c.ConnMaxIdleTime.Value)
	c.ConnMaxLifetime.Value = GetEnv(c.ConnMaxLifetime.EnVarName, c.ConnMaxLifetime.Value)

	c.MigrationEnable.Value = GetEnv(c.MigrationEnable.EnVarName, c.MigrationEnable.Value)
}

// Validate validates the database configuration values
func (c *DatabaseConfig) Validate() error {
	if !slices.Contains(strings.Split(domain.ValidDatabaseKind, "|"), c.Kind.Value) {
		return &InvalidConfigurationError{
			Field:   "database.kind",
			Value:   c.Kind.Value,
			Message: fmt.Sprintf("invalid database kind, must be one of: %s", domain.ValidDatabaseKind),
		}
	}

	if c.Port.Value <= domain.ValidDatabaseMinPort || c.Port.Value >= domain.ValidDatabaseMaxPort {
		return &InvalidConfigurationError{
			Field:   "database.port",
			Value:   fmt.Sprintf("%d", c.Port.Value),
			Message: fmt.Sprintf("invalid database port, must be between %d and %d", domain.ValidDatabaseMinPort, domain.ValidDatabaseMaxPort),
		}
	}

	if c.Username.Value == "" || len(c.Username.Value) < domain.ValidDatabaseUsernameMinLen || len(c.Username.Value) > domain.ValidDatabaseUsernameMaxLen {
		return &InvalidConfigurationError{
			Field:   "database.username",
			Value:   c.Username.Value,
			Message: fmt.Sprintf("invalid database username, must be between %d and %d characters", domain.ValidDatabaseUsernameMinLen, domain.ValidDatabaseUsernameMaxLen),
		}
	}

	if c.Password.Value == "" || len(c.Password.Value) < domain.ValidDatabasePasswordMinLen || len(c.Password.Value) > domain.ValidDatabasePasswordMaxLen {
		return &InvalidConfigurationError{
			Field:   "database.password",
			Value:   c.Password.Value,
			Message: fmt.Sprintf("invalid database password, must be between %d and %d characters", domain.ValidDatabasePasswordMinLen, domain.ValidDatabasePasswordMaxLen),
		}
	}

	if c.Name.Value == "" || len(c.Name.Value) < domain.ValidDatabaseNameMinLen || len(c.Name.Value) > domain.ValidDatabaseNameMaxLen {
		return &InvalidConfigurationError{
			Field:   "database.name",
			Value:   c.Name.Value,
			Message: fmt.Sprintf("invalid database name, must be between %d and %d characters", domain.ValidDatabaseNameMinLen, domain.ValidDatabaseNameMaxLen),
		}
	}

	// A certificate without its key cannot form a keypair; pgx would fail at
	// connection time with a message about TLS rather than about configuration.
	if (c.SSLCertFile.Value == "") != (c.SSLKeyFile.Value == "") {
		return &InvalidConfigurationError{
			Field:   "database.ssl.cert.file",
			Value:   c.SSLCertFile.Value,
			Message: "database.ssl.cert.file and database.ssl.key.file must be set together",
		}
	}

	// Certificate paths with SSL off are the same trap as the cache: they read
	// as though the connection is protected while it is still cleartext.
	if c.SSLMode.Value == "disable" {
		for field, value := range map[string]string{
			"database.ssl.root.cert.file": c.SSLRootCertFile.Value,
			"database.ssl.cert.file":      c.SSLCertFile.Value,
			"database.ssl.key.file":       c.SSLKeyFile.Value,
		} {
			if value != "" {
				return &InvalidConfigurationError{
					Field:   field,
					Value:   value,
					Message: "SSL files are configured but database.ssl.mode is disable; the connection would still be cleartext",
				}
			}
		}
	}

	for field, value := range map[string]string{
		"database.ssl.root.cert.file": c.SSLRootCertFile.Value,
		"database.ssl.cert.file":      c.SSLCertFile.Value,
		"database.ssl.key.file":       c.SSLKeyFile.Value,
	} {
		if value == "" {
			continue
		}

		if _, err := os.Stat(value); err != nil {
			return &InvalidConfigurationError{
				Field:   field,
				Value:   value,
				Message: "SSL file cannot be read: " + err.Error(),
			}
		}
	}

	if !slices.Contains(strings.Split(domain.ValidDatabaseSSLModes, "|"), c.SSLMode.Value) {
		return &InvalidConfigurationError{
			Field:   "database.sslmode",
			Value:   c.SSLMode.Value,
			Message: fmt.Sprintf("invalid database SSL mode, must be one of: %s", domain.ValidDatabaseSSLModes),
		}
	}

	if c.TimeZone.Value == "" || len(c.TimeZone.Value) < domain.ValidDatabaseTimeZoneMinLen || len(c.TimeZone.Value) > domain.ValidDatabaseTimeZoneMaxLen {
		return &InvalidConfigurationError{
			Field:   "database.timezone",
			Value:   c.TimeZone.Value,
			Message: fmt.Sprintf("invalid database timezone, must be between %d and %d characters", domain.ValidDatabaseTimeZoneMinLen, domain.ValidDatabaseTimeZoneMaxLen),
		}
	}

	if c.MaxConns.Value < domain.ValidDatabaseMinMaxConns || c.MaxConns.Value > domain.ValidDatabaseMaxMaxConns {
		return &InvalidConfigurationError{
			Field:   "database.max_conns",
			Value:   fmt.Sprintf("%d", c.MaxConns.Value),
			Message: fmt.Sprintf("invalid database max connections, must be between %d and %d", domain.ValidDatabaseMinMaxConns, domain.ValidDatabaseMaxMaxConns),
		}
	}

	if c.MinConns.Value < domain.ValidDatabaseMinMinConns || c.MinConns.Value > domain.ValidDatabaseMaxMinConns {
		return &InvalidConfigurationError{
			Field:   "database.min_conns",
			Value:   fmt.Sprintf("%d", c.MinConns.Value),
			Message: fmt.Sprintf("invalid database min connections, must be between %d and %d", domain.ValidDatabaseMinMinConns, domain.ValidDatabaseMaxMinConns),
		}
	}

	if c.MaxPingTimeout.Value < domain.ValidDatabaseMinPingTimeout || c.MaxPingTimeout.Value > domain.ValidDatabaseMaxPingTimeout {
		return &InvalidConfigurationError{
			Field:   "database.max_ping_timeout",
			Value:   fmt.Sprintf("%d", c.MaxPingTimeout.Value),
			Message: fmt.Sprintf("invalid database max ping timeout, must be between %d and %d", domain.ValidDatabaseMinPingTimeout, domain.ValidDatabaseMaxPingTimeout),
		}
	}

	if c.MaxQueryTimeout.Value < domain.ValidDatabaseMinQueryTimeout || c.MaxQueryTimeout.Value > domain.ValidDatabaseMaxQueryTimeout {
		return &InvalidConfigurationError{
			Field:   "database.max_query_timeout",
			Value:   fmt.Sprintf("%d", c.MaxQueryTimeout.Value),
			Message: fmt.Sprintf("invalid database max query timeout, must be between %d and %d", domain.ValidDatabaseMinQueryTimeout, domain.ValidDatabaseMaxQueryTimeout),
		}
	}

	if c.ConnMaxIdleTime.Value < domain.ValidDatabaseConnMinIdleTime || c.ConnMaxIdleTime.Value > domain.ValidDatabaseConnMaxIdleTime {
		return &InvalidConfigurationError{
			Field:   "database.conn_max_idle_time",
			Value:   fmt.Sprintf("%d", c.ConnMaxIdleTime.Value),
			Message: fmt.Sprintf("invalid database max idle time, must be between %d and %d", domain.ValidDatabaseConnMinIdleTime, domain.ValidDatabaseConnMaxIdleTime),
		}
	}

	if c.ConnMaxLifetime.Value < domain.ValidDatabaseConnMinLifetime || c.ConnMaxLifetime.Value > domain.ValidDatabaseConnMaxLifetime {
		return &InvalidConfigurationError{
			Field:   "database.conn_max_lifetime",
			Value:   fmt.Sprintf("%d", c.ConnMaxLifetime.Value),
			Message: fmt.Sprintf("invalid database max connection lifetime, must be between %d and %d", domain.ValidDatabaseConnMinLifetime, domain.ValidDatabaseConnMaxLifetime),
		}
	}

	return nil
}
