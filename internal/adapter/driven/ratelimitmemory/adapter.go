package ratelimitmemory

import (
	"context"
	"sync"
	"time"

	"github.com/slashdevops/ratelimiter"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/ratelimit"
)

// Adapter is an in-process [ratelimit.Limiter].
type Adapter struct {
	// buckets maps a bucket key to its limiter. The value carries the budget it
	// was built from, so a reload can tell "same parameters, keep the bucket"
	// from "different parameters, rebuild it".
	buckets map[string]*entry
	mu      sync.Mutex
}

type entry struct {
	limiter ratelimiter.Limiter
	budget  ratelimit.Budget
	seen    time.Time
}

// New creates an in-process limiter.
func New() *Adapter {
	return &Adapter{buckets: make(map[string]*entry)}
}

// Allow charges one request against the bucket for key.
//
// It never returns a non-nil error for an unreachable store: there is nothing to
// be unreachable. That is what makes it usable as the fallback -- a fallback
// that could itself fail would need a fallback of its own. The only error it
// returns is a rule this service cannot build a limiter from.
//
// cost is accepted for signature compatibility and must be 1; see the port's
// doc.go. The library's Reservation covers a single token, so a weighted cost
// would have to be emulated by N reservations, and a partial failure halfway
// through would spend budget for a request that was then refused.
func (ref *Adapter) Allow(_ context.Context, key string, budget ratelimit.Budget, _ int) (ratelimit.Decision, error) {
	lim, err := ref.limiterFor(key, budget)
	if err != nil {
		return ratelimit.Decision{}, err
	}

	// Reserver is an OPTIONAL capability. Feature-detect it: with it the refusal
	// carries a real Retry-After, without it the caller still gets a correct
	// allow/deny and a Retry-After derived from the window. Assuming it is
	// present would work today and break silently the day a strategy stops
	// implementing it.
	reserver, ok := lim.(ratelimiter.Reserver)
	if !ok {
		if lim.Allow() {
			return ratelimit.Decision{Allowed: true, Remaining: -1}, nil
		}

		return ratelimit.Decision{Allowed: false, Remaining: 0, RetryAfter: retryAfterFor(budget)}, nil
	}

	r := reserver.Reserve()
	if !r.OK() {
		// The bucket can never satisfy this request -- capacity is smaller than
		// what was asked for. A zero RetryAfter is correct: waiting will not
		// help, and any other value would promise a moment that never arrives.
		return ratelimit.Decision{Allowed: false, Remaining: 0, RetryAfter: 0}, nil
	}

	if delay := r.Delay(); delay > 0 {
		// Not admissible now. Cancel returns the token, because a REFUSED
		// request that still spent budget makes the observed limit lower than
		// the configured one -- and lower in a way that gets worse the harder a
		// client retries.
		r.Cancel()

		return ratelimit.Decision{Allowed: false, Remaining: 0, RetryAfter: delay}, nil
	}

	// Remaining is -1 rather than a guess. Neither strategy exposes a cheap
	// "tokens left" that means the same thing in both, and a number invented
	// here would be published as RateLimit-Remaining and believed.
	return ratelimit.Decision{Allowed: true, Remaining: -1}, nil
}

// retryAfterFor is the fallback Retry-After when the limiter cannot report one.
//
// One window divided by the request count is the average spacing between
// admissions, which is the honest answer when the exact next instant is unknown.
// Zero would tell a client to retry immediately, turning a refusal into a spin.
func retryAfterFor(budget ratelimit.Budget) time.Duration {
	if budget.Requests <= 0 || budget.Period <= 0 {
		return time.Second
	}

	d := budget.Period / time.Duration(budget.Requests)
	if d <= 0 {
		return time.Second
	}

	return d
}

// limiterFor returns the limiter for key, building it if the key is new or its
// budget changed.
func (ref *Adapter) limiterFor(key string, budget ratelimit.Budget) (ratelimiter.Limiter, error) {
	ref.mu.Lock()
	defer ref.mu.Unlock()

	if e, ok := ref.buckets[key]; ok && e.budget == budget {
		e.seen = time.Now()
		return e.limiter, nil
	}

	strategy, err := ratelimiter.ParseStrategy(budget.Strategy)
	if err != nil {
		// A strategy the library cannot build is a broken rule. Refusing here
		// rather than defaulting is the whole point of the column: a rule that
		// says leaky_bucket and silently enforces a token bucket is worse than
		// an error, because nothing anywhere would say so.
		return nil, &domain.InvalidRateLimitStrategyError{Strategy: budget.Strategy}
	}

	// Burst is passed through unchanged. ratelimiter.Limit documents zero as
	// meaning Requests, so defaulting it here would duplicate the library and
	// then disagree with it the day one of the two changes. The domain's
	// EffectiveBurst exists for the API and the form, which have no library to
	// ask.
	lim, err := ratelimiter.NewLimiterFunc(strategy, ratelimiter.Limit{
		Requests: budget.Requests,
		Period:   budget.Period,
		Burst:    budget.Burst,
	})
	if err != nil {
		return nil, err
	}

	built := lim()
	ref.buckets[key] = &entry{limiter: built, budget: budget, seen: time.Now()}

	return built, nil
}

// Sweep drops buckets untouched for longer than idle.
//
// Without it the map grows one entry per distinct bucket key forever, and an
// ip-scoped rule has one key per client address -- so a scan or a botnet is an
// unbounded allocation. Dropping an idle bucket is safe: a bucket idle for
// longer than its own window has refilled completely, so recreating it is
// indistinguishable from keeping it.
func (ref *Adapter) Sweep(idle time.Duration) int {
	cutoff := time.Now().Add(-idle)

	ref.mu.Lock()
	defer ref.mu.Unlock()

	removed := 0

	for key, e := range ref.buckets {
		// Never drop a bucket sooner than its own window, whatever idle says --
		// that would hand back a full budget to a caller who is still spending.
		if e.seen.Before(cutoff) && e.seen.Before(time.Now().Add(-e.budget.Period)) {
			delete(ref.buckets, key)

			removed++
		}
	}

	return removed
}

// Size reports how many buckets are held, for the gauge that makes a leak
// visible before it is an outage.
func (ref *Adapter) Size() int {
	ref.mu.Lock()
	defer ref.mu.Unlock()

	return len(ref.buckets)
}

// Close releases nothing; the map is garbage collected.
func (ref *Adapter) Close() error { return nil }
