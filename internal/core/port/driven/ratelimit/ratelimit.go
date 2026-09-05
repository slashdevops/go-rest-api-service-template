package ratelimit

import (
	"context"
	"time"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/ratelimit.go -source=ratelimit.go Limiter

// Budget is one window's allowance, in the terms the limiter needs.
//
// Requests over Period with a capacity of Burst, plus the Strategy that decides
// how the two are turned into an admission pattern. It is deliberately a
// value type: the mirror hands the same Budget to many goroutines.
type Budget struct {
	// Strategy is the domain strategy string, unchanged. It is not parsed here
	// because parsing it is the adapter's job and a parse failure must be
	// loud -- a strategy this service cannot build is a broken rule, not a
	// reason to quietly pick the default.
	Strategy string

	Period   time.Duration
	Requests int
	Burst    int
}

// Decision is the answer for one request against one budget.
type Decision struct {
	// RetryAfter is how long until the next request would be admitted. It is
	// only meaningful when Allowed is false.
	RetryAfter time.Duration

	// Remaining is how much of the budget is left after this request. It backs
	// the RateLimit-Remaining header; -1 means the limiter cannot say.
	Remaining int

	Allowed bool
}

// Limiter answers whether one request fits a budget.
//
// A non-nil error means the answer is UNKNOWN, never "allowed" -- see the
// package comment.
type Limiter interface {
	// Allow charges cost against the bucket identified by key.
	//
	// key already encodes the rule, the window and the scope value, so two
	// different rules never share a bucket and editing a rule's numbers does
	// not reach into another rule's budget.
	Allow(ctx context.Context, key string, budget Budget, cost int) (Decision, error)

	// Close releases whatever the implementation holds. Called at shutdown.
	Close() error
}

// ChangeNotifier tells other replicas that the rule set changed, so a write on
// one is not invisible on the others until the reload ticker comes round.
//
// # The payload is a SIGNAL, never the rules
//
// A message says only "something changed"; the receiver then queries. That is
// what makes a lost message cost a delay and a duplicated message cost a query,
// rather than either one installing a wrong rule set. Shipping the rules in the
// message would make delivery order load-bearing, and pub/sub offers no order
// across a reconnect.
//
// # It is an optimisation, never the mechanism
//
// The reload ticker remains the floor. Everything here may fail -- the transport
// is optional (cache.enabled=false is supported), a publish can fail, a
// subscription can drop -- and the only consequence must be that a change takes
// up to ratelimit.reload.interval to appear, which is exactly the behaviour
// before any of this existed.
type ChangeNotifier interface {
	// Notify announces a change. A failure is not fatal to the write that
	// caused it: the write already succeeded, and the ticker will carry it.
	Notify(ctx context.Context) error

	// Watch calls onChange once per notification until ctx is done. It blocks,
	// and it is responsible for reconnecting: a dropped subscription that stays
	// dropped silently returns this service to ticker-only propagation with
	// nothing to say so.
	Watch(ctx context.Context, onChange func()) error

	// Close releases the subscription.
	Close() error
}
