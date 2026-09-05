// Package ratelimit is the driven port for enforcing a rate-limit decision.
//
// # Why this port exists at all
//
// The enforcement itself lives in github.com/slashdevops/ratelimiter, and the
// shared counter lives in Valkey. Neither may be imported from internal/core --
// TestCoreHasNoInfraImports breaks the build if they are. So the use-case layer
// asks this interface a question about a budget, and the composition root
// decides what answers it.
//
// # The contract
//
// [Limiter.Allow] returns a [Decision] and an error. The two are NOT
// interchangeable, and conflating them is the mistake this package exists to
// prevent:
//
//   - Decision.Allowed == false means the budget is spent. The caller answers
//     429 with the reported Retry-After.
//   - a non-nil error means the budget could not be charged, so WHETHER it is
//     spent is unknown.
//
// An implementation that reaches a store returns an error when that store
// cannot answer. An in-process implementation has no store to be unreachable,
// so its only error is a BUDGET it cannot build a limiter from -- an invalid
// Strategy. Callers that distinguish the two must not infer which happened from
// the error alone: it is the implementation that knows, and the middleware
// tells them apart by which layer it asked. Reporting a malformed rule as a
// store outage drove a store-health gauge to zero and, under fail-closed,
// refused every request the rule matched.
//
// An unknown answer must not be reported as "allowed". That is the same shape as
// the revoked-token denylist, which fails closed for the same reason: treating
// an unreachable store as an empty one re-validates every credential anyone has
// logged out of, and treating an unreachable counter as an empty bucket removes
// the limit precisely when the system is least healthy.
//
// What the caller does with the error is a POLICY choice, not this port's:
// `store.fail.mode` selects between refusing (closed) and falling back to the
// per-replica limiter (local). The port's job is only to make the distinction
// impossible to lose.
//
// That policy applies to a STORE that could not answer, and to nothing else. A
// budget this port could not build is not an availability question, so the fail
// mode has no bearing on it in either direction.
//
// # What this port does NOT do
//
// It does not resolve which rule applies -- that is [domain.ResolveRateLimits],
// a pure function with no infrastructure. It does not decide the bucket key, and
// it does not know what a scope is. It is handed a key and a budget, and reports
// whether one request fits.
//
// Nor does it filter out rules that cannot be enforced. A rule with no window or
// an unbuildable strategy is dropped by the mirror, in
// usecase.EnforceableRateLimits, BEFORE resolution -- because resolution picks
// one winner per scope, so a malformed rule that reached it could outrank a
// working one and switch off a limit rather than add none.
//
// # Cost is always 1, for now
//
// There is no weighted variant. An expensive request may cost far more than a
// listing costs a row scan, so charging them the same is visibly wrong -- but a
// cost that varies with the endpoint is a second knob per rule, and the shape of
// that knob is not yet clear. The signature takes a cost so adding it later does
// not change every call site.
package ratelimit
