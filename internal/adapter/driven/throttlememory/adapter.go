// Package throttlememory is the driven adapter that satisfies
// [throttle.Throttle] with an in-process token bucket per key, built on
// github.com/slashdevops/ratelimiter — the same machinery the per-IP HTTP
// limiter uses.
//
// # Per replica, deliberately
//
// The budget is held in this process, so N replicas give an attacker N times
// the attempts. That is a real limitation and it is still the right trade for
// a throttle: the alternative is a shared store, and the two candidates both
// make it worse. Valkey is optional — `cache.enabled=false` is supported — so a
// cache-backed throttle would silently not exist in a valid configuration.
// Postgres would mean a write on every failed login, which is a
// denial-of-service amplifier: unauthenticated traffic that costs a disk write.
//
// N times a small number is still a small number, and it is bounded, which is
// what the unthrottled path was not. A shared store belongs here only if
// deployments grow large enough for the multiplier to matter.
//
// # Eviction
//
// Idle keys are swept by the underlying bucket limiter after the configured
// interval, so the map does not grow without bound. Sweeping an idle key is
// equivalent to Reset: a key with a full budget and no record are the same
// thing.
package throttlememory

import (
	"time"

	"github.com/slashdevops/ratelimiter"
	"golang.org/x/time/rate"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/throttle"
)

// Throttle implements [throttle.Throttle].
type Throttle struct {
	limiter *ratelimiter.BucketLimiter[string]
}

// Conf configures a [Throttle].
type Conf struct {
	// MaxAttempts is how many failures may occur back to back before the key is
	// refused — the bucket's burst.
	MaxAttempts int

	// Window is how long a fully spent budget takes to refill. Tokens return
	// steadily rather than all at once, so a refused key recovers one attempt
	// every Window/MaxAttempts.
	Window time.Duration

	// IdleAfter is how long a key may go untouched before it is evicted.
	IdleAfter time.Duration
}

// New builds a Throttle. The caller owns its lifecycle and must Close it: the
// underlying limiter runs a background eviction goroutine.
func New(conf Conf) *Throttle {
	// A spent budget refills over Window, so each token takes Window/MaxAttempts.
	refill := rate.Limit(float64(conf.MaxAttempts) / conf.Window.Seconds())

	return &Throttle{
		limiter: ratelimiter.NewBucketLimiter(
			ratelimiter.NewRateLimiterFunc(refill, conf.MaxAttempts),
			conf.IdleAfter,
			ratelimiter.NewInMemoryStorage[string, ratelimiter.Limiter](),
		),
	}
}

// Attempt implements [throttle.Throttle].
//
// The reservation is cancelled only when the attempt is refused. A cancelled
// reservation that was immediately available does not return its token —
// golang.org/x/time/rate declines to restore one whose time to act has already
// passed — so cancelling on the allowed path would be a no-op that merely looks
// like a refund. The refund is [Throttle.Succeed]'s job instead.
func (ref *Throttle) Attempt(key string) (time.Duration, bool) {
	limiter := ref.limiter.GetOrAdd(key)

	reserver, ok := limiter.(ratelimiter.Reserver)
	if !ok {
		// No timing information available from this backend; the decision is
		// still correct, only Retry-After is less precise.
		return 0, limiter.Allow()
	}

	reservation := reserver.Reserve()
	delay := reservation.Delay()

	if !reservation.OK() || delay > 0 {
		// Future-dated, so this genuinely returns the token.
		reservation.Cancel()

		return delay, false
	}

	return 0, true
}

// Succeed implements [throttle.Throttle].
func (ref *Throttle) Succeed(key string) {
	ref.limiter.Remove(key)
}

// Close releases the background eviction goroutine.
func (ref *Throttle) Close() error {
	return ref.limiter.Close()
}

// compile-time check that the adapter satisfies the port.
var _ throttle.Throttle = (*Throttle)(nil)
