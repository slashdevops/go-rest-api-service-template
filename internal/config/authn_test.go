package config

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestNewAuthConfig(t *testing.T) {
	config := NewAuthConfig()

	if config.PrivateKeyFile.Value.Name() != DefaultAuthnPrivateKeyFile.Name() {
		t.Errorf("Expected PrivateKeyFile to be %s, got %s", DefaultAuthnPrivateKeyFile.Name(), config.PrivateKeyFile.Value.Name())
	}

	if config.PublicKeyFile.Value.Name() != DefaultAuthnPublicKeyFile.Name() {
		t.Errorf("Expected PublicKeyFile to be %s, got %s", DefaultAuthnPublicKeyFile.Name(), config.PublicKeyFile.Value.Name())
	}

	if config.SymmetricKeyFile.Value.Name() != DefaultAuthnSymmetricKeyFile.Name() {
		t.Errorf("Expected SymmetricKeyFile to be %s, got %s", DefaultAuthnSymmetricKeyFile.Name(), config.SymmetricKeyFile.Value.Name())
	}

	if config.Issuer.Value != DefaultAuthnIssuer {
		t.Errorf("Expected Issuer to be %s, got %s", DefaultAuthnIssuer, config.Issuer.Value)
	}

	if config.AccessTokenDuration.Value != DefaultAuthnAccessTokenDuration {
		t.Errorf("Expected AccessTokenDuration to be %v, got %v", DefaultAuthnAccessTokenDuration, config.AccessTokenDuration.Value)
	}

	if config.RefreshTokenDuration.Value != DefaultAuthnRefreshTokenDuration {
		t.Errorf("Expected RefreshTokenDuration to be %v, got %v", DefaultAuthnRefreshTokenDuration, config.RefreshTokenDuration.Value)
	}

	if config.UserVerificationWebEndpoint.Value != DefaultAuthnUserVerificationWebEndpoint {
		t.Errorf("Expected UserVerificationWebEndpoint to be %s, got %s", DefaultAuthnUserVerificationWebEndpoint, config.UserVerificationWebEndpoint.Value)
	}

	if config.UserVerificationTokenTTL.Value != DefaultAuthnUserVerificationTokenTTL {
		t.Errorf("Expected UserVerificationTokenTTL to be %v, got %v", DefaultAuthnUserVerificationTokenTTL, config.UserVerificationTokenTTL.Value)
	}
}

func TestParseEnvVars_authn(t *testing.T) {
	os.Setenv("AUTHN_PRIVATE_KEY_FILE", "/tmp/test_private.key")
	os.Setenv("AUTHN_PUBLIC_KEY_FILE", "/tmp/test_public.key")
	os.Setenv("AUTHN_SYMMETRIC_KEY_FILE", "/tmp/test_symmetric.key")
	os.Setenv("AUTHN_ISSUER", "https://test.example.com")
	os.Setenv("AUTHN_ACCESS_TOKEN_DURATION", "10m")
	os.Setenv("AUTHN_REFRESH_TOKEN_DURATION", "48h")
	os.Setenv("AUTHN_USER_VERIFICATION_WEB_ENDPOINT", "http://test.localhost:9090/verify")
	os.Setenv("AUTHN_USER_VERIFICATION_TOKEN_TTL", "48h")

	config := NewAuthConfig()
	config.ParseEnvVars()

	// Note: The file parsing might create file objects, so we test the name
	if config.Issuer.Value != "https://test.example.com" {
		t.Errorf("Expected Issuer to be https://test.example.com, got %s", config.Issuer.Value)
	}
	if config.AccessTokenDuration.Value != 10*time.Minute {
		t.Errorf("Expected AccessTokenDuration to be 10m, got %v", config.AccessTokenDuration.Value)
	}
	if config.RefreshTokenDuration.Value != 48*time.Hour {
		t.Errorf("Expected RefreshTokenDuration to be 48h, got %v", config.RefreshTokenDuration.Value)
	}
	if config.UserVerificationWebEndpoint.Value != "http://test.localhost:9090/verify" {
		t.Errorf("Expected UserVerificationWebEndpoint to be http://test.localhost:9090/verify, got %s", config.UserVerificationWebEndpoint.Value)
	}
	if config.UserVerificationTokenTTL.Value != 48*time.Hour {
		t.Errorf("Expected UserVerificationTokenTTL to be 48h, got %v", config.UserVerificationTokenTTL.Value)
	}

	// Clean up environment variables
	os.Unsetenv("AUTHN_PRIVATE_KEY_FILE")
	os.Unsetenv("AUTHN_PUBLIC_KEY_FILE")
	os.Unsetenv("AUTHN_SYMMETRIC_KEY_FILE")
	os.Unsetenv("AUTHN_ISSUER")
	os.Unsetenv("AUTHN_ACCESS_TOKEN_DURATION")
	os.Unsetenv("AUTHN_REFRESH_TOKEN_DURATION")
	os.Unsetenv("AUTHN_USER_VERIFICATION_WEB_ENDPOINT")
	os.Unsetenv("AUTHN_USER_VERIFICATION_TOKEN_TTL")
}

func TestValidate_authn(t *testing.T) {
	config := NewAuthConfig()

	// Test valid configuration
	err := config.Validate()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test invalid PrivateKeyFile
	originalPrivateKeyFile := config.PrivateKeyFile.Value
	config.PrivateKeyFile.Value = FileVar{os.NewFile(0, "x"), os.O_RDONLY}
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "authn.private.key.file" {
		t.Errorf("Expected InvalidConfigurationError with field 'authn.private.key.file', got %v", err)
	}
	config.PrivateKeyFile.Value = originalPrivateKeyFile

	// Test invalid PublicKeyFile
	originalPublicKeyFile := config.PublicKeyFile.Value
	config.PublicKeyFile.Value = FileVar{os.NewFile(0, "y"), os.O_RDONLY}
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "authn.public.key.file" {
		t.Errorf("Expected InvalidConfigurationError with field 'authn.public.key.file', got %v", err)
	}
	config.PublicKeyFile.Value = originalPublicKeyFile

	// Test invalid SymmetricKeyFile
	originalSymmetricKeyFile := config.SymmetricKeyFile.Value
	config.SymmetricKeyFile.Value = FileVar{os.NewFile(0, "z"), os.O_RDONLY}
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "authn.symmetric.key.file" {
		t.Errorf("Expected InvalidConfigurationError with field 'authn.symmetric.key.file', got %v", err)
	}
	config.SymmetricKeyFile.Value = originalSymmetricKeyFile

	// Test invalid Issuer (empty)
	config.Issuer.Value = ""
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "authn.issuer" {
		t.Errorf("Expected InvalidConfigurationError with field 'authn.issuer', got %v", err)
	}
	config.Issuer.Value = DefaultAuthnIssuer

	// Test invalid AccessTokenDuration (too short)
	config.AccessTokenDuration.Value = 30 * time.Second
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "authn.access.token.duration" {
		t.Errorf("Expected InvalidConfigurationError with field 'authn.access.token.duration', got %v", err)
	}
	config.AccessTokenDuration.Value = DefaultAuthnAccessTokenDuration

	// Test invalid RefreshTokenDuration (too short)
	config.RefreshTokenDuration.Value = 1 * time.Minute
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "authn.refresh.token.duration" {
		t.Errorf("Expected InvalidConfigurationError with field 'authn.refresh.token.duration', got %v", err)
	}
	config.RefreshTokenDuration.Value = DefaultAuthnRefreshTokenDuration

	// Test invalid UserVerificationWebEndpoint
	config.UserVerificationWebEndpoint.Value = ":/invalid-url"
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "authn.user.verification.web.endpoint" {
		t.Errorf("Expected InvalidConfigurationError with field 'authn.user.verification.web.endpoint', got %v", err)
	}
	config.UserVerificationWebEndpoint.Value = DefaultAuthnUserVerificationWebEndpoint

	// Test invalid UserVerificationTokenTTL (too short)
	config.UserVerificationTokenTTL.Value = 30 * time.Minute
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "authn.user.verification.token.ttl" {
		t.Errorf("Expected InvalidConfigurationError with field 'authn.user.verification.token.ttl', got %v", err)
	}
	config.UserVerificationTokenTTL.Value = DefaultAuthnUserVerificationTokenTTL
}
