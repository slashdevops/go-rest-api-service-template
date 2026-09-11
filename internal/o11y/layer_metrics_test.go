package o11y

import (
	"strings"
	"testing"

	sdkMetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// NewLayerMetrics replaced ~84 copies of the same twenty lines, each of which
// independently chose a name and a description. It is now the only thing that
// builds the pair, so the names it chooses are the contract every dashboard
// query is written against.
func TestNewLayerMetricsBuildsTheSharedPair(t *testing.T) {
	t.Parallel()

	reader := sdkMetric.NewManualReader()
	provider := sdkMetric.NewMeterProvider(sdkMetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })

	metrics, err := NewLayerMetrics(provider.Meter("test"), "")
	if err != nil {
		t.Fatalf("NewLayerMetrics: %v", err)
	}

	if metrics.Counter == nil || metrics.Histogram == nil {
		t.Fatal("both instruments must be built")
	}

	metrics.Counter.Add(t.Context(), 1)
	metrics.Histogram.Record(t.Context(), 0.5)

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	names := map[string]bool{}

	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			names[m.Name] = true
		}
	}

	for _, want := range []string{MetricCalls, MetricCallDuration} {
		if !names[want] {
			t.Errorf("%s was not recorded; collected %v", want, names)
		}
	}
}

// The prefix is the per-deployment metric prefix, applied exactly as the
// per-layer constructors applied it, so an existing MetricsPrefix setting keeps
// working across the rename.
func TestNewLayerMetricsHonoursThePrefix(t *testing.T) {
	t.Parallel()

	reader := sdkMetric.NewManualReader()
	provider := sdkMetric.NewMeterProvider(sdkMetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })

	metrics, err := NewLayerMetrics(provider.Meter("test"), "acme_")
	if err != nil {
		t.Fatalf("NewLayerMetrics: %v", err)
	}

	metrics.Counter.Add(t.Context(), 1)

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var found bool

	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "acme_"+MetricCalls {
				found = true
			}

			if strings.HasPrefix(m.Name, "acme_acme_") {
				t.Errorf("the prefix was applied twice: %s", m.Name)
			}
		}
	}

	if !found {
		t.Errorf("expected acme_%s", MetricCalls)
	}
}

// Every registration of a name must agree on its description, or the SDK
// reports a duplicate-instrument conflict and exports the disagreeing
// registrations as separate, half-populated series. Two calls must therefore
// produce the same instrument, which is what makes ~84 call sites safe.
func TestTwoCallsProduceOneInstrument(t *testing.T) {
	t.Parallel()

	reader := sdkMetric.NewManualReader()
	provider := sdkMetric.NewMeterProvider(sdkMetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })

	first, err := NewLayerMetrics(provider.Meter("test"), "")
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	second, err := NewLayerMetrics(provider.Meter("test"), "")
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	first.Counter.Add(t.Context(), 1)
	second.Counter.Add(t.Context(), 1)

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var matching int

	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == MetricCalls {
				matching++
			}
		}
	}

	if matching != 1 {
		t.Errorf("got %d instruments named %s, want 1; two registrations disagreed", matching, MetricCalls)
	}
}
