package cachevalkey

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/c3e"
)

// Metric instrument names. Counters keep the _total suffix so the Prometheus
// OTLP receiver does not double it (matching the handler/service/repository
// convention).
const (
	metricCacheRequests = "cache_requests_total"
	metricCacheDuration = "cache_duration_seconds"
	metricCacheRefresh  = "cache_refresh_total"
)

// Cache operation and result label values. The get results are supplied by
// [c3e.Result] and are reported verbatim; only invalidate and refresh, which
// c3e reports as a plain error, need their own vocabulary.
const (
	opGet        = "get"
	opInvalidate = "invalidate"

	resultOK    = "ok"
	resultError = "error"
)

// Instruments holds the cache OTEL instruments and adapts them to the
// observability callbacks c3e invokes.
//
// The outcome of a cache read is not something the adapter can observe from
// outside. It used to try: it set a flag inside the fetcher closure and read
// the flag once c3e returned, inferring "the fetcher ran, so it was a miss".
// That was wrong twice over. c3e also calls that closure from the detached
// stale-while-revalidate goroutine, which outlives the read, so the flag was a
// genuine data race — the race detector reports it at the write in the closure
// against the read after the call. And a stale serve, which does run the
// fetcher, was reported as a hit, so the hit ratio quietly counted expired data
// as fresh.
//
// [c3e.Hooks] reports the outcome from inside, where it is known:
// hit / stale / miss / timeout / error, plus the fate of each background
// refresh. Using it removes the race and the guesswork together, and leaves
// [Adapter] with nothing to do but translate types.
//
// A nil *Instruments is a no-op, so the cache works unchanged when telemetry
// is not configured.
type Instruments struct {
	requests metric.Int64Counter
	refresh  metric.Int64Counter
	duration metric.Float64Histogram
}

// NewInstruments builds the cache instruments from meter. A nil meter
// (telemetry disabled) yields a nil *Instruments whose methods are no-ops.
func NewInstruments(meter metric.Meter) (*Instruments, error) {
	if meter == nil {
		return nil, nil
	}

	requests, err := meter.Int64Counter(
		metricCacheRequests,
		metric.WithDescription("Total cache operations by operation and result (hit/stale/miss/timeout/error)."),
	)
	if err != nil {
		return nil, err
	}

	refresh, err := meter.Int64Counter(
		metricCacheRefresh,
		metric.WithDescription("Total background stale-while-revalidate refreshes by result."),
	)
	if err != nil {
		return nil, err
	}

	duration, err := meter.Float64Histogram(
		metricCacheDuration,
		metric.WithDescription("Cache operation duration in seconds."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	return &Instruments{requests: requests, refresh: refresh, duration: duration}, nil
}

// Hooks returns the callbacks to hand to [c3e.SafeCacheManagerConfig]. The
// zero value is a no-op, so a nil *Instruments disables instrumentation
// without disabling the cache.
//
// c3e invokes these from the request goroutine and, for OnRefresh, from the
// detached refresh goroutine. OTEL instruments are safe for concurrent use and
// nothing here blocks, which is what the Hooks contract requires.
func (i *Instruments) Hooks() c3e.Hooks {
	if i == nil {
		return c3e.Hooks{}
	}

	return c3e.Hooks{
		OnGet:        i.onGet,
		OnRefresh:    i.onRefresh,
		OnInvalidate: i.onInvalidate,
	}
}

// onGet records one read. result is c3e's own outcome, so stale is visible as
// stale rather than being folded into hit.
func (i *Instruments) onGet(ctx context.Context, id c3e.CacheIdentifier, result c3e.Result, took time.Duration) {
	i.record(ctx, opGet, id.Type, string(result), took)
}

// onRefresh records the fate of a background stale-while-revalidate refresh.
// A rising error rate here means entries are being served stale for longer
// than SoftTTL suggests, which no other signal exposes.
func (i *Instruments) onRefresh(ctx context.Context, id c3e.CacheIdentifier, err error) {
	// ctx is c3e's detached refresh context — a WithoutCancel copy of the
	// request context, so it still carries the trace span the refresh belongs
	// to even though the request has long returned.
	i.refresh.Add(ctx, 1, metric.WithAttributes(
		attribute.String("entity", id.Type),
		attribute.String("result", resultOf(err)),
	))
}

func (i *Instruments) onInvalidate(ctx context.Context, id c3e.CacheIdentifier, took time.Duration, err error) {
	i.record(ctx, opInvalidate, id.Type, resultOf(err), took)
}

// record emits the request counter and duration histogram for one operation.
// entity is the cached entity type (e.g. "user", "project") — a bounded,
// low-cardinality set.
func (i *Instruments) record(ctx context.Context, operation, entity, result string, took time.Duration) {
	i.requests.Add(ctx, 1, metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("entity", entity),
		attribute.String("result", result),
	))
	i.duration.Record(ctx, took.Seconds(), metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("entity", entity),
	))
}

func resultOf(err error) string {
	if err != nil {
		return resultError
	}

	return resultOK
}
