package config

import (
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/slashdevops/go-rest-api-service-template/pkg/cslog"
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

// SlogLevel is consulted twice -- by the standard handler and by the minimum
// severity the log pipeline enforces on its exporter -- so a level it gets
// wrong is wrong in two places at once.
func TestSlogLevel(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  slog.Level
	}{
		{"debug", "debug", slog.LevelDebug},
		{"info", "info", slog.LevelInfo},
		{"warn", "warn", slog.LevelWarn},
		{"error", "error", slog.LevelError},

		// Validate rewrites "ctrace" and "cfatal" to these before anything
		// reads them, which is why SlogLevel matches on the level strings and
		// not on the words an operator types.
		{"trace_after_validate", cslog.LogLevelTrace.String(), cslog.LogLevelTrace},
		{"fatal_after_validate", cslog.LogLevelFatal.String(), cslog.LogLevelFatal},

		{"unknown_falls_back_to_info", "verbose", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewLogConfig()
			c.Level.Value = tt.value

			if got := c.SlogLevel(); got != tt.want {
				t.Errorf("SlogLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

// The hidden levels survive a round trip through Validate, which is the only
// path an operator's "ctrace" actually takes.
func TestSlogLevelAfterValidate(t *testing.T) {
	for _, tt := range []struct {
		typed string
		want  slog.Level
	}{
		{"ctrace", cslog.LogLevelTrace},
		{"cfatal", cslog.LogLevelFatal},
	} {
		t.Run(tt.typed, func(t *testing.T) {
			c := NewLogConfig()
			c.Level.Value = tt.typed

			if err := c.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil: %q is accepted but undocumented", err, tt.typed)
			}

			if got := c.SlogLevel(); got != tt.want {
				t.Errorf("SlogLevel() after Validate = %v, want %v", got, tt.want)
			}
		})
	}
}
