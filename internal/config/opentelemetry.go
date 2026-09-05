package config

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	// ExporterNoop is the exporter that exports nothing. It is a value the
	// health check has to recognise -- "configured" and "exporting" are not
	// the same thing, and only this value tells them apart.
	ExporterNoop = "noop"

	// ExporterOTLPHTTP is the exporter that ships to a collector, and so the
	// only one with a host the health check can reach for.
	ExporterOTLPHTTP = "otlp-http"

	ValidTraceExporters    = "console|otlp-http|noop"
	ValidMetricExporters   = "console|otlp-http|prometheus|noop"
	ValidTaceSamplingMin   = 0
	ValidTaceSamplingMax   = 100
	ValidMetricMinInterval = 1 * time.Second
	ValidMetricMinPort     = 1
	ValidMetricMaxPort     = 65535
	ValidTraceMinPort      = 1
	ValidTraceMaxPort      = 65535

	DefaultTraceEndpoint             = "localhost"
	DefaultTracePort                 = 4318
	DefaultTraceExporter             = "console"
	DefaultTraceExporterBatchTimeout = 5 * time.Second
	DefaultTraceSampling             = 100
	DefaultMetricEndpoint            = "localhost"
	DefaultMetricPort                = 9090
	DefaultMetricExporter            = "console"
	DefaultMetricInterval            = 15 * time.Second
)

type OpenTelemetryConfig struct {
	TraceEndpoint Field[string]
	TraceExporter Field[string]

	MetricEndpoint Field[string]
	MetricExporter Field[string]

	AttributeServiceName      string
	AttributeServiceVersion   string
	TracePort                 Field[int]
	TraceExporterBatchTimeout Field[time.Duration]
	TraceSampling             Field[int]

	MetricPort     Field[int]
	MetricInterval Field[time.Duration]
}

func NewOpenTelemetryConfig(appName string, appVersion string) *OpenTelemetryConfig {
	return &OpenTelemetryConfig{
		TraceEndpoint:             NewField("opentelemetry.trace.endpoint", "OPENTELEMETRY_TRACE_ENDPOINT", "OpenTelemetry Endpoint to send traces to", DefaultTraceEndpoint),
		TracePort:                 NewField("opentelemetry.trace.port", "OPENTELEMETRY_TRACE_PORT", "OpenTelemetry Port to send traces to", DefaultTracePort),
		TraceExporter:             NewField("opentelemetry.trace.exporter", "OPENTELEMETRY_TRACE_EXPORTER", "OpenTelemetry Exporter to send traces to. Possible values ["+ValidTraceExporters+"]", DefaultTraceExporter),
		TraceExporterBatchTimeout: NewField("opentelemetry.trace.exporter.batch.timeout", "OPENTELEMETRY_TRACE_EXPORTER_BATCH_TIMEOUT", "OpenTelemetry Exporter Batch Timeout", DefaultTraceExporterBatchTimeout),
		TraceSampling:             NewField("opentelemetry.trace.sampling", "OPENTELEMETRY_TRACE_SAMPLING", "OpenTelemetry Exporter trace sampling", DefaultTraceSampling),

		MetricEndpoint: NewField("opentelemetry.metric.endpoint", "OPENTELEMETRY_METRIC_ENDPOINT", "OpenTelemetry Endpoint to send metrics to", DefaultMetricEndpoint),
		MetricPort:     NewField("opentelemetry.metric.port", "OPENTELEMETRY_METRIC_PORT", "OpenTelemetry Port to send metrics to", DefaultMetricPort),
		MetricExporter: NewField("opentelemetry.metric.exporter", "OPENTELEMETRY_METRIC_EXPORTER", "OpenTelemetry Exporter to send metrics to. Possible values ["+ValidMetricExporters+"]", DefaultMetricExporter),
		MetricInterval: NewField("opentelemetry.metric.interval", "OPENTELEMETRY_METRIC_INTERVAL", "OpenTelemetry Interval in to send metrics", DefaultMetricInterval),

		AttributeServiceVersion: appVersion,
		AttributeServiceName:    appName,
	}
}

// ParseEnvVars reads the OpenTracing configuration from environment variables
// and sets the values in the configuration
func (c *OpenTelemetryConfig) ParseEnvVars() {
	c.TraceEndpoint.Value = GetEnv(c.TraceEndpoint.EnVarName, c.TraceEndpoint.Value)
	c.TracePort.Value = GetEnv(c.TracePort.EnVarName, c.TracePort.Value)
	c.TraceExporter.Value = GetEnv(c.TraceExporter.EnVarName, c.TraceExporter.Value)
	c.TraceExporterBatchTimeout.Value = GetEnv(c.TraceExporterBatchTimeout.EnVarName, c.TraceExporterBatchTimeout.Value)
	c.TraceSampling.Value = GetEnv(c.TraceSampling.EnVarName, c.TraceSampling.Value)

	c.MetricEndpoint.Value = GetEnv(c.MetricEndpoint.EnVarName, c.MetricEndpoint.Value)
	c.MetricPort.Value = GetEnv(c.MetricPort.EnVarName, c.MetricPort.Value)
	c.MetricExporter.Value = GetEnv(c.MetricExporter.EnVarName, c.MetricExporter.Value)
	c.MetricInterval.Value = GetEnv(c.MetricInterval.EnVarName, c.MetricInterval.Value)
}

// Validate validates the OpenTracing configuration values
func (c *OpenTelemetryConfig) Validate() error {
	if !slices.Contains(strings.Split(ValidTraceExporters, "|"), c.TraceExporter.Value) {
		return &InvalidConfigurationError{
			Field:   "opentelemetry.trace.exporter",
			Value:   c.TraceExporter.Value,
			Message: "invalid trace exporter, must be one of [" + ValidTraceExporters + "]",
		}
	}

	if !slices.Contains(strings.Split(ValidMetricExporters, "|"), c.MetricExporter.Value) {
		return &InvalidConfigurationError{
			Field:   "opentelemetry.metric.exporter",
			Value:   c.MetricExporter.Value,
			Message: "invalid metric exporter, must be one of [" + ValidMetricExporters + "]",
		}
	}

	if c.TraceSampling.Value < ValidTaceSamplingMin || c.TraceSampling.Value > ValidTaceSamplingMax {
		return &InvalidConfigurationError{
			Field:   "opentelemetry.trace.sampling",
			Value:   strconv.Itoa(c.TraceSampling.Value),
			Message: "invalid sampling, must be between [" + strconv.Itoa(ValidTaceSamplingMin) + "] and [" + strconv.Itoa(ValidTaceSamplingMax) + "]",
		}
	}

	if c.MetricInterval.Value < ValidMetricMinInterval {
		return &InvalidConfigurationError{
			Field:   "opentelemetry.metric.interval",
			Value:   c.MetricInterval.Value.String(),
			Message: "invalid metric interval, must be greater than [" + ValidMetricMinInterval.String() + "]",
		}
	}

	if c.MetricPort.Value < ValidMetricMinPort || c.MetricPort.Value > ValidMetricMaxPort {
		return &InvalidConfigurationError{
			Field:   "opentelemetry.metric.port",
			Value:   strconv.Itoa(c.MetricPort.Value),
			Message: "invalid metric port, must be between [" + strconv.Itoa(ValidMetricMinPort) + "] and [" + strconv.Itoa(ValidMetricMaxPort) + "]",
		}
	}

	if c.TracePort.Value < ValidTraceMinPort || c.TracePort.Value > ValidTraceMaxPort {
		return &InvalidConfigurationError{
			Field:   "opentelemetry.trace.port",
			Value:   strconv.Itoa(c.TracePort.Value),
			Message: "invalid trace port, must be between [" + strconv.Itoa(ValidTraceMinPort) + "] and [" + strconv.Itoa(ValidTraceMaxPort) + "]",
		}
	}

	return nil
}
