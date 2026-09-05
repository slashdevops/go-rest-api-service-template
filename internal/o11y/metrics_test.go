package o11y

import (
	"context"
	"testing"
	"time"
)

func TestNewOpenTelemetryMeter(t *testing.T) {
	t.Parallel()

	res := newTestResource(t)
	conf := &OpenTelemetryMeterConfig{
		Name:           "test-meter",
		Resources:      res,
		MetricEndpoint: "localhost",
		MetricPort:     4318,
		MetricExporter: "noop",
		MetricInterval: 10 * time.Second,
	}

	meter := NewOpenTelemetryMeter(context.Background(), conf)

	if meter == nil {
		t.Fatal("expected non-nil meter")
	}
	if meter.name != "test-meter" {
		t.Errorf("name = %q, want %q", meter.name, "test-meter")
	}
	if meter.metricEndpoint != "localhost" {
		t.Errorf("metricEndpoint = %q, want %q", meter.metricEndpoint, "localhost")
	}
	if meter.metricPort != 4318 {
		t.Errorf("metricPort = %d, want %d", meter.metricPort, 4318)
	}
	if meter.metricExporter != "noop" {
		t.Errorf("metricExporter = %q, want %q", meter.metricExporter, "noop")
	}
	if meter.Meter == nil {
		t.Fatal("expected non-nil Meter")
	}
}

func TestOpenTelemetryMeter_SetupMetrics_noop(t *testing.T) {
	t.Parallel()

	res := newTestResource(t)
	meter := NewOpenTelemetryMeter(context.Background(), &OpenTelemetryMeterConfig{
		Name:           "test",
		Resources:      res,
		MetricExporter: "noop",
	})

	err := meter.SetupMetrics()
	if err != nil {
		t.Fatalf("SetupMetrics() with noop exporter should not fail: %v", err)
	}

	if meter.Meter == nil {
		t.Fatal("expected non-nil Meter after setup")
	}
}

func TestOpenTelemetryMeter_SetupMetrics_console(t *testing.T) {
	t.Parallel()

	res := newTestResource(t)
	meter := NewOpenTelemetryMeter(context.Background(), &OpenTelemetryMeterConfig{
		Name:           "test",
		Resources:      res,
		MetricExporter: "console",
		MetricInterval: 1 * time.Second,
	})

	err := meter.SetupMetrics()
	if err != nil {
		t.Fatalf("SetupMetrics() with console exporter should not fail: %v", err)
	}

	if meter.mp == nil {
		t.Fatal("expected non-nil MeterProvider after setup")
	}

	meter.Shutdown()
}

func TestOpenTelemetryMeter_SetupMetrics_unknown_exporter(t *testing.T) {
	t.Parallel()

	res := newTestResource(t)
	meter := NewOpenTelemetryMeter(context.Background(), &OpenTelemetryMeterConfig{
		Name:           "test",
		Resources:      res,
		MetricExporter: "unknown-exporter",
	})

	err := meter.SetupMetrics()
	if err == nil {
		t.Fatal("SetupMetrics() with unknown exporter should fail")
	}
}

func TestOpenTelemetryMeter_Shutdown_nil_provider(t *testing.T) {
	t.Parallel()

	res := newTestResource(t)
	meter := NewOpenTelemetryMeter(context.Background(), &OpenTelemetryMeterConfig{
		Name:           "test",
		Resources:      res,
		MetricExporter: "noop",
	})

	// Setup with noop, which doesn't set mp
	_ = meter.SetupMetrics()

	// Should not panic with nil mp
	meter.Shutdown()
}

func TestOpenTelemetryMeter_Shutdown_with_provider(t *testing.T) {
	t.Parallel()

	res := newTestResource(t)
	meter := NewOpenTelemetryMeter(context.Background(), &OpenTelemetryMeterConfig{
		Name:           "test",
		Resources:      res,
		MetricExporter: "console",
		MetricInterval: 1 * time.Second,
	})

	err := meter.SetupMetrics()
	if err != nil {
		t.Fatalf("SetupMetrics() failed: %v", err)
	}

	// Should cleanly shutdown
	meter.Shutdown()
}
