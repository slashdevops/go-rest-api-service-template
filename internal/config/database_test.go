package config

import (
	"errors"
	"os"
	"testing"
)

func TestNewDatabaseConfig(t *testing.T) {
	config := NewDatabaseConfig()

	if config.Kind.Value != DefaultDatabaseKind {
		t.Errorf("Expected Kind to be %s, got %s", DefaultDatabaseKind, config.Kind.Value)
	}
	if config.Address.Value != DefaultDatabaseAddress {
		t.Errorf("Expected Address to be %s, got %s", DefaultDatabaseAddress, config.Address.Value)
	}
	if config.Port.Value != DefaultDatabasePort {
		t.Errorf("Expected Port to be %d, got %d", DefaultDatabasePort, config.Port.Value)
	}
	if config.Username.Value != DefaultDatabaseUsername {
		t.Errorf("Expected Username to be %s, got %s", DefaultDatabaseUsername, config.Username.Value)
	}
	if config.Password.Value != DefaultDatabasePassword {
		t.Errorf("Expected Password to be %s, got %s", DefaultDatabasePassword, config.Password.Value)
	}
	if config.Name.Value != DefaultDatabaseName {
		t.Errorf("Expected Name to be %s, got %s", DefaultDatabaseName, config.Name.Value)
	}
	if config.SSLMode.Value != DefaultDatabaseSSLMode {
		t.Errorf("Expected SSLMode to be %s, got %s", DefaultDatabaseSSLMode, config.SSLMode.Value)
	}
	if config.TimeZone.Value != DefaultDatabaseTimeZone {
		t.Errorf("Expected TimeZone to be %s, got %s", DefaultDatabaseTimeZone, config.TimeZone.Value)
	}
}

func TestParseEnvVars_database(t *testing.T) {
	os.Setenv("DATABASE_KIND", "postgres")
	os.Setenv("DATABASE_ADDRESS", "127.0.0.1")
	os.Setenv("DATABASE_PORT", "5433")
	os.Setenv("DATABASE_USERNAME", "testuser")
	os.Setenv("DATABASE_PASSWORD", "testpass")
	os.Setenv("DATABASE_NAME", "testdb")
	os.Setenv("DATABASE_SSL_MODE", "require")
	os.Setenv("DATABASE_TIME_ZONE", "PST")

	config := NewDatabaseConfig()
	config.ParseEnvVars()

	if config.Kind.Value != "postgres" {
		t.Errorf("Expected Kind to be postgres, got %s", config.Kind.Value)
	}
	if config.Address.Value != "127.0.0.1" {
		t.Errorf("Expected Address to be 127.0.0.1, got %s", config.Address.Value)
	}
	if config.Port.Value != 5433 {
		t.Errorf("Expected Port to be 5433, got %d", config.Port.Value)
	}
	if config.Username.Value != "testuser" {
		t.Errorf("Expected Username to be testuser, got %s", config.Username.Value)
	}
	if config.Password.Value != "testpass" {
		t.Errorf("Expected Password to be testpass, got %s", config.Password.Value)
	}
	if config.Name.Value != "testdb" {
		t.Errorf("Expected Name to be testdb, got %s", config.Name.Value)
	}
	if config.SSLMode.Value != "require" {
		t.Errorf("Expected SSLMode to be require, got %s", config.SSLMode.Value)
	}
	if config.TimeZone.Value != "PST" {
		t.Errorf("Expected TimeZone to be PST, got %s", config.TimeZone.Value)
	}

	// Clean up environment variables
	os.Unsetenv("DATABASE_KIND")
	os.Unsetenv("DATABASE_ADDRESS")
	os.Unsetenv("DATABASE_PORT")
	os.Unsetenv("DATABASE_USERNAME")
	os.Unsetenv("DATABASE_PASSWORD")
	os.Unsetenv("DATABASE_NAME")
	os.Unsetenv("DATABASE_SSL_MODE")
	os.Unsetenv("DATABASE_TIME_ZONE")
}

func TestValidate_database(t *testing.T) {
	config := NewDatabaseConfig()

	// Test valid configuration
	err := config.Validate()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test invalid Kind
	config.Kind.Value = "invalid"
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "database.kind" {
		t.Errorf("Expected InvalidConfigurationError with field 'database.kind', got %v", err)
	}
	config.Kind.Value = DefaultDatabaseKind

	// Test invalid Port
	config.Port.Value = -1
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "database.port" {
		t.Errorf("Expected InvalidConfigurationError with field 'database.port', got %v", err)
	}
	config.Port.Value = DefaultDatabasePort

	// Test invalid Username
	config.Username.Value = ""
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "database.username" {
		t.Errorf("Expected InvalidConfigurationError with field 'database.username', got %v", err)
	}
	config.Username.Value = DefaultDatabaseUsername

	// Test invalid Password
	config.Password.Value = ""
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "database.password" {
		t.Errorf("Expected InvalidConfigurationError with field 'database.password', got %v", err)
	}
	config.Password.Value = DefaultDatabasePassword

	// Test invalid Name
	config.Name.Value = ""
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "database.name" {
		t.Errorf("Expected InvalidConfigurationError with field 'database.name', got %v", err)
	}
	config.Name.Value = DefaultDatabaseName

	// Test invalid SSLMode
	config.SSLMode.Value = "invalid"
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "database.sslmode" {
		t.Errorf("Expected InvalidConfigurationError with field 'database.sslmode', got %v", err)
	}
	config.SSLMode.Value = DefaultDatabaseSSLMode

	// Test invalid TimeZone
	config.TimeZone.Value = ""
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "database.timezone" {
		t.Errorf("Expected InvalidConfigurationError with field 'database.timezone', got %v", err)
	}
	config.TimeZone.Value = DefaultDatabaseTimeZone

	// Test invalid MaxConns
	config.MaxConns.Value = -1
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "database.max_conns" {
		t.Errorf("Expected InvalidConfigurationError with field 'database.max_conns', got %v", err)
	}
	config.MaxConns.Value = DefaultDatabaseMaxConns

	// Test invalid MinConns
	config.MinConns.Value = -1
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "database.min_conns" {
		t.Errorf("Expected InvalidConfigurationError with field 'database.min_conns', got %v", err)
	}
	config.MinConns.Value = DefaultDatabaseMinConns

	// Test invalid MaxPingTimeout
	config.MaxPingTimeout.Value = 0
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "database.max_ping_timeout" {
		t.Errorf("Expected InvalidConfigurationError with field 'database.max_ping_timeout', got %v", err)
	}
	config.MaxPingTimeout.Value = DefaultDatabaseMaxPingTimeout

	// Test invalid MaxQueryTimeout
	config.MaxQueryTimeout.Value = 0
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "database.max_query_timeout" {
		t.Errorf("Expected InvalidConfigurationError with field 'database.max_query_timeout', got %v", err)
	}
	config.MaxQueryTimeout.Value = DefaultDatabaseMaxQueryTimeout

	// Test invalid ConnMaxIdleTime
	config.ConnMaxIdleTime.Value = 0
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "database.conn_max_idle_time" {
		t.Errorf("Expected InvalidConfigurationError with field 'database.conn_max_idle_time', got %v", err)
	}
	config.ConnMaxIdleTime.Value = DefaultDatabaseConnMaxIdleTime

	// Test invalid ConnMaxLifetime
	config.ConnMaxLifetime.Value = 0
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "database.conn_max_lifetime" {
		t.Errorf("Expected InvalidConfigurationError with field 'database.conn_max_lifetime', got %v", err)
	}
	config.ConnMaxLifetime.Value = DefaultDatabaseConnMaxLifetime
}
