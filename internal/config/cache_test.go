package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewCacheConfig(t *testing.T) {
	config := NewCacheConfig()

	if config.ServerKind.Value != DefaultCacheServerKind {
		t.Errorf("Expected ServerKind to be %s, got %s", DefaultCacheServerKind, config.ServerKind.Value)
	}
	if config.ServerAddresses.Value.String() != DefaultCacheServerAddresses.String() {
		t.Errorf("Expected Addresses to be %s, got %s", DefaultCacheServerAddresses.String(), config.ServerAddresses.Value.String())
	}
	if config.ServerUsername.Value != DefaultCacheServerUsername {
		t.Errorf("Expected Username to be %s, got %s", DefaultCacheServerUsername, config.ServerUsername.Value)
	}
	if config.ServerPassword.Value != DefaultCacheServerPassword {
		t.Errorf("Expected Password to be %s, got %s", DefaultCacheServerPassword, config.ServerPassword.Value)
	}
	if config.ServerDB.Value != DefaultCacheServerDB {
		t.Errorf("Expected DB to be %d, got %d", DefaultCacheServerDB, config.ServerDB.Value)
	}
	if config.MaxQueryTimeout.Value != DefaultCacheMaxQueryTimeout {
		t.Errorf("Expected MaxQueryTimeout to be %v, got %v", DefaultCacheMaxQueryTimeout, config.MaxQueryTimeout.Value)
	}
	if config.EntitiesHardTTL.Value != DefaultCacheHardEntitiesTTL {
		t.Errorf("Expected EntitiesHardTTL to be %v, got %v", DefaultCacheHardEntitiesTTL, config.EntitiesHardTTL.Value)
	}
	if config.EntitiesSoftTTL.Value != DefaultCacheSoftEntitiesTTL {
		t.Errorf("Expected EntitiesSoftTTL to be %v, got %v", DefaultCacheSoftEntitiesTTL, config.EntitiesSoftTTL.Value)
	}
	if config.TTLJitterPercent.Value != DefaultCacheTTLJitterPercent {
		t.Errorf("Expected TTLJitterPercent to be %v, got %v", DefaultCacheTTLJitterPercent, config.TTLJitterPercent.Value)
	}
	if config.Enabled.Value != DefaultCacheEnabled {
		t.Errorf("Expected Enabled to be %v, got %v", DefaultCacheEnabled, config.Enabled.Value)
	}
	if config.EnableOnClient.Value != DefaultCacheEnableOnClient {
		t.Errorf("Expected EnableOnClient to be %v, got %v", DefaultCacheEnableOnClient, config.EnableOnClient.Value)
	}
	if config.EncoderType.Value != DefaultCacheEncoderType.String() {
		t.Errorf("Expected EncoderType to be %s, got %s", DefaultCacheEncoderType.String(), config.EncoderType.Value)
	}
}

func TestParseEnvVars_cache(t *testing.T) {
	os.Setenv("CACHE_SERVER_KIND", "valkey")
	os.Setenv("CACHE_SERVER_ADDRESSES", "valkey1:6379,valkey2:6380")
	os.Setenv("CACHE_SERVER_USERNAME", "cacheuser")
	os.Setenv("CACHE_SERVER_PASSWORD", "cachepass")
	os.Setenv("CACHE_SERVER_DB", "2")
	os.Setenv("CACHE_MAX_QUERY_TIMEOUT", "100ms")
	os.Setenv("CACHE_ENTITIES_HARD_TTL", "24h")
	os.Setenv("CACHE_ENTITIES_SOFT_TTL", "12h")
	os.Setenv("CACHE_TTL_JITTER_PERCENT", "0.2")
	os.Setenv("CACHE_CLIENT_ENABLED", "false")
	os.Setenv("CACHE_ENCODER_TYPE", "json")
	os.Setenv("CACHE_ENABLED", "false")

	config := NewCacheConfig()
	config.ParseEnvVars()

	if config.ServerKind.Value != "valkey" {
		t.Errorf("Expected ServerKind to be valkey, got %s", config.ServerKind.Value)
	}
	if config.ServerUsername.Value != "cacheuser" {
		t.Errorf("Expected ServerUsername to be cacheuser, got %s", config.ServerUsername.Value)
	}
	if config.ServerPassword.Value != "cachepass" {
		t.Errorf("Expected ServerPassword to be cachepass, got %s", config.ServerPassword.Value)
	}
	if config.ServerDB.Value != 2 {
		t.Errorf("Expected ServerDB to be 2, got %d", config.ServerDB.Value)
	}
	if config.MaxQueryTimeout.Value != 100*time.Millisecond {
		t.Errorf("Expected MaxQueryTimeout to be 100ms, got %v", config.MaxQueryTimeout.Value)
	}
	if config.EntitiesHardTTL.Value != 24*time.Hour {
		t.Errorf("Expected EntitiesHardTTL to be 24h, got %v", config.EntitiesHardTTL.Value)
	}
	if config.EntitiesSoftTTL.Value != 12*time.Hour {
		t.Errorf("Expected EntitiesSoftTTL to be 12h, got %v", config.EntitiesSoftTTL.Value)
	}
	if config.TTLJitterPercent.Value != 0.2 {
		t.Errorf("Expected TTLJitterPercent to be 0.2, got %v", config.TTLJitterPercent.Value)
	}
	if config.EnableOnClient.Value != false {
		t.Errorf("Expected EnableOnClient to be false, got %v", config.EnableOnClient.Value)
	}
	if config.EncoderType.Value != "json" {
		t.Errorf("Expected EncoderType to be json, got %s", config.EncoderType.Value)
	}
	if config.Enabled.Value != false {
		t.Errorf("Expected Enabled to be false, got %v", config.Enabled.Value)
	}

	// Clean up environment variables
	os.Unsetenv("CACHE_SERVER_KIND")
	os.Unsetenv("CACHE_SERVER_ADDRESSES")
	os.Unsetenv("CACHE_SERVER_USERNAME")
	os.Unsetenv("CACHE_SERVER_PASSWORD")
	os.Unsetenv("CACHE_SERVER_DB")
	os.Unsetenv("CACHE_MAX_QUERY_TIMEOUT")
	os.Unsetenv("CACHE_ENTITIES_HARD_TTL")
	os.Unsetenv("CACHE_ENTITIES_SOFT_TTL")
	os.Unsetenv("CACHE_TTL_JITTER_PERCENT")
	os.Unsetenv("CACHE_CLIENT_ENABLED")
	os.Unsetenv("CACHE_ENCODER_TYPE")
	os.Unsetenv("CACHE_ENABLED")
}

func TestValidate_cache(t *testing.T) {
	config := NewCacheConfig()

	// Test valid configuration
	err := config.Validate()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test invalid ServerKind
	config.ServerKind.Value = "invalid"
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "cache.server.kind" {
		t.Errorf("Expected InvalidConfigurationError with field 'cache.server.kind', got %v", err)
	}
	config.ServerKind.Value = DefaultCacheServerKind

	// Test invalid Addresses (empty)
	config.ServerAddresses.Value = SliceStringVar{}
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "cache.server.addresses" {
		t.Errorf("Expected InvalidConfigurationError with field 'cache.server.addresses', got %v", err)
	}
	config.ServerAddresses.Value = DefaultCacheServerAddresses

	// Test invalid Addresses (bad format)
	config.ServerAddresses.Value = SliceStringVar{"invalid-address"}
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "cache.server.addresses" {
		t.Errorf("Expected InvalidConfigurationError with field 'cache.server.addresses', got %v", err)
	}

	// Test invalid Addresses (bad port)
	config.ServerAddresses.Value = SliceStringVar{"localhost:abc"}
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "cache.server.addresses" {
		t.Errorf("Expected InvalidConfigurationError with field 'cache.server.addresses', got %v", err)
	}

	// Test invalid Addresses (port out of range)
	config.ServerAddresses.Value = SliceStringVar{"localhost:99999"}
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "cache.server.addresses" {
		t.Errorf("Expected InvalidConfigurationError with field 'cache.server.addresses', got %v", err)
	}

	// Test invalid Addresses (short hostname)
	config.ServerAddresses.Value = SliceStringVar{"ab:6379"}
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "cache.server.addresses" {
		t.Errorf("Expected InvalidConfigurationError with field 'cache.server.addresses', got %v", err)
	}
	config.ServerAddresses.Value = DefaultCacheServerAddresses

	// Test invalid DB
	config.ServerDB.Value = -1
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "cache.server.db" {
		t.Errorf("Expected InvalidConfigurationError with field 'cache.server.db', got %v", err)
	}
	config.ServerDB.Value = DefaultCacheServerDB

	// Test invalid DB (one past the end: valkey ships `databases 16`, so the
	// valid SELECT indexes are 0..15 and 16 fails to connect)
	config.ServerDB.Value = 16
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "cache.server.db" {
		t.Errorf("Expected InvalidConfigurationError with field 'cache.server.db', got %v", err)
	}
	config.ServerDB.Value = DefaultCacheServerDB

	// Test invalid MaxQueryTimeout (too short)
	config.MaxQueryTimeout.Value = 5 * time.Millisecond
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "cache.max.query.timeout" {
		t.Errorf("Expected InvalidConfigurationError with field 'cache.max.query.timeout', got %v", err)
	}
	config.MaxQueryTimeout.Value = DefaultCacheMaxQueryTimeout

	// Test invalid MaxQueryTimeout (too long). c3e's SafeCacheManager rejects
	// anything above 500ms, so a value accepted here would abort startup.
	config.MaxQueryTimeout.Value = 800 * time.Millisecond
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "cache.max.query.timeout" {
		t.Errorf("Expected InvalidConfigurationError with field 'cache.max.query.timeout', got %v", err)
	}
	config.MaxQueryTimeout.Value = DefaultCacheMaxQueryTimeout

	// Test invalid EntitiesHardTTL (too short)
	config.EntitiesHardTTL.Value = 30 * time.Minute
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "cache.entities.hard.ttl" {
		t.Errorf("Expected InvalidConfigurationError with field 'cache.entities.hard.ttl', got %v", err)
	}
	config.EntitiesHardTTL.Value = DefaultCacheHardEntitiesTTL

	// Test invalid EntitiesSoftTTL (too short)
	config.EntitiesSoftTTL.Value = 30 * time.Second
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "cache.entities.soft.ttl" {
		t.Errorf("Expected InvalidConfigurationError with field 'cache.entities.soft.ttl', got %v", err)
	}
	config.EntitiesSoftTTL.Value = DefaultCacheSoftEntitiesTTL

	// Test soft TTL past the hard TTL. Both pass their own bounds — an entry
	// that can never go stale is only visible by comparing them, and c3e
	// rejects the pair at startup with an error naming neither flag.
	config.EntitiesHardTTL.Value = 2 * time.Hour
	config.EntitiesSoftTTL.Value = 10 * time.Hour
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "cache.entities.soft.ttl" {
		t.Errorf("Expected InvalidConfigurationError with field 'cache.entities.soft.ttl', got %v", err)
	}
	config.EntitiesHardTTL.Value = DefaultCacheHardEntitiesTTL
	config.EntitiesSoftTTL.Value = DefaultCacheSoftEntitiesTTL

	// Test invalid TTLJitterPercent (out of range)
	config.TTLJitterPercent.Value = 1.5
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "cache.ttl.jitter.percent" {
		t.Errorf("Expected InvalidConfigurationError with field 'cache.ttl.jitter.percent', got %v", err)
	}
	config.TTLJitterPercent.Value = DefaultCacheTTLJitterPercent
}

// TestCacheTLSValidation covers the branches that exist to stop an operator
// believing a connection is encrypted when it is not — configuring a CA while
// TLS is off is the shape of that mistake.
func TestCacheTLSValidation(t *testing.T) {
	pem := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(pem, []byte("not a real certificate"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	expectInvalid := func(t *testing.T, c *CacheConfig, field string) {
		t.Helper()

		err := c.Validate()
		if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != field {
			t.Errorf("expected InvalidConfigurationError on %q, got %v", field, err)
		}
	}

	t.Run("defaults validate", func(t *testing.T) {
		if err := NewCacheConfig().Validate(); err != nil {
			t.Errorf("the default configuration must validate, got %v", err)
		}
	})

	t.Run("ca file without tls enabled is refused", func(t *testing.T) {
		c := NewCacheConfig()
		c.TLSCAFile.Value = pem
		expectInvalid(t, c, "cache.tls.ca.file")
	})

	t.Run("skip verify without tls enabled is refused", func(t *testing.T) {
		c := NewCacheConfig()
		c.TLSInsecureSkipVerify.Value = true
		expectInvalid(t, c, "cache.tls.insecure.skip.verify")
	})

	t.Run("cert without key is refused", func(t *testing.T) {
		c := NewCacheConfig()
		c.TLSEnabled.Value = true
		c.TLSCertFile.Value = pem
		expectInvalid(t, c, "cache.tls.cert.file")
	})

	t.Run("key without cert is refused", func(t *testing.T) {
		c := NewCacheConfig()
		c.TLSEnabled.Value = true
		c.TLSKeyFile.Value = pem
		expectInvalid(t, c, "cache.tls.cert.file")
	})

	t.Run("a CA together with skip verify is refused", func(t *testing.T) {
		c := NewCacheConfig()
		c.TLSEnabled.Value = true
		c.TLSCAFile.Value = pem
		c.TLSInsecureSkipVerify.Value = true
		expectInvalid(t, c, "cache.tls.insecure.skip.verify")
	})

	t.Run("a missing file is refused", func(t *testing.T) {
		c := NewCacheConfig()
		c.TLSEnabled.Value = true
		c.TLSCAFile.Value = filepath.Join(t.TempDir(), "absent.pem")
		expectInvalid(t, c, "cache.tls.ca.file")
	})

	t.Run("tls with a readable CA validates", func(t *testing.T) {
		c := NewCacheConfig()
		c.TLSEnabled.Value = true
		c.TLSCAFile.Value = pem

		if err := c.Validate(); err != nil {
			t.Errorf("expected this to validate, got %v", err)
		}
	})
}
