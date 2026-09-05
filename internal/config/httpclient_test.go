package config

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestNewHTTPClientConfig(t *testing.T) {
	config := NewHTTPClientConfig()

	if config.MaxIdleConns.Value != DefaultHTTPClientMaxIdleConns {
		t.Errorf("Expected MaxIdleConns to be %d, got %d", DefaultHTTPClientMaxIdleConns, config.MaxIdleConns.Value)
	}
	if config.MaxIdleConnsPerHost.Value != DefaultHTTPClientMaxIdleConns {
		t.Errorf("Expected MaxIdleConnsPerHost to be %d, got %d", DefaultHTTPClientMaxIdleConns, config.MaxIdleConnsPerHost.Value)
	}
	if config.IdleConnTimeout.Value != DefaultHTTPClientIdleConnTimeout {
		t.Errorf("Expected IdleConnTimeout to be %v, got %v", DefaultHTTPClientIdleConnTimeout, config.IdleConnTimeout.Value)
	}
	if config.TLSHandshakeTimeout.Value != DefaultHTTPClientTLSHandshakeTimeout {
		t.Errorf("Expected TLSHandshakeTimeout to be %v, got %v", DefaultHTTPClientTLSHandshakeTimeout, config.TLSHandshakeTimeout.Value)
	}
	if config.ExpectContinueTimeout.Value != DefaultHTTPClientExpectContinueTimeout {
		t.Errorf("Expected ExpectContinueTimeout to be %v, got %v", DefaultHTTPClientExpectContinueTimeout, config.ExpectContinueTimeout.Value)
	}
	if config.DisableKeepAlives.Value != DefaultHTTPClientDisableKeepAlives {
		t.Errorf("Expected DisableKeepAlives to be %v, got %v", DefaultHTTPClientDisableKeepAlives, config.DisableKeepAlives.Value)
	}
	if config.Timeout.Value != DefaultHTTPClientTimeout {
		t.Errorf("Expected Timeout to be %v, got %v", DefaultHTTPClientTimeout, config.Timeout.Value)
	}
	if config.MaxRetries.Value != DefaultHTTPClientMaxRetries {
		t.Errorf("Expected MaxRetries to be %d, got %d", DefaultHTTPClientMaxRetries, config.MaxRetries.Value)
	}
	if config.RetryStrategy.Value != DefaultHTTPClientRetryStrategy {
		t.Errorf("Expected RetryStrategy to be %s, got %s", DefaultHTTPClientRetryStrategy, config.RetryStrategy.Value)
	}
}

func TestParseEnvVars_httpclient(t *testing.T) {
	os.Setenv("HTTP_CLIENT_MAX_IDLE_CONNS", "50")
	os.Setenv("HTTP_CLIENT_MAX_IDLE_CONNS_PER_HOST", "25")
	os.Setenv("HTTP_CLIENT_IDLE_CONN_TIMEOUT", "60s")
	os.Setenv("HTTP_CLIENT_TLS_HANDSHAKE_TIMEOUT", "5s")
	os.Setenv("HTTP_CLIENT_EXPECT_CONTINUE_TIMEOUT", "2s")
	os.Setenv("HTTP_CLIENT_DISABLE_KEEP_ALIVES", "true")
	os.Setenv("HTTP_CLIENT_TIMEOUT", "10s")
	os.Setenv("HTTP_CLIENT_MAX_RETRIES", "5")
	os.Setenv("HTTP_CLIENT_RETRY_STRATEGY", "exponential")

	config := NewHTTPClientConfig()
	config.ParseEnvVars()

	if config.MaxIdleConns.Value != 50 {
		t.Errorf("Expected MaxIdleConns to be 50, got %d", config.MaxIdleConns.Value)
	}
	if config.MaxIdleConnsPerHost.Value != 25 {
		t.Errorf("Expected MaxIdleConnsPerHost to be 25, got %d", config.MaxIdleConnsPerHost.Value)
	}
	if config.IdleConnTimeout.Value != 60*time.Second {
		t.Errorf("Expected IdleConnTimeout to be 60s, got %v", config.IdleConnTimeout.Value)
	}
	if config.TLSHandshakeTimeout.Value != 5*time.Second {
		t.Errorf("Expected TLSHandshakeTimeout to be 5s, got %v", config.TLSHandshakeTimeout.Value)
	}
	if config.ExpectContinueTimeout.Value != 2*time.Second {
		t.Errorf("Expected ExpectContinueTimeout to be 2s, got %v", config.ExpectContinueTimeout.Value)
	}
	if config.DisableKeepAlives.Value != true {
		t.Errorf("Expected DisableKeepAlives to be true, got %v", config.DisableKeepAlives.Value)
	}
	if config.Timeout.Value != 10*time.Second {
		t.Errorf("Expected Timeout to be 10s, got %v", config.Timeout.Value)
	}
	if config.MaxRetries.Value != 5 {
		t.Errorf("Expected MaxRetries to be 5, got %d", config.MaxRetries.Value)
	}
	if config.RetryStrategy.Value != "exponential" {
		t.Errorf("Expected RetryStrategy to be exponential, got %s", config.RetryStrategy.Value)
	}

	// Clean up environment variables
	os.Unsetenv("HTTP_CLIENT_MAX_IDLE_CONNS")
	os.Unsetenv("HTTP_CLIENT_MAX_IDLE_CONNS_PER_HOST")
	os.Unsetenv("HTTP_CLIENT_IDLE_CONN_TIMEOUT")
	os.Unsetenv("HTTP_CLIENT_TLS_HANDSHAKE_TIMEOUT")
	os.Unsetenv("HTTP_CLIENT_EXPECT_CONTINUE_TIMEOUT")
	os.Unsetenv("HTTP_CLIENT_DISABLE_KEEP_ALIVES")
	os.Unsetenv("HTTP_CLIENT_TIMEOUT")
	os.Unsetenv("HTTP_CLIENT_MAX_RETRIES")
	os.Unsetenv("HTTP_CLIENT_RETRY_STRATEGY")
}

func TestValidate_httpclient(t *testing.T) {
	config := NewHTTPClientConfig()

	// Test valid configuration
	err := config.Validate()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test invalid MaxIdleConns
	config.MaxIdleConns.Value = 0
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "http.client.max.idle.conns" {
		t.Errorf("Expected InvalidConfigurationError with field 'http.client.max.idle.conns', got %v", err)
	}
	config.MaxIdleConns.Value = DefaultHTTPClientMaxIdleConns

	// Test invalid MaxIdleConnsPerHost
	config.MaxIdleConnsPerHost.Value = 0
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "http.client.max.idle.conns.per.host" {
		t.Errorf("Expected InvalidConfigurationError with field 'http.client.max.idle.conns.per.host', got %v", err)
	}
	config.MaxIdleConnsPerHost.Value = DefaultHTTPClientMaxIdleConns

	// Test invalid IdleConnTimeout (too short)
	config.IdleConnTimeout.Value = 500 * time.Millisecond
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "http.client.idle.conn.timeout" {
		t.Errorf("Expected InvalidConfigurationError with field 'http.client.idle.conn.timeout', got %v", err)
	}
	config.IdleConnTimeout.Value = DefaultHTTPClientIdleConnTimeout

	// Test invalid TLSHandshakeTimeout (too short)
	config.TLSHandshakeTimeout.Value = 500 * time.Millisecond
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "http.client.tls.handshake.timeout" {
		t.Errorf("Expected InvalidConfigurationError with field 'http.client.tls.handshake.timeout', got %v", err)
	}
	config.TLSHandshakeTimeout.Value = DefaultHTTPClientTLSHandshakeTimeout

	// Test invalid ExpectContinueTimeout (too short)
	config.ExpectContinueTimeout.Value = 500 * time.Millisecond
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "http.client.expect.continue.timeout" {
		t.Errorf("Expected InvalidConfigurationError with field 'http.client.expect.continue.timeout', got %v", err)
	}
	config.ExpectContinueTimeout.Value = DefaultHTTPClientExpectContinueTimeout

	// Test invalid Timeout (too short)
	config.Timeout.Value = 500 * time.Millisecond
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "http.client.timeout" {
		t.Errorf("Expected InvalidConfigurationError with field 'http.client.timeout', got %v", err)
	}
	config.Timeout.Value = DefaultHTTPClientTimeout

	// Test invalid RetryStrategy
	config.RetryStrategy.Value = "invalid"
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "http.client.retry.strategy" {
		t.Errorf("Expected InvalidConfigurationError with field 'http.client.retry.strategy', got %v", err)
	}
	config.RetryStrategy.Value = DefaultHTTPClientRetryStrategy

	// Test invalid MaxRetries (too low)
	config.MaxRetries.Value = 0
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "http.client.max.retries" {
		t.Errorf("Expected InvalidConfigurationError with field 'http.client.max.retries', got %v", err)
	}
	config.MaxRetries.Value = DefaultHTTPClientMaxRetries
}
