package o11y

import (
	"context"

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

type OpenTelemetry struct {
	Traces  *OpenTelemetryTracer
	Metrics *OpenTelemetryMeter

	// Errors is what the SDK could not export. Both pipelines are batched and
	// asynchronous, so this is the only place an export failure is visible —
	// see [ExportErrors].
	Errors *ExportErrors
}

func New(ctx context.Context, conf *config.OpenTelemetryConfig) (*OpenTelemetry, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(conf.AttributeServiceName),
			semconv.ServiceVersionKey.String(conf.AttributeServiceVersion),
		),
	)
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

	op := &OpenTelemetry{
		Traces:  NewOpenTelemetryTracer(ctx, tracerConf),
		Metrics: NewOpenTelemetryMeter(ctx, meterConf),
		Errors:  SetGlobalErrorHandler(&ExportErrors{}),
	}

	return op, nil
}

func (ref *OpenTelemetry) Start() error {
	if err := ref.Traces.SetupTraces(); err != nil {
		return err
	}

	if err := ref.Metrics.SetupMetrics(); err != nil {
		return err
	}

	return nil
}

func (ref *OpenTelemetry) Shutdown() {
	ref.Traces.Shutdown()
	ref.Metrics.Shutdown()
}
