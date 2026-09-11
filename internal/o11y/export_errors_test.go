//go:build unit

package o11y

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	sdkMetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestExportErrorsStartsClean(t *testing.T) {
	ref := &ExportErrors{}

	if ref.Failing(time.Minute) {
		t.Error("Failing = true with no errors recorded")
	}

	count, _, _ := ref.Snapshot()
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestExportErrorsIgnoresNil(t *testing.T) {
	// The SDK's handler contract does not promise a non-nil error, and counting
	// a nil one would report a fault that never happened.
	ref := &ExportErrors{}
	ref.Handle(nil)

	if count, _, _ := ref.Snapshot(); count != 0 {
		t.Errorf("count = %d, want 0 after a nil error", count)
	}
}

func TestExportErrorsRecordsTheMostRecentMessage(t *testing.T) {
	ref := &ExportErrors{}
	ref.Handle(errors.New("first"))
	ref.Handle(errors.New("second"))

	count, last, lastErr := ref.Snapshot()

	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if lastErr != "second" {
		t.Errorf("lastErr = %q, want %q", lastErr, "second")
	}
	if last.IsZero() {
		t.Error("last is zero, want the time of the most recent failure")
	}
}

func TestExportErrorsForgetsAnOldFailure(t *testing.T) {
	// A collector restarted an hour ago must not leave the health check
	// degraded forever: that reports a fault which is already fixed, and an
	// operator who sees it every day stops reading it.
	ref := &ExportErrors{}
	ref.Handle(errors.New("connection refused"))

	if !ref.Failing(time.Minute) {
		t.Error("Failing = false immediately after a failure")
	}

	ref.mu.Lock()
	ref.last = time.Now().Add(-2 * time.Hour)
	ref.mu.Unlock()

	if ref.Failing(time.Minute) {
		t.Error("Failing = true for a failure two hours outside the window")
	}

	// The count survives, because it happened.
	if count, _, _ := ref.Snapshot(); count != 1 {
		t.Errorf("count = %d, want the count to outlive the window", count)
	}
}

func TestExportErrorsIsSafeUnderConcurrency(t *testing.T) {
	// Handle runs on the exporter's goroutines while Snapshot runs on whichever
	// goroutine is serving /health/detailed. Run with -race.
	ref := &ExportErrors{}
	done := make(chan struct{})

	go func() {
		defer close(done)
		for range 500 {
			ref.Handle(errors.New("boom"))
		}
	}()

	for range 500 {
		ref.Snapshot()
		ref.Failing(time.Minute)
	}

	<-done

	if count, _, _ := ref.Snapshot(); count != 500 {
		t.Errorf("count = %d, want 500", count)
	}
}

// countingHandler counts records instead of writing them.
type countingHandler struct{ n *int }

func (h countingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h countingHandler) Handle(context.Context, slog.Record) error {
	*h.n++

	return nil
}
func (h countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h countingHandler) WithGroup(string) slog.Handler      { return h }

// Handle must never log.
//
// Since the log pipeline exports through this same error handler, a slog call
// here is a loop: the line is emitted, its export fails, the SDK calls Handle,
// Handle logs again. It would spin exactly when telemetry is already broken,
// and the first symptom would be a pegged CPU rather than a degraded health
// check.
//
// This is pinned by a test rather than by a comment because the natural thing
// to write when adding a field to ExportErrors is a slog.Warn next to it.
func TestExportErrorsHandleDoesNotLog(t *testing.T) {
	var n int

	previous := slog.Default()
	slog.SetDefault(slog.New(countingHandler{n: &n}))
	t.Cleanup(func() { slog.SetDefault(previous) })

	ref := &ExportErrors{}
	for range 100 {
		ref.Handle(errors.New("collector refused the connection"))
	}

	if n != 0 {
		t.Errorf("ExportErrors.Handle wrote %d log records; it must write none, or a failing log exporter feeds itself", n)
	}

	if count, _, _ := ref.Snapshot(); count != 100 {
		t.Errorf("Snapshot count = %d, want 100", count)
	}
}

// The counter is what makes a log-collector outage visible while every
// dashboard keeps working, so the callback has to report the real count and
// not merely register without error.
func TestRegisterMetricsPublishesTheCount(t *testing.T) {
	reader := sdkMetric.NewManualReader()
	provider := sdkMetric.NewMeterProvider(sdkMetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	ref := &ExportErrors{}
	if err := ref.RegisterMetrics(provider.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}

	for range 7 {
		ref.Handle(errors.New("collector refused the connection"))
	}

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	got, found := int64(0), false

	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "telemetry_export_errors" {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("telemetry_export_errors is %T, want an int64 sum", m.Data)
			}

			for _, point := range sum.DataPoints {
				got, found = point.Value, true
			}
		}
	}

	if !found {
		t.Fatal("telemetry_export_errors was not collected")
	}

	if got != 7 {
		t.Errorf("telemetry_export_errors = %d, want 7", got)
	}
}
