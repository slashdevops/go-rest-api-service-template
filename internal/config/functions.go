package config

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Validator is an interface for validating configuration values
// Implement this interface for configuration structs
// and add the validation logic in the Validate method
type Validator interface {
	Validate() error
}

// EnvVarsParser is an interface for parsing configuration values from environment variables
// Implement this interface for configuration structs
// and add the parsing logic in the ParseEnvVars method
type EnvVarsParser interface {
	ParseEnvVars()
}

// Validate validates the configuration values
// by calling the Validate method of each configuration struct
// and returns the first error encountered
func Validate(configs ...Validator) error {
	for _, config := range configs {
		if err := config.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// ParseEnvVars reads the configuration from environment variables
// and sets the values in the configuration
// by calling the ParseEnvVars method of each configuration struct
func ParseEnvVars(configs ...EnvVarsParser) {
	for _, config := range configs {
		config.ParseEnvVars()
	}
}

// SliceStringVar is a custom flag type for string slices
// This should implement the Value interface of the flag package
// Reference: https://pkg.go.dev/gg-scm.io/tool/internal/flag#FlagSet.Var
type SliceStringVar []string

// String presents the current value as a string.
func (s *SliceStringVar) String() string {
	return strings.Join(*s, ", ")
}

// Set is called once, in command line order, for each flag present.
func (s *SliceStringVar) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// Get returns the contents of the Value.
func (s *SliceStringVar) Get() any {
	return *s
}

// IsBoolFlag returns true if the flag is a boolean flag
func (s *SliceStringVar) IsBoolFlag() bool {
	return false
}

// FileVar is a custom flag type for files
// This should implement the Value interface of the flag package
// Reference: https://pkg.go.dev/gg-scm.io/tool/internal/flag#FlagSet.Var
type FileVar struct {
	*os.File

	// flag is the flag to open the file with
	// os.O_APPEND|os.O_CREATE|os.O_WRONLY
	Flag int
}

// String presents the current value as a string.
func (f *FileVar) String() string {
	if f.File == nil {
		return ""
	}

	return f.Name()
}

// Set is called once, in command line order, for each flag present.
func (f *FileVar) Set(value string) error {
	file, err := os.OpenFile(value, f.Flag, 0o644)
	if err != nil {
		return err
	}

	f.File = file
	return nil
}

// Get returns the contents of the Value.
func (f *FileVar) Get() any {
	return f.File
}

// IsBoolFlag returns true if the flag is a boolean flag
func (f *FileVar) IsBoolFlag() bool {
	return false
}

// idpPartsSeparator is the separator used for IDP parts
const idpPartsSeparator = "#"

// IDP is a custom flag type for Identity Provider (IDP) configuration
// This should implement the Value interface of the flag package
// Reference: https://pkg.go.dev/gg-scm.io/tool/internal/flag#FlagSet.Var
type IDP struct {
	// Name is the name of the IDP
	Name string

	// CallbackURL is the URL to redirect to after authentication (callback URL)
	CallbackURL string

	// LoginRedirectURL is the URL to redirect to after login authentication (callback URL)
	LoginRedirectURL string

	// RegisterRedirectURL is the URL to redirect to after registration authentication (callback URL)
	RegisterRedirectURL string

	// ClientID is the client ID for the IDP
	ClientID string

	// ClientSecret is the client secret for the IDP
	ClientSecret string
}

// SliceIDPVar is a custom flag type for slices of IDP configurations
type SliceIDPVar []IDP

// String presents the current value as a string.
func (s *SliceIDPVar) String() string {
	var idps []string

	for _, idp := range *s {
		idps = append(idps,
			fmt.Sprintf("%s%s%s%s%s%s%s%s%s%s%s",
				idp.Name,
				idpPartsSeparator,
				idp.CallbackURL,
				idpPartsSeparator,
				idp.LoginRedirectURL,
				idpPartsSeparator,
				idp.RegisterRedirectURL,
				idpPartsSeparator,
				idp.ClientID,
				idpPartsSeparator,
				idp.ClientSecret,
			),
		)
	}

	return strings.Join(idps, ",")
}

// Set is called once, in command line order, for each flag present.
func (s *SliceIDPVar) Set(value string) error {
	// clear existing IDPs before setting new ones
	*s = nil

	parts := strings.SplitSeq(value, ",")
	for part := range parts {
		idpParts := strings.SplitN(part, "#", 6)

		if len(idpParts) != 6 {
			return fmt.Errorf(
				"invalid IDP format: %s. This should be in the format 'name%sredirectURL%sloginRedirectURL%sregisterRedirectURL%sclientID%sclientSecret'",
				part,
				idpPartsSeparator, idpPartsSeparator, idpPartsSeparator, idpPartsSeparator, idpPartsSeparator,
			)
		}

		// trim leading and trailing whitespace from each part
		// in case the idps was separated by commas with spaces (, )
		// of parts was separated by dashes with spaces (# )
		*s = append(*s, IDP{
			Name:                strings.TrimSpace(idpParts[0]),
			CallbackURL:         strings.TrimSpace(idpParts[1]),
			LoginRedirectURL:    strings.TrimSpace(idpParts[2]),
			RegisterRedirectURL: strings.TrimSpace(idpParts[3]),
			ClientID:            strings.TrimSpace(idpParts[4]),
			ClientSecret:        strings.TrimSpace(idpParts[5]),
		})
	}

	return nil
}

// Get returns the contents of the Value.
func (s *SliceIDPVar) Get() any {
	return *s
}

// IsBoolFlag returns true if the flag is a boolean flag
func (s *SliceIDPVar) IsBoolFlag() bool {
	return false
}

// Field is a generic configuration field for structs
// It holds the flag name, environment variable name, description, and value
// This should be used to define configuration fields in structs
type Field[T any] struct {

	// Value is the value of the configuration item
	Value T
	// FlagName is the name used for the command line flag
	FlagName string

	// FlagDescription is the description used for the command line flag
	FlagDescription string

	// EnVarName is the name used for the environment variable
	EnVarName string
}

// NewField creates a new configuration field
func NewField[T any](flagName string, enVarName string, flagDescription string, value T) Field[T] {
	ret := Field[T]{
		FlagName:        flagName,
		FlagDescription: flagDescription,
		EnVarName:       enVarName,
		Value:           value,
	}

	if enVarName != "" {
		ret.FlagDescription += "\nEnvVar: " + enVarName
	}

	return ret
}

// GetEnv retrieves the value of an environment variable or returns a default value
func GetEnv[T any](key string, defaultValue T) T {
	if value, exists := os.LookupEnv(key); exists {
		switch any(defaultValue).(type) {
		case string:
			return any(value).(T)
		case int:
			if intValue, err := strconv.Atoi(value); err == nil {
				return any(intValue).(T)
			}
		case time.Duration:
			if durationValue, err := time.ParseDuration(value); err == nil {
				return any(durationValue).(T)
			}
		case bool:
			if boolValue, err := strconv.ParseBool(value); err == nil {
				return any(boolValue).(T)
			}
		case int32:
			if intValue, err := strconv.ParseInt(value, 10, 32); err == nil {
				return any(int32(intValue)).(T)
			}
		case int64:
			if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
				return any(int64(intValue)).(T)
			}
		case float32:
			if floatValue, err := strconv.ParseFloat(value, 32); err == nil {
				return any(float32(floatValue)).(T)
			}
		case float64:
			if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
				return any(floatValue).(T)
			}
		case FileVar:
			// create a file using the value as the path
			file := &FileVar{
				Flag: os.O_APPEND | os.O_CREATE | os.O_WRONLY,
			}

			if err := file.Set(value); err != nil {
				return defaultValue
			}

			return any(*file).(T)
		case SliceStringVar:
			slices := SliceStringVar{}
			if err := slices.Set(value); err != nil {
				return defaultValue
			}

			return any(slices).(T)
		case SliceIDPVar:

			idps := SliceIDPVar{}
			if err := idps.Set(value); err != nil {
				return defaultValue
			}

			return any(idps).(T)

		default:
			return defaultValue
		}
	}

	return defaultValue
}

// SetEnvVarFromFile loads all .env files in the current current working directory and sets the key-value pairs in the environment
// if there are multiple .env files, it returns an error
func SetEnvVarFromFile() error {
	// Get the current working directory
	execDir, err := os.Getwd()
	if err != nil {
		return err
	}

	// Find all .env files in the execution directory
	envFiles, err := filepath.Glob(filepath.Join(execDir, "*.env"))
	if err != nil {
		return err
	}

	if len(envFiles) > 1 {
		return fmt.Errorf("multiple .env files found in the execution directory: %v", envFiles)
	}

	// Load each .env file
	for _, envFile := range envFiles {
		file, err := os.Open(envFile)
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			// Skip comments and empty lines
			if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") || strings.TrimSpace(line) == "" {
				continue
			}

			// trim leading and trailing whitespace
			line = strings.TrimSpace(line)

			// Split the line into key and value
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				slog.Warn("invalid line in .env file", "line", line)
				continue
			}

			key := strings.TrimSpace(parts[0])

			// remove quotes from value
			parts[1] = strings.Trim(parts[1], "\"")

			value := strings.TrimSpace(parts[1])

			// Set the environment variable
			slog.Debug("setting environment variable", "key", key, "value", value)
			if err := os.Setenv(key, value); err != nil {
				slog.Error("failed to set environment variable", "key", key, "value", value, "error", err)
				return err
			}

		}

		if err := scanner.Err(); err != nil {
			return err
		}
	}

	return nil
}

// LoadAndValidate is a convenience function that loads environment variables
// and validates multiple configuration objects in a single call.
//
// It calls ParseEnvVars() on each config that implements EnvVarsParser,
// then calls Validate() on each config that implements Validator.
//
// Returns the first validation error encountered, or nil if all configs are valid.
//
// Example usage:
//
//	dbCfg := NewDatabaseConfig()
//	cacheCfg := NewCacheConfig()
//	if err := LoadAndValidate(dbCfg, cacheCfg); err != nil {
//	    log.Fatal(err)
//	}
func LoadAndValidate(configs ...any) error {
	// First pass: Parse environment variables
	for _, cfg := range configs {
		if parser, ok := cfg.(EnvVarsParser); ok {
			parser.ParseEnvVars()
		}
	}

	// Second pass: Validate configurations
	for _, cfg := range configs {
		if validator, ok := cfg.(Validator); ok {
			if err := validator.Validate(); err != nil {
				return err
			}
		}
	}

	return nil
}
