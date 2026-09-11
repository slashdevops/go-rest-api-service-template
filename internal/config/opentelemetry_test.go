package config

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestNewOpenTelemetryConfig(t *testing.T) {
	appName := "test-app"
	appVersion := "1.0.0"
	config := NewOpenTelemetryConfig(appName, appVersion)

	if config.TraceEndpoint.Value != DefaultTraceEndpoint {
		t.Errorf("Expected TraceEndpoint to be %s, got %s", DefaultTraceEndpoint, config.TraceEndpoint.Value)
	}
	if config.TracePort.Value != DefaultTracePort {
		t.Errorf("Expected TracePort to be %d, got %d", DefaultTracePort, config.TracePort.Value)
	}
	if config.TraceExporter.Value != DefaultTraceExporter {
		t.Errorf("Expected TraceExporter to be %s, got %s", DefaultTraceExporter, config.TraceExporter.Value)
	}
	if config.TraceExporterBatchTimeout.Value != DefaultTraceExporterBatchTimeout {
		t.Errorf("Expected TraceExporterBatchTimeout to be %v, got %v", DefaultTraceExporterBatchTimeout, config.TraceExporterBatchTimeout.Value)
	}
	if config.TraceSampling.Value != DefaultTraceSampling {
		t.Errorf("Expected TraceSampling to be %d, got %d", DefaultTraceSampling, config.TraceSampling.Value)
	}
	if config.MetricEndpoint.Value != DefaultMetricEndpoint {
		t.Errorf("Expected MetricEndpoint to be %s, got %s", DefaultMetricEndpoint, config.MetricEndpoint.Value)
	}
	if config.MetricPort.Value != DefaultMetricPort {
		t.Errorf("Expected MetricPort to be %d, got %d", DefaultMetricPort, config.MetricPort.Value)
	}
	if config.MetricExporter.Value != DefaultMetricExporter {
		t.Errorf("Expected MetricExporter to be %s, got %s", DefaultMetricExporter, config.MetricExporter.Value)
	}
	if config.MetricInterval.Value != DefaultMetricInterval {
		t.Errorf("Expected MetricInterval to be %v, got %v", DefaultMetricInterval, config.MetricInterval.Value)
	}
	if config.AttributeServiceName != appName {
		t.Errorf("Expected AttributeServiceName to be %s, got %s", appName, config.AttributeServiceName)
	}
	if config.AttributeServiceVersion != appVersion {
		t.Errorf("Expected AttributeServiceVersion to be %s, got %s", appVersion, config.AttributeServiceVersion)
	}
}

func TestParseEnvVars_opentelemetry(t *testing.T) {
	os.Setenv("OPENTELEMETRY_TRACE_ENDPOINT", "trace.example.com")
	os.Setenv("OPENTELEMETRY_TRACE_PORT", "4317")
	os.Setenv("OPENTELEMETRY_TRACE_EXPORTER", "otlp-http")
	os.Setenv("OPENTELEMETRY_TRACE_EXPORTER_BATCH_TIMEOUT", "10s")
	os.Setenv("OPENTELEMETRY_TRACE_SAMPLING", "50")
	os.Setenv("OPENTELEMETRY_METRIC_ENDPOINT", "metric.example.com")
	os.Setenv("OPENTELEMETRY_METRIC_PORT", "8080")
	os.Setenv("OPENTELEMETRY_METRIC_EXPORTER", "prometheus")
	os.Setenv("OPENTELEMETRY_METRIC_INTERVAL", "30s")

	config := NewOpenTelemetryConfig("test-app", "1.0.0")
	config.ParseEnvVars()

	if config.TraceEndpoint.Value != "trace.example.com" {
		t.Errorf("Expected TraceEndpoint to be trace.example.com, got %s", config.TraceEndpoint.Value)
	}
	if config.TracePort.Value != 4317 {
		t.Errorf("Expected TracePort to be 4317, got %d", config.TracePort.Value)
	}
	if config.TraceExporter.Value != "otlp-http" {
		t.Errorf("Expected TraceExporter to be otlp-http, got %s", config.TraceExporter.Value)
	}
	if config.TraceExporterBatchTimeout.Value != 10*time.Second {
		t.Errorf("Expected TraceExporterBatchTimeout to be 10s, got %v", config.TraceExporterBatchTimeout.Value)
	}
	if config.TraceSampling.Value != 50 {
		t.Errorf("Expected TraceSampling to be 50, got %d", config.TraceSampling.Value)
	}
	if config.MetricEndpoint.Value != "metric.example.com" {
		t.Errorf("Expected MetricEndpoint to be metric.example.com, got %s", config.MetricEndpoint.Value)
	}
	if config.MetricPort.Value != 8080 {
		t.Errorf("Expected MetricPort to be 8080, got %d", config.MetricPort.Value)
	}
	if config.MetricExporter.Value != "prometheus" {
		t.Errorf("Expected MetricExporter to be prometheus, got %s", config.MetricExporter.Value)
	}
	if config.MetricInterval.Value != 30*time.Second {
		t.Errorf("Expected MetricInterval to be 30s, got %v", config.MetricInterval.Value)
	}

	// Clean up environment variables
	os.Unsetenv("OPENTELEMETRY_TRACE_ENDPOINT")
	os.Unsetenv("OPENTELEMETRY_TRACE_PORT")
	os.Unsetenv("OPENTELEMETRY_TRACE_EXPORTER")
	os.Unsetenv("OPENTELEMETRY_TRACE_EXPORTER_BATCH_TIMEOUT")
	os.Unsetenv("OPENTELEMETRY_TRACE_SAMPLING")
	os.Unsetenv("OPENTELEMETRY_METRIC_ENDPOINT")
	os.Unsetenv("OPENTELEMETRY_METRIC_PORT")
	os.Unsetenv("OPENTELEMETRY_METRIC_EXPORTER")
	os.Unsetenv("OPENTELEMETRY_METRIC_INTERVAL")
}

func TestValidate_opentelemetry(t *testing.T) {
	config := NewOpenTelemetryConfig("test-app", "1.0.0")

	// Test valid configuration
	err := config.Validate()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test invalid trace exporter
	config.TraceExporter.Value = "invalid"
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "opentelemetry.trace.exporter" {
		t.Errorf("Expected InvalidConfigurationError with field 'opentelemetry.trace.exporter', got %v", err)
	}
	config.TraceExporter.Value = DefaultTraceExporter

	// Test invalid metric exporter
	config.MetricExporter.Value = "invalid"
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "opentelemetry.metric.exporter" {
		t.Errorf("Expected InvalidConfigurationError with field 'opentelemetry.metric.exporter', got %v", err)
	}
	config.MetricExporter.Value = DefaultMetricExporter

	// Test invalid trace sampling (too low)
	config.TraceSampling.Value = -1
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "opentelemetry.trace.sampling" {
		t.Errorf("Expected InvalidConfigurationError with field 'opentelemetry.trace.sampling', got %v", err)
	}

	// Test invalid trace sampling (too high)
	config.TraceSampling.Value = 101
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "opentelemetry.trace.sampling" {
		t.Errorf("Expected InvalidConfigurationError with field 'opentelemetry.trace.sampling', got %v", err)
	}
	config.TraceSampling.Value = DefaultTraceSampling

	// Test invalid metric interval (too short)
	config.MetricInterval.Value = 500 * time.Millisecond
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "opentelemetry.metric.interval" {
		t.Errorf("Expected InvalidConfigurationError with field 'opentelemetry.metric.interval', got %v", err)
	}
	config.MetricInterval.Value = DefaultMetricInterval

	// Test invalid metric port (too low)
	config.MetricPort.Value = 0
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "opentelemetry.metric.port" {
		t.Errorf("Expected InvalidConfigurationError with field 'opentelemetry.metric.port', got %v", err)
	}
	config.MetricPort.Value = DefaultMetricPort

	// Test invalid trace port (too high)
	config.TracePort.Value = 99999
	err = config.Validate()
	if invalidErr, ok := errors.AsType[*InvalidConfigurationError](err); err == nil || !ok || invalidErr.Field != "opentelemetry.trace.port" {
		t.Errorf("Expected InvalidConfigurationError with field 'opentelemetry.trace.port', got %v", err)
	}
	config.TracePort.Value = DefaultTracePort
}

func newValidLogTelemetryConfig() *OpenTelemetryConfig {
	c := NewOpenTelemetryConfig("test-app", "0.0.0-test")
	c.LogExporter.Value = ExporterOTLPHTTP

	return c
}

func TestNewOpenTelemetryConfigLogDefaults(t *testing.T) {
	c := NewOpenTelemetryConfig("test-app", "0.0.0-test")

	// noop is the one default that differs from the other two signals, and it
	// is what makes this setting invisible to an existing deployment: the
	// standard logger keeps writing to log.output and nothing is exported.
	if c.LogExporter.Value != DefaultLogExporter {
		t.Errorf("LogExporter = %q, want %q", c.LogExporter.Value, DefaultLogExporter)
	}
	if c.LogExporter.Value != ExporterNoop {
		t.Errorf("the log exporter must default to %q so an upgrade changes nothing, got %q", ExporterNoop, c.LogExporter.Value)
	}
	if c.LogEndpoint.Value != DefaultLogEndpoint {
		t.Errorf("LogEndpoint = %q, want %q", c.LogEndpoint.Value, DefaultLogEndpoint)
	}
	if c.LogPort.Value != DefaultLogPort {
		t.Errorf("LogPort = %d, want %d", c.LogPort.Value, DefaultLogPort)
	}
	if c.LogPath.Value != DefaultLogPath {
		t.Errorf("LogPath = %q, want %q", c.LogPath.Value, DefaultLogPath)
	}
	if c.LogExporterBatchTimeout.Value != DefaultLogExporterBatchTimeout {
		t.Errorf("LogExporterBatchTimeout = %v, want %v", c.LogExporterBatchTimeout.Value, DefaultLogExporterBatchTimeout)
	}
}

func TestValidateLogExporter(t *testing.T) {
	for _, exporter := range []string{"noop", "console", "otlp-http"} {
		t.Run("accepts_"+exporter, func(t *testing.T) {
			c := NewOpenTelemetryConfig("test-app", "0.0.0-test")
			c.LogExporter.Value = exporter

			if err := c.Validate(); err != nil {
				t.Errorf("Validate() = %v, want nil for exporter %q", err, exporter)
			}
		})
	}

	t.Run("refuses_unknown", func(t *testing.T) {
		c := NewOpenTelemetryConfig("test-app", "0.0.0-test")
		c.LogExporter.Value = "loki"

		err := c.Validate()
		if err == nil {
			t.Fatal("Validate() = nil, want an error for an unknown exporter")
		}

		var invalid *InvalidConfigurationError
		if !errors.As(err, &invalid) {
			t.Fatalf("Validate() = %T, want *InvalidConfigurationError", err)
		}

		// The error has to name the setting to change, not the value's
		// neighbour: this is the setting that selects the behaviour.
		if invalid.Field != "opentelemetry.log.exporter" {
			t.Errorf("Field = %q, want opentelemetry.log.exporter", invalid.Field)
		}
	})
}

// Port, path and batch timeout are only read when something is dialled, so
// only otlp-http validates them. Refusing to start over a value that will
// never be read would send an operator to fix a setting with no effect.
func TestValidateLogSettingsOnlyApplyToOTLP(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*OpenTelemetryConfig)
	}{
		{"empty_endpoint", func(c *OpenTelemetryConfig) { c.LogEndpoint.Value = "" }},
		{"port_zero", func(c *OpenTelemetryConfig) { c.LogPort.Value = 0 }},
		{"port_too_high", func(c *OpenTelemetryConfig) { c.LogPort.Value = 65536 }},
		{"path_without_leading_slash", func(c *OpenTelemetryConfig) { c.LogPath.Value = "otlp/v1/logs" }},
		{"batch_timeout_too_small", func(c *OpenTelemetryConfig) { c.LogExporterBatchTimeout.Value = time.Millisecond }},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_refused_on_otlp", func(t *testing.T) {
			c := newValidLogTelemetryConfig()
			tt.mutate(c)

			if err := c.Validate(); err == nil {
				t.Error("Validate() = nil, want an error")
			}
		})

		t.Run(tt.name+"_ignored_on_noop", func(t *testing.T) {
			c := NewOpenTelemetryConfig("test-app", "0.0.0-test")
			tt.mutate(c)

			if err := c.Validate(); err != nil {
				t.Errorf("Validate() = %v, want nil: the value is never read with the noop exporter", err)
			}
		})
	}
}

func TestParseEnvVarsLogs(t *testing.T) {
	t.Setenv("OPENTELEMETRY_LOG_ENDPOINT", "loki.example.com")
	t.Setenv("OPENTELEMETRY_LOG_PORT", "3200")
	t.Setenv("OPENTELEMETRY_LOG_EXPORTER", "otlp-http")
	t.Setenv("OPENTELEMETRY_LOG_PATH", "/v1/logs")
	t.Setenv("OPENTELEMETRY_LOG_EXPORTER_BATCH_TIMEOUT", "9s")

	c := NewOpenTelemetryConfig("test-app", "0.0.0-test")
	c.ParseEnvVars()

	if c.LogEndpoint.Value != "loki.example.com" {
		t.Errorf("LogEndpoint = %q", c.LogEndpoint.Value)
	}
	if c.LogPort.Value != 3200 {
		t.Errorf("LogPort = %d", c.LogPort.Value)
	}
	if c.LogExporter.Value != "otlp-http" {
		t.Errorf("LogExporter = %q", c.LogExporter.Value)
	}
	if c.LogPath.Value != "/v1/logs" {
		t.Errorf("LogPath = %q", c.LogPath.Value)
	}
	if c.LogExporterBatchTimeout.Value != 9*time.Second {
		t.Errorf("LogExporterBatchTimeout = %v", c.LogExporterBatchTimeout.Value)
	}
}
