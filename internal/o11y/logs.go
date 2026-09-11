package o11y

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	otelLog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdkLog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
)

// ExportedLogFloor is the lowest severity that may leave the process.
//
// # Why there is a floor at all
//
// TRACE (cslog.LogLevelTrace) is where this service logs the things that are
// useful next to a terminal and dangerous anywhere else: every SQL statement
// with its arguments -- which on the authentication path are password hashes
// and token hashes -- and every outbound LLM request body, which is a tenant's
// prompt and the chunks retrieved for it. `.air.toml` and `run.sh` run the dev
// stack at that level on purpose.
//
// A searchable, retained log store is exactly the wrong destination for those
// lines. The floor is a constant rather than a setting because a setting can be
// raised: with an `opentelemetry.log.level` flag, shipping SQL arguments to a
// log store would be one operator's typo away, in production, with no review.
// There is no value in that flag that anybody wants and cannot get another way.
//
// So `-log.level=ctrace` keeps doing what it has always done -- everything on
// stdout, in front of the developer who asked for it -- and the exporter sees
// DEBUG and above. The two sinks disagree deliberately, and this is the only
// place they do.
const ExportedLogFloor = slog.LevelDebug

// OpenTelemetryLoggerConfig represents the configuration of the OpenTelemetry logger.
type OpenTelemetryLoggerConfig struct {

	// ConsoleWriter is where the "console" exporter writes. Nil means
	// os.Stdout, which is what every caller outside a test wants; a test sets
	// it to assert on what the pipeline would have shipped without standing up
	// a collector.
	ConsoleWriter io.Writer
	Resources     *resource.Resource
	Name          string

	LogEndpoint string
	LogExporter string
	LogPath     string

	// Level is the application's configured log level. It gates what the
	// exporter is offered, floored by [ExportedLogFloor].
	Level slog.Level

	LogPort                 int
	LogExporterBatchTimeout time.Duration

	// AddSource mirrors log.add.source. The bridge reads the caller from the
	// slog record, so this only decides whether the record carries one.
	AddSource bool
}

// OpenTelemetryLogger is the log pipeline: it turns records written through
// [log/slog] into OpenTelemetry log records and ships them.
//
// # What it is not
//
// It is not the logger. The standard logger keeps writing to log.output
// whatever this is configured to do, and [Handler] is composed WITH it rather
// than in place of it -- see App.initTelemetry. Two reasons: the lines written
// before telemetry exists (configuration errors, a refused seeded administrator)
// have nowhere else to go, and a container platform that collects stdout must
// keep working while the log collector does not.
//
// # Ordering
//
// SetupLogs runs after the tracer and the meter, so a record emitted during
// setup already has a tracer to be correlated against; Shutdown runs before
// them, so the flush still has one. See [OpenTelemetry.Start] and
// [OpenTelemetry.Shutdown].
type OpenTelemetryLogger struct {
	ctx context.Context

	// Handler is the slog handler that feeds this pipeline, or nil when the
	// exporter is noop. A nil Handler means "the standard logger is the only
	// sink", which is the default.
	Handler slog.Handler

	consoleWriter io.Writer

	res *resource.Resource

	// lp is the SDK provider. It is what Shutdown flushes.
	lp *sdkLog.LoggerProvider

	name string

	logEndpoint             string
	logExporter             string
	logPath                 string
	level                   slog.Level
	logPort                 int
	logExporterBatchTimeout time.Duration
	addSource               bool
}

func NewOpenTelemetryLogger(ctx context.Context, conf *OpenTelemetryLoggerConfig) *OpenTelemetryLogger {
	return &OpenTelemetryLogger{
		ctx:  ctx,
		name: conf.Name,

		logEndpoint:             conf.LogEndpoint,
		logPort:                 conf.LogPort,
		logExporter:             conf.LogExporter,
		logPath:                 conf.LogPath,
		logExporterBatchTimeout: conf.LogExporterBatchTimeout,
		level:                   conf.Level,
		addSource:               conf.AddSource,
		consoleWriter:           conf.ConsoleWriter,

		res: conf.Resources,
	}
}

// SetupLogs builds the pipeline and leaves [OpenTelemetryLogger.Handler] ready
// to be composed into the default logger.
func (ref *OpenTelemetryLogger) SetupLogs() error {
	// noop leaves Handler nil: the standard logger stays the only sink, which
	// is what every deployment did before this pipeline existed.
	if ref.logExporter == config.ExporterNoop {
		slog.Debug("no log exporter configured, logs are written to log.output only")

		return nil
	}

	exp, err := ref.newLogExporter(ref.ctx)
	if err != nil {
		return err
	}

	lp := sdkLog.NewLoggerProvider(
		sdkLog.WithResource(ref.res),
		sdkLog.WithProcessor(
			newSeverityGate(
				ref.exportedLevel(),
				sdkLog.NewBatchProcessor(
					exp,
					sdkLog.WithExportInterval(ref.logExporterBatchTimeout),
				),
			),
		),
	)
	ref.lp = lp

	global.SetLoggerProvider(lp)

	ref.Handler = otelslog.NewHandler(ref.name,
		otelslog.WithLoggerProvider(lp),
		otelslog.WithSource(ref.addSource),
	)

	return nil
}

// exportedLevel is the configured level raised to [ExportedLogFloor].
//
// Raised, never lowered: a service running at warn exports warn and above, not
// debug and above.
func (ref *OpenTelemetryLogger) exportedLevel() slog.Level {
	return max(ref.level, ExportedLogFloor)
}

func (ref *OpenTelemetryLogger) Shutdown() {
	if ref.lp != nil {
		if err := ref.lp.Shutdown(ref.ctx); err != nil {
			slog.Error("failed to shutdown OpenTelemetry logger provider", "error", err)
		}
	}
}

// newLogExporter creates a new log exporter based on the configuration.
func (ref *OpenTelemetryLogger) newLogExporter(ctx context.Context) (sdkLog.Exporter, error) {
	switch ref.logExporter {
	case "console":
		// The console exporter is for seeing the RECORD -- its resource,
		// severity number and attributes -- not for reading logs: everything it
		// prints is already on stdout in the handler's own format. It is how a
		// developer checks what the OTLP sink would receive without standing up
		// a collector.
		w := ref.consoleWriter
		if w == nil {
			w = os.Stdout
		}

		return stdoutlog.New(stdoutlog.WithWriter(w), stdoutlog.WithPrettyPrint())

	case config.ExporterOTLPHTTP:
		// The path is a setting because the two things this ships to disagree
		// about it: Loki serves /otlp/v1/logs, an OpenTelemetry Collector
		// serves /v1/logs. WithEndpointURL takes the whole URL, the way the
		// metric exporter does for Prometheus' OTLP prefix.
		return otlploghttp.New(ctx,
			otlploghttp.WithInsecure(),
			otlploghttp.WithCompression(otlploghttp.GzipCompression),
			otlploghttp.WithEndpointURL(
				fmt.Sprintf("http://%s:%d%s", ref.logEndpoint, ref.logPort, ref.logPath),
			),
		)

	default:
		return nil, fmt.Errorf("unknown log exporter: %s", ref.logExporter)
	}
}

// severityGate drops records below a minimum severity on their way to the
// exporter.
//
// # Why the SDK does not do this
//
// Neither the provider nor the batch processor has a minimum severity of its
// own: the SDK filters nothing by level. Without this the exporter receives
// every record the STANDARD handler admits -- including TRACE, which is the one
// thing that must never leave the process ([ExportedLogFloor]).
//
// Both halves of [sdkLog.Processor] matter here, and for different reasons.
// Enabled is the cheap half: the slog bridge asks it before building a record,
// so a suppressed cslog.Trace call costs an integer comparison rather than an
// allocation. OnEmit is the correct half: the SDK documents that OnEmit is
// called independently of Enabled, so the drop has to happen there too or a
// bridge that skips the question ships the record anyway.
type severityGate struct {
	next sdkLog.Processor
	min  otelLog.Severity
}

// newSeverityGate wraps next so that nothing below level reaches it.
func newSeverityGate(level slog.Level, next sdkLog.Processor) *severityGate {
	return &severityGate{next: next, min: severityOf(level)}
}

// severityOf converts a [slog.Level] to an OpenTelemetry severity the way the
// slog bridge does: the two scales are offset by a constant, and both of this
// service's custom levels land where they should -- cslog.LogLevelTrace (-8) on
// SeverityTrace, cslog.LogLevelFatal (12) on SeverityFatal.
func severityOf(level slog.Level) otelLog.Severity {
	const offset = slog.Level(otelLog.SeverityDebug) - slog.LevelDebug

	return otelLog.Severity(level + offset)
}

func (g *severityGate) OnEmit(ctx context.Context, record *sdkLog.Record) error {
	if record.Severity() < g.min {
		return nil
	}

	return g.next.OnEmit(ctx, record)
}

func (g *severityGate) Enabled(ctx context.Context, param sdkLog.EnabledParameters) bool {
	if param.Severity < g.min {
		return false
	}

	return g.next.Enabled(ctx, param)
}

func (g *severityGate) Shutdown(ctx context.Context) error {
	return g.next.Shutdown(ctx)
}

func (g *severityGate) ForceFlush(ctx context.Context) error {
	return g.next.ForceFlush(ctx)
}

var _ sdkLog.Processor = (*severityGate)(nil)
