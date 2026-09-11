package o11y

import (
	"context"
	"math"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// ExportErrors summarises what the OpenTelemetry SDK could not send.
//
// # Why this exists
//
// Every exporter this service configures is asynchronous and batched. A span is
// handed to a BatchSpanProcessor and the call returns long before anything
// reaches a collector, so a collector that is refusing connections cannot be
// discovered at the call site — there is no error to return, and none of the
// instrumented code is in a position to notice.
//
// The SDK reports those failures to the process-wide handler installed with
// [otel.SetErrorHandler]. The DEFAULT handler writes them to the standard
// logger and forgets them, which is why a total telemetry outage was invisible
// to /health/detailed: it reported "telemetry active, healthy" on the strength
// of a non-nil pointer, which is equally true of a service that has not
// successfully exported a span in a week.
//
// # What it deliberately does not do
//
// It does not keep the errors. An export failure repeats once per batch — every
// [OpenTelemetryTracerConfig.TraceExporterBatchTimeout] — so a slice would grow
// without bound during exactly the outage it exists to report. A count, a
// timestamp and the most recent message are enough to tell an operator that
// telemetry is failing and roughly since when; the individual failures are in
// the log.
//
// It also does not distinguish traces from metrics from logs. The SDK's error
// handler is global and its errors carry no signal identifying which pipeline
// raised them, so splitting the count would mean inventing an attribution the
// SDK does not provide.
//
// # It must never log
//
// Since the log pipeline exports through the same handler, a slog call here
// would be a loop: the line is emitted, the exporter fails on it, the SDK calls
// Handle, Handle logs again. It would spin exactly when telemetry is already
// broken, and the first symptom would be a pegged CPU rather than a degraded
// health check. TestExportErrorsHandleDoesNotLog pins it.
type ExportErrors struct {
	last    time.Time
	lastErr string
	count   uint64
	mu      sync.Mutex
}

// Handle records an SDK error. It satisfies [otel.ErrorHandler].
//
// This runs on the exporter's own goroutine while a batch is failing, so it
// must stay cheap and must never block: a handler that did real work here would
// slow the pipeline it is reporting on.
func (ref *ExportErrors) Handle(err error) {
	if err == nil {
		return
	}

	ref.mu.Lock()
	defer ref.mu.Unlock()

	ref.count++
	ref.last = time.Now()
	ref.lastErr = err.Error()
}

// Snapshot returns the current summary: how many export errors have been seen,
// when the most recent one arrived, and what it said.
//
// A zero count means nothing has failed — not that nothing has been exported.
// The two are indistinguishable from here, which is why the health check reads
// the configured exporter as well.
func (ref *ExportErrors) Snapshot() (count uint64, last time.Time, lastErr string) {
	ref.mu.Lock()
	defer ref.mu.Unlock()

	return ref.count, ref.last, ref.lastErr
}

// Failing reports whether an export has failed recently enough for the pipeline
// to count as failing NOW.
//
// The window matters because an exporter retries on its own schedule, so a
// failure is evidence of a current outage only while it is newer than the
// interval the exporter works on. Anything older is history: a collector that
// was restarted an hour ago must not leave the health check degraded forever,
// or the component reports a fault that is already fixed and an operator learns
// to ignore it.
func (ref *ExportErrors) Failing(window time.Duration) bool {
	ref.mu.Lock()
	defer ref.mu.Unlock()

	if ref.count == 0 {
		return false
	}

	return time.Since(ref.last) < window
}

// RegisterMetrics publishes the export-failure count as a metric.
//
// # Why a metric for something the health check already reports
//
// /health/detailed is pulled by an operator or a probe; this is scraped, so it
// can be alerted on and it has history -- "exports started failing at 14:02" is
// a question the health endpoint cannot answer.
//
// It is genuinely useful in exactly one situation, and that situation is the
// common one: the metric pipeline is fine and the LOG collector is down. The
// counter then rises in Prometheus while every dashboard keeps working, which
// is the only way that outage announces itself. When the metric pipeline is
// what broke, this is as silent as everything else -- which is what the health
// check and its TCP probe are for.
//
// An ObservableCounter over the existing count rather than a Counter
// incremented in Handle: Handle runs on the exporter's goroutine while a batch
// is failing and must stay cheap, and the SDK reads a callback on its own
// schedule instead.
func (ref *ExportErrors) RegisterMetrics(meter metric.Meter) error {
	_, err := meter.Int64ObservableCounter(
		"telemetry_export_errors",
		metric.WithDescription("Exports the OpenTelemetry SDK could not deliver, across the trace, metric and log pipelines"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			count, _, _ := ref.Snapshot()

			// The SDK counts in uint64 and the instrument is int64. The
			// conversion cannot overflow in practice -- it would take more
			// failed batches than a process can produce in its lifetime -- and
			// a saturating cast is still cheaper to read than a panic.
			if count > uint64(math.MaxInt64) {
				o.Observe(math.MaxInt64)

				return nil
			}

			o.Observe(int64(count))

			return nil
		}),
	)

	return err
}

// SetGlobalErrorHandler installs ref as the process-wide OpenTelemetry error
// handler and returns it.
//
// The handler is global because the SDK's is: [otel.SetErrorHandler] takes one
// for the whole process. Calling this twice replaces the first, which is
// correct for a test that wants its own and wrong for anything else.
func SetGlobalErrorHandler(ref *ExportErrors) *ExportErrors {
	otel.SetErrorHandler(ref)
	return ref
}
