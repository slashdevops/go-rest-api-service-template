package ratelimitbreaker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/ratelimit"
)

// countingLimiter records how many times the store was actually called, which
// is the whole property under test.
type countingLimiter struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (c *countingLimiter) Allow(context.Context, string, ratelimit.Budget, int) (ratelimit.Decision, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls++

	if c.err != nil {
		return ratelimit.Decision{}, c.err
	}

	return ratelimit.Decision{Allowed: true, Remaining: 7}, nil
}

func (c *countingLimiter) Close() error { return nil }

func (c *countingLimiter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.calls
}

func (c *countingLimiter) setErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.err = err
}

func charge(t *testing.T, l ratelimit.Limiter, n int) (allowed, failed int) {
	t.Helper()

	for range n {
		if _, err := l.Allow(t.Context(), "k", ratelimit.Budget{Requests: 10, Period: time.Second}, 1); err != nil {
			failed++
		} else {
			allowed++
		}
	}

	return allowed, failed
}

var errStore = errors.New("valkey: connection refused")

// THE property, and the one HANDOFF.md said could not be tested without this:
// during an outage a request makes NO network call.
//
// Before the breaker, every request paid a failed round trip and waited out
// ratelimit.store.timeout to rediscover what the previous thousand requests had
// already established.
func TestOnceOpenTheStoreIsNotCalledAtAll(t *testing.T) {
	t.Parallel()

	store := &countingLimiter{err: errStore}
	b := New(Config{Limiter: store, Threshold: 3, Cooldown: time.Minute})

	// Three failures open it, and 97 more must cost nothing.
	_, failed := charge(t, b, 100)

	if failed != 100 {
		t.Fatalf("%d of 100 requests failed; every one must, the store is down", failed)
	}

	if got := store.count(); got != 3 {
		t.Fatalf("the store was called %d times, want exactly 3 (the threshold). "+
			"Every call past the threshold is a failed round trip spent rediscovering a known fact", got)
	}
}

// The breaker must not answer "allowed" while open. The port's contract is that
// an error means the budget is UNKNOWN, and a breaker that reported fine would
// remove the limit exactly when the system is least healthy.
func TestAnOpenBreakerReturnsAnErrorNeverAnAllowedDecision(t *testing.T) {
	t.Parallel()

	store := &countingLimiter{err: errStore}
	b := New(Config{Limiter: store, Threshold: 1, Cooldown: time.Minute})

	charge(t, b, 1) // opens it

	decision, err := b.Allow(t.Context(), "k", ratelimit.Budget{}, 1)
	if err == nil {
		t.Fatal("an open breaker must return an error; 'allowed' would remove the limit during an outage")
	}

	if !errors.Is(err, ErrOpen) {
		t.Fatalf("error = %v, want ErrOpen", err)
	}

	if decision.Allowed {
		t.Fatal("an open breaker returned Allowed=true")
	}
}

// A healthy store is never skipped, and the breaker adds no behaviour of its own.
func TestAHealthyStoreIsAlwaysCalled(t *testing.T) {
	t.Parallel()

	store := &countingLimiter{}
	b := New(Config{Limiter: store, Threshold: 3, Cooldown: time.Minute})

	allowed, failed := charge(t, b, 50)

	if allowed != 50 || failed != 0 {
		t.Fatalf("allowed=%d failed=%d, want 50/0", allowed, failed)
	}

	if got := store.count(); got != 50 {
		t.Fatalf("the store was called %d times, want 50", got)
	}
}

// The threshold counts CONSECUTIVE failures. A store that fails intermittently
// is not down, and opening on it would turn a blip into a self-inflicted
// degradation.
func TestASuccessResetsTheFailureCount(t *testing.T) {
	t.Parallel()

	store := &countingLimiter{}
	b := New(Config{Limiter: store, Threshold: 3, Cooldown: time.Minute})

	for range 10 {
		store.setErr(errStore)
		charge(t, b, 2) // two failures, below the threshold

		store.setErr(nil)
		charge(t, b, 1) // one success resets
	}

	if b.(*Breaker).Open() {
		t.Fatal("the breaker opened on intermittent failures; the threshold counts CONSECUTIVE ones")
	}

	if got := store.count(); got != 30 {
		t.Fatalf("the store was called %d times, want all 30 -- it was never skipped", got)
	}
}

// After the cooldown, exactly ONE request probes the store. Closing
// optimistically would send the full load back at a store that is still down.
func TestHalfOpenAdmitsExactlyOneProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &countingLimiter{err: errStore}
		b := New(Config{Limiter: store, Threshold: 1, Cooldown: 30 * time.Second})

		charge(t, b, 1) // opens
		if got := store.count(); got != 1 {
			t.Fatalf("store called %d times before the cooldown", got)
		}

		synctest.Sleep(31 * time.Second)

		// A burst arrives the instant the cooldown expires.
		charge(t, b, 100)

		if got := store.count(); got != 2 {
			t.Fatalf("the store was called %d times, want 2: the opening failure and ONE probe. "+
				"Letting a burst through at a recovering store is how it gets knocked over again", got)
		}
	})
}

// A probe that succeeds closes the breaker and traffic resumes.
func TestASuccessfulProbeClosesTheBreaker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &countingLimiter{err: errStore}
		b := New(Config{Limiter: store, Threshold: 1, Cooldown: 30 * time.Second})

		charge(t, b, 1)

		synctest.Sleep(31 * time.Second)
		store.setErr(nil)

		if _, failed := charge(t, b, 1); failed != 0 {
			t.Fatal("the probe should have reached a healthy store and succeeded")
		}

		if b.(*Breaker).Open() {
			t.Fatal("a successful probe must close the breaker")
		}

		if allowed, _ := charge(t, b, 10); allowed != 10 {
			t.Fatalf("only %d of 10 requests were served after recovery", allowed)
		}
	})
}

// A cancelled REQUEST is the caller giving up, not the store failing. Counting
// it would let a burst of client timeouts open the breaker against a store that
// is answering perfectly well.
func TestACancelledRequestDoesNotCountAsAStoreFailure(t *testing.T) {
	t.Parallel()

	store := &countingLimiter{err: context.Canceled}
	b := New(Config{Limiter: store, Threshold: 2, Cooldown: time.Minute})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	for range 10 {
		_, _ = b.Allow(ctx, "k", ratelimit.Budget{}, 1)
	}

	if b.(*Breaker).Open() {
		t.Fatal("cancelled requests opened the breaker; the caller gave up, the store did not fail")
	}
}

// The state change drives a gauge, so it must fire on each transition and not
// on every request.
func TestTheStateChangeFiresOnTransitionsOnly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var changes atomic.Int64

		store := &countingLimiter{err: errStore}
		b := New(Config{
			Limiter: store, Threshold: 1, Cooldown: 30 * time.Second,
			OnStateChange: func(bool) { changes.Add(1) },
		})

		charge(t, b, 50) // one transition: closed -> open

		if got := changes.Load(); got != 1 {
			t.Fatalf("state change fired %d times while opening, want 1", got)
		}

		synctest.Sleep(31 * time.Second)
		store.setErr(nil)
		charge(t, b, 10) // one transition: open -> closed

		if got := changes.Load(); got != 2 {
			t.Fatalf("state change fired %d times in total, want 2 (open, then close)", got)
		}
	})
}

// A threshold of zero returns the limiter unchanged: an operator who turned the
// breaker off should pay nothing for it, not carry a disabled wrapper.
func TestADisabledBreakerIsNotAWrapper(t *testing.T) {
	t.Parallel()

	store := &countingLimiter{}

	if got := New(Config{Limiter: store, Threshold: 0, Cooldown: time.Minute}); got != ratelimit.Limiter(store) {
		t.Fatal("a disabled breaker must return the limiter itself")
	}

	if got := New(Config{Limiter: store, Threshold: 3, Cooldown: 0}); got != ratelimit.Limiter(store) {
		t.Fatal("a zero cooldown must also disable the breaker")
	}
}

// Concurrent traffic must not send more than one probe, nor corrupt the counts.
func TestConcurrentUseIsSafe(t *testing.T) {
	t.Parallel()

	store := &countingLimiter{err: errStore}
	b := New(Config{Limiter: store, Threshold: 5, Cooldown: time.Minute})

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() { charge(t, b, 20) })
	}

	wg.Wait()

	// Racing callers may each be mid-flight when the threshold is crossed, so
	// the exact count is not pinned -- but it must be bounded near the
	// threshold rather than near the 1000 requests that were made.
	if got := store.count(); got > 100 {
		t.Fatalf("the store was called %d times out of 1000 requests; the breaker is not bounding the calls", got)
	}
}
