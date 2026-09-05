//go:build unit

package o11y

import (
	"errors"
	"testing"
	"time"
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
