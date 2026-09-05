package ratelimitvalkey

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/ratelimit"
)

// KeyPrefix namespaces every counter this package writes, so a rate-limit key
// can never collide with a cache entry in a shared Valkey.
const KeyPrefix = "ratelimit:"

// Adapter is a shared fixed-window counter in Valkey.
type Adapter struct {
	client valkey.Client

	// timeout bounds one round trip. It exists because the HTTP server sets no
	// ReadTimeout or WriteTimeout deliberately, so a request context often has
	// no deadline of its own -- without this, an unresponsive Valkey would hold
	// a request open for as long as the connection allowed.
	timeout time.Duration
}

// Config configures the adapter.
type Config struct {
	Client  valkey.Client
	Timeout time.Duration
}

// New creates a Valkey-backed limiter.
func New(conf Config) (*Adapter, error) {
	if conf.Client == nil {
		return nil, errors.New("ratelimitvalkey: client is nil")
	}

	if conf.Timeout <= 0 {
		return nil, errors.New("ratelimitvalkey: timeout must be positive; an unbounded round trip is what the HTTP server's absent ReadTimeout would otherwise expose")
	}

	return &Adapter{client: conf.Client, timeout: conf.Timeout}, nil
}

// Allow charges one request against the shared counter for key.
//
// A non-nil error means the answer is UNKNOWN. It is never paired with
// Allowed: true.
func (ref *Adapter) Allow(ctx context.Context, key string, budget ratelimit.Budget, _ int) (ratelimit.Decision, error) {
	if budget.Period <= 0 || budget.Requests <= 0 {
		return ratelimit.Decision{}, fmt.Errorf("ratelimitvalkey: invalid budget %d per %s", budget.Requests, budget.Period)
	}

	ctx, cancel := context.WithTimeout(ctx, ref.timeout)
	defer cancel()

	now := time.Now()
	windowKey, resetsIn := ref.windowKey(key, budget, now)

	count, err := ref.incr(ctx, windowKey)
	if err != nil {
		return ratelimit.Decision{}, err
	}

	// PEXPIRE only on the first increment of a window.
	//
	// NOT because the window would otherwise never roll -- it rolls because the
	// KEY changes with the clock index, whatever the TTL says. An earlier
	// comment here claimed otherwise and was wrong.
	//
	// The real reasons are narrower and both worth having: it halves the round
	// trips on every request after the first, and it stops a key under
	// continuous load from having its expiry pushed forward indefinitely, which
	// would leave every past window's key resident for as long as traffic
	// continued. The TTL is a janitor, not part of the decision.
	if count == 1 {
		if err := ref.expire(ctx, windowKey, budget.Period); err != nil {
			// The counter is already incremented and correct. Failing the whole
			// request over a missing TTL would refuse traffic that the limit
			// admits; the key expires with the next window's key regardless,
			// and the worst case is one stale key.
			return ref.decide(count, budget, resetsIn), nil
		}
	}

	return ref.decide(count, budget, resetsIn), nil
}

// capacity is what the shared counter admits in one window.
//
// Burst, not Requests. The per-replica token bucket in front of this one is what
// paces requests within the window; the shared counter's job is the total. Using
// Requests here when a rule sets a larger Burst would make the shared counter
// stricter than the rule, so the configured burst would never be reachable.
func capacity(budget ratelimit.Budget) int {
	if budget.Burst > budget.Requests {
		return budget.Burst
	}

	return budget.Requests
}

func (ref *Adapter) decide(count int64, budget ratelimit.Budget, resetsIn time.Duration) ratelimit.Decision {
	limit := int64(capacity(budget))

	if count > limit {
		return ratelimit.Decision{Allowed: false, Remaining: 0, RetryAfter: resetsIn}
	}

	return ratelimit.Decision{Allowed: true, Remaining: int(limit - count)}
}

// windowKey derives the counter key and how long until the window rolls.
//
// The window index comes from the WALL CLOCK divided by the period, so every
// replica computes the same key for the same instant without coordinating. That
// is the property that makes INCR sufficient and a script unnecessary.
func (ref *Adapter) windowKey(key string, budget ratelimit.Budget, now time.Time) (string, time.Duration) {
	period := budget.Period.Milliseconds()
	if period <= 0 {
		period = 1
	}

	ms := now.UnixMilli()
	index := ms / period
	resetsIn := time.Duration((index+1)*period-ms) * time.Millisecond

	return KeyPrefix + key + ":" + strconv.FormatInt(index, 10), resetsIn
}

func (ref *Adapter) incr(ctx context.Context, key string) (int64, error) {
	res := ref.client.Do(ctx, ref.client.B().Incr().Key(key).Build())
	if err := res.Error(); err != nil {
		return 0, fmt.Errorf("ratelimitvalkey: incr: %w", err)
	}

	n, err := res.AsInt64()
	if err != nil {
		return 0, fmt.Errorf("ratelimitvalkey: incr result: %w", err)
	}

	return n, nil
}

func (ref *Adapter) expire(ctx context.Context, key string, period time.Duration) error {
	// One period plus a second of slack. Exactly one period races the window
	// roll: a key can expire a moment before the index advances, which lets the
	// same window start counting from zero a second time.
	ttl := period + time.Second

	res := ref.client.Do(ctx, ref.client.B().Pexpire().Key(key).Milliseconds(ttl.Milliseconds()).Build())
	if err := res.Error(); err != nil {
		return fmt.Errorf("ratelimitvalkey: pexpire: %w", err)
	}

	return nil
}

// Close releases nothing: the Valkey client is owned by the composition root and
// shared with the cache. Closing it here would take the cache down with it.
func (ref *Adapter) Close() error { return nil }
