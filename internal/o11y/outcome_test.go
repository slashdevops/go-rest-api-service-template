//go:build unit

package o11y

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkMetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace/noop"
)

// errRefusal stands for an error the service answers with a 4xx.
type errRefusal struct{ what string }

func (e *errRefusal) Error() string { return e.what }

// classifyRefusals installs a classifier for the test and removes it after.
func classifyRefusals(t *testing.T) {
	t.Helper()

	SetRefusalClassifier(func(err error) bool {
		_, ok := errors.AsType[*errRefusal](err)

		return ok
	})
	t.Cleanup(func() { SetRefusalClassifier(nil) })
}

// outcomes records one call per error through RecordError / RecordSuccess and
// returns, per instrument, how many data points carry each app.outcome and
// each successful value.
func outcomes(t *testing.T, errs ...error) map[string]map[string]int64 {
	t.Helper()

	reader := sdkMetric.NewManualReader()
	provider := sdkMetric.NewMeterProvider(sdkMetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })

	metrics, err := NewLayerMetrics(provider.Meter("test"), "")
	if err != nil {
		t.Fatalf("NewLayerMetrics: %v", err)
	}

	tracer := noop.NewTracerProvider().Tracer("test")

	for _, callErr := range errs {
		ctx, span, attrs := SetupTrace(t.Context(), tracer, Metadata{Layer: LayerRepository, Domain: "Things"}, "List")
		if callErr != nil {
			_ = RecordError(ctx, span, time.Now(), callErr, metrics, attrs)
		} else {
			RecordSuccess(ctx, span, time.Now(), metrics, attrs, "listed")
		}
		span.End()
	}

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	got := map[string]map[string]int64{}
	add := func(name string, set attribute.Set, n int64) {
		if got[name] == nil {
			got[name] = map[string]int64{}
		}

		outcome, _ := set.Value(AttrOutcome)
		series := outcome.AsString()

		// The boolean app.outcome replaced was kept for one release
		// (v0.0.4) and is gone: a point that still carries it is a series
		// of its own here, and fails the comparison.
		if _, kept := set.Value("successful"); kept {
			series += "/successful"
		}

		got[name][series] += n
	}

	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch data := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, point := range data.DataPoints {
					add(m.Name, point.Attributes, point.Value)
				}
			case metricdata.Histogram[float64]:
				for _, point := range data.DataPoints {
					add(m.Name, point.Attributes, int64(point.Count))
				}
			}
		}
	}

	return got
}

// TestACallSaysHowItEnded: `successful="false"` counted a refusal and a
// failure as one thing, so a burst of bad requests raised every error ratio
// and a database that had gone away raised it no more than that. Each call
// now says which it was, on both instruments.
func TestACallSaysHowItEnded(t *testing.T) {
	captureLogs(t)
	classifyRefusals(t)

	notFound := &errRefusal{what: "no such thing"}

	got := outcomes(t,
		nil, nil, nil, // three calls that worked
		notFound, // refused
		fmt.Errorf("listing things: %w", notFound),                         // refused, wrapped by a layer above
		errors.Join(errors.New("while closing"), notFound),                 // refused, joined
		errors.New("connection refused"),                                   // failed
		fmt.Errorf("listing things: %w", errors.New("connection refused")), // failed, wrapped
	)

	want := map[string]int64{
		"ok":      3,
		"refused": 3,
		"failed":  2,
	}

	for _, instrument := range []string{MetricCalls, MetricCallDuration} {
		if len(got[instrument]) != len(want) {
			t.Errorf("%s: series %v, want %v", instrument, got[instrument], want)
		}

		for series, n := range want {
			if got[instrument][series] != n {
				t.Errorf("%s{%s} = %d, want %d (all: %v)", instrument, series, got[instrument][series], n, got[instrument])
			}
		}
	}
}

// TestWithNoClassifierEveryErrorIsAFailure: a test, or a tool that serves no
// request, never sets one. Calling an error a refusal needs a reason; with
// none it is a failure.
func TestWithNoClassifierEveryErrorIsAFailure(t *testing.T) {
	captureLogs(t)
	SetRefusalClassifier(nil)

	if got := Outcome(&errRefusal{what: "no such thing"}); got != OutcomeFailed {
		t.Errorf("Outcome = %q with no classifier, want %q", got, OutcomeFailed)
	}

	if got := Outcome(nil); got != OutcomeOK {
		t.Errorf("Outcome(nil) = %q, want %q", got, OutcomeOK)
	}

	got := outcomes(t, &errRefusal{what: "no such thing"})
	if got[MetricCalls]["failed"] != 1 {
		t.Errorf("recorded %v, want one failed call", got[MetricCalls])
	}
}

// TestTheOutcomeIsOneOfThree: the attribute is low-cardinality by
// construction -- three values, whatever the error.
func TestTheOutcomeIsOneOfThree(t *testing.T) {
	classifyRefusals(t)

	for _, err := range []error{nil, &errRefusal{}, errors.New("x"), fmt.Errorf("%w", &errRefusal{})} {
		switch got := Outcome(err); got {
		case OutcomeOK, OutcomeRefused, OutcomeFailed:
		default:
			t.Errorf("Outcome(%v) = %q", err, got)
		}
	}

	if AttrOutcome != "app.outcome" {
		t.Errorf("AttrOutcome = %q: the dashboards read app_outcome", AttrOutcome)
	}
}
