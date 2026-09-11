package o11y

import (
	"context"
	"uuid"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
)

type OpenTelemetryTracerService interface {
	SetupTraces() error
	Shutdown()
}

type OpenTelemetryMeterService interface {
	SetupMetrics() error
	Shutdown()
}

type OpenTelemetryLoggerService interface {
	SetupLogs() error
	Shutdown()
}

type OpenTelemetry struct {
	Traces  *OpenTelemetryTracer
	Metrics *OpenTelemetryMeter

	// Logs is the third pipeline. Unlike the other two it does not own its
	// signal: the standard logger keeps writing to log.output regardless, and
	// this only adds a second sink. Its Handler is nil when the exporter is
	// noop, which is the default.
	Logs *OpenTelemetryLogger

	// Errors is what the SDK could not export. Both pipelines are batched and
	// asynchronous, so this is the only place an export failure is visible —
	// see [ExportErrors].
	Errors *ExportErrors
}

// New builds the three pipelines from configuration. logConf is needed because
// the log pipeline's minimum severity is the application's log level -- there is
// no separate knob for it, by design; see [ExportedLogFloor].
func New(ctx context.Context, conf *config.OpenTelemetryConfig, logConf *config.LogConfig) (*OpenTelemetry, error) {
	res, err := newResource(ctx, conf)
	if err != nil {
		return nil, err
	}

	tracerConf := &OpenTelemetryTracerConfig{
		Name:                      conf.AttributeServiceName,
		Resources:                 res,
		TraceEndpoint:             conf.TraceEndpoint.Value,
		TracePort:                 conf.TracePort.Value,
		TraceExporter:             conf.TraceExporter.Value,
		TraceExporterBatchTimeout: conf.TraceExporterBatchTimeout.Value,
		TraceSampling:             conf.TraceSampling.Value,
	}

	meterConf := &OpenTelemetryMeterConfig{
		Name:           conf.AttributeServiceName,
		Resources:      res,
		MetricEndpoint: conf.MetricEndpoint.Value,
		MetricPort:     conf.MetricPort.Value,
		MetricExporter: conf.MetricExporter.Value,
		MetricInterval: conf.MetricInterval.Value,
	}

	loggerConf := &OpenTelemetryLoggerConfig{
		Name:                    conf.AttributeServiceName,
		Resources:               res,
		LogEndpoint:             conf.LogEndpoint.Value,
		LogPort:                 conf.LogPort.Value,
		LogExporter:             conf.LogExporter.Value,
		LogPath:                 conf.LogPath.Value,
		LogExporterBatchTimeout: conf.LogExporterBatchTimeout.Value,
		Level:                   logConf.SlogLevel(),
		AddSource:               logConf.AddSource.Value,
	}

	op := &OpenTelemetry{
		Traces:  NewOpenTelemetryTracer(ctx, tracerConf),
		Metrics: NewOpenTelemetryMeter(ctx, meterConf),
		Logs:    NewOpenTelemetryLogger(ctx, loggerConf),
		Errors:  SetGlobalErrorHandler(&ExportErrors{}),
	}

	return op, nil
}

// Start brings up the three pipelines.
//
// Logs go LAST so that a record emitted while the tracer and meter are being
// built has a tracer to be correlated against rather than an invalid span
// context baked into it.
// newResource builds the attributes every signal carries.
//
// A resource attribute is set once and attached to every trace, metric and log
// the process produces, which makes it the right home for the four facts that
// identify a replica -- and the wrong home for anything per-request, since
// Loki promotes resource attributes to LABELS and a label is an index entry.
//
// # service.instance.id, and why it is set here
//
// The SDK only sets it behind an experimental flag, so it was absent. That was
// not visible from the code: the dashboards carry an `instance` variable keyed
// on it, and a variable over a label nothing sets resolves to empty, while
// `service_instance_id=~""` still matches every series. So the panels worked,
// the variable was dead, and nothing said so. It was found by asking Prometheus
// which service_* labels it actually had.
//
// It is a fresh uuid per process, which is what the specification asks for: a
// restarted process IS a new instance, and an id that survived a restart would
// make two different processes' series look like one. host.name is set
// alongside it for the friendly name -- in Kubernetes that is the pod.
func newResource(ctx context.Context, conf *config.OpenTelemetryConfig) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String(conf.AttributeServiceName),
		semconv.ServiceVersionKey.String(conf.AttributeServiceVersion),
		semconv.ServiceInstanceID(uuid.NewV7().String()),
	}

	// Empty means the operator did not say, and saying nothing is better than
	// guessing: a wrong environment label is acted on, a missing one is asked
	// about.
	if conf.Environment.Value != "" {
		attrs = append(attrs, semconv.DeploymentEnvironmentNameKey.String(conf.Environment.Value))
	}

	detected, err := resource.New(ctx,
		resource.WithHost(),
		resource.WithProcessPID(),
		resource.WithAttributes(attrs...),
	)
	if err != nil {
		// A partial resource is returned alongside the error when only some
		// detectors failed -- a container without a readable hostname, say.
		// Losing host.name must not stop the service from starting, so the
		// partial resource is kept and the reason is reported through the SDK's
		// own handler rather than swallowed.
		if detected == nil {
			return nil, err
		}

		otel.Handle(err)
	}

	// Merge, not New: resource.Default() carries the telemetry.sdk.* attributes
	// and honours OTEL_RESOURCE_ATTRIBUTES, which is how a deployment adds its
	// own without a code change.
	return resource.Merge(resource.Default(), detected)
}

func (ref *OpenTelemetry) Start() error {
	if err := ref.Traces.SetupTraces(); err != nil {
		return err
	}

	if err := ref.Metrics.SetupMetrics(); err != nil {
		return err
	}

	if err := ref.Logs.SetupLogs(); err != nil {
		return err
	}

	// After the meter exists, because it registers an instrument on it.
	if ref.Errors != nil {
		if err := ref.Errors.RegisterMetrics(ref.Metrics.Meter); err != nil {
			return err
		}
	}

	return nil
}

// Shutdown flushes the three pipelines.
//
// Logs go FIRST, the mirror of Start: the log provider's flush is the last
// thing that can produce a span, and shutting the tracer first would leave that
// flush with a provider that has already stopped. Records written after this
// returns reach the standard logger only, which is why "shutting down
// telemetry" is logged before it is called and not after.
func (ref *OpenTelemetry) Shutdown() {
	ref.Logs.Shutdown()
	ref.Traces.Shutdown()
	ref.Metrics.Shutdown()
}
