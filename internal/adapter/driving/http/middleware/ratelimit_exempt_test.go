//go:build unit

package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// exemptions builds the gate the composition root builds, failing the test on a
// configuration error rather than swallowing it.
func exemptions(t *testing.T, excludedIPs, bypassPrefixes []string) *middleware.RateLimitExemptions {
	t.Helper()

	e, err := middleware.NewRateLimitExemptions(newResolver(t), excludedIPs, bypassPrefixes)
	if err != nil {
		t.Fatalf("NewRateLimitExemptions: %v", err)
	}

	return e
}

// alwaysRefuses is a limiter middleware that refuses everything, so any request
// that reaches it is unambiguous: a 200 means the request never got there.
func alwaysRefuses(reached *int) middleware.Middleware {
	return func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			*reached++
			w.WriteHeader(http.StatusTooManyRequests)
		})
	}
}

func serveWrapped(mdw middleware.Middleware, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()

	mdw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, r)

	return rec
}

// THE bug this file exists for.
//
// Both exemptions used to live inside RateLimit, which only runs when
// ratelimit.rules.enabled is on -- and it is OFF by default. So in the shipped
// posture /health was rate limited: measured against a live service at 1 req/s,
// eight probes came back 1 x 200 then 7 x 429. A load balancer sharing a source
// address with real traffic could have the replica evicted, which is precisely
// what the bypass was written to prevent.
//
// The gate is now a decorator over WHICHEVER limiter the composition root chose,
// so this holds for the flag limiter and the rule limiter alike. Reverting it
// into either limiter fails this.
func TestHealthIsExemptFromWhicheverLimiterIsRunning(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/health/live", "/health/ready", "/health/status", "/version"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			reached := 0
			gate := exemptions(t, nil, []string{"/health", "/version"})

			rec := serveWrapped(gate.Wrap(alwaysRefuses(&reached)), rlRequest(http.MethodGet, path))

			if rec.Code != http.StatusOK {
				t.Fatalf("%s got %d, want 200: a readiness probe must never be rate limited, "+
					"or a limiter outage becomes an eviction from the load balancer", path, rec.Code)
			}

			if reached != 0 {
				t.Fatalf("%s reached the limiter %d times; an exempt request must SKIP it, not be "+
					"allowed by it -- under fail-closed, asking the store at all is what fails", path, reached)
			}
		})
	}
}

// The other half: a path the bypass does not name still reaches the limiter.
// Without this, the test above would pass on a gate that exempted everything.
func TestANonBypassedPathStillReachesTheLimiter(t *testing.T) {
	t.Parallel()

	reached := 0
	gate := exemptions(t, nil, []string{"/health", "/version"})

	rec := serveWrapped(gate.Wrap(alwaysRefuses(&reached)), rlRequest(http.MethodGet, "/models"))

	if rec.Code != http.StatusTooManyRequests || reached != 1 {
		t.Fatalf("got %d after %d limiter calls; /models is not exempt and must be limited", rec.Code, reached)
	}
}

// An excluded address is exempt whichever limiter is running -- the second half
// of the same bug. rlRequest uses 203.0.113.9.
func TestAnExcludedAddressIsExemptFromWhicheverLimiterIsRunning(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{"203.0.113.9", "203.0.113.0/24"} {
		t.Run(entry, func(t *testing.T) {
			t.Parallel()

			reached := 0
			gate := exemptions(t, []string{entry}, nil)

			rec := serveWrapped(gate.Wrap(alwaysRefuses(&reached)), rlRequest(http.MethodGet, "/models"))

			if rec.Code != http.StatusOK || reached != 0 {
				t.Fatalf("%s: got %d after %d limiter calls; an excluded address must never be limited",
					entry, rec.Code, reached)
			}
		})
	}
}

// A block that does not contain the address must not exempt it.
func TestAnAddressOutsideEveryExcludedBlockIsLimited(t *testing.T) {
	t.Parallel()

	reached := 0
	gate := exemptions(t, []string{"198.51.100.0/24"}, nil)

	rec := serveWrapped(gate.Wrap(alwaysRefuses(&reached)), rlRequest(http.MethodGet, "/models"))

	if rec.Code != http.StatusTooManyRequests || reached != 1 {
		t.Fatalf("got %d after %d limiter calls; 203.0.113.9 is not inside 198.51.100.0/24",
			rec.Code, reached)
	}
}

// A malformed entry stops startup rather than being dropped. A list that
// silently exempts nothing is the failure this type exists to correct.
func TestAMalformedExcludedEntryIsRefused(t *testing.T) {
	t.Parallel()

	if _, err := middleware.NewRateLimitExemptions(newResolver(t), []string{"not-an-address"}, nil); err == nil {
		t.Fatal("a malformed excluded IP was accepted; it would silently exempt nothing")
	}
}

// A blank entry is not an error -- a trailing comma in a flag is ordinary -- and
// must not become a prefix that matches every path.
func TestBlankEntriesAreSkippedNotMatchedAgainst(t *testing.T) {
	t.Parallel()

	reached := 0
	gate := exemptions(t, []string{"203.0.113.1", "  "}, []string{"/health", ""})

	rec := serveWrapped(gate.Wrap(alwaysRefuses(&reached)), rlRequest(http.MethodGet, "/models"))

	if rec.Code != http.StatusTooManyRequests || reached != 1 {
		t.Fatalf("got %d after %d limiter calls; a blank bypass prefix must not exempt every path "+
			"(strings.HasPrefix(path, \"\") is true for all of them)", rec.Code, reached)
	}
}

// The gate wraps the rule limiter as well, end to end: an excluded address is
// exempt from a rule that would otherwise refuse it.
func TestTheGateExemptsTheRuleLimiterToo(t *testing.T) {
	t.Parallel()

	rules := staticRules{known: true, rules: []domain.RateLimit{mwRule("ip", domain.RateLimitScopeIP)}}
	lim := newCountingLimiter(false)

	limiter := middleware.RateLimit(middleware.RateLimitConfig{
		Rules: rules, Local: lim, Resolver: newResolver(t), Stage: middleware.RateLimitStagePreAuth,
	})

	gate := exemptions(t, []string{"203.0.113.0/24"}, nil)

	rec := serveWrapped(gate.Wrap(limiter), rlRequest(http.MethodGet, "/models"))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}

	if got := len(lim.charges()); got != 0 {
		t.Fatalf("charged %d buckets for an excluded address, want 0", got)
	}
}

// Wrap(nil) is a pass-through, so the composition root does not have to branch
// on whether a limiter exists.
func TestWrappingNoLimiterPassesThrough(t *testing.T) {
	t.Parallel()

	gate := exemptions(t, nil, []string{"/health"})

	if rec := serveWrapped(gate.Wrap(nil), rlRequest(http.MethodGet, "/models")); rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}

// The property the bypass exists for, end to end: with fail-closed and a store
// that cannot answer, the readiness probe still succeeds.
//
// Without the gate this is a 429 RATE_LIMIT_UNAVAILABLE, and a Valkey outage
// becomes an eviction from the load balancer -- a cache problem turned into a
// capacity problem.
func TestHealthSurvivesAStoreOutageUnderFailClosed(t *testing.T) {
	t.Parallel()

	shared := newCountingLimiter(false)
	shared.err = errors.New("valkey is unreachable")

	limiter := middleware.RateLimit(middleware.RateLimitConfig{
		Rules:    staticRules{known: true, rules: []domain.RateLimit{mwRule("ip", domain.RateLimitScopeIP)}},
		Shared:   shared,
		Resolver: newResolver(t),
		Stage:    middleware.RateLimitStagePreAuth,
		FailMode: middleware.RateLimitFailClosed,
	})

	gate := exemptions(t, nil, []string{"/health", "/version"})

	rec := serveWrapped(gate.Wrap(limiter), rlRequest(http.MethodGet, "/health/live"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; with fail-closed a store outage would evict this replica from the load balancer", rec.Code)
	}
}
