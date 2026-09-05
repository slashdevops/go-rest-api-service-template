package o11y

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
	"go.opentelemetry.io/otel"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/trace/noop"

	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	otelTrace "go.opentelemetry.io/otel/trace"
)

// OpenTelemetryTracerConfig represents the configuration of the OpenTelemetry tracer.
type OpenTelemetryTracerConfig struct {
	Resources                 *resource.Resource
	Name                      string
	TraceEndpoint             string
	TraceExporter             string
	TracePort                 int
	TraceExporterBatchTimeout time.Duration
	// TraceSampling is the head-sampling percentage (0-100). It is applied as a
	// ParentBased(TraceIDRatioBased) sampler so this service honours the
	// sampling decision of an upstream caller in a distributed trace.
	TraceSampling int
}

// OpenTelemetryTracer represents the traces of the service.
// It is used to collect and export traces using OpenTelemetry.
// It is initialized with the OpenTelemetryTracerConfig and provides methods to set up and shutdown the traces.
type OpenTelemetryTracer struct {
	ctx context.Context

	// Tracer is the OpenTelemetry trace tracer.
	Tracer otelTrace.Tracer

	// Resource is the OpenTelemetry resource.
	res *resource.Resource

	// TracerProvider is the OpenTelemetry trace provider.
	tp *trace.TracerProvider

	name string

	traceEndpoint             string
	traceExporter             string
	tracePort                 int
	traceExporterBatchTimeout time.Duration
	traceSampling             int
}

func NewOpenTelemetryTracer(ctx context.Context, conf *OpenTelemetryTracerConfig) *OpenTelemetryTracer {
	return &OpenTelemetryTracer{
		ctx:  ctx,
		name: conf.Name,

		traceEndpoint:             conf.TraceEndpoint,
		tracePort:                 conf.TracePort,
		traceExporter:             conf.TraceExporter,
		traceExporterBatchTimeout: conf.TraceExporterBatchTimeout,
		traceSampling:             conf.TraceSampling,

		res: conf.Resources,

		Tracer: otel.Tracer(conf.Name),
	}
}

func (ref *OpenTelemetryTracer) SetupTraces() error {
	// when testing, we don't want to set up the tracer
	if ref.traceExporter == config.ExporterNoop {
		slog.Warn("No tracer exporter configured, use 'noop' for testing purposes only")

		tp := noop.NewTracerProvider()

		otel.SetTracerProvider(tp)
		ref.Tracer = tp.Tracer(ref.name)

		return nil
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace exporter.
	tExp, err := ref.newTraceExporter(ref.ctx)
	if err != nil {
		return err
	}

	// Set up trace provider.
	tp, err := ref.newTraceProvider(tExp)
	if err != nil {
		return err
	}
	ref.tp = tp

	otel.SetTracerProvider(tp)
	ref.Tracer = tp.Tracer(ref.name)

	return nil
}

func (ref *OpenTelemetryTracer) Shutdown() {
	if ref.Tracer != nil {
		if ref.tp != nil {
			if err := ref.tp.Shutdown(ref.ctx); err != nil {
				slog.Error("failed to shutdown OpenTelemetry tracer", "error", err)
			}
		}
	}
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func (ref *OpenTelemetryTracer) newTraceExporter(ctx context.Context) (trace.SpanExporter, error) {
	var exporter trace.SpanExporter
	var err error

	switch ref.traceExporter {
	case "console":
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, err
		}
	case "otlp-http":
		insecureOpt := otlptracehttp.WithInsecure()
		withCompression := otlptracehttp.WithCompression(otlptracehttp.GzipCompression)
		endpointOpt := otlptracehttp.WithEndpoint(fmt.Sprintf("%s:%d", ref.traceEndpoint, ref.tracePort))
		exporter, err = otlptracehttp.New(ctx, insecureOpt, endpointOpt, withCompression)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown trace exporter: %s", ref.traceExporter)
	}

	return exporter, nil
}

func (ref *OpenTelemetryTracer) newTraceProvider(exp trace.SpanExporter) (*trace.TracerProvider, error) {
	// ParentBased honours the upstream caller's sampling decision in a
	// distributed trace; only root spans fall back to the ratio sampler.
	// TraceSampling is a 0-100 percentage.
	ratio := float64(ref.traceSampling) / 100.0
	sampler := trace.ParentBased(trace.TraceIDRatioBased(ratio))

	p := trace.NewTracerProvider(
		trace.WithResource(ref.res),
		trace.WithBatcher(
			exp,
			trace.WithBatchTimeout(ref.traceExporterBatchTimeout),
		),
		trace.WithSampler(sampler),
	)

	return p, nil
}
