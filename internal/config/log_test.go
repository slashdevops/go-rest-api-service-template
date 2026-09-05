package config

import (
	"errors"
	"os"
	"testing"
)

func TestNewLogConfig(t *testing.T) {
	config := NewLogConfig()

	if config.Level.Value != DefaultLogLevel {
		t.Errorf("expected default log level %s, got %s", DefaultLogLevel, config.Level.Value)
	}

	if config.Format.Value != DefaultLogFormat {
		t.Errorf("expected default log format %s, got %s", DefaultLogFormat, config.Format.Value)
	}

	if config.Output.Value != DefaultLogOutput {
		t.Errorf("expected default log output %v, got %v", DefaultLogOutput, config.Output.Value)
	}

	if config.Debug.Value != DefaultLogDebug {
		t.Errorf("expected default debug mode %v, got %v", DefaultLogDebug, config.Debug.Value)
	}
}

func TestParseEnvVars_Log(t *testing.T) {
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("LOG_FORMAT", "json")
	os.Setenv("DEBUG", "true")

	config := NewLogConfig()
	config.ParseEnvVars()

	if config.Level.Value != "debug" {
		t.Errorf("expected log level debug, got %s", config.Level.Value)
	}

	if config.Format.Value != "json" {
		t.Errorf("expected log format json, got %s", config.Format.Value)
	}

	if config.Debug.Value != true {
		t.Errorf("expected debug mode true, got %v", config.Debug.Value)
	}
}

func TestValidate_Log(t *testing.T) {
	config := NewLogConfig()

	// Test valid configuration
	config.Level.Value = "info"
	config.Format.Value = "text"
	if err := config.Validate(); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Test invalid log level
	config.Level.Value = "invalid-level"
	err := config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "log.level" {
		t.Errorf("Expected InvalidConfigurationError with field 'log.level', got %v", err)
	}

	// Test invalid log format
	config.Level.Value = "info"
	config.Format.Value = "invalid-format"
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "log.format" {
		t.Errorf("Expected InvalidConfigurationError with field 'log.format', got %v", err)
	}
}
