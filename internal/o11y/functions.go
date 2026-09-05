package o11y

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"

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

// FullName creates the dot-notation string (e.g. "service.IDPs.GetByID") for the Span Name
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

	return ctx, span, attrs
}

// SetupTraceHTTP handles HTTP specific attributes alongside your metadata.
//
// action names the operation; see [SetupTrace] for why it is a parameter.
func SetupTraceHTTP(r *http.Request, tracer trace.Tracer, meta Metadata, action string) (context.Context, trace.Span, []attribute.KeyValue) {
	meta.Action = action

	ctx, span := tracer.Start(r.Context(), meta.FullName(), trace.WithSpanKind(trace.SpanKindServer))

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

// RecordError records err on the span (status + RecordError), logs it, and
// updates the duration/outcome metrics. It centralizes error handling to reduce
// duplication across layers.
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

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)

		pc, file, line, ok := runtime.Caller(2)
		if !ok {
			file = "unknown"
			line = 0
		}

		funcName := runtime.FuncForPC(pc).Name()

		// Log structured error
		slog.Error("operation_failed",
			"error", err,
			"func", funcName,
			"file", file,
			"line", line,
			// Add context attributes to logs for correlation
			slog.Group("context", attrsToAny(baseAttrs)...),
		)
	} else {
		mgs := "operation_successful"
		if len(message) != 0 {
			mgs = message[0]
		}

		span.SetStatus(codes.Ok, mgs)
	}

	finalAttrs = append(finalAttrs, attribute.Bool(AttrSuccessful, err == nil))

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
