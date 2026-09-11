package app

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
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
	span := trace.SpanContextFromContext(ctx)
	if !span.IsValid() {
		return h.Handler.Handle(ctx, record)
	}

	// Handle must not mutate the caller's record, and a Record is copied by
	// value, so adding to this copy is safe and is what the slog documentation
	// prescribes.
	record.AddAttrs(
		slog.String("trace_id", span.TraceID().String()),
		slog.String("span_id", span.SpanID().String()),
	)

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
