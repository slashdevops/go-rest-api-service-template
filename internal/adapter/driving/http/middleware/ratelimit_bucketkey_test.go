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

// ruleWith builds a rule with a caller-chosen RULE id and window id, so a test
// can simulate what a PUT actually does: the rule id is stable, and the window
// set is replaced wholesale with fresh uuids.
//
// Holding the rule id fixed matters -- an earlier draft of this test let mwRule
// mint a new one, which changed the key for a reason no edit ever would and
// made the test fail against the fix.
func ruleWith(ruleID, windowID uuid.UUID, requests, burst int, period time.Duration) domain.RateLimit {
	rule := mwRule("edited", domain.RateLimitScopeIP,
		domain.RateLimitWindow{ID: windowID, Requests: requests, Period: period, Burst: burst})
	rule.ID = ruleID

	return rule
}

// THE PocketBase trap, and it was live.
//
// A bucket used to be keyed on the WINDOW ID. PUT replaces a rule's window set
// wholesale and mints fresh uuids, so renaming a rule -- or editing only its
// description -- handed every caller their full allowance back. Measured
// against the running service: spend 4 of 6, edit the description, and the next
// 4 requests were all admitted.
//
// The adapter's own guard could not catch it. That one keeps a live bucket for
// an unchanged budget under the same KEY, and the key was what changed.
func TestAnEditThatKeepsTheNumbersKeepsTheSpentBudget(t *testing.T) {
	t.Parallel()

	local := ratelimitmemory.New()
	t.Cleanup(func() { _ = local.Close() })

	serve := func(rule domain.RateLimit) int {
		return rlServe(t, middleware.RateLimitConfig{
			Rules:    staticRules{known: true, rules: []domain.RateLimit{rule}},
			Local:    local,
			Resolver: newResolver(t),
			Stage:    middleware.RateLimitStagePreAuth,
		}, rlRequest(http.MethodGet, "/models")).Code
	}

	ruleID := uuid.NewV7()
	before := ruleWith(ruleID, uuid.NewV7(), 6, 6, time.Hour)

	for i := range 4 {
		if code := serve(before); code != http.StatusOK {
			t.Fatalf("request %d got %d, want 200 within a budget of 6", i, code)
		}
	}

	// The edit: same numbers, brand-new window id, exactly as PUT produces.
	after := ruleWith(ruleID, uuid.NewV7(), 6, 6, time.Hour)

	admitted := 0

	for range 4 {
		if serve(after) == http.StatusOK {
			admitted++
		}
	}

	if admitted != 2 {
		t.Fatalf("%d of 4 requests admitted after an edit, want 2. Four means the bucket was "+
			"REFILLED by an edit that changed no number -- the trap PocketBase falls into, and "+
			"the difference between a rate limit and a rate suggestion", admitted)
	}
}

// The other half: changing the numbers MUST rebuild the bucket, or tightening a
// limit would apply from whatever the old one had left.
func TestChangingTheNumbersRebuildsTheBucket(t *testing.T) {
	t.Parallel()

	local := ratelimitmemory.New()
	t.Cleanup(func() { _ = local.Close() })

	serve := func(rule domain.RateLimit) int {
		return rlServe(t, middleware.RateLimitConfig{
			Rules:    staticRules{known: true, rules: []domain.RateLimit{rule}},
			Local:    local,
			Resolver: newResolver(t),
			Stage:    middleware.RateLimitStagePreAuth,
		}, rlRequest(http.MethodGet, "/models")).Code
	}

	ruleID, windowID := uuid.NewV7(), uuid.NewV7()
	small := ruleWith(ruleID, windowID, 2, 2, time.Hour)

	for range 2 {
		serve(small)
	}

	if serve(small) != http.StatusTooManyRequests {
		t.Fatal("the budget of 2 should be spent")
	}

	// Same window id, bigger budget: a different bucket, because the numbers
	// are part of the key.
	bigger := ruleWith(ruleID, windowID, 10, 10, time.Hour)

	if code := serve(bigger); code != http.StatusOK {
		t.Fatalf("got %d after raising the budget; a changed budget must not keep the spent bucket", code)
	}
}

// Two windows on one rule stay separate, which is what the parameters in the
// key have to preserve: a rule carrying 10/s and 300/min holds two buckets.
func TestTwoWindowsOnOneRuleKeepSeparateBuckets(t *testing.T) {
	t.Parallel()

	lim := newCountingLimiter(true)

	rule := mwRule("two", domain.RateLimitScopeIP,
		domain.RateLimitWindow{ID: uuid.NewV7(), Requests: 5, Period: time.Second, Burst: 5},
		domain.RateLimitWindow{ID: uuid.NewV7(), Requests: 50, Period: time.Minute, Burst: 50})

	rlServe(t, middleware.RateLimitConfig{
		Rules:    staticRules{known: true, rules: []domain.RateLimit{rule}},
		Local:    lim,
		Resolver: newResolver(t),
		Stage:    middleware.RateLimitStagePreAuth,
	}, rlRequest(http.MethodGet, "/models"))

	charges := lim.charges()
	if len(charges) != 2 {
		t.Fatalf("charged %d buckets, want 2 -- one per window", len(charges))
	}

	if charges[0] == charges[1] {
		t.Fatalf("both windows charged the same bucket %q; two windows must not share a budget", charges[0])
	}
}
