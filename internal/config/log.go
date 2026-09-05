package config

import (
	"os"
	"slices"
	"strings"

	"github.com/slashdevops/go-rest-api-service-template/pkg/cslog"
)

const (
	// ValidLogLevel is what the flag help advertises.
	//
	// Validate also accepts "ctrace" and "cfatal" -- see ValidLogLevelHidden.
	ValidLogLevel = "debug|info|warn|error"

	// ValidLogLevelHidden are two extra levels Validate accepts but the help
	// does not advertise: ctrace is below debug (it logs every SQL query) and
	// cfatal is above error.
	//
	// They are named here rather than left as bare strings in Validate because
	// .air.toml RUNS ON ctrace -- so the dev stack uses a level the help text
	// calls invalid, and an operator copying the dev configuration hits a value
	// they cannot find documented anywhere.
	ValidLogLevelHidden = "ctrace|cfatal"
	ValidLogFormat      = "text|json"

	DefaultLogLevel     = "info"
	DefaultLogFormat    = "text"
	DefaultLogDebug     = false
	DefaultLogAddSource = false
)

// DefaultLogOutput is the default log output destination
var DefaultLogOutput = FileVar{os.Stdout, os.O_APPEND | os.O_CREATE | os.O_WRONLY}

// LogConfig is the configuration for the logger
type LogConfig struct {
	Level     Field[string]
	Format    Field[string]
	Output    Field[FileVar]
	Debug     Field[bool]
	AddSource Field[bool]
}

// NewLogConfig creates a new logger configuration
func NewLogConfig() *LogConfig {
	return &LogConfig{
		Level:     NewField("log.level", "LOG_LEVEL", "Log Level. Possible values ["+ValidLogLevel+"]. Also accepted, for development: ["+ValidLogLevelHidden+"] -- ctrace logs every SQL query", DefaultLogLevel),
		Format:    NewField("log.format", "LOG_FORMAT", "Log Format. Possible values ["+ValidLogFormat+"]", DefaultLogFormat),
		Output:    NewField("log.output", "LOG_OUTPUT", "Log Output", DefaultLogOutput),
		Debug:     NewField("debug", "DEBUG", "Debug mode. Short hand for log.level=debug", DefaultLogDebug),
		AddSource: NewField("log.add.source", "LOG_ADD_SOURCE", "Add source file and line number to log output", DefaultLogAddSource),
	}
}

// ParseEnvVars reads the logger configuration from environment variables
// and sets the values in the configuration
func (c *LogConfig) ParseEnvVars() {
	c.Level.Value = GetEnv(c.Level.EnVarName, c.Level.Value)
	c.Format.Value = GetEnv(c.Format.EnVarName, c.Format.Value)
	c.Output.Value = GetEnv(c.Output.EnVarName, c.Output.Value)
	c.Debug.Value = GetEnv(c.Debug.EnVarName, c.Debug.Value)
	c.AddSource.Value = GetEnv(c.AddSource.EnVarName, c.AddSource.Value)
}

// Validate validates the logger configuration values
func (c *LogConfig) Validate() error {
	if !slices.Contains(strings.Split(ValidLogLevel, "|"), c.Level.Value) {

		// hidden log level trace, this is not documented to users
		// but can be used for very verbose logging during development
		// used for logging sql queries for example
		if c.Level.Value == "ctrace" {
			c.Level.Value = cslog.LogLevelTrace.String()
			return nil
		}

		if c.Level.Value == "cfatal" {
			c.Level.Value = cslog.LogLevelFatal.String()
			return nil
		}

		return &InvalidConfigurationError{
			Field:   "log.level",
			Value:   c.Level.Value,
			Message: "Log level must be one of [" + ValidLogLevel + "]",
		}
	}

	if !slices.Contains(strings.Split(ValidLogFormat, "|"), c.Format.Value) {
		return &InvalidConfigurationError{
			Field:   "log.format",
			Value:   c.Format.Value,
			Message: "Log format must be one of [" + ValidLogFormat + "]",
		}
	}

	return nil
}
