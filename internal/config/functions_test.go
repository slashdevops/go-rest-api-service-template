package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		value        string
		defaultValue any
		expected     any
	}{
		{
			name:         "string environment variable",
			key:          "STRING_ENV",
			value:        "test_value",
			defaultValue: "default_value",
			expected:     "test_value",
		},
		{
			name:         "int environment variable",
			key:          "INT_ENV",
			value:        "42",
			defaultValue: 0,
			expected:     42,
		},
		{
			name:         "time.Duration environment variable",
			key:          "DURATION_ENV",
			value:        "1h",
			defaultValue: time.Minute,
			expected:     time.Hour,
		},
		{
			name:         "bool environment variable",
			key:          "BOOL_ENV",
			value:        "true",
			defaultValue: false,
			expected:     true,
		},
		{
			name:         "int32 environment variable",
			key:          "INT32_ENV",
			value:        "32",
			defaultValue: int32(0),
			expected:     int32(32),
		},
		{
			name:         "int64 environment variable",
			key:          "INT64_ENV",
			value:        "64",
			defaultValue: int64(0),
			expected:     int64(64),
		},
		{
			name:         "float32 environment variable",
			key:          "FLOAT32_ENV",
			value:        "3.14",
			defaultValue: float32(0.0),
			expected:     float32(3.14),
		},
		{
			name:         "float64 environment variable",
			key:          "FLOAT64_ENV",
			value:        "6.28",
			defaultValue: float64(0.0),
			expected:     float64(6.28),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(tt.key, tt.value)
			defer os.Unsetenv(tt.key)

			got := GetEnv(tt.key, tt.defaultValue)
			if got != tt.expected {
				t.Errorf("GetEnv() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSetEnvVarFromFile(t *testing.T) {
	// Create a temporary directory to hold the .env file
	tempDir := t.TempDir()

	// Create a .env file in the temporary directory
	envFilePath := filepath.Join(tempDir, ".env")
	envFileContent := `
# This is a comment
STRING_ENV=test_value
INT_ENV=42
BOOL_ENV=true
FLOAT_ENV=3.14
  WHITESPACE_ENV  =  whitespace_value
`
	if err := os.WriteFile(envFilePath, []byte(envFileContent), 0o644); err != nil {
		t.Fatalf("Failed to create .env file: %v", err)
	}

	// Change the current working directory to the temporary directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	if err := os.Chdir(originalDir); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Call the SetEnvVarFromFile function
	if err := SetEnvVarFromFile(); err != nil {
		t.Fatalf("SetEnvVarFromFile() error = %v", err)
	}

	// Check if the environment variables are set correctly
	tests := []struct {
		key      string
		expected string
	}{
		{"STRING_ENV", "test_value"},
		{"INT_ENV", "42"},
		{"BOOL_ENV", "true"},
		{"FLOAT_ENV", "3.14"},
		{"WHITESPACE_ENV", "whitespace_value"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := os.Getenv(tt.key); got != tt.expected {
				t.Errorf("os.Getenv(%q) = %v, want %v", tt.key, got, tt.expected)
			}
		})
	}
}

func TestLoadAndValidate_Success(t *testing.T) {
	// Set up environment variables
	os.Setenv("CACHE_KIND", "valkey")
	os.Setenv("LOG_LEVEL", "info")
	defer os.Unsetenv("CACHE_KIND")
	defer os.Unsetenv("LOG_LEVEL")

	// Create configs
	cacheCfg := NewCacheConfig()
	logCfg := NewLogConfig()

	// Test LoadAndValidate
	err := LoadAndValidate(cacheCfg, logCfg)
	if err != nil {
		t.Errorf("LoadAndValidate() returned unexpected error: %v", err)
	}

	// Verify that environment variables were parsed
	if cacheCfg.ServerKind.Value != "valkey" {
		t.Errorf("Expected cache server kind to be 'valkey', got '%s'", cacheCfg.ServerKind.Value)
	}

	if logCfg.Level.Value != "info" {
		t.Errorf("Expected log level to be 'info', got '%s'", logCfg.Level.Value)
	}
}

func TestLoadAndValidate_ValidationError(t *testing.T) {
	// Set up invalid environment variable
	os.Setenv("CACHE_SERVER_KIND", "invalid")
	defer os.Unsetenv("CACHE_SERVER_KIND")

	cacheCfg := NewCacheConfig()

	// Test LoadAndValidate with invalid config
	err := LoadAndValidate(cacheCfg)
	if err == nil {
		t.Error("LoadAndValidate() should have returned a validation error")
	}

	// Verify it's the right kind of error
	if _, ok := err.(*InvalidConfigurationError); !ok {
		t.Errorf("Expected InvalidConfigurationError, got %T", err)
	}
}

func TestLoadAndValidate_EmptyList(t *testing.T) {
	// Test with no configs
	err := LoadAndValidate()
	if err != nil {
		t.Errorf("LoadAndValidate() with no configs should not return an error, got: %v", err)
	}
}

func TestLoadAndValidate_MultipleConfigs(t *testing.T) {
	// Set up environment variables for multiple configs
	os.Setenv("CACHE_KIND", "valkey")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("CACHE_DB", "0")
	defer os.Unsetenv("CACHE_KIND")
	defer os.Unsetenv("LOG_LEVEL")
	defer os.Unsetenv("CACHE_DB")

	// Create multiple configs
	cacheCfg := NewCacheConfig()
	logCfg := NewLogConfig()

	// Test LoadAndValidate with multiple configs
	err := LoadAndValidate(cacheCfg, logCfg)
	if err != nil {
		t.Errorf("LoadAndValidate() with multiple configs returned unexpected error: %v", err)
	}

	// Verify all configs were parsed and validated
	if cacheCfg.ServerKind.Value != "valkey" {
		t.Errorf("Expected cache server kind to be 'valkey', got '%s'", cacheCfg.ServerKind.Value)
	}

	if logCfg.Level.Value != "debug" {
		t.Errorf("Expected log level to be 'debug', got '%s'", logCfg.Level.Value)
	}
}
