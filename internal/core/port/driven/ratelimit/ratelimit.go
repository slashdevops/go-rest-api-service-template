package ratelimit

import (
	"context"

	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/changenotify"
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

// ChangeNotifier is [changenotify.Notifier] under the name this port had before
// the token-lifetimes mirror needed the same thing. Kept as an alias so the
// existing adapter, wiring and tests read unchanged; new code should name the
// changenotify package directly.
type ChangeNotifier = changenotify.Notifier
