// Package ratelimitbreaker stops a failing rate-limit store being asked again
// on every request.
//
// # The cost it removes
//
// Without it, a Valkey outage costs a failed round trip PER REQUEST: the
// middleware calls the shared counter, the call waits out
// ratelimit.store.timeout (70ms), and the answer is the one already known from
// the previous thousand requests. At any volume that is a great deal of latency
// spent rediscovering the same fact, and it is spent on the hot path of the
// component whose job is to shed load.
//
// # What it deliberately does not change
//
// It returns an ERROR while open, never "allowed". The port's contract is that
// an error means the budget is UNKNOWN, and a breaker that answered "fine" would
// remove the limit exactly when the system is least healthy -- which is the
// failure ratelimit.store.fail.mode exists to let an operator choose about.
//
// So the observable behaviour under an outage is identical to before: fail-closed
// still refuses, fail-local still falls back to the per-replica limiter. Only the
// waiting is gone.
//
// # Why not ratelimiter.BackendLimiter
//
// The library ships a breaker, but only inside BackendLimiter, which is a
// Limiter for ONE key with ONE fixed Limit. This service's budgets vary per rule
// and per window, and the middleware already owns the layering, the fail-mode
// handling and the distinction between a store fault and a malformed rule.
// Adopting BackendLimiter would replace all of that to gain one property; a
// decorator adds the property and leaves the rest alone.
package ratelimitbreaker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/ratelimit"
)

// ErrOpen is returned instead of calling the store while the breaker is open.
//
// It is an error, not a decision, so every caller that already distinguishes
// "the store could not answer" from "the budget is spent" keeps working without
// knowing this type exists.
var ErrOpen = errors.New("rate limit store circuit breaker is open; the store is not being called")

// Config configures [New].
type Config struct {
	// Limiter is the store to protect. Required.
	Limiter ratelimit.Limiter

	// OnStateChange is called on each transition, for a gauge or a log. Optional.
	OnStateChange func(open bool)

	// Threshold is how many CONSECUTIVE failures open the breaker. Zero or less
	// disables it, which means accepting a failed round trip on every request
	// during an outage.
	Threshold int

	// Cooldown is how long the breaker stays open before letting one request
	// through to test the store.
	Cooldown time.Duration
}

// Breaker is a [ratelimit.Limiter] that stops calling a failing store.
//
// # Half-open admits exactly one probe
//
// When the cooldown expires the breaker does not close: it lets ONE request
// through and decides on that. Closing optimistically would send the full load
// back at a store that is still down, which is how a recovering datastore is
// knocked over again by the traffic that was waiting for it.
type Breaker struct {
	openUntil time.Time

	limiter   ratelimit.Limiter
	onChange  func(open bool)
	threshold int
	cooldown  time.Duration

	consecutiveFailures int

	// mu guards the whole state machine. A mutex rather than atomics because
	// the transitions are multi-field and must agree with each other; the cost
	// is nanoseconds against a network call it is often avoiding entirely.
	mu sync.Mutex

	// probing is true while a half-open request is in flight, so a burst does
	// not send a thousand probes at a store that is still down.
	probing bool
}

var _ ratelimit.Limiter = (*Breaker)(nil)

// New wraps limiter in a circuit breaker.
//
// A Threshold of zero or less returns the limiter unchanged rather than a
// disabled breaker: an operator who turned it off should pay no overhead for it,
// and a no-op wrapper in the call stack is a thing to reason about for nothing.
func New(conf Config) ratelimit.Limiter {
	if conf.Limiter == nil {
		return nil
	}

	if conf.Threshold <= 0 || conf.Cooldown <= 0 {
		return conf.Limiter
	}

	return &Breaker{
		limiter:   conf.Limiter,
		threshold: conf.Threshold,
		cooldown:  conf.Cooldown,
		onChange:  conf.OnStateChange,
	}
}

// Allow charges the budget, skipping the store entirely while open.
func (ref *Breaker) Allow(ctx context.Context, key string, budget ratelimit.Budget, cost int) (ratelimit.Decision, error) {
	if !ref.admit() {
		return ratelimit.Decision{}, ErrOpen
	}

	decision, err := ref.limiter.Allow(ctx, key, budget, cost)
	ref.record(ctx, err)

	return decision, err
}

// admit reports whether this request may reach the store.
func (ref *Breaker) admit() bool {
	ref.mu.Lock()
	defer ref.mu.Unlock()

	if ref.openUntil.IsZero() {
		return true
	}

	if time.Now().Before(ref.openUntil) {
		return false
	}

	// Cooldown elapsed. Exactly one request becomes the probe; everything else
	// keeps failing fast until that probe reports back.
	if ref.probing {
		return false
	}

	ref.probing = true

	return true
}

// record folds one outcome into the state machine.
func (ref *Breaker) record(ctx context.Context, err error) {
	// A cancelled or expired REQUEST is not the store failing -- the caller gave
	// up. Counting it would let a burst of client timeouts open the breaker
	// against a store that is answering perfectly well.
	if err != nil && (errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled)) {
		ref.mu.Lock()
		ref.probing = false
		ref.mu.Unlock()

		return
	}

	ref.mu.Lock()

	wasOpen := !ref.openUntil.IsZero()
	ref.probing = false

	if err == nil {
		ref.consecutiveFailures = 0
		ref.openUntil = time.Time{}
	} else {
		ref.consecutiveFailures++
		if ref.consecutiveFailures >= ref.threshold {
			ref.openUntil = time.Now().Add(ref.cooldown)
		}
	}

	isOpen := !ref.openUntil.IsZero()
	changed := wasOpen != isOpen
	failures := ref.consecutiveFailures

	ref.mu.Unlock()

	if !changed {
		return
	}

	if isOpen {
		slog.WarnContext(
			ctx, "rate-limit store circuit breaker opened; the store will not be called",
			"consecutive_failures", failures,
			"cooldown", ref.cooldown,
			"consequence", "requests are answered from ratelimit.store.fail.mode without a round trip",
		)
	} else {
		slog.InfoContext(ctx, "rate-limit store circuit breaker closed; the store is answering again")
	}

	if ref.onChange != nil {
		ref.onChange(isOpen)
	}
}

// Open reports whether the store is currently being skipped.
func (ref *Breaker) Open() bool {
	ref.mu.Lock()
	defer ref.mu.Unlock()

	return !ref.openUntil.IsZero() && time.Now().Before(ref.openUntil)
}

// Close closes the wrapped limiter.
func (ref *Breaker) Close() error { return ref.limiter.Close() }
