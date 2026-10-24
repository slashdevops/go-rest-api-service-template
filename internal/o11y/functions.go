package o11y

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"
	"unicode/utf8"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	AttrLayer      = "app.layer"
	AttrDomain     = "app.domain"
	AttrAction     = "app.action"
	AttrSuccessful = "successful"
)

// Metadata holds the 3-level architecture context
type Metadata struct {
	// Layer represents the application layer, e.g., "handler", "service", "repository"
	// this is usually the package name in lowercase
	Layer string

	// Domain represents the domain or component, e.g., "Users", "Projects"
	// this is usually a struct that represents a specific area of functionality
	Domain string

	// Action represents the specific action or operation, e.g., "GetByID", "Insert"
	// this is usually a method or function name.
	//
	// It is supplied per call, to [SetupTrace], [SetupTraceHTTP] or
	// [SetupTraceWithTimeout], and NOT stored. A handler, service or repository
	// keeps one Metadata for its layer and domain, and that value is shared by
	// every request the instance serves concurrently -- so writing Action on it
	// per request is a data race, and it mislabels whichever span and metric
	// loses. That is exactly what this codebase used to do, at 447 sites; see
	// TestNoSharedMetadataActionWrite.
	Action string
}

// FullName creates the dot-notation string (e.g. "usecase.IDPs.GetByID") for the Span Name
func (m Metadata) FullName() string {
	return fmt.Sprintf("%s.%s.%s", m.Layer, m.Domain, m.Action)
}

// ToAttributes converts metadata to OTel attributes
func (m Metadata) ToAttributes() []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(AttrLayer, m.Layer),
		attribute.String(AttrDomain, m.Domain),
		attribute.String(AttrAction, m.Action),
	}
}

// operationKey carries the current operation, so a log record written anywhere
// inside it can be attributed without the call site saying so.
type operationKey struct{}

// WithOperation puts meta on the context.
//
// # Why this exists
//
// app.layer, app.domain and app.action are on the span and on the metric.
// On a LOG record they were on exactly one line -- operation_failed -- because
// every other call site would have had to pass them by hand, and 368 call
// sites passing three attributes each is a rule nobody keeps.
//
// So the operation travels on the context instead, put there once by the
// function that already knows it, and a handler adds the attributes to every
// record written while it is in scope. The result is that a log store can
// filter on layer, domain and action the same way the metrics do, and no call
// site mentions them.
//
// It is deliberately NOT read from the span. A span name is not readable from
// a SpanContext -- only the SDK's ReadOnlySpan has it -- and reaching for the
// SDK from a log handler would tie the two pipelines together for a string
// this package already has in hand.
func WithOperation(ctx context.Context, meta Metadata) context.Context {
	return context.WithValue(ctx, operationKey{}, meta)
}

// OperationFrom returns the operation in scope, and whether there was one.
// A line written outside any instrumented operation -- during startup, say --
// has none, and gets no attributes rather than empty ones.
func OperationFrom(ctx context.Context) (Metadata, bool) {
	meta, ok := ctx.Value(operationKey{}).(Metadata)

	return meta, ok
}

// SetupTrace starts a span for an in-process operation (service or repository
// layer). It is created with SpanKindInternal; the HTTP entrypoint span (see
// SetupTraceHTTP) is the SpanKindServer root of the trace.
//
// action names the operation and is applied to a COPY of meta -- Metadata is
// passed by value, so the caller's shared instance is never touched.
//
// IMPORTANT: It does NOT defer span.End(). The caller must do that.
func SetupTrace(ctx context.Context, tracer trace.Tracer, meta Metadata, action string) (context.Context, trace.Span, []attribute.KeyValue) {
	meta.Action = action

	// Start span with the structured name
	ctx, span := tracer.Start(ctx, meta.FullName(), trace.WithSpanKind(trace.SpanKindInternal))

	// Generate standard attributes
	attrs := meta.ToAttributes()
	span.SetAttributes(attrs...)

	// And on the context, so every log record written inside this operation
	// carries them too -- see [WithOperation].
	ctx = WithOperation(ctx, meta)

	return ctx, span, attrs
}

// SetupTraceHTTP handles HTTP specific attributes alongside your metadata.
//
// action names the operation; see [SetupTrace] for why it is a parameter.
func SetupTraceHTTP(r *http.Request, tracer trace.Tracer, meta Metadata, action string) (context.Context, trace.Span, []attribute.KeyValue) {
	meta.Action = action

	ctx, span := tracer.Start(r.Context(), meta.FullName(), trace.WithSpanKind(trace.SpanKindServer))

	ctx = WithOperation(ctx, meta)

	baseAttrs := meta.ToAttributes()

	// HTTP semantic-convention attributes (semconv v1.41). http.route falls back
	// to the action name since the mux pattern is not exposed to the handler.
	httpAttrs := []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String(meta.Action),
	}

	allAttrs := append(baseAttrs, httpAttrs...)
	span.SetAttributes(allAttrs...)

	return ctx, span, allAttrs
}

// Helper functions for common patterns

// SetupTraceWithTimeout creates a context with timeout and starts a span with standard attributes.
// Returns the new context, span, and common metric attributes.
//
// action names the operation; see [SetupTrace] for why it is a parameter.
func SetupTraceWithTimeout(ctx context.Context, tracer trace.Tracer, timeout time.Duration, meta Metadata, action string) (context.Context, trace.Span, []attribute.KeyValue, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	ctx, span, attrs := SetupTrace(ctx, tracer, meta, action)

	return ctx, span, attrs, cancel
}

// Helper to convert OTel attributes to slog args
func attrsToAny(attrs []attribute.KeyValue) []any {
	args := make([]any, len(attrs)*2)
	for i, kv := range attrs {
		args[i*2] = string(kv.Key)
		args[i*2+1] = kv.Value.AsString()
	}
	return args
}

// RecordError records err on the span (status + RecordError) and updates the
// duration/outcome metrics. It centralizes error handling to reduce
// duplication across layers.
//
// It does not log a failure that happens inside a request: it hands it to the
// request's failure holder, and the access log writes one line for the
// request once it knows the status it was answered with (see failureHolder).
// Outside a request -- a reloader, startup -- it logs
// `operation_failed` at ERROR itself, because nothing else will.
//
// Parameters:
//   - ctx: request context (used for metric recording)
//   - span: the active span; its status is set to Error and the error is attached
//   - startTime: when the operation began, for the duration histogram
//   - err: the error to record (must be non-nil to be meaningful)
//   - metrics: optional counter/histogram instruments; nil disables metrics
//   - baseAttrs: low-cardinality attributes (layer/domain/action) for metrics + logs
//
// It returns err unchanged so callers can `return o11y.RecordError(...)`.
func RecordError(ctx context.Context, span trace.Span, startTime time.Time, err error, metrics *LayerMetrics, baseAttrs []attribute.KeyValue) error {
	// RecordResult returns err unchanged, so returning it here keeps the
	// chainable `return o11y.RecordError(...)` contract.
	return RecordResult(ctx, span, startTime, metrics, baseAttrs, err)
}

// RecordSuccess records a successful operation on the span and updates the
// duration/outcome metrics. It centralizes success handling to reduce
// duplication across layers.
//
// extraAttrs are attached to the span ONLY (not to metrics), so callers can
// safely pass identifying, potentially high-cardinality values (e.g. a
// role.id or user.email) for trace debugging without polluting metric
// cardinality.
//
// Parameters:
//   - ctx: request context (used for metric recording)
//   - span: the active span; its status is set to Ok
//   - startTime: when the operation began, for the duration histogram
//   - metrics: optional counter/histogram instruments; nil disables metrics
//   - baseAttrs: low-cardinality attributes (layer/domain/action) for metrics
//   - msg: human-readable success message (used as the span status description source)
//   - extraAttrs: optional span-only attributes
func RecordSuccess(ctx context.Context, span trace.Span, startTime time.Time, metrics *LayerMetrics, baseAttrs []attribute.KeyValue, msg string, extraAttrs ...attribute.KeyValue) {
	if len(extraAttrs) > 0 {
		span.SetAttributes(extraAttrs...)
	}

	// Success path: RecordResult returns nil, nothing to propagate.
	_ = RecordResult(ctx, span, startTime, metrics, baseAttrs, nil, msg)
}

// detailed is an error that carries what an operator reads about it and a
// caller never does (domain.FailureDetail). It is matched by its method so
// that this package does not import the domain.
type detailed interface {
	error
	FailureDetail() string
}

// maxFailureDetail bounds a detail: a provider's answer can carry a whole
// error page, and the detail is exported.
const maxFailureDetail = 1024

// failureDetail is the detail of the first error in err's chain that has
// one, bounded to maxFailureDetail bytes on a rune boundary.
func failureDetail(err error) string {
	d, ok := errors.AsType[detailed](err)
	if !ok {
		return ""
	}

	detail := d.FailureDetail()
	if len(detail) <= maxFailureDetail {
		return detail
	}

	cut := maxFailureDetail
	for cut > 0 && !utf8.RuneStart(detail[cut]) {
		cut--
	}

	return detail[:cut] + "…"
}

// errorType is a stable key for "which failure is this".
//
// # Why the message is not one
//
// Counting failures needs something that does not change when the wording
// does. The message is free text, it is often a wrapped vendor string, and a
// dependency bump rewrites it -- at which point a dashboard counting "how many
// times did THIS failure happen" silently starts counting two things, or
// nothing. The concrete Go type does not move.
//
// It reports the type at the BOTTOM of the wrap chain, because that is where
// the cause is: fmt.Errorf("selecting the user: %w", pgx.ErrNoRows) is a
// *fmt.wrapError all the way up, and every distinct failure in the service
// would share that one value.
//
// It is deliberately not put on the metrics. The set of error types is bounded
// by the code rather than by traffic, but it is large, and the metric already
// carries app.outcome with the layer, domain and action -- which is the
// question a metric should answer. "Which error" is a log question, and the log
// line is joined to the metric by the trace id.
func errorType(err error) string {
	if err == nil {
		return ""
	}

	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			// The context's sentinel is an *errors.errorString, as any
			// errors.New is: the type would name nothing. A request its
			// caller abandoned says what it was.
			if errors.Is(err, context.Canceled) {
				return "context.Canceled"
			}

			return fmt.Sprintf("%T", err)
		}

		err = unwrapped
	}
}

// EventRefused is the span event a refused call carries in place of an Error
// status.
const EventRefused = "refused"

// markSpan says on the span how a call that returned err ended.
//
// # A refused call is not a red span
//
// Every call that returned an error used to be marked Error, with an
// exception event. A request answered 404 was then a trace with a red span
// at every layer it passed through, and a trace backend's "status = error"
// was a list of the requests clients got wrong. The server span has
// followed the HTTP rule since it was introduced (only a 5xx is an Error);
// the spans under it did not.
//
// So the status follows the outcome the metrics already record
// (app.outcome, see SetRefusalClassifier):
//
//   - failed: Error, and the error as an exception event, as before.
//   - refused: the status is left unset, as OpenTelemetry asks of a 4xx on
//     the server side, and the refusal is an event named "refused" with the
//     error's type and text. It is still on the span, for whoever opens the
//     trace of a complaint; it is not what an error search returns.
//
// It is decided per call, as the metric is, and for the same reason: a span
// ends when its call returns, before anything knows how the request will be
// answered, and a status cannot be set on a span that has ended.
func markSpan(span trace.Span, outcome string, err error) {
	if outcome == OutcomeRefused {
		span.AddEvent(EventRefused, trace.WithAttributes(
			attribute.String("error.type", errorType(err)),
			attribute.String("error.message", err.Error()),
		))

		return
	}

	span.SetStatus(codes.Error, err.Error())
	span.RecordError(err)
}

// LayerMetrics holds the metrics instruments for the handler package.
type LayerMetrics struct {
	Counter   metric.Int64Counter
	Histogram metric.Float64Histogram
}

// RecordResult records both the outcome (Counter) and the duration (Histogram).
// It combines RecordError and RecordSuccess logic to keep things DRY.
func RecordResult(
	ctx context.Context,
	span trace.Span,
	startTime time.Time,
	metrics *LayerMetrics,
	baseAttrs []attribute.KeyValue,
	err error,
	message ...string,
) error {
	// 1. Calculate Duration
	duration := time.Since(startTime).Seconds()

	// 2. Prepare Attributes
	finalAttrs := make([]attribute.KeyValue, len(baseAttrs))
	copy(finalAttrs, baseAttrs)

	outcome := Outcome(err)

	// On the span as well, so a trace can be asked for by how a call ended
	// ({ span.app.outcome = "failed" }) the way the metrics are.
	span.SetAttributes(attribute.String(AttrOutcome, outcome))

	if err != nil {
		markSpan(span, outcome, err)

		pc, file, line, ok := runtime.Caller(2)
		if !ok {
			file = "unknown"
			line = 0
		}

		funcName := runtime.FuncForPC(pc).Name()

		failure := Failure{Err: err, Type: errorType(err), Detail: failureDetail(err), Func: funcName, File: file, Line: line}
		failure.Operation, _ = OperationFrom(ctx)

		// Inside a request the failure is held, not logged: the access log
		// writes one line for it once it knows how the request was answered
		// (see failureHolder). Outside one, or once that line is written,
		// there is nobody to write it later.
		if !holdFailure(ctx, failure) {
			// It takes ctx -- ErrorContext, not Error -- because the
			// OpenTelemetry bridge reads the span out of the context and from
			// nowhere else.
			//
			// The layer, domain and action are NOT repeated here: they are on
			// the context from [SetupTrace], and the handler adds them to
			// every record written inside the operation.
			args := []any{
				"error", err,
				"error_type", failure.Type,
				"func", funcName,
				"file", file,
				"line", line,
			}

			if failure.Detail != "" {
				args = append(args, "detail", failure.Detail)
			}

			slog.ErrorContext(ctx, "operation_failed", args...)
		}
	} else {
		mgs := "operation_successful"
		if len(message) != 0 {
			mgs = message[0]
		}

		span.SetStatus(codes.Ok, mgs)
	}

	// How the call ended, and for one more release the boolean it replaces:
	// `successful` cannot tell a refusal from a failure, and the dashboards
	// read `app.outcome` now (see SetRefusalClassifier).
	finalAttrs = append(finalAttrs,
		attribute.String(AttrOutcome, outcome),
		attribute.Bool(AttrSuccessful, err == nil),
	)

	// 3. Record Metrics
	if metrics != nil {
		// Increment Counter
		if metrics.Counter != nil {
			metrics.Counter.Add(ctx, 1, metric.WithAttributes(finalAttrs...))
		}
		// Record Duration to Histogram
		if metrics.Histogram != nil {
			metrics.Histogram.Record(ctx, duration, metric.WithAttributes(finalAttrs...))
		}
	}

	return err
}
