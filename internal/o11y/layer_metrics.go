package o11y

import (
	"fmt"
	"time"

	"go.opentelemetry.io/otel/metric"
	sdkMetric "go.opentelemetry.io/otel/sdk/metric"
)

// The layers a call can be attributed to.
//
// # They are port ROLES, not package names
//
// `repository` is the driven port for persistence, whatever adapter implements
// it. Swapping repositorypg for another driver must not rename a metric or
// invalidate a dashboard, so the value names the role the hexagon gives it and
// not the package that happens to fill it today.
//
// The mapping to the hexagon's rings -- driving adapter, core, driven adapters
// -- lives in docs/architecture/observability.md. It is deliberately NOT a
// second attribute: no panel would ever ask for "all driven adapters
// together", it is derivable from the layer, and a label nobody queries is
// cardinality nobody wanted.
//
// `usecase` used to be spelled `service`. The package has been usecase since
// the ports refactor, and next to service.name and service_name the old
// spelling read as "the whole binary" rather than "the core".
const (
	LayerHandler    = "handler"
	LayerUsecase    = "usecase"
	LayerRepository = "repository"
	LayerCache      = "cache"
	LayerMail       = "mail"
)

// The two instruments every layer records on.
//
// # One pair, with the layer as an attribute
//
// These used to be six instruments -- handler_calls_total, service_calls_total,
// repository_calls_total and a duration histogram for each -- so the layer was
// encoded in the metric NAME. That is backwards from how OpenTelemetry's
// semantic conventions work, where a dimension is an attribute
// (http.server.request.duration{http.route}) and never part of the name, and it
// had two costs that were paid daily:
//
//   - A panel comparing layers had to regex over metric names:
//     {__name__=~"(handler|service|repository)_calls_total"}. With one
//     instrument it is sum by (app_layer)(rate(app_calls_total[...])), and a
//     dashboard variable over the layer becomes possible at all.
//   - Adding a layer meant a fourth copy of a per-package constants file and
//     two more names for every dashboard to learn, which is why the driven
//     adapters were instrumented unevenly.
//
// Total series count is unchanged: the same layer x domain x action
// combinations exist, in one family instead of three.
const (
	MetricCalls        = "app_calls_total"
	MetricCallDuration = "app_call_duration_seconds"
)

// metricCallsDescription and metricCallDurationDescription are fixed strings,
// and that matters more than it looks.
//
// Every construction of an instrument with the same name must agree on its
// description, or the SDK reports a duplicate-instrument conflict and the
// disagreeing registrations are exported as separate, half-populated series.
// With the layer in the name that was invisible: each layer had its own
// instrument, so each could describe itself differently, and three spellings
// had drifted in ("Duration of %s calls", "Duration of %s handler calls",
// "Duration of %s service calls" -- the middle one nonsense for a repository).
// Sharing one name makes agreement mandatory, so the text lives here once.
const (
	metricCallsDescription        = "Calls made at a layer of the application, by domain and action"
	metricCallDurationDescription = "Duration of a call at a layer of the application, by domain and action"
)

// NewLayerMetrics builds the counter/histogram pair a handler, use case,
// repository or driven adapter records on.
//
// prefix is the optional per-deployment metric prefix; it is applied exactly as
// the per-layer constructors applied it, so an existing MetricsPrefix setting
// keeps working.
//
// It replaces ~84 copies of the same twenty lines. Each copy independently
// chose a name and a description, which is how the descriptions drifted apart.
func NewLayerMetrics(meter metric.Meter, prefix string) (*LayerMetrics, error) {
	counter, err := meter.Int64Counter(
		prefix+MetricCalls,
		metric.WithDescription(metricCallsDescription),
	)
	if err != nil {
		return nil, fmt.Errorf("creating the %s counter: %w", MetricCalls, err)
	}

	histogram, err := meter.Float64Histogram(
		prefix+MetricCallDuration,
		metric.WithDescription(metricCallDurationDescription),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating the %s histogram: %w", MetricCallDuration, err)
	}

	return &LayerMetrics{Counter: counter, Histogram: histogram}, nil
}

// callDurationBuckets are the histogram boundaries for [MetricCallDuration].
//
// # Why they are set explicitly
//
// The SDK's default boundaries stop at 10 seconds. That was survivable while
// every layer was a database call measured in milliseconds; it is not, now that
// the same instrument also times generation through an LLM, which routinely
// runs for tens of seconds and is allowed to run for twenty minutes (there is
// deliberately no write timeout on the server for that reason -- see
// docs/architecture/http-server-timeouts.md). Everything past the last bucket
// lands in +Inf, so a p95 over generation latency would have been pinned at
// 10s and told nobody anything.
//
// The boundaries run 1ms to 10min, roughly doubling. A repository call lands in
// the low buckets with enough resolution to see a regression, and a slow
// outbound call lands somewhere real instead of in the overflow.
var callDurationBuckets = []float64{
	0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5,
	1, 2.5, 5, 10, 30, 60, 120, 300, 600,
}

// CallDurationView applies [callDurationBuckets] to the shared duration
// histogram, whatever prefix a deployment set.
func CallDurationView() sdkMetric.View {
	return sdkMetric.NewView(
		sdkMetric.Instrument{
			Name: "*" + MetricCallDuration,
			Kind: sdkMetric.InstrumentKindHistogram,
		},
		sdkMetric.Stream{
			Aggregation: sdkMetric.AggregationExplicitBucketHistogram{
				Boundaries: callDurationBuckets,
			},
		},
	)
}

// compile-time proof the bucket list is ordered and has no duplicates: an
// unordered boundary list is accepted by the SDK and produces a histogram whose
// quantiles are silently wrong.
var _ = func() struct{} {
	for i := 1; i < len(callDurationBuckets); i++ {
		if callDurationBuckets[i] <= callDurationBuckets[i-1] {
			panic(fmt.Sprintf("callDurationBuckets must be strictly increasing; %v <= %v at index %d",
				callDurationBuckets[i], callDurationBuckets[i-1], i))
		}
	}

	// Keep the time import honest about what the numbers mean.
	if callDurationBuckets[len(callDurationBuckets)-1] != (10 * time.Minute).Seconds() {
		panic("the last bucket should be 10 minutes")
	}

	return struct{}{}
}()
