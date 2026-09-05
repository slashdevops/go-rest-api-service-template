// Package ratelimitvalkey implements [ratelimit.Limiter] over a shared Valkey
// counter, with a per-replica fallback.
//
// # No Lua, deliberately
//
// The obvious implementation is an EVAL script that reads, decides and writes
// atomically. This one does not, and the reason is operational rather than
// aesthetic: a Lua script is a second program, in a second language, deployed by
// being sent to the server, with its own failure mode when a replica has a
// different version cached. Nothing here needs it.
//
// What is used instead is INCR plus PEXPIRE, which is atomic per key without a
// script:
//
//	INCR   ratelimit:{rule}:{window}:{scope}:{bucket}   -> n
//	PEXPIRE <same key> <window ms>  (only when n == 1)
//
// INCR is atomic. The window is a FIXED window derived from the wall clock, so
// two replicas incrementing the same key in the same window agree without
// coordinating. PEXPIRE on the first increment is what makes the key
// self-cleaning; a crash between the two leaves at most one key without a TTL,
// which the next window's key does not inherit.
//
// # The fixed window is a real trade, not an oversight
//
// A fixed window admits up to 2N requests across a window boundary -- N at the
// end of one and N at the start of the next. A sliding window does not, but
// costs a second key and a weighted read on every request.
//
// This is acceptable HERE because it is not the only limit: the per-replica
// limiter runs in front of it and is a token bucket, which smooths exactly that
// burst. The layering is the answer to the fixed window's weakness, and removing
// either half makes the other's trade worse.
//
// # A fault is UNKNOWN, never "allowed"
//
// [Adapter.Allow] returns an error when Valkey cannot answer. It does NOT report
// Allowed: true. Whether the caller then refuses or falls back to the
// per-replica limiter is `store.fail.mode`, a policy decision made above this
// package -- see the port's doc.go. Reporting "allowed" here would remove the
// limit precisely when the system is least healthy, which is the same mistake
// the token denylist is written to avoid.
//
// # Why not ratelimiter.BackendLimiter
//
// The library's BackendLimiter already supplies the fallback, the circuit
// breaker and the timeout, and this package deliberately does NOT reimplement
// them -- it implements only [ratelimiter.Backend], and the composition root
// wraps it. Duplicating the breaker here would give two breakers with different
// state, and the one that opened first would decide.
//
// As shipped the composition root does NOT wrap it in a BackendLimiter -- the
// middleware does the layering itself. The consequence is that the breaker and
// the per-call timeout are not in play, so during a store outage every request
// still attempts a round trip and waits out ratelimit.store.timeout. That is a
// known refinement, recorded in the review tracker, not an oversight in this
// package: what is written here stays correct either way.
//
// # Naming
//
// This package is named after the ratelimit PORT it implements, like every other
// driven adapter here -- cachevalkey, throttlememory, tokenjwt. It was
// ratelimitervalkey, after the library, which is one letter from the library's
// own package name and precisely what made it easy to misread.
package ratelimitvalkey
