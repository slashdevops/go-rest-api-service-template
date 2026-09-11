//go:build unit

package app

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// A configured log exporter ADDS a sink. It must never replace the one that
// was already there: the lines written before telemetry exists have nowhere
// else to go, and a platform that collects stdout has to keep working while
// the collector does not.
func TestComposeLoggerKeepsTheStandardSink(t *testing.T) {
	var stdout, exported bytes.Buffer

	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	// What setupLogger leaves behind: a handler with the two identifying
	// attributes attached to it.
	slog.SetDefault(slog.New(slog.NewJSONHandler(&stdout, nil)).With(
		slog.String("application", "test-app"),
		slog.String("version", "0.0.0-test"),
	))

	a := &App{
		configs: &Configs{
			Telemetry: config.NewOpenTelemetryConfig("test-app", "0.0.0-test"),
			Log:       config.NewLogConfig(),
		},
	}
	a.configs.Telemetry.TraceExporter.Value = config.ExporterNoop
	a.configs.Telemetry.MetricExporter.Value = config.ExporterNoop
	a.configs.Telemetry.LogExporter.Value = "console"

	telemetry, err := o11y.New(t.Context(), a.configs.Telemetry, a.configs.Log)
	if err != nil {
		t.Fatalf("o11y.New: %v", err)
	}

	telemetry.Logs = o11y.NewOpenTelemetryLogger(t.Context(), &o11y.OpenTelemetryLoggerConfig{
		Name:          "test-app",
		LogExporter:   "console",
		Level:         slog.LevelInfo,
		ConsoleWriter: &exported,
	})

	if err := telemetry.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	a.telemetry = telemetry
	a.composeLogger()

	slog.InfoContext(t.Context(), "a line that must reach both sinks")

	telemetry.Shutdown()

	if !strings.Contains(stdout.String(), "a line that must reach both sinks") {
		t.Errorf("the standard sink lost the record; got %s", stdout.String())
	}

	if !strings.Contains(exported.String(), "a line that must reach both sinks") {
		t.Errorf("the export sink never saw the record; got %s", exported.String())
	}

	// application and version belong to the standard handler. On the exported
	// side the same two facts are resource attributes carried once per batch,
	// and repeating them per record would pay for them per line and give a
	// query two spellings of one thing.
	if !strings.Contains(stdout.String(), `"application":"test-app"`) {
		t.Errorf("the standard sink must keep its identifying attributes; got %s", stdout.String())
	}

	if strings.Contains(exported.String(), "application") {
		t.Errorf("the exported record must not repeat application per line; got %s", exported.String())
	}
}

// With no log exporter the default logger is left exactly as it was, rather
// than wrapped in a MultiHandler with a nil second sink.
func TestComposeLoggerIsANoOpWithoutAnExporter(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	marker := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	slog.SetDefault(marker)

	a := &App{
		configs: &Configs{
			Telemetry: config.NewOpenTelemetryConfig("test-app", "0.0.0-test"),
			Log:       config.NewLogConfig(),
		},
		telemetry: &o11y.OpenTelemetry{
			Logs: o11y.NewOpenTelemetryLogger(t.Context(), &o11y.OpenTelemetryLoggerConfig{
				LogExporter: config.ExporterNoop,
			}),
		},
	}

	a.composeLogger()

	if slog.Default() != marker {
		t.Error("the default logger was replaced even though no log exporter is configured")
	}
}

// The ordering invariant.
//
// Anything that captures slog.Default() for its own use -- the cache client
// and the outbound HTTP client both do -- keeps whatever logger existed when
// it was wired. If a phase that captures the logger moved above telemetry, it
// would hold a logger with one sink and its lines would silently stop
// appearing in the log store, with nothing failing.
//
// initTelemetry is therefore the first phase of Build. This asserts it from
// the source rather than from behaviour, because the failure is invisible at
// runtime.
func TestLoggerConsumersAreWiredAfterTelemetry(t *testing.T) {
	raw, err := os.ReadFile("builder.go")
	if err != nil {
		t.Fatalf("read builder.go: %v", err)
	}

	source := string(raw)

	telemetryAt := strings.Index(source, "app.initTelemetry(")
	if telemetryAt < 0 {
		t.Fatal("initTelemetry is no longer called from Build; this test needs rewriting, not deleting")
	}

	// The two known captures of the default logger, both in initServices.
	for _, phase := range []string{"app.initServices(", "app.initDatabase("} {
		at := strings.Index(source, phase)
		if at < 0 {
			continue
		}

		if at < telemetryAt {
			t.Errorf("%s runs before initTelemetry; anything capturing slog.Default() there keeps a logger with only the standard sink", phase)
		}
	}
}
