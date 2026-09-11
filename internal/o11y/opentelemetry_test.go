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
		LogEndpoint:               config.NewField("", "", "", "localhost"),
		LogPort:                   config.NewField("", "", "", 3100),
		LogExporter:               config.NewField("", "", "", "noop"),
		LogPath:                   config.NewField("", "", "", "/otlp/v1/logs"),
		LogExporterBatchTimeout:   config.NewField("", "", "", 5*time.Second),
	}

	ot, err := New(context.Background(), conf, config.NewLogConfig())
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

	if ot.Logs == nil {
		t.Fatal("expected non-nil Logs")
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
		LogEndpoint:               config.NewField("", "", "", "localhost"),
		LogPort:                   config.NewField("", "", "", 3100),
		LogExporter:               config.NewField("", "", "", "noop"),
		LogPath:                   config.NewField("", "", "", "/otlp/v1/logs"),
		LogExporterBatchTimeout:   config.NewField("", "", "", 5*time.Second),
	}

	ot, err := New(context.Background(), conf, config.NewLogConfig())
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

// resourceAttrs flattens a resource for assertion.
func resourceAttrs(t *testing.T, conf *config.OpenTelemetryConfig) map[string]string {
	t.Helper()

	res, err := newResource(t.Context(), conf)
	if err != nil {
		t.Fatalf("newResource: %v", err)
	}

	out := map[string]string{}
	for _, kv := range res.Attributes() {
		out[string(kv.Key)] = kv.Value.String()
	}

	return out
}

// The dashboards carry an `instance` variable keyed on service.instance.id, and
// the SDK only sets it behind an experimental flag. Without this the variable
// resolved to empty while `service_instance_id=~""` still matched every series,
// so the panels worked and the variable was dead, with nothing saying so.
func TestResourceIdentifiesTheReplica(t *testing.T) {
	t.Parallel()

	conf := config.NewOpenTelemetryConfig("test-service", "1.0.0")

	attrs := resourceAttrs(t, conf)

	if attrs["service.instance.id"] == "" {
		t.Error("service.instance.id is not set; the instance dashboard variable has nothing to resolve")
	}

	if attrs["service.name"] != "test-service" {
		t.Errorf("service.name = %q", attrs["service.name"])
	}

	if attrs["service.version"] != "1.0.0" {
		t.Errorf("service.version = %q", attrs["service.version"])
	}

	if attrs["host.name"] == "" {
		t.Error("host.name is not set; a line cannot be pinned to a machine or a pod")
	}

	// resource.Default()'s attributes must survive the merge, or
	// OTEL_RESOURCE_ATTRIBUTES stops working and a deployment loses the way it
	// adds its own attributes without a code change.
	if attrs["telemetry.sdk.name"] != "opentelemetry" {
		t.Errorf("the default resource was lost in the merge; telemetry.sdk.name = %q", attrs["telemetry.sdk.name"])
	}
}

// A restarted process is a NEW instance. An id that survived a restart would
// make two different processes' series look like one.
func TestEachProcessGetsItsOwnInstanceID(t *testing.T) {
	t.Parallel()

	conf := config.NewOpenTelemetryConfig("test-service", "1.0.0")

	first := resourceAttrs(t, conf)["service.instance.id"]
	second := resourceAttrs(t, conf)["service.instance.id"]

	if first == second {
		t.Errorf("two resources share an instance id (%q); a restart must look like a new instance", first)
	}
}

// There is no honest default environment: "development" would label a
// production replica wrongly and "production" would do the reverse. Both are
// worse than nothing, because a wrong label is acted on and a missing one is
// asked about.
func TestEnvironmentIsOmittedUntilItIsSet(t *testing.T) {
	t.Parallel()

	conf := config.NewOpenTelemetryConfig("test-service", "1.0.0")

	if _, present := resourceAttrs(t, conf)["deployment.environment.name"]; present {
		t.Error("deployment.environment.name is set by default; an unset environment must carry no label at all")
	}

	conf.Environment.Value = "production"

	if got := resourceAttrs(t, conf)["deployment.environment.name"]; got != "production" {
		t.Errorf("deployment.environment.name = %q, want production", got)
	}
}
