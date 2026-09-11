package o11y

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	otelLog "go.opentelemetry.io/otel/log"
	sdkLog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
	"github.com/slashdevops/go-rest-api-service-template/pkg/cslog"
)

// testResource is the resource a real New() would build: the two attributes
// that identify this replica in a log store.
func testResource() *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String("test-service"),
		semconv.ServiceVersionKey.String("0.0.0-test"),
	)
}

func newTestLoggerConfig(exporter string) *OpenTelemetryLoggerConfig {
	return &OpenTelemetryLoggerConfig{
		Name:                    "test-service",
		LogEndpoint:             "localhost",
		LogPort:                 3100,
		LogExporter:             exporter,
		LogPath:                 "/otlp/v1/logs",
		LogExporterBatchTimeout: 5 * time.Second,
		Level:                   slog.LevelInfo,
	}
}

// A noop exporter must leave Handler nil: that is how App.composeLogger knows
// the standard logger is the only sink, and it is the default configuration.
func TestSetupLogsNoopLeavesNoHandler(t *testing.T) {
	t.Parallel()

	l := NewOpenTelemetryLogger(context.Background(), newTestLoggerConfig(config.ExporterNoop))

	if err := l.SetupLogs(); err != nil {
		t.Fatalf("SetupLogs: %v", err)
	}

	if l.Handler != nil {
		t.Error("expected no handler for the noop exporter; a non-nil one would make every record cost a second sink that goes nowhere")
	}

	// Shutdown must be safe even though nothing was built.
	l.Shutdown()
}

func TestSetupLogsBuildsAHandler(t *testing.T) {
	t.Parallel()

	for _, exporter := range []string{"console", config.ExporterOTLPHTTP} {
		t.Run(exporter, func(t *testing.T) {
			t.Parallel()

			l := NewOpenTelemetryLogger(context.Background(), newTestLoggerConfig(exporter))

			if err := l.SetupLogs(); err != nil {
				t.Fatalf("SetupLogs: %v", err)
			}

			if l.Handler == nil {
				t.Fatal("expected a handler")
			}

			l.Shutdown()
		})
	}
}

func TestSetupLogsRejectsAnUnknownExporter(t *testing.T) {
	t.Parallel()

	l := NewOpenTelemetryLogger(context.Background(), newTestLoggerConfig("kafka"))

	err := l.SetupLogs()
	if err == nil {
		t.Fatal("expected an error for an unknown exporter")
	}

	if !strings.Contains(err.Error(), "kafka") {
		t.Errorf("the error should name the value it refused, got %q", err)
	}
}

// The OTLP exporter is built lazily, so an unreachable collector is NOT a
// setup error. That is the whole reason ExportErrors exists.
func TestSetupLogsSucceedsWithAnUnreachableCollector(t *testing.T) {
	t.Parallel()

	conf := newTestLoggerConfig(config.ExporterOTLPHTTP)
	conf.LogPort = 1 // nothing listens here

	l := NewOpenTelemetryLogger(context.Background(), conf)

	if err := l.SetupLogs(); err != nil {
		t.Fatalf("SetupLogs must not fail on an unreachable collector, got %v", err)
	}

	l.Shutdown()
}

// The floor is the point of D11: TRACE never leaves the process, at any
// configured level.
func TestExportedLevelIsFlooredAtDebug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level slog.Level
		want  slog.Level
	}{
		{"trace_is_raised_to_debug", cslog.LogLevelTrace, slog.LevelDebug},
		{"debug_stays", slog.LevelDebug, slog.LevelDebug},
		{"info_stays", slog.LevelInfo, slog.LevelInfo},
		{"warn_stays", slog.LevelWarn, slog.LevelWarn},
		{"error_stays", slog.LevelError, slog.LevelError},
		{"fatal_stays", cslog.LogLevelFatal, cslog.LogLevelFatal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conf := newTestLoggerConfig(config.ExporterNoop)
			conf.Level = tt.level

			if got := NewOpenTelemetryLogger(context.Background(), conf).exportedLevel(); got != tt.want {
				t.Errorf("exportedLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSeverityOfMatchesTheBridge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level slog.Level
		want  otelLog.Severity
	}{
		{"trace", cslog.LogLevelTrace, otelLog.SeverityTrace},
		{"debug", slog.LevelDebug, otelLog.SeverityDebug},
		{"info", slog.LevelInfo, otelLog.SeverityInfo},
		{"warn", slog.LevelWarn, otelLog.SeverityWarn},
		{"error", slog.LevelError, otelLog.SeverityError},
		{"fatal", cslog.LogLevelFatal, otelLog.SeverityFatal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := severityOf(tt.level); got != tt.want {
				t.Errorf("severityOf(%v) = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}

// recordingProcessor captures what reaches the exporter side of the gate.
type recordingProcessor struct {
	emitted []otelLog.Severity
}

func (p *recordingProcessor) OnEmit(_ context.Context, r *sdkLog.Record) error {
	p.emitted = append(p.emitted, r.Severity())

	return nil
}
func (p *recordingProcessor) Enabled(context.Context, sdkLog.EnabledParameters) bool { return true }
func (p *recordingProcessor) Shutdown(context.Context) error                         { return nil }
func (p *recordingProcessor) ForceFlush(context.Context) error                       { return nil }

func TestSeverityGateDropsBelowTheMinimum(t *testing.T) {
	t.Parallel()

	rec := &recordingProcessor{}
	gate := newSeverityGate(slog.LevelInfo, rec)

	for _, sev := range []otelLog.Severity{
		otelLog.SeverityTrace, otelLog.SeverityDebug, otelLog.SeverityInfo, otelLog.SeverityError,
	} {
		var r sdkLog.Record
		r.SetSeverity(sev)

		if err := gate.OnEmit(context.Background(), &r); err != nil {
			t.Fatalf("OnEmit: %v", err)
		}
	}

	want := []otelLog.Severity{otelLog.SeverityInfo, otelLog.SeverityError}
	if len(rec.emitted) != len(want) {
		t.Fatalf("emitted %v, want %v", rec.emitted, want)
	}

	for i := range want {
		if rec.emitted[i] != want[i] {
			t.Errorf("emitted[%d] = %v, want %v", i, rec.emitted[i], want[i])
		}
	}
}

// Enabled is the cheap half: it answers before a record is built, so a
// suppressed cslog.Trace costs a comparison rather than an allocation.
func TestSeverityGateEnabled(t *testing.T) {
	t.Parallel()

	gate := newSeverityGate(slog.LevelInfo, &recordingProcessor{})

	if gate.Enabled(context.Background(), sdkLog.EnabledParameters{Severity: otelLog.SeverityTrace}) {
		t.Error("trace must not be enabled at info")
	}

	if !gate.Enabled(context.Background(), sdkLog.EnabledParameters{Severity: otelLog.SeverityError}) {
		t.Error("error must be enabled at info")
	}
}

// The end-to-end proof of D11: a TRACE record written through the composed
// logger reaches the standard handler and never the exporter.
func TestTraceRecordsReachStdoutAndNeverTheExporter(t *testing.T) {
	var stdout, exported bytes.Buffer

	conf := newTestLoggerConfig("console")
	conf.Level = cslog.LogLevelTrace

	conf.ConsoleWriter = &exported

	l := NewOpenTelemetryLogger(t.Context(), conf)

	if err := l.SetupLogs(); err != nil {
		t.Fatalf("SetupLogs: %v", err)
	}

	stdoutHandler := slog.NewJSONHandler(&stdout, &slog.HandlerOptions{Level: cslog.LogLevelTrace})
	logger := slog.New(slog.NewMultiHandler(stdoutHandler, l.Handler))

	logger.Log(t.Context(), cslog.LogLevelTrace, "a query with its arguments")
	logger.InfoContext(t.Context(), "an ordinary line")

	l.Shutdown()

	if !strings.Contains(stdout.String(), "a query with its arguments") {
		t.Error("the trace record must reach the standard logger; that is what -log.level=ctrace is for")
	}

	if strings.Contains(exported.String(), "a query with its arguments") {
		t.Error("the trace record must NOT be exported: that level carries SQL arguments and LLM payloads")
	}

	if !strings.Contains(exported.String(), "an ordinary line") {
		t.Error("records at or above the floor must still be exported")
	}
}

// A record carries the resource, so service.name is on the batch and does not
// need repeating per line.
func TestExportedRecordsCarryTheServiceResource(t *testing.T) {
	var exported bytes.Buffer

	conf := newTestLoggerConfig("console")
	conf.Resources = testResource()
	conf.ConsoleWriter = &exported

	l := NewOpenTelemetryLogger(t.Context(), conf)

	if err := l.SetupLogs(); err != nil {
		t.Fatalf("SetupLogs: %v", err)
	}

	slog.New(l.Handler).InfoContext(t.Context(), "hello")
	l.Shutdown()

	var got map[string]any
	if err := json.Unmarshal(exported.Bytes(), &got); err != nil {
		t.Fatalf("the console exporter must emit a JSON record: %v\n%s", err, exported.String())
	}

	if got["Body"] == nil {
		t.Errorf("expected a record body, got %v", got)
	}

	if !strings.Contains(exported.String(), "test-service") {
		t.Errorf("expected the service name in the record's resource, got %s", exported.String())
	}

	if !strings.Contains(exported.String(), "0.0.0-test") {
		t.Errorf("expected the service version in the record's resource, got %s", exported.String())
	}
}

// The gate wraps a processor; everything it does not filter it must pass
// through, or a flush on shutdown silently drops the last batch.
func TestSeverityGateDelegatesFlushAndShutdown(t *testing.T) {
	t.Parallel()

	delegate := &countingProcessor{}
	gate := newSeverityGate(slog.LevelInfo, delegate)

	if err := gate.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	if err := gate.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if delegate.flushes != 1 {
		t.Errorf("ForceFlush reached the delegate %d times, want 1", delegate.flushes)
	}

	if delegate.shutdowns != 1 {
		t.Errorf("Shutdown reached the delegate %d times, want 1", delegate.shutdowns)
	}
}

// The gate only ever narrows: a delegate that refuses a severity must still be
// asked, or wrapping a processor would quietly widen what is exported.
func TestSeverityGateAsksTheDelegateAboveTheFloor(t *testing.T) {
	t.Parallel()

	gate := newSeverityGate(slog.LevelInfo, &countingProcessor{enabled: false})

	if gate.Enabled(context.Background(), sdkLog.EnabledParameters{Severity: otelLog.SeverityError}) {
		t.Error("the gate must return the delegate's answer for a severity it does not itself refuse")
	}
}

type countingProcessor struct {
	flushes   int
	shutdowns int
	enabled   bool
}

func (p *countingProcessor) OnEmit(context.Context, *sdkLog.Record) error { return nil }
func (p *countingProcessor) Enabled(context.Context, sdkLog.EnabledParameters) bool {
	return p.enabled
}

func (p *countingProcessor) Shutdown(context.Context) error {
	p.shutdowns++

	return nil
}

func (p *countingProcessor) ForceFlush(context.Context) error {
	p.flushes++

	return nil
}
