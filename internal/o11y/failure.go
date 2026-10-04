package o11y

import (
	"context"
	"errors"
	"runtime"
	"sync"
)

// Failure is the failure a request is answered for: what failed and where it
// arose.
type Failure struct {
	// Err is the error as it was recorded.
	Err error

	// Operation is the operation it was recorded in, when there was one.
	Operation Metadata

	// Type is the concrete type at the bottom of Err's wrap chain: a stable
	// key for "which failure is this" (see errorType).
	Type string

	// Func, File and Line are where it was recorded.
	Func string
	File string
	Line int
}

// failureKey carries the per-request failure holder.
type failureKey struct{}

// failureHolder keeps the first failure of a request.
//
// # Why a failure is held and not logged
//
// Every layer records a failure through RecordError, and RecordError used to
// write an ERROR line, `operation_failed`, each time. A request with a bad
// field is refused by the repository, then by the use-case that returns the
// error, then by the handler that answers it: three ERROR lines for one
// refusal, which the service then answers, correctly, with a 400. Measured in
// svc-qu3ry-core, where this was built, on the log of two runs of its
// integration suite: 3 038 of 3 090 such lines (98%) were about requests
// refused on purpose; eight requests were answered with a 5xx.
//
// Two things were wrong, and neither can be put right inside a layer:
//
//   - the level. Whether a failure is the service's fault is decided by how
//     the request is answered, and no layer below the HTTP boundary knows
//     that: a NotFoundError is a 404 on one route and a 400 on another, where
//     the missing thing was named in the body. Only the handler decides, and
//     only the middleware sees what it decided.
//   - the count. Each layer logging what it returns is the same error three
//     times. An error is logged once, where it is handled.
//
// So a layer records what failed and where into this holder, which the
// tracing middleware installed above everything, and the access log, which
// knows the status, writes the one line: at ERROR for a 5xx, with the cause;
// at INFO for a refusal, with a DEBUG line for the detail.
//
// It is the mechanism the request's subject already uses to reach the access
// log, for the same reason: a context flows down and never back up, but a
// pointer placed in it once is reachable from both ends.
//
// # Which failure is kept
//
// The one that is travelling up, where it arose. The deepest layer records
// first -- the repository returns before the use-case does -- and the layers
// above record the same error, or a wrap of it, as they pass it on: those do
// not replace it, so what is kept is the function, file and line worth
// reading.
//
// A failure that is NOT the held one, or a wrap of it, replaces it. A
// use-case recovers from some errors: a first sign-in through an identity
// provider looks the identity up, is told "not found", and goes on to create
// the account. That lookup was recorded. If the account's insert then fails
// and the request is answered 500, the line must name the insert, and with
// "the first one wins" it named the lookup -- the real cause was written
// nowhere. The error a caller recovered from is, by definition, not the one
// that comes up next.
//
// # Sealed once the line is written
//
// The access log takes the failure (TakeFailure) and the holder is sealed.
// A context outlives its request: the cache refreshes a stale entry on
// context.WithoutCancel(ctx), which keeps the context's values, and so the
// holder. A failure recorded there, after the line was written, would be held
// for a line nobody will write. A sealed holder is no holder: the layer logs
// the failure itself, as it does for any work that is not a request.
//
// # On concurrency
//
// A use-case may record from a goroutine it started (an embedding batch),
// so, unlike the subject's holder, this one is locked.
type failureHolder struct {
	failure *Failure
	mu      sync.Mutex
	sealed  bool
}

// WithFailure installs the holder. It is called once per request, by the
// tracing middleware; everything that records a failure runs below it.
func WithFailure(ctx context.Context) context.Context {
	return context.WithValue(ctx, failureKey{}, &failureHolder{})
}

// TakeFailure returns the failure held under ctx, and whether there was one,
// and seals the holder: it is for the one reader that writes the line, the
// access log. What is recorded afterwards is logged by its layer.
func TakeFailure(ctx context.Context) (Failure, bool) {
	holder, _ := ctx.Value(failureKey{}).(*failureHolder)
	if holder == nil {
		return Failure{}, false
	}

	holder.mu.Lock()
	defer holder.mu.Unlock()

	holder.sealed = true

	if holder.failure == nil {
		return Failure{}, false
	}

	return *holder.failure, true
}

// FailureFrom returns the failure held under ctx, and whether there was one,
// without sealing the holder.
func FailureFrom(ctx context.Context) (Failure, bool) {
	holder, _ := ctx.Value(failureKey{}).(*failureHolder)
	if holder == nil {
		return Failure{}, false
	}

	holder.mu.Lock()
	defer holder.mu.Unlock()

	if holder.failure == nil {
		return Failure{}, false
	}

	return *holder.failure, true
}

// holdFailure gives f to the request's holder and reports whether a line
// will be written for it. With no holder -- a reloader, startup: work that
// is not a request -- or with one whose line was already
// written, nothing will write it later, and the caller logs the failure
// itself.
//
// The held failure stays when f is the same error passed on (itself or a
// wrap of it); otherwise f replaces it. See failureHolder.
func holdFailure(ctx context.Context, f Failure) bool {
	holder, _ := ctx.Value(failureKey{}).(*failureHolder)
	if holder == nil {
		return false
	}

	holder.mu.Lock()
	defer holder.mu.Unlock()

	if holder.sealed {
		return false
	}

	if holder.failure == nil || !errors.Is(f.Err, holder.failure.Err) {
		holder.failure = &f
	}

	return true
}

// NoteFailure hands err to the request's failure holder from outside an
// instrumented operation -- the code that writes a 500 for an error nothing
// below recorded -- and reports whether a line will be written for it. With
// no holder, or a sealed one, the caller logs the error itself. When a layer
// already recorded the same error the held one is kept: it is where the
// error arose.
func NoteFailure(ctx context.Context, err error) bool {
	failure := Failure{Err: err, Type: errorType(err)}

	// The caller of NoteFailure's caller: the handler or the middleware that
	// answered (or respond.WriteError, when the answer went through it), not
	// the helper that wrote the 500.
	if pc, file, line, ok := runtime.Caller(2); ok {
		failure.Func, failure.File, failure.Line = runtime.FuncForPC(pc).Name(), file, line
	}

	failure.Operation, _ = OperationFrom(ctx)

	return holdFailure(ctx, failure)
}
