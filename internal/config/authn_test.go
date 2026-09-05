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

	if config.TokenLifetimesReloadInterval.Value != DefaultAuthnTokenLifetimesReloadInterval {
		t.Errorf("Expected TokenLifetimesReloadInterval to be %v, got %v", DefaultAuthnTokenLifetimesReloadInterval, config.TokenLifetimesReloadInterval.Value)
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
	os.Setenv("AUTHN_TOKEN_LIFETIMES_RELOAD_INTERVAL", "2m")
	os.Setenv("AUTHN_USER_VERIFICATION_WEB_ENDPOINT", "http://test.localhost:9090/verify")
	os.Setenv("AUTHN_USER_VERIFICATION_TOKEN_TTL", "48h")

	config := NewAuthConfig()
	config.ParseEnvVars()

	// Note: The file parsing might create file objects, so we test the name
	if config.Issuer.Value != "https://test.example.com" {
		t.Errorf("Expected Issuer to be https://test.example.com, got %s", config.Issuer.Value)
	}
	if config.TokenLifetimesReloadInterval.Value != 2*time.Minute {
		t.Errorf("Expected TokenLifetimesReloadInterval to be 2m, got %v", config.TokenLifetimesReloadInterval.Value)
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
	os.Unsetenv("AUTHN_TOKEN_LIFETIMES_RELOAD_INTERVAL")
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

	// The token lifetimes are not settings any more, so there is nothing to
	// refuse here; what remains is the interval the mirror re-reads them on.
	config.TokenLifetimesReloadInterval.Value = 0
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "authn.token.lifetimes.reload.interval" {
		t.Errorf("Expected InvalidConfigurationError with field 'authn.token.lifetimes.reload.interval', got %v", err)
	}
	config.TokenLifetimesReloadInterval.Value = DefaultAuthnTokenLifetimesReloadInterval

	// The two cross-checks that used to compare against the lifetimes now
	// compare against the shortest lifetime an operator may configure.
	config.RefreshTokenRotationGrace.Value = 12 * time.Hour
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "authn.refresh.token.rotation.grace" {
		t.Errorf("Expected InvalidConfigurationError with field 'authn.refresh.token.rotation.grace', got %v", err)
	}
	config.RefreshTokenRotationGrace.Value = DefaultAuthnRefreshTokenRotationGrace

	config.AccessTokenRevocationReloadInterval.Value = 2 * time.Minute
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "authn.access.token.revocation.reload.interval" {
		t.Errorf("Expected InvalidConfigurationError with field 'authn.access.token.revocation.reload.interval', got %v", err)
	}
	config.AccessTokenRevocationReloadInterval.Value = DefaultAuthnAccessTokenRevocationReloadInterval

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
