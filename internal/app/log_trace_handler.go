package app

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// traceAttrsHandler adds trace_id and span_id to records written while a span
// is current.
//
// # Why the standard sink needs this at all
//
// The OpenTelemetry bridge reads the span out of the context and puts the ids
// on the exported record. Nothing does that for the handler writing to
// log.output, so without this the same line is correlated in Loki and not
// correlated in `kubectl logs` -- and `kubectl logs` is where an operator
// looks first, and the only place that works before a log store exists.
//
// The two sinks are meant to differ in exactly one way: TRACE records are
// never exported (see o11y.ExportedLogFloor). This is not that difference.
//
// # The names
//
// trace_id and span_id, snake_case, matching what Loki shows for the exported
// records. A line scraped from stdout and a line delivered by OTLP then carry
// the same field under the same name, so one Grafana derived-field rule covers
// both and a query written against one works against the other.
type traceAttrsHandler struct {
	slog.Handler
}

func (h *traceAttrsHandler) Handle(ctx context.Context, record slog.Record) error {
	// Handle must not mutate the caller's record, and a Record is copied by
	// value, so adding to this copy is safe and is what the slog documentation
	// prescribes.
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", span.TraceID().String()),
			slog.String("span_id", span.SpanID().String()),
		)
	}

	return h.Handler.Handle(ctx, record)
}

// WithAttrs and WithGroup must return a handler of THIS type, or the wrapper is
// dropped the first time a caller does logger.With(...) -- and the c3e cache
// client and the outbound HTTP client both do exactly that at wiring time, so
// their records would silently lose their trace ids.
func (h *traceAttrsHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceAttrsHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *traceAttrsHandler) WithGroup(name string) slog.Handler {
	return &traceAttrsHandler{Handler: h.Handler.WithGroup(name)}
}

// operationAttrsHandler adds the layer, domain and action of the operation a
// record was written inside.
//
// # Why a handler and not the call sites
//
// app.layer, app.domain and app.action are on the span and on the metric. On a
// log record they were on exactly one line -- operation_failed -- because every
// other call site would have had to pass them by hand, and 368 call sites
// passing three attributes each is a rule nobody keeps.
//
// So the operation rides the context, put there by o11y.SetupTrace, which
// already knows it, and is added here once. A log store can then filter by
// layer, domain and action the same way the metrics do: "every line this
// repository call wrote" becomes a query rather than a regex over message text.
//
// # Why it wraps the COMPOSED logger and not the standard handler
//
// It has to reach both sinks, and that is the one thing the first version of
// this got wrong. traceAttrsHandler wraps the standard handler only, which is
// correct for trace ids -- the OpenTelemetry bridge puts those on the exported
// record itself -- but nothing puts the operation on the exported side. The
// attributes went to stdout and not to the log store, and the bridge supplying
// trace_id there hid it: the records looked correlated, so the gap showed up
// only as "3 of 294 exported lines carry app_layer" when the live stack was
// asked directly.
type operationAttrsHandler struct {
	slog.Handler
}

func (h *operationAttrsHandler) Handle(ctx context.Context, record slog.Record) error {
	meta, ok := o11y.OperationFrom(ctx)
	if !ok {
		// Written outside any instrumented operation -- during startup, say.
		// No attributes rather than three empty ones, which would read as "the
		// empty layer" and pollute every filter.
		return h.Handler.Handle(ctx, record)
	}

	record.AddAttrs(
		slog.String(o11y.AttrLayer, meta.Layer),
		slog.String(o11y.AttrDomain, meta.Domain),
		slog.String(o11y.AttrAction, meta.Action),
	)

	return h.Handler.Handle(ctx, record)
}

// WithAttrs and WithGroup must return this type, or the wrapper is dropped the
// first time a caller does logger.With(...) -- and two components do exactly
// that at wiring time.
func (h *operationAttrsHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &operationAttrsHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *operationAttrsHandler) WithGroup(name string) slog.Handler {
	return &operationAttrsHandler{Handler: h.Handler.WithGroup(name)}
}
