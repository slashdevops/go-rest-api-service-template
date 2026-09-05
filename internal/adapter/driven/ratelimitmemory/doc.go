// Package ratelimitmemory implements [ratelimit.Limiter] in process, over
// github.com/slashdevops/ratelimiter.
//
// # What it is for
//
// Two things, and the second is the one that is easy to forget:
//
//  1. `cache.enabled=false` is a supported deployment. There is no shared
//     counter there, so this IS the limiter.
//  2. It is the FALLBACK when the shared counter cannot be reached. A limit
//     enforced N times, once per replica, is not the limit that was configured
//     -- but it is bounded, which the alternative of no limit at all is not.
//
// # The budget is per replica, and that is a deliberate trade
//
// N replicas means N x the configured rate in the worst case. The alternatives
// were considered and rejected: a Valkey-backed store cannot be the only path
// because `cache.enabled=false` must keep working, and Postgres would mean a
// write per unauthenticated request. This is the same reasoning the login
// throttle already records.
//
// # One limiter per key, and why the factory matters
//
// [ratelimiter.WithLimiterFactoryForKey] gives each bucket key its own limiter
// built from that RULE's parameters. Without it every key would share one set of
// parameters, which is wrong the moment two rules with different budgets are
// active at once -- and would be invisible, because the wrong limiter still
// limits.
//
// # Reloads must not reset a live bucket
//
// The key is (rule, window, scope value). A rule whose numbers did not change
// keeps its bucket across a mirror reload, so an operator editing an unrelated
// rule does not refill the budget of every caller currently being limited. This
// is the trap PocketBase's implementation falls into, and it is the difference
// between a rate limit and a rate suggestion.
//
// # Its only error is a rule it cannot build, never a store fault
//
// There is no store here to be unreachable -- which is exactly what makes this
// usable as the fallback, since a fallback that could itself fail would need one
// of its own. So [Adapter.Allow] returns an error in ONE case: a
// [ratelimit.Budget] whose Strategy the library cannot parse.
//
// Refusing there rather than defaulting is the point of the strategy column. A
// rule that says leaky_bucket while a token bucket is silently enforced is worse
// than an error, because nothing in the response, the logs or the metrics would
// say the operator did not get what they asked for.
//
// Callers must tell this apart from the shared counter failing. The middleware
// treated the two alike once, and one malformed row reported itself as a Valkey
// outage: rate_limit_store_up went to zero and, under the default fail-closed,
// every request the rule matched was refused.
//
// # Naming
//
// This package is named after the ratelimit PORT it implements, like every other
// driven adapter here -- cachevalkey, throttlememory, tokenjwt. It was
// ratelimitermemory, after the library, which is one letter from the library's
// own package name and precisely what made it easy to misread.
package ratelimitmemory
