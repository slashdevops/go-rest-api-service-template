//go:build unit

package middleware_test

import (
	"net/http"
	"testing"
	"time"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/ratelimitmemory"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// Each strategy, carried from the rule all the way to a real limiter, enforces
// the budget it was given.
//
// # What this can and cannot prove
//
// It cannot prove the two strategies BEHAVE differently, and no test can: they
// are duals, and at equal parameters they admit identically -- which
// ratelimitmemory.TestBothStrategiesAdmitIdenticallyAtEqualParameters now
// verifies rather than assuming.
//
// That is why the tracker's open item -- "assert leaky_bucket paces where
// token_bucket bursts" -- is not written here. It asks for a difference that
// does not exist. Written anyway, it passes for the wrong reason: the first
// attempt gave the two strategies different BURSTS and went green with the
// strategy hardcoded to token_bucket, measuring the burst it varied rather than
// the column it named.
//
// What is worth asserting end to end is that neither value breaks the path --
// a rule saying leaky_bucket is enforced, not dropped or silently defaulted --
// and that an unbuildable value is refused rather than defaulted, which is what
// proves the column is read at all.
func TestEachStrategyIsEnforcedEndToEnd(t *testing.T) {
	t.Parallel()

	for _, strategy := range []domain.RateLimitStrategy{
		domain.RateLimitStrategyTokenBucket,
		domain.RateLimitStrategyLeakyBucket,
	} {
		t.Run(string(strategy), func(t *testing.T) {
			t.Parallel()

			rule := mwRule("s", domain.RateLimitScopeIP,
				domain.RateLimitWindow{ID: uuid.NewV7(), Requests: 1, Period: time.Hour, Burst: 2})
			rule.Strategy = strategy

			local := ratelimitmemory.New()
			t.Cleanup(func() { _ = local.Close() })

			conf := middleware.RateLimitConfig{
				Rules:    staticRules{known: true, rules: []domain.RateLimit{rule}},
				Local:    local,
				Resolver: newResolver(t),
				Stage:    middleware.RateLimitStagePreAuth,
			}

			for i := range 2 {
				if code := rlServe(t, conf, rlRequest(http.MethodGet, "/models")).Code; code != http.StatusOK {
					t.Fatalf("%s: request %d got %d, want 200 within a burst of 2", strategy, i, code)
				}
			}

			if code := rlServe(t, conf, rlRequest(http.MethodGet, "/models")).Code; code != http.StatusTooManyRequests {
				t.Fatalf("%s: got %d after the burst was spent, want 429; the strategy was carried but "+
					"nothing was enforced", strategy, code)
			}
		})
	}
}
