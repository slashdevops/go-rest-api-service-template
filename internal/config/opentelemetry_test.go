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
