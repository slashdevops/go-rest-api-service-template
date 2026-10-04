//go:build unit

package o11y

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"
)

// captureLogs replaces the default logger for the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var out bytes.Buffer

	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return &out
}

func fail(ctx context.Context, layer string, err error) error {
	ctx, span, attrs := SetupTrace(ctx, noop.NewTracerProvider().Tracer("test"), Metadata{Layer: layer, Domain: "Things"}, "List")
	defer span.End()

	return RecordError(ctx, span, time.Now(), err, nil, attrs)
}

// TestAFailureOutsideARequestIsStillLogged: a reloader, startup. Nothing will write an access line for them, so RecordError logs
// the failure itself, at ERROR, as it always did.
func TestAFailureOutsideARequestIsStillLogged(t *testing.T) {
	out := captureLogs(t)

	if err := fail(t.Context(), LayerUsecase, errors.New("the build was refused")); err == nil {
		t.Fatal("RecordError must return the error it was given")
	}

	logged := out.String()
	if !strings.Contains(logged, "level=ERROR") || !strings.Contains(logged, "msg=operation_failed") {
		t.Fatalf("a failure with no request around it was not logged:\n%s", logged)
	}

	if !strings.Contains(logged, "the build was refused") || !strings.Contains(logged, "error_type=*errors.errorString") {
		t.Errorf("the line lacks the error or its type:\n%s", logged)
	}
}

// TestAFailureInsideARequestIsHeldNotLogged: with a holder in the context,
// recording a failure writes nothing. The access log writes the one line,
// once it knows the status.
func TestAFailureInsideARequestIsHeldNotLogged(t *testing.T) {
	out := captureLogs(t)

	ctx := WithFailure(t.Context())
	cause := errors.New("no such row")

	if err := fail(ctx, LayerRepository, cause); !errors.Is(err, cause) {
		t.Fatalf("RecordError returned %v, want the error unchanged", err)
	}

	if out.Len() != 0 {
		t.Fatalf("a failure inside a request was logged by the layer:\n%s", out.String())
	}

	held, ok := FailureFrom(ctx)
	if !ok {
		t.Fatal("the failure was not held")
	}

	if !errors.Is(held.Err, cause) || held.Type != "*errors.errorString" {
		t.Errorf("held %v (%s)", held.Err, held.Type)
	}

	if held.Operation.Layer != LayerRepository || held.Operation.Domain != "Things" || held.Operation.Action != "List" {
		t.Errorf("the operation was not kept: %+v", held.Operation)
	}

	if !strings.HasSuffix(held.File, "failure_test.go") || held.Line == 0 || held.Func == "" {
		t.Errorf("where it arose was not kept: %s %s:%d", held.Func, held.File, held.Line)
	}
}

// TestAnErrorPassedUpKeepsWhereItArose: the repository returns before the
// use-case and the handler do, so it records first, and the layers above
// record the same error, or a wrap of it. That does not replace it: what is
// worth reading is where the error arose, not where it was passed on.
func TestAnErrorPassedUpKeepsWhereItArose(t *testing.T) {
	captureLogs(t)

	ctx := WithFailure(t.Context())
	cause := errors.New("connection refused")

	_ = fail(ctx, LayerRepository, cause)
	_ = fail(ctx, LayerUsecase, fmt.Errorf("listing things: %w", cause))
	_ = fail(ctx, LayerHandler, fmt.Errorf("listing things: %w", cause))

	held, ok := FailureFrom(ctx)
	if !ok || held.Operation.Layer != LayerRepository {
		t.Fatalf("kept the failure of layer %q, want the repository's", held.Operation.Layer)
	}
}

// TestNoFailureIsNoFailure: a request in which nothing failed, and a context
// with no holder at all.
func TestNoFailureIsNoFailure(t *testing.T) {
	if _, ok := FailureFrom(WithFailure(t.Context())); ok {
		t.Error("a holder nothing wrote to reports a failure")
	}

	if _, ok := FailureFrom(t.Context()); ok {
		t.Error("a context with no holder reports a failure")
	}

	if NoteFailure(t.Context(), errors.New("x")) {
		t.Error("NoteFailure reports a holder where there is none")
	}
}

// TestAFailureTheCallerRecoveredFromDoesNotMaskTheNext: the report this
// rule came from. A first sign-in through an identity provider looks the
// identity up and is told "not found" -- the ordinary answer, which the
// use-case recovers from by creating the account. The lookup was recorded.
// When the insert then fails and the request is answered 500, the line has
// to name the insert. With "the first one wins" it named the lookup, and the
// real cause was written nowhere.
func TestAFailureTheCallerRecoveredFromDoesNotMaskTheNext(t *testing.T) {
	out := captureLogs(t)

	ctx := WithFailure(t.Context())

	// Recorded by the repository, recovered from by the use-case.
	_ = fail(ctx, LayerRepository, errors.New("no account is linked to this identity"))

	// The fault, passed up through the use-case and the handler.
	insert := errors.New("insert into users: connection refused")
	_ = fail(ctx, LayerRepository, insert)
	_ = fail(ctx, LayerUsecase, fmt.Errorf("provisioning the account: %w", insert))
	_ = fail(ctx, LayerHandler, fmt.Errorf("provisioning the account: %w", insert))

	held, ok := FailureFrom(ctx)
	if !ok || !errors.Is(held.Err, insert) {
		t.Fatalf("kept %v, want the insert that failed", held.Err)
	}

	if held.Operation.Layer != LayerRepository {
		t.Errorf("kept the failure as recorded by %q, want where it arose", held.Operation.Layer)
	}

	if out.Len() != 0 {
		t.Errorf("a layer logged:\n%s", out.String())
	}
}

// TestNoteFailureKeepsTheOriginOfTheSameError: the helper that writes a 500
// notes its error. When a layer recorded that error already, deeper, the
// deeper origin stays; an error nothing recorded is the one kept.
func TestNoteFailureKeepsTheOriginOfTheSameError(t *testing.T) {
	captureLogs(t)

	ctx := WithFailure(t.Context())
	deepest := errors.New("deepest")
	_ = fail(ctx, LayerRepository, deepest)

	if !NoteFailure(ctx, fmt.Errorf("answering: %w", deepest)) {
		t.Fatal("NoteFailure found no holder")
	}

	if held, _ := FailureFrom(ctx); held.Operation.Layer != LayerRepository || held.Err != deepest {
		t.Fatalf("kept %v (layer %q), want the repository's record", held.Err, held.Operation.Layer)
	}

	// An error no layer recorded: the handler answered with something else.
	if !NoteFailure(ctx, errors.New("noted later")) {
		t.Fatal("NoteFailure found no holder")
	}

	if held, _ := FailureFrom(ctx); held.Err.Error() != "noted later" {
		t.Fatalf("kept %q, want the error the request was answered with", held.Err)
	}

	alone := WithFailure(t.Context())
	if !NoteFailure(alone, errors.New("only this")) {
		t.Fatal("NoteFailure found no holder")
	}

	if held, ok := FailureFrom(alone); !ok || held.Err.Error() != "only this" {
		t.Fatalf("held %v, %v", held.Err, ok)
	}
}

// TestAHolderWhoseLineIsWrittenIsNoHolder: a context outlives its request.
// The cache refreshes a stale entry on context.WithoutCancel(ctx), which
// keeps the holder; a failure recorded there after the access line was
// written would wait for a line nobody will write. Once the line's reader
// has taken the failure, a layer logs for itself again.
func TestAHolderWhoseLineIsWrittenIsNoHolder(t *testing.T) {
	out := captureLogs(t)

	request := WithFailure(t.Context())
	_ = fail(request, LayerRepository, errors.New("refused in the request"))

	// Reading is not taking: the holder still holds.
	if _, ok := FailureFrom(request); !ok {
		t.Fatal("nothing held")
	}

	_ = fail(request, LayerUsecase, errors.New("still inside the request"))
	if out.Len() != 0 {
		t.Fatalf("a layer logged before the line was written:\n%s", out.String())
	}

	// The access log writes its line.
	if taken, ok := TakeFailure(request); !ok || taken.Err.Error() != "still inside the request" {
		t.Fatalf("took %v, %v", taken.Err, ok)
	}

	// Work that outlived the request, on a copy of its context.
	detached := context.WithoutCancel(request)
	_ = fail(detached, LayerRepository, errors.New("the refresh could not reach the database"))

	logged := out.String()
	if !strings.Contains(logged, "level=ERROR") || !strings.Contains(logged, "msg=operation_failed") ||
		!strings.Contains(logged, "the refresh could not reach the database") {
		t.Fatalf("a failure recorded after the line was written is logged nowhere:\n%s", logged)
	}

	if NoteFailure(detached, errors.New("x")) {
		t.Error("NoteFailure reports a line to come from a sealed holder")
	}

	// And what was taken is not replaced by what came after.
	if held, _ := FailureFrom(request); held.Err.Error() != "still inside the request" {
		t.Errorf("the sealed holder now holds %q", held.Err)
	}

	if _, ok := TakeFailure(t.Context()); ok {
		t.Error("TakeFailure reports a failure where there is no holder")
	}
}

// TestTheHolderIsSafeFromSeveralGoroutines: a use-case may record from
// goroutines it started. Run with -race.
func TestTheHolderIsSafeFromSeveralGoroutines(t *testing.T) {
	captureLogs(t)

	ctx := WithFailure(t.Context())

	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			_ = fail(ctx, LayerUsecase, fmt.Errorf("failure %d", i))
			_, _ = FailureFrom(ctx)
		})
	}
	wg.Wait()

	if _, ok := FailureFrom(ctx); !ok {
		t.Fatal("no failure was kept")
	}
}
