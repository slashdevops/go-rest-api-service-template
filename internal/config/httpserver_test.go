package config

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestNewHTTPServerConfig(t *testing.T) {
	config := NewHTTPServerConfig()

	if config.Address.Value != DefaultHTTPServerAddress {
		t.Errorf("Expected Address to be %s, got %s", DefaultHTTPServerAddress, config.Address.Value)
	}
	if config.Port.Value != DefaultHTTPServerPort {
		t.Errorf("Expected Port to be %d, got %d", DefaultHTTPServerPort, config.Port.Value)
	}
	if config.ShutdownTimeout.Value != DefaultHTTPServerShutdownTimeout {
		t.Errorf("Expected ShutdownTimeout to be %v, got %v", DefaultHTTPServerShutdownTimeout, config.ShutdownTimeout.Value)
	}
	if config.TLSEnabled.Value != DefaultHTTPServerTLSEnabled {
		t.Errorf("Expected TLSEnabled to be %v, got %v", DefaultHTTPServerTLSEnabled, config.TLSEnabled.Value)
	}
	if config.PprofEnabled.Value != DefaultHTTPServerPprofEnabled {
		t.Errorf("Expected PprofEnabled to be %v, got %v", DefaultHTTPServerPprofEnabled, config.PprofEnabled.Value)
	}
	if config.CorsEnabled.Value != DefaultHTTPServerCorsEnabled {
		t.Errorf("Expected CorsEnabled to be %v, got %v", DefaultHTTPServerCorsEnabled, config.CorsEnabled.Value)
	}
	if config.CorsAllowCredentials.Value != DefaultHTTPServerCorsAllowCredentials {
		t.Errorf("Expected CorsAllowCredentials to be %v, got %v", DefaultHTTPServerCorsAllowCredentials, config.CorsAllowCredentials.Value)
	}
	if config.CorsAllowedOrigins.Value != DefaultHTTPServerCorsAllowedOrigins {
		t.Errorf("Expected CorsAllowedOrigins to be %s, got %s", DefaultHTTPServerCorsAllowedOrigins, config.CorsAllowedOrigins.Value)
	}
	if config.CorsAllowedMethods.Value != DefaultHTTPServerCorsAllowedMethods {
		t.Errorf("Expected CorsAllowedMethods to be %s, got %s", DefaultHTTPServerCorsAllowedMethods, config.CorsAllowedMethods.Value)
	}
	if config.CorsAllowedHeaders.Value != DefaultHTTPServerCorsAllowedHeaders {
		t.Errorf("Expected CorsAllowedHeaders to be %s, got %s", DefaultHTTPServerCorsAllowedHeaders, config.CorsAllowedHeaders.Value)
	}
}

func TestParseEnvVars_httpserver(t *testing.T) {
	os.Setenv("HTTP_SERVER_ADDRESS", "127.0.0.1")
	os.Setenv("HTTP_SERVER_PORT", "9090")
	os.Setenv("HTTP_SERVER_SHUTDOWN_TIMEOUT", "10s")
	os.Setenv("HTTP_SERVER_TLS_ENABLED", "true")
	os.Setenv("HTTP_SERVER_PPROF_ENABLED", "true")
	os.Setenv("HTTP_SERVER_CORS_ENABLED", "true")
	os.Setenv("HTTP_SERVER_CORS_ALLOW_CREDENTIALS", "false")
	os.Setenv("HTTP_SERVER_CORS_ALLOWED_ORIGINS", "http://example.com")
	os.Setenv("HTTP_SERVER_CORS_ALLOWED_METHODS", "GET,POST")
	os.Setenv("HTTP_SERVER_CORS_ALLOWED_HEADERS", "Content-Type,Authorization")

	config := NewHTTPServerConfig()
	config.ParseEnvVars()

	if config.Address.Value != "127.0.0.1" {
		t.Errorf("Expected Address to be 127.0.0.1, got %s", config.Address.Value)
	}
	if config.Port.Value != 9090 {
		t.Errorf("Expected Port to be 9090, got %d", config.Port.Value)
	}
	if config.ShutdownTimeout.Value != 10*time.Second {
		t.Errorf("Expected ShutdownTimeout to be 10s, got %v", config.ShutdownTimeout.Value)
	}
	if config.TLSEnabled.Value != true {
		t.Errorf("Expected TLSEnabled to be true, got %v", config.TLSEnabled.Value)
	}
	if config.PprofEnabled.Value != true {
		t.Errorf("Expected PprofEnabled to be true, got %v", config.PprofEnabled.Value)
	}
	if config.CorsEnabled.Value != true {
		t.Errorf("Expected CorsEnabled to be true, got %v", config.CorsEnabled.Value)
	}
	if config.CorsAllowCredentials.Value != false {
		t.Errorf("Expected CorsAllowCredentials to be false, got %v", config.CorsAllowCredentials.Value)
	}
	if config.CorsAllowedOrigins.Value != "http://example.com" {
		t.Errorf("Expected CorsAllowedOrigins to be http://example.com, got %s", config.CorsAllowedOrigins.Value)
	}
	if config.CorsAllowedMethods.Value != "GET,POST" {
		t.Errorf("Expected CorsAllowedMethods to be GET,POST, got %s", config.CorsAllowedMethods.Value)
	}
	if config.CorsAllowedHeaders.Value != "Content-Type,Authorization" {
		t.Errorf("Expected CorsAllowedHeaders to be Content-Type,Authorization, got %s", config.CorsAllowedHeaders.Value)
	}

	// Clean up environment variables
	os.Unsetenv("HTTP_SERVER_ADDRESS")
	os.Unsetenv("HTTP_SERVER_PORT")
	os.Unsetenv("HTTP_SERVER_SHUTDOWN_TIMEOUT")
	os.Unsetenv("HTTP_SERVER_TLS_ENABLED")
	os.Unsetenv("HTTP_SERVER_PPROF_ENABLED")
	os.Unsetenv("HTTP_SERVER_CORS_ENABLED")
	os.Unsetenv("HTTP_SERVER_CORS_ALLOW_CREDENTIALS")
	os.Unsetenv("HTTP_SERVER_CORS_ALLOWED_ORIGINS")
	os.Unsetenv("HTTP_SERVER_CORS_ALLOWED_METHODS")
	os.Unsetenv("HTTP_SERVER_CORS_ALLOWED_HEADERS")
}

func TestValidate_httpserver(t *testing.T) {
	config := NewHTTPServerConfig()

	// Test valid configuration
	err := config.Validate()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test invalid Address
	config.Address.Value = ""
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "http.server.address" {
		t.Errorf("Expected InvalidConfigurationError with field 'http.server.address', got %v", err)
	}
	config.Address.Value = DefaultHTTPServerAddress

	// Test invalid Port
	config.Port.Value = -1
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "http.server.port" {
		t.Errorf("Expected InvalidConfigurationError with field 'http.server.port', got %v", err)
	}
	config.Port.Value = DefaultHTTPServerPort

	// Test invalid ShutdownTimeout
	config.ShutdownTimeout.Value = 0
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "http.server.shutdown.timeout" {
		t.Errorf("Expected InvalidConfigurationError with field 'http.server.shutdown.timeout', got %v", err)
	}
	config.ShutdownTimeout.Value = DefaultHTTPServerShutdownTimeout

	// Test invalid CORS configuration
	config.CorsEnabled.Value = true
	config.CorsAllowedOrigins.Value = ""
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "http.server.cors.allowed.origins" {
		t.Errorf("Expected InvalidConfigurationError with field 'http.server.cors.allowed.origins', got %v", err)
	}
	config.CorsAllowedOrigins.Value = DefaultHTTPServerCorsAllowedOrigins

	config.CorsAllowedMethods.Value = "INVALID"
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "http.server.cors.allowed.methods" {
		t.Errorf("Expected InvalidConfigurationError with field 'http.server.cors.allowed.methods', got %v", err)
	}
	config.CorsAllowedMethods.Value = DefaultHTTPServerCorsAllowedMethods

	config.CorsAllowedHeaders.Value = "A"
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "http.server.cors.allowed.headers" {
		t.Errorf("Expected InvalidConfigurationError with field 'http.server.cors.allowed.headers', got %v", err)
	}
	config.CorsAllowedHeaders.Value = DefaultHTTPServerCorsAllowedHeaders
}

func TestHTTPServerTimeoutDefaults(t *testing.T) {
	config := NewHTTPServerConfig()

	if config.ReadHeaderTimeout.Value != DefaultHTTPServerReadHeaderTimeout {
		t.Errorf("Expected ReadHeaderTimeout to be %v, got %v", DefaultHTTPServerReadHeaderTimeout, config.ReadHeaderTimeout.Value)
	}
	if config.IdleTimeout.Value != DefaultHTTPServerIdleTimeout {
		t.Errorf("Expected IdleTimeout to be %v, got %v", DefaultHTTPServerIdleTimeout, config.IdleTimeout.Value)
	}
	if config.MaxHeaderBytes.Value != DefaultHTTPServerMaxHeaderBytes {
		t.Errorf("Expected MaxHeaderBytes to be %d, got %d", DefaultHTTPServerMaxHeaderBytes, config.MaxHeaderBytes.Value)
	}

	// These two are 0 on purpose, not by omission. ReadTimeout would bound a
	// bulk ingest upload and WriteTimeout would cap total request duration,
	// aborting a retried generation. If a change flips either default on, it
	// should have to change this test and explain itself.
	if config.ReadTimeout.Value != 0 {
		t.Errorf("Expected ReadTimeout to default to 0 (disabled), got %v", config.ReadTimeout.Value)
	}
	if config.WriteTimeout.Value != 0 {
		t.Errorf("Expected WriteTimeout to default to 0 (disabled), got %v", config.WriteTimeout.Value)
	}
}

func TestHTTPServerTimeoutEnvVars(t *testing.T) {
	t.Setenv("HTTP_SERVER_READ_HEADER_TIMEOUT", "7s")
	t.Setenv("HTTP_SERVER_READ_TIMEOUT", "30s")
	t.Setenv("HTTP_SERVER_WRITE_TIMEOUT", "45s")
	t.Setenv("HTTP_SERVER_IDLE_TIMEOUT", "90s")
	t.Setenv("HTTP_SERVER_MAX_HEADER_BYTES", "65536")

	config := NewHTTPServerConfig()
	config.ParseEnvVars()

	if config.ReadHeaderTimeout.Value != 7*time.Second {
		t.Errorf("Expected ReadHeaderTimeout to be 7s, got %v", config.ReadHeaderTimeout.Value)
	}
	if config.ReadTimeout.Value != 30*time.Second {
		t.Errorf("Expected ReadTimeout to be 30s, got %v", config.ReadTimeout.Value)
	}
	if config.WriteTimeout.Value != 45*time.Second {
		t.Errorf("Expected WriteTimeout to be 45s, got %v", config.WriteTimeout.Value)
	}
	if config.IdleTimeout.Value != 90*time.Second {
		t.Errorf("Expected IdleTimeout to be 90s, got %v", config.IdleTimeout.Value)
	}
	if config.MaxHeaderBytes.Value != 65536 {
		t.Errorf("Expected MaxHeaderBytes to be 65536, got %d", config.MaxHeaderBytes.Value)
	}
}

func TestHTTPServerTimeoutValidation(t *testing.T) {
	expectInvalid := func(t *testing.T, c *HTTPServerConfig, field string) {
		t.Helper()

		err := c.Validate()
		if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != field {
			t.Errorf("Expected InvalidConfigurationError with field %q, got %v", field, err)
		}
	}

	t.Run("read_header_timeout_cannot_be_disabled", func(t *testing.T) {
		c := NewHTTPServerConfig()
		c.ReadHeaderTimeout.Value = 0
		expectInvalid(t, c, "http.server.read.header.timeout")
	})

	t.Run("read_header_timeout_above_max", func(t *testing.T) {
		c := NewHTTPServerConfig()
		c.ReadHeaderTimeout.Value = ValidHTTPServerMaxReadHeaderTimeout + time.Second
		expectInvalid(t, c, "http.server.read.header.timeout")
	})

	t.Run("zero_read_and_write_timeouts_are_valid", func(t *testing.T) {
		c := NewHTTPServerConfig()
		c.ReadTimeout.Value = 0
		c.WriteTimeout.Value = 0

		if err := c.Validate(); err != nil {
			t.Errorf("Expected 0 to be accepted as disabled, got %v", err)
		}
	})

	t.Run("read_timeout_above_max", func(t *testing.T) {
		c := NewHTTPServerConfig()
		c.ReadTimeout.Value = ValidHTTPServerMaxReadTimeout + time.Second
		expectInvalid(t, c, "http.server.read.timeout")
	})

	t.Run("read_timeout_below_read_header_timeout", func(t *testing.T) {
		c := NewHTTPServerConfig()
		c.ReadHeaderTimeout.Value = 10 * time.Second
		c.ReadTimeout.Value = 5 * time.Second
		expectInvalid(t, c, "http.server.read.timeout")
	})

	t.Run("write_timeout_above_max", func(t *testing.T) {
		c := NewHTTPServerConfig()
		c.WriteTimeout.Value = ValidHTTPServerMaxWriteTimeout + time.Second
		expectInvalid(t, c, "http.server.write.timeout")
	})

	t.Run("idle_timeout_cannot_be_disabled", func(t *testing.T) {
		c := NewHTTPServerConfig()
		c.IdleTimeout.Value = 0
		expectInvalid(t, c, "http.server.idle.timeout")
	})

	t.Run("max_header_bytes_below_min", func(t *testing.T) {
		c := NewHTTPServerConfig()
		c.MaxHeaderBytes.Value = ValidHTTPServerMinMaxHeaderBytes - 1
		expectInvalid(t, c, "http.server.max.header.bytes")
	})

	t.Run("defaults_are_valid", func(t *testing.T) {
		if err := NewHTTPServerConfig().Validate(); err != nil {
			t.Errorf("Expected the default configuration to validate, got %v", err)
		}
	})
}
