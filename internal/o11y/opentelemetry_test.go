package o11y

import (
	"context"
	"testing"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	conf := &config.OpenTelemetryConfig{
		AttributeServiceName:      "test-service",
		AttributeServiceVersion:   "1.0.0",
		TraceEndpoint:             config.NewField("", "", "", "localhost"),
		TracePort:                 config.NewField("", "", "", 4318),
		TraceExporter:             config.NewField("", "", "", "noop"),
		TraceExporterBatchTimeout: config.NewField("", "", "", 5*time.Second),
		MetricEndpoint:            config.NewField("", "", "", "localhost"),
		MetricPort:                config.NewField("", "", "", 4318),
		MetricExporter:            config.NewField("", "", "", "noop"),
		MetricInterval:            config.NewField("", "", "", 10*time.Second),
	}

	ot, err := New(context.Background(), conf)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if ot == nil {
		t.Fatal("expected non-nil OpenTelemetry")
	}

	if ot.Traces == nil {
		t.Fatal("expected non-nil Traces")
	}

	if ot.Metrics == nil {
		t.Fatal("expected non-nil Metrics")
	}
}

func TestOpenTelemetry_Start_and_Shutdown(t *testing.T) {
	t.Parallel()

	conf := &config.OpenTelemetryConfig{
		AttributeServiceName:      "test-service",
		AttributeServiceVersion:   "1.0.0",
		TraceEndpoint:             config.NewField("", "", "", "localhost"),
		TracePort:                 config.NewField("", "", "", 4318),
		TraceExporter:             config.NewField("", "", "", "noop"),
		TraceExporterBatchTimeout: config.NewField("", "", "", 5*time.Second),
		MetricEndpoint:            config.NewField("", "", "", "localhost"),
		MetricPort:                config.NewField("", "", "", 4318),
		MetricExporter:            config.NewField("", "", "", "noop"),
		MetricInterval:            config.NewField("", "", "", 10*time.Second),
	}

	ot, err := New(context.Background(), conf)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	err = ot.Start()
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Should not panic
	ot.Shutdown()
}
