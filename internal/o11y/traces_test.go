package o11y

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

func newTestResource(t *testing.T) *resource.Resource {
	t.Helper()
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("test-service"),
			semconv.ServiceVersionKey.String("0.0.1"),
		),
	)
	if err != nil {
		t.Fatalf("failed to create test resource: %v", err)
	}
	return res
}

func TestNewOpenTelemetryTracer(t *testing.T) {
	t.Parallel()

	res := newTestResource(t)
	conf := &OpenTelemetryTracerConfig{
		Name:                      "test-tracer",
		Resources:                 res,
		TraceEndpoint:             "localhost",
		TracePort:                 4318,
		TraceExporter:             "noop",
		TraceExporterBatchTimeout: 5 * time.Second,
	}

	tracer := NewOpenTelemetryTracer(context.Background(), conf)

	if tracer == nil {
		t.Fatal("expected non-nil tracer")
	}
	if tracer.name != "test-tracer" {
		t.Errorf("name = %q, want %q", tracer.name, "test-tracer")
	}
	if tracer.traceEndpoint != "localhost" {
		t.Errorf("traceEndpoint = %q, want %q", tracer.traceEndpoint, "localhost")
	}
	if tracer.tracePort != 4318 {
		t.Errorf("tracePort = %d, want %d", tracer.tracePort, 4318)
	}
	if tracer.traceExporter != "noop" {
		t.Errorf("traceExporter = %q, want %q", tracer.traceExporter, "noop")
	}
	if tracer.Tracer == nil {
		t.Fatal("expected non-nil Tracer")
	}
}

func TestOpenTelemetryTracer_SetupTraces_noop(t *testing.T) {
	t.Parallel()

	res := newTestResource(t)
	tracer := NewOpenTelemetryTracer(context.Background(), &OpenTelemetryTracerConfig{
		Name:          "test",
		Resources:     res,
		TraceExporter: "noop",
	})

	err := tracer.SetupTraces()
	if err != nil {
		t.Fatalf("SetupTraces() with noop exporter should not fail: %v", err)
	}

	if tracer.Tracer == nil {
		t.Fatal("expected non-nil Tracer after setup")
	}
}

func TestOpenTelemetryTracer_SetupTraces_console(t *testing.T) {
	t.Parallel()

	res := newTestResource(t)
	tracer := NewOpenTelemetryTracer(context.Background(), &OpenTelemetryTracerConfig{
		Name:                      "test",
		Resources:                 res,
		TraceExporter:             "console",
		TraceExporterBatchTimeout: 1 * time.Second,
	})

	err := tracer.SetupTraces()
	if err != nil {
		t.Fatalf("SetupTraces() with console exporter should not fail: %v", err)
	}

	if tracer.tp == nil {
		t.Fatal("expected non-nil TracerProvider after setup")
	}

	tracer.Shutdown()
}

func TestOpenTelemetryTracer_SetupTraces_unknown_exporter(t *testing.T) {
	t.Parallel()

	res := newTestResource(t)
	tracer := NewOpenTelemetryTracer(context.Background(), &OpenTelemetryTracerConfig{
		Name:          "test",
		Resources:     res,
		TraceExporter: "unknown-exporter",
	})

	err := tracer.SetupTraces()
	if err == nil {
		t.Fatal("SetupTraces() with unknown exporter should fail")
	}
}

func TestOpenTelemetryTracer_Shutdown_nil_provider(t *testing.T) {
	t.Parallel()

	res := newTestResource(t)
	tracer := NewOpenTelemetryTracer(context.Background(), &OpenTelemetryTracerConfig{
		Name:          "test",
		Resources:     res,
		TraceExporter: "noop",
	})

	// Setup with noop, which doesn't set tp
	_ = tracer.SetupTraces()

	// Should not panic even with nil tp
	tracer.Shutdown()
}

func TestOpenTelemetryTracer_Shutdown_with_provider(t *testing.T) {
	t.Parallel()

	res := newTestResource(t)
	tracer := NewOpenTelemetryTracer(context.Background(), &OpenTelemetryTracerConfig{
		Name:                      "test",
		Resources:                 res,
		TraceExporter:             "console",
		TraceExporterBatchTimeout: 1 * time.Second,
	})

	err := tracer.SetupTraces()
	if err != nil {
		t.Fatalf("SetupTraces() failed: %v", err)
	}

	// Should cleanly shutdown
	tracer.Shutdown()
}

func TestNewPropagator(t *testing.T) {
	t.Parallel()

	prop := newPropagator()
	if prop == nil {
		t.Fatal("expected non-nil propagator")
	}

	fields := prop.Fields()
	if len(fields) == 0 {
		t.Error("expected propagator to have fields")
	}
}
