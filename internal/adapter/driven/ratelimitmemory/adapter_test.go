package ratelimitmemory

import (
	"context"
	"testing"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/ratelimit"
)

func budget(requests int, period time.Duration, burst int, strategy string) ratelimit.Budget {
	return ratelimit.Budget{Requests: requests, Period: period, Burst: burst, Strategy: strategy}
}

func TestAllowsUpToBurstThenRefuses(t *testing.T) {
	t.Parallel()

	a := New()
	b := budget(10, time.Second, 3, "token_bucket")

	for i := range 3 {
		d, err := a.Allow(t.Context(), "k", b, 1)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}

		if !d.Allowed {
			t.Fatalf("request %d refused, but the burst is 3", i)
		}
	}

	d, err := a.Allow(t.Context(), "k", b, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if d.Allowed {
		t.Fatal("the fourth request should be refused; burst is 3 and no time has passed")
	}

	if d.RetryAfter <= 0 {
		t.Fatal("a refusal must carry a Retry-After; zero tells the client to spin")
	}
}

// A refused request must not spend budget. If it did, a client retrying hard
// would drive the observed limit below the configured one -- and the harder it
// retried, the lower the limit would go.
func TestARefusalDoesNotSpendBudget(t *testing.T) {
	t.Parallel()

	a := New()
	b := budget(1, time.Hour, 1, "token_bucket")

	if d, _ := a.Allow(t.Context(), "k", b, 1); !d.Allowed {
		t.Fatal("the first request should be allowed")
	}

	first, _ := a.Allow(t.Context(), "k", b, 1)

	// Twenty more refusals. Without Cancel each one would push the next
	// admission an hour further out.
	for range 20 {
		if d, _ := a.Allow(t.Context(), "k", b, 1); d.Allowed {
			t.Fatal("budget is spent, these must all be refused")
		}
	}

	last, _ := a.Allow(t.Context(), "k", b, 1)

	// The reported wait must not have grown. Allow a second of slack for the
	// clock rather than asserting exact equality.
	if last.RetryAfter > first.RetryAfter+time.Second {
		t.Fatalf("refusals are spending budget: Retry-After grew from %s to %s over 21 refused requests",
			first.RetryAfter, last.RetryAfter)
	}
}

func TestDifferentKeysGetDifferentBuckets(t *testing.T) {
	t.Parallel()

	a := New()
	b := budget(1, time.Hour, 1, "token_bucket")

	if d, _ := a.Allow(t.Context(), "alice", b, 1); !d.Allowed {
		t.Fatal("alice's first request should be allowed")
	}

	if d, _ := a.Allow(t.Context(), "alice", b, 1); d.Allowed {
		t.Fatal("alice's second request should be refused")
	}

	// Without WithLimiterFactoryForKey semantics this would fail: bob would be
	// sharing alice's bucket, and the limit would be global while looking
	// per-key.
	if d, _ := a.Allow(t.Context(), "bob", b, 1); !d.Allowed {
		t.Fatal("bob must have his own bucket; a shared one is a global limit wearing a per-key label")
	}
}

// The PocketBase trap. An operator editing an unrelated rule must not refill the
// budget of every caller currently being limited -- that is the difference
// between a rate limit and a rate suggestion.
func TestReEvaluatingTheSameBudgetKeepsTheLiveBucket(t *testing.T) {
	t.Parallel()

	a := New()
	b := budget(1, time.Hour, 1, "token_bucket")

	if d, _ := a.Allow(t.Context(), "k", b, 1); !d.Allowed {
		t.Fatal("first request should be allowed")
	}

	// Same parameters, a fresh Budget value -- exactly what a mirror reload
	// hands over. It must resolve to the SAME bucket.
	same := budget(1, time.Hour, 1, "token_bucket")
	if d, _ := a.Allow(t.Context(), "k", same, 1); d.Allowed {
		t.Fatal("a reload that did not change the rule refilled a spent bucket")
	}

	if got := a.Size(); got != 1 {
		t.Fatalf("an identical budget must reuse the bucket, not add one; have %d", got)
	}
}

// The other half of the same contract: when the numbers DO change, the old
// bucket must not silently keep enforcing the old limit.
func TestChangingTheBudgetRebuildsTheBucket(t *testing.T) {
	t.Parallel()

	a := New()

	if d, _ := a.Allow(t.Context(), "k", budget(1, time.Hour, 1, "token_bucket"), 1); !d.Allowed {
		t.Fatal("first request should be allowed")
	}

	if d, _ := a.Allow(t.Context(), "k", budget(1, time.Hour, 1, "token_bucket"), 1); d.Allowed {
		t.Fatal("second request against the same budget should be refused")
	}

	// The operator raised the limit. The next request must be admitted, or the
	// change they saved did nothing until the process restarted.
	if d, _ := a.Allow(t.Context(), "k", budget(100, time.Hour, 100, "token_bucket"), 1); !d.Allowed {
		t.Fatal("raising a rule's budget must take effect immediately")
	}
}

// An unknown strategy must be refused, never silently defaulted. A rule that
// says one thing and enforces another is worse than an error: nothing in the
// response, the logs or the metrics would say the operator did not get what they
// asked for.
func TestAnUnknownStrategyIsAnErrorNotADefault(t *testing.T) {
	t.Parallel()

	a := New()

	d, err := a.Allow(t.Context(), "k", budget(10, time.Second, 10, "sliding_window"), 1)
	if err == nil {
		t.Fatal("an unbuildable strategy must be an error")
	}

	if d.Allowed {
		t.Fatal("an error must not come back as an allowed request")
	}
}

// Both strategies must actually build and enforce. Asserting only that a factory
// was returned would pass even if one of them never limited anything.
func TestBothStrategiesEnforce(t *testing.T) {
	t.Parallel()

	for _, strategy := range []string{"token_bucket", "leaky_bucket"} {
		t.Run(strategy, func(t *testing.T) {
			t.Parallel()

			a := New()
			b := budget(1, time.Hour, 1, strategy)

			if d, err := a.Allow(t.Context(), "k", b, 1); err != nil || !d.Allowed {
				t.Fatalf("first request should be allowed: %v", err)
			}

			if d, _ := a.Allow(t.Context(), "k", b, 1); d.Allowed {
				t.Fatalf("%s did not limit anything", strategy)
			}
		})
	}
}

// A CONTRACT test, not a test of code here: ratelimiter.Limit documents Burst 0
// as meaning Requests, and this adapter relies on that rather than duplicating
// it. Written the other way round -- with the defaulting in this package -- the
// assertion could not fail, because removing the duplication changed nothing.
// That is how it was written first, and mutation testing caught it.
func TestZeroBurstMeansRequests(t *testing.T) {
	t.Parallel()

	a := New()
	b := budget(3, time.Hour, 0, "token_bucket")

	for i := range 3 {
		if d, _ := a.Allow(t.Context(), "k", b, 1); !d.Allowed {
			t.Fatalf("request %d refused; burst 0 should mean a capacity of 3", i)
		}
	}

	if d, _ := a.Allow(t.Context(), "k", b, 1); d.Allowed {
		t.Fatal("the fourth request should be refused")
	}
}

// An ip-scoped rule has one key per client address, so without a sweep the map
// grows forever and a scan is an unbounded allocation.
func TestSweepDropsIdleBucketsButNotLiveOnes(t *testing.T) {
	t.Parallel()

	a := New()

	// A long window: idle or not, this bucket may still hold spent state, so it
	// must survive a sweep whose cutoff is shorter than the window.
	if _, err := a.Allow(t.Context(), "long", budget(1, time.Hour, 1, "token_bucket"), 1); err != nil {
		t.Fatal(err)
	}

	if got := a.Sweep(time.Nanosecond); got != 0 {
		t.Fatalf("swept %d buckets whose window has not elapsed; that hands a full budget back to a caller still spending", got)
	}

	if a.Size() != 1 {
		t.Fatal("the bucket should still be there")
	}
}

func TestSizeCountsBuckets(t *testing.T) {
	t.Parallel()

	a := New()
	b := budget(10, time.Second, 10, "token_bucket")

	for _, k := range []string{"a", "b", "c"} {
		if _, err := a.Allow(t.Context(), k, b, 1); err != nil {
			t.Fatal(err)
		}
	}

	if got := a.Size(); got != 3 {
		t.Fatalf("Size = %d, want 3 — this gauge is what makes a bucket leak visible before it is an outage", got)
	}
}

func TestConcurrentAllowIsSafe(t *testing.T) {
	t.Parallel()

	a := New()
	b := budget(1000, time.Second, 1000, "token_bucket")

	done := make(chan struct{})

	for i := range 16 {
		go func() {
			defer func() { done <- struct{}{} }()

			for range 50 {
				//nolint:errcheck // the assertion here is that -race finds nothing.
				_, _ = a.Allow(context.Background(), "shared", b, 1)
			}
		}()

		_ = i
	}

	for range 16 {
		<-done
	}
}
