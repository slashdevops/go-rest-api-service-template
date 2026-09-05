package domain

import "time"

const (
	ValidDatabaseKind           = "pgxpool"
	ValidDatabaseSSLModes       = "disable|allow|prefer|require|verify-ca|verify-full"
	ValidDatabaseMaxPort        = 65535
	ValidDatabaseMinPort        = 0
	ValidDatabaseUsernameMaxLen = 32
	ValidDatabaseUsernameMinLen = 2
	ValidDatabasePasswordMaxLen = 128
	ValidDatabasePasswordMinLen = 2
	ValidDatabaseNameMaxLen     = 32
	ValidDatabaseNameMinLen     = 2
	ValidDatabaseTimeZoneMaxLen = 32
	ValidDatabaseTimeZoneMinLen = 2

	ValidDatabaseMaxMaxConns = 200
	ValidDatabaseMinMaxConns = 10
	ValidDatabaseMaxMinConns = 10
	ValidDatabaseMinMinConns = 0

	ValidDatabaseMaxPingTimeout = 30 * time.Second
	ValidDatabaseMinPingTimeout = 1 * time.Second

	ValidDatabaseMaxQueryTimeout = 30 * time.Second
	ValidDatabaseMinQueryTimeout = 1 * time.Second

	ValidDatabaseConnMaxIdleTime = 8 * time.Hour
	ValidDatabaseConnMinIdleTime = 1 * time.Minute

	ValidDatabaseConnMaxLifetime = 8 * time.Hour
	ValidDatabaseConnMinLifetime = 1 * time.Minute
)
