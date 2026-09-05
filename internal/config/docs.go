// Package config provides type-safe configuration management with support for
// command-line flags, environment variables, and .env files.
//
// # Architecture
//
// The package is built around a consistent pattern across all configuration modules:
//  1. Each module defines its configuration struct with generic Field[T] types
//  2. Default values are defined as constants
//  3. Configuration can be loaded from environment variables via ParseEnvVars()
//  4. All configurations implement the Validator interface for validation
//
// # Naming Convention
//
// The package follows a strict naming convention:
//   - CLI Flags: Use dot notation (e.g., "cache.query.timeout")
//   - Environment Variables: Use uppercase with underscores (e.g., "CACHE_QUERY_TIMEOUT")
//
// This allows flexible configuration via both command-line flags and environment variables.
//
// # Usage
//
// Basic usage pattern:
//
//	cfg := config.NewCacheConfig()
//	cfg.ParseEnvVars()
//	if err := cfg.Validate(); err != nil {
//	    log.Fatal(err)
//	}
//
// For multiple configurations:
//
//	dbCfg := config.NewDatabaseConfig()
//	cacheCfg := config.NewCacheConfig()
//	config.ParseEnvVars(dbCfg, cacheCfg)
//	if err := config.Validate(dbCfg, cacheCfg); err != nil {
//	    log.Fatal(err)
//	}
//
// # Environment Files
//
// The package supports loading environment variables from .env files:
//
//	if err := config.SetEnvVarFromFile(); err != nil {
//	    log.Printf("Warning: could not load .env file: %v", err)
//	}
//
// # Custom Flag Types
//
// The package provides custom flag types for complex configurations:
//   - SliceStringVar: For string slices
//   - FileVar: For file handles
//   - SliceIDPVar: For Identity Provider configurations
//
// All custom types implement the standard flag.Value interface.
//
// # Generic Field Type
//
// The Field[T] generic type provides type-safe configuration with automatic
// environment variable parsing and flag registration:
//
//	type Config struct {
//	    Port Field[int]
//	}
//
//	cfg := Config{
//	    Port: NewField("server.port", "SERVER_PORT", "Server port", 8080),
//	}
//
// The generic GetEnv[T] function handles type conversion automatically.
package config
