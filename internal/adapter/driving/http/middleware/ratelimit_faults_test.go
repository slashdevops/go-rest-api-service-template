//go:build unit

package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/ratelimit"
)

// failingLocal is a per-replica limiter that cannot build a limiter for the
// rule -- what ratelimitmemory does with a strategy it cannot parse, and the
// only error that adapter can return at all.
type failingLocal struct{ err error }

func (f failingLocal) Allow(context.Context, string, ratelimit.Budget, int) (ratelimit.Decision, error) {
	return ratelimit.Decision{}, f.err
}

func (failingLocal) Close() error { return nil }

func newRuleFaultMetrics(t *testing.T) (*sdkmetric.ManualReader, *middleware.RateLimitMetrics) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test")

	m, err := middleware.NewRateLimitMetrics(meter, "")
	if err != nil {
		t.Fatalf("NewRateLimitMetrics: %v", err)
	}

	return reader, m
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) *metricdata.ResourceMetrics {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	return &rm
}

// A malformed RULE is not a store outage, and conflating them did three wrong
// things at once: it drove rate_limit_store_up to zero (paging whoever is on
// call for a Valkey that is fine), it consulted the store fail mode, and under
// the default fail-closed it answered 429 RATE_LIMIT_UNAVAILABLE to every
// request the rule matched -- an outage of the endpoint the operator was trying
// to protect, caused by one bad row.
//
// The shared store is deliberately present and healthy here: if a rule fault
// were still being read as a store fault, fail-closed would refuse.
func TestABrokenRuleIsNotReportedAsAStoreFault(t *testing.T) {
	t.Parallel()

	shared := newCountingLimiter(true)

	rec := rlServe(t, middleware.RateLimitConfig{
		Rules:    staticRules{known: true, rules: []domain.RateLimit{mwRule("broken", domain.RateLimitScopeIP)}},
		Local:    failingLocal{err: &domain.InvalidRateLimitStrategyError{Strategy: "not_a_strategy"}},
		Shared:   shared,
		Resolver: newResolver(t),
		Stage:    middleware.RateLimitStagePreAuth,
		FailMode: middleware.RateLimitFailClosed,
	}, rlRequest(http.MethodGet, "/models"))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: a rule no limiter can be built from is skipped, not turned into a refusal. "+
			"fail-closed answers what an unreachable COUNTER means, and the counter here is fine", rec.Code)
	}

	if got := len(shared.charges()); got != 0 {
		t.Fatalf("the shared store was charged %d times for a bucket whose rule could not be built", got)
	}
}

// The mirror image, so the split cannot pass by treating every error as a rule
// fault: a genuine store fault must still fail closed with its own code.
func TestAStoreFaultIsStillAStoreFaultAfterTheRuleFaultSplit(t *testing.T) {
	t.Parallel()

	shared := newCountingLimiter(true)
	shared.err = errors.New("valkey: connection refused")

	rec := rlServe(t, middleware.RateLimitConfig{
		Rules:    staticRules{known: true, rules: []domain.RateLimit{mwRule("ip", domain.RateLimitScopeIP)}},
		Shared:   shared,
		Resolver: newResolver(t),
		Stage:    middleware.RateLimitStagePreAuth,
		FailMode: middleware.RateLimitFailClosed,
	}, rlRequest(http.MethodGet, "/models"))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429: an unreachable shared counter is UNKNOWN, never allowed", rec.Code)
	}
}

// The gauge is the alerting signal, so what does NOT move it matters as much as
// what does. TestAStoreFaultDropsTheStoreUpGauge covers the other direction.
func TestABrokenRuleLeavesTheStoreUpGaugeAlone(t *testing.T) {
	t.Parallel()

	reader, metrics := newRuleFaultMetrics(t)

	rlServe(t, middleware.RateLimitConfig{
		Rules:    staticRules{known: true, rules: []domain.RateLimit{mwRule("broken", domain.RateLimitScopeIP)}},
		Local:    failingLocal{err: &domain.InvalidRateLimitStrategyError{Strategy: "nope"}},
		Metrics:  metrics,
		Resolver: newResolver(t),
		Stage:    middleware.RateLimitStagePreAuth,
		FailMode: middleware.RateLimitFailClosed,
	}, rlRequest(http.MethodGet, "/models"))

	rm := collectMetrics(t, reader)

	for _, dp := range findGauge(t, rm, "rate_limit_store_up") {
		if dp.Value != 1 {
			t.Fatalf("rate_limit_store_up is %d after a RULE fault; the shared counter was never asked, "+
				"and pinning this to 0 alerts the wrong team about the wrong subsystem", dp.Value)
		}
	}

	if len(findSum(t, rm, "rate_limit_rule_faults_total")) == 0 {
		t.Fatal("no rate_limit_rule_faults_total recorded; a rule enforcing nothing must be countable")
	}

	// Absent, not zero: an OTEL counter that was never incremented exports no
	// series at all, so its absence IS the assertion that no store fault was
	// recorded.
	if sumRecorded(rm, "rate_limit_store_faults_total") {
		t.Fatal("a rule fault was counted as a store fault; the shared counter was never consulted")
	}
}

// sumRecorded reports whether a counter exported any series.
func sumRecorded(rm *metricdata.ResourceMetrics, name string) bool {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return true
			}
		}
	}

	return false
}

// Fail-local and fail-closed must agree here. The fail mode is about a counter
// that cannot answer, and this one answered -- so consulting it at all is the
// bug, in either direction.
func TestABrokenRuleIsSkippedInFailLocalToo(t *testing.T) {
	t.Parallel()

	rec := rlServe(t, middleware.RateLimitConfig{
		Rules:    staticRules{known: true, rules: []domain.RateLimit{mwRule("broken", domain.RateLimitScopeIP)}},
		Local:    failingLocal{err: &domain.InvalidRateLimitStrategyError{Strategy: "nope"}},
		Resolver: newResolver(t),
		Stage:    middleware.RateLimitStagePreAuth,
		FailMode: middleware.RateLimitFailLocal,
	}, rlRequest(http.MethodGet, "/models"))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}
