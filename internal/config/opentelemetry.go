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
	ValidLogExporters      = "console|otlp-http|noop"
	ValidMetricExporters   = "console|otlp-http|prometheus|noop"
	ValidTaceSamplingMin   = 0
	ValidTaceSamplingMax   = 100
	ValidMetricMinInterval = 1 * time.Second
	ValidMetricMinPort     = 1
	ValidMetricMaxPort     = 65535
	ValidTraceMinPort      = 1
	ValidTraceMaxPort      = 65535
	ValidLogMinPort        = 1
	ValidLogMaxPort        = 65535

	// ValidLogMinExporterBatchTimeout bounds how long the log pipeline may sit
	// on a batch. It is the same floor the metric interval has, for the same
	// reason: below a second the exporter spends more time shipping than the
	// process spends producing.
	ValidLogMinExporterBatchTimeout = 1 * time.Second

	DefaultTraceEndpoint             = "localhost"
	DefaultTracePort                 = 4318
	DefaultTraceExporter             = "console"
	DefaultTraceExporterBatchTimeout = 5 * time.Second
	DefaultTraceSampling             = 100
	DefaultMetricEndpoint            = "localhost"
	DefaultMetricPort                = 9090
	DefaultMetricExporter            = "console"
	DefaultMetricInterval            = 15 * time.Second

	// DefaultLogExporter is "noop", and it is the only one of the three
	// signals that defaults to shipping nothing.
	//
	// Traces and metrics default to "console" because without an exporter
	// there is nowhere at all to see them. Logs already have somewhere: every
	// record reaches the standard logger on log.output, and it keeps doing so
	// whatever this is set to. A "console" default would therefore print every
	// line a second time, in a different shape, to the same terminal.
	//
	// "noop" is also exactly the behaviour of every deployment that existed
	// before this setting, so an upgrade changes nothing until an operator
	// asks for it.
	DefaultLogExporter = "noop"

	// DefaultLogEndpoint and DefaultLogPort point at a Loki in the development
	// pod. 3100 is Loki's HTTP port; the OTLP receiver lives on it, under
	// DefaultLogPath.
	DefaultLogEndpoint = "localhost"
	DefaultLogPort     = 3100

	// DefaultLogPath is Loki's OTLP endpoint. It is a setting rather than a
	// constant in the exporter because the path is the one thing that differs
	// between the two things this service can ship logs to: Loki serves
	// /otlp/v1/logs, an OpenTelemetry Collector serves /v1/logs.
	DefaultLogPath = "/otlp/v1/logs"

	// DefaultLogExporterBatchTimeout is how long the log pipeline may sit on a
	// batch before shipping it. It is deliberately its own constant with the
	// same value as the trace one: sharing a constant between two settings
	// makes them impossible to move apart, and the two pipelines have no
	// reason to stay equal.
	DefaultLogExporterBatchTimeout = 5 * time.Second

	// DefaultEnvironment is empty, and empty means the attribute is not
	// emitted at all.
	//
	// There is no honest default. "development" would label a production
	// replica as development until somebody noticed, and "production" would do
	// the reverse; both are worse than saying nothing, because a wrong label is
	// acted on and a missing one is asked about. An operator who wants the
	// dimension sets it, and until then every series simply lacks it.
	DefaultEnvironment = ""

	// ValidEnvironmentMaxLength bounds the value. It becomes a Loki LABEL, so
	// it is an index key; a long one is a large index entry repeated on every
	// stream.
	ValidEnvironmentMaxLength = 63
)

type OpenTelemetryConfig struct {
	TraceEndpoint Field[string]
	TraceExporter Field[string]

	MetricEndpoint Field[string]
	MetricExporter Field[string]

	LogEndpoint Field[string]
	LogExporter Field[string]
	LogPath     Field[string]

	// Environment is deployment.environment.name: which deployment this is.
	//
	// It is a resource attribute, so it reaches all three signals at once, and
	// Loki promotes it to a label -- which is the point. One Grafana serving a
	// development stack and a production one cannot otherwise tell them apart,
	// and their series are summed together with nothing saying so.
	Environment Field[string]

	AttributeServiceName      string
	AttributeServiceVersion   string
	TracePort                 Field[int]
	TraceExporterBatchTimeout Field[time.Duration]
	TraceSampling             Field[int]

	MetricPort     Field[int]
	MetricInterval Field[time.Duration]

	LogPort                 Field[int]
	LogExporterBatchTimeout Field[time.Duration]
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

		LogEndpoint:             NewField("opentelemetry.log.endpoint", "OPENTELEMETRY_LOG_ENDPOINT", "OpenTelemetry Endpoint to send logs to", DefaultLogEndpoint),
		LogPort:                 NewField("opentelemetry.log.port", "OPENTELEMETRY_LOG_PORT", "OpenTelemetry Port to send logs to", DefaultLogPort),
		LogExporter:             NewField("opentelemetry.log.exporter", "OPENTELEMETRY_LOG_EXPORTER", "OpenTelemetry Exporter to send logs to. Possible values ["+ValidLogExporters+"]. The standard logger keeps writing to log.output whichever is chosen; noop means only it does", DefaultLogExporter),
		LogPath:                 NewField("opentelemetry.log.path", "OPENTELEMETRY_LOG_PATH", "OpenTelemetry HTTP path the logs exporter posts to. Loki serves /otlp/v1/logs, an OpenTelemetry Collector serves /v1/logs", DefaultLogPath),
		LogExporterBatchTimeout: NewField("opentelemetry.log.exporter.batch.timeout", "OPENTELEMETRY_LOG_EXPORTER_BATCH_TIMEOUT", "OpenTelemetry Exporter Batch Timeout for logs", DefaultLogExporterBatchTimeout),

		Environment: NewField("opentelemetry.environment", "OPENTELEMETRY_ENVIRONMENT", "Deployment environment reported as deployment.environment.name on every trace, metric and log. Empty means the attribute is omitted; there is no safe default, because a wrong one is acted on", DefaultEnvironment),

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

	c.Environment.Value = GetEnv(c.Environment.EnVarName, c.Environment.Value)

	c.LogEndpoint.Value = GetEnv(c.LogEndpoint.EnVarName, c.LogEndpoint.Value)
	c.LogPort.Value = GetEnv(c.LogPort.EnVarName, c.LogPort.Value)
	c.LogExporter.Value = GetEnv(c.LogExporter.EnVarName, c.LogExporter.Value)
	c.LogPath.Value = GetEnv(c.LogPath.EnVarName, c.LogPath.Value)
	c.LogExporterBatchTimeout.Value = GetEnv(c.LogExporterBatchTimeout.EnVarName, c.LogExporterBatchTimeout.Value)
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

	// Bounded because it becomes a Loki label, which is an index key repeated
	// on every stream. An empty value is fine: it means the attribute is
	// omitted.
	if len(c.Environment.Value) > ValidEnvironmentMaxLength {
		return &InvalidConfigurationError{
			Field:   "opentelemetry.environment",
			Value:   c.Environment.Value,
			Message: "invalid environment, must be at most " + strconv.Itoa(ValidEnvironmentMaxLength) + " characters; it becomes an index label",
		}
	}

	if err := c.validateLogs(); err != nil {
		return err
	}

	return nil
}

// validateLogs checks the log-exporter settings.
//
// Everything but the exporter itself is checked ONLY when the exporter is
// otlp-http. A port or a path is meaningless for noop and console -- nothing
// is dialled -- and refusing to start over a value that will never be read
// would make an operator fix a setting that has no effect on their
// deployment.
func (c *OpenTelemetryConfig) validateLogs() error {
	if !slices.Contains(strings.Split(ValidLogExporters, "|"), c.LogExporter.Value) {
		return &InvalidConfigurationError{
			Field:   "opentelemetry.log.exporter",
			Value:   c.LogExporter.Value,
			Message: "invalid log exporter, must be one of [" + ValidLogExporters + "]",
		}
	}

	if c.LogExporter.Value != ExporterOTLPHTTP {
		return nil
	}

	if c.LogEndpoint.Value == "" {
		return &InvalidConfigurationError{
			Field:   "opentelemetry.log.endpoint",
			Value:   c.LogEndpoint.Value,
			Message: "invalid log endpoint, must not be empty when opentelemetry.log.exporter is [" + ExporterOTLPHTTP + "]",
		}
	}

	if c.LogPort.Value < ValidLogMinPort || c.LogPort.Value > ValidLogMaxPort {
		return &InvalidConfigurationError{
			Field:   "opentelemetry.log.port",
			Value:   strconv.Itoa(c.LogPort.Value),
			Message: "invalid log port, must be between [" + strconv.Itoa(ValidLogMinPort) + "] and [" + strconv.Itoa(ValidLogMaxPort) + "]",
		}
	}

	// A path without a leading slash produces a URL that silently posts to the
	// wrong place: "http://host:3100" + "otlp/v1/logs" is
	// "http://host:3100otlp/v1/logs", which fails at dial time with a message
	// about the host, not about the path.
	if !strings.HasPrefix(c.LogPath.Value, "/") {
		return &InvalidConfigurationError{
			Field:   "opentelemetry.log.path",
			Value:   c.LogPath.Value,
			Message: "invalid log path, must start with [/]",
		}
	}

	if c.LogExporterBatchTimeout.Value < ValidLogMinExporterBatchTimeout {
		return &InvalidConfigurationError{
			Field:   "opentelemetry.log.exporter.batch.timeout",
			Value:   c.LogExporterBatchTimeout.Value.String(),
			Message: "invalid log exporter batch timeout, must be greater than [" + ValidLogMinExporterBatchTimeout.String() + "]",
		}
	}

	return nil
}
