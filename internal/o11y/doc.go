// Package o11y provides a unified observability layer for the
// application — distributed tracing, metrics, and structured error
// recording on top of [OpenTelemetry].
//
// # Why the name?
//
// "o11y" is the standard industry numeronym for "observability": the
// letter O, the digit 11 counting the eleven omitted middle letters
// "bservabilit", and the trailing letter y. It is the same contraction
// pattern as k8s (Kubernetes, 8 omitted letters) and i18n
// (Internationalization, 18 omitted letters).
//
// Go's style guide discourages opaque "container" package names — but
// it accepts widely-understood numeronyms when they are documented.
// This docs.go is that documentation; the package name is shorter at
// call sites (o11y.RecordError, o11y.SetupTraceHTTP) than the full
// spelling would be.
//
// If the abbreviation is ever a barrier, the package can be re-aliased
// at the import site:
//
//	import telemetry "github.com/slashdevops/go-rest-api-service-template/internal/o11y"
//
// # Architecture
//
// The package is organised around three pillars of observability:
//
//   - Tracing — distributed-trace propagation using OpenTelemetry
//     spans, so request flows can be followed across service
//     boundaries.
//   - Metrics — counter and histogram instruments measuring operation
//     counts and latency distributions per layer, domain, and action.
//   - Structured metadata — a [Metadata] type that encodes the
//     application's three-level architecture (Layer / Domain / Action)
//     into span names and metric attributes for consistent, queryable
//     telemetry.
//
// # Core types
//
// [OpenTelemetry] is the top-level entry point. It composes an
// [OpenTelemetryTracer] and an [OpenTelemetryMeter]; each is
// independently configurable with pluggable exporters (console,
// otlp-http, noop).
//
// [Metadata] describes the calling context using three fields:
//
//   - Layer  — the architectural layer, e.g. "handler", "service",
//     "repository".
//   - Domain — the business domain, e.g. "Users", "Embeddings".
//   - Action — the specific operation, e.g. "GetByID", "Insert".
//
// These fields are projected into span names in dot-notation
// (e.g. "service.Users.GetByID") and into OpenTelemetry attributes
// for filtering and aggregation in the backend.
//
// [LayerMetrics] bundles a counter and a histogram for a given layer.
// It is typically constructed once per handler, service, or repository
// and passed to the recording functions on every operation.
//
// # Recording functions
//
// Three helpers unify span-status updates, structured logging, and
// metric emissions so callers don't have to repeat the same boilerplate
// at every return site:
//
//   - [RecordResult]  — the core function. Computes duration, sets the
//     span status (Ok or Error), logs errors with caller information,
//     and records both the counter and histogram metrics.
//   - [RecordError]   — convenience wrapper around RecordResult for
//     error paths. Returns the original error so callers can chain
//     `return o11y.RecordError(...)`.
//   - [RecordSuccess] — convenience wrapper around RecordResult for
//     success paths, with an optional message.
//
// # Trace setup helpers
//
// Three more helpers create spans with consistent naming and
// attributes:
//
//   - [SetupTrace]            — starts a span using [Metadata] and
//     returns the enriched context, span, and attributes. The caller
//     must defer span.End().
//   - [SetupTraceHTTP]        — like SetupTrace, but also adds
//     HTTP-specific attributes (method, route) extracted from the
//     incoming request.
//   - [SetupTraceWithTimeout] — like SetupTrace, but also wraps the
//     context with a timeout and returns the cancel function. The
//     caller must defer cancel() alongside defer span.End().
//
// # Exporters
//
// Both the tracer and the meter accept multiple exporter backends,
// selected by string identifier:
//
//   - "noop"      — no-op exporter. Useful for tests; nothing is
//     emitted.
//   - "console"   — exports to stdout in a human-readable format.
//   - "otlp-http" — exports via OTLP over HTTP with gzip compression
//     (the production default).
//
// # Usage
//
// A typical service or repository builds its [Metadata] once, for its layer and
// domain, and names the operation per call:
//
//	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(
//	    ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "GetByID",
//	)
//	defer cancel()
//	defer span.End()
//
//	result, err := ref.db.Query(ctx, query)
//	if err != nil {
//	    return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
//	}
//
//	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "fetched user")
//
// # The Action is a parameter, and that is not a style choice
//
// This documentation used to show the action being ASSIGNED to the stored
// metadata -- `ref.metricsMetadata.Action = "GetByID"` -- and 447 call sites
// followed it. That field lives on the handler, service or repository, which is
// shared by every request the instance serves concurrently, so the assignment
// is a data race: two requests in flight write the same word, and whichever
// loses has its span and its metric filed under the other one's action.
//
// It went unnoticed for a long time because nothing drove one instance
// concurrently under `-race`: no unit test did, and the integration suite runs
// a binary built without it. It surfaced the moment two endpoints of one
// handler were driven at once in a test.
//
// [Metadata] is passed by value, so [SetupTrace] and friends set Action on
// their own copy and the caller's instance is never touched.
// TestNoSharedMetadataActionWrite fails the build if the old pattern comes
// back.
//
// [OpenTelemetry]: https://opentelemetry.io
package o11y
