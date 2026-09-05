package middleware

import (
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

// RateLimitExemptions decides which requests never reach a rate limiter at all.
//
// # Why this is not inside a limiter
//
// There are two limiters and they are mutually exclusive: [RateLimit] when
// ratelimit.rules.enabled is on, [IPRateLimiter] when it is not -- and the
// second is what SHIPS BY DEFAULT. Both exemptions used to live inside
// [RateLimit], so in the default posture neither existed:
//
//   - /health/* and /version were rate limited, measured at 1 x 200 then 7 x 429
//     against a limiter set to 1 req/s. A load balancer sharing a source address
//     with real traffic, or simply a high enough probe rate, could have the
//     replica evicted -- which is the exact failure the bypass was written to
//     prevent.
//   - ratelimit.excluded.ips did nothing. An operator reaching for it during an
//     incident found it inert, with no error and no log line.
//
// Putting the decision here, ahead of the choice of limiter, is what makes it
// true in both postures. It is deliberately NOT a context flag each limiter then
// re-reads: two readers of one flag is two places to forget it, and the mistake
// this fixes was exactly a check that existed in one path and not the other.
//
// # It skips the limiter, it does not merely allow
//
// [RateLimitExemptions.Wrap] hands an exempt request to the handler WITHOUT
// invoking the limiter, so no bucket is touched and no shared counter is
// consulted. That matters under ratelimit.store.fail.mode=closed: an exempt
// request must survive a store outage, and it cannot do that if it still has to
// ask the store first.
//
// # Health is exempt in code, not by rule
//
// The bypass cannot be a database rule, because a rule can be deleted through
// the CRUD API -- and a deployment that deleted it would discover the loss as a
// readiness probe failing during a Valkey outage, which is the worst possible
// moment.
type RateLimitExemptions struct {
	resolver *ClientIPResolver

	// excluded are networks whose requests are never limited. Config rather
	// than database rows, so the escape hatch does not depend on the thing that
	// is broken when it is reached for.
	excluded []netip.Prefix

	// bypassPrefixes are path prefixes answered without any limiter.
	bypassPrefixes []string
}

// NewRateLimitExemptions parses the exemption configuration.
//
// A malformed entry is an error, never a dropped entry: a list that silently
// exempts nothing is the failure mode this whole type exists to correct, and
// reintroducing it one layer up would be worse for having been fixed once.
func NewRateLimitExemptions(resolver *ClientIPResolver, excludedIPs, bypassPrefixes []string) (*RateLimitExemptions, error) {
	excluded, err := ParseIPMatchers(excludedIPs, "excluded IP")
	if err != nil {
		return nil, err
	}

	prefixes := make([]string, 0, len(bypassPrefixes))

	for _, raw := range bypassPrefixes {
		if p := strings.TrimSpace(raw); p != "" {
			prefixes = append(prefixes, p)
		}
	}

	return &RateLimitExemptions{
		resolver:       resolver,
		excluded:       excluded,
		bypassPrefixes: prefixes,
	}, nil
}

// Exempt reports whether this request must not be rate limited.
func (ref *RateLimitExemptions) Exempt(r *http.Request) bool {
	if ref == nil {
		return false
	}

	if bypassed(r.URL.Path, ref.bypassPrefixes) {
		return true
	}

	// Resolved through the trusted-proxy boundary, never from a raw header: an
	// exemption keyed on something a caller can set is an exemption a caller can
	// grant themselves.
	return ref.ipExcluded(ref.resolver.ClientIP(r))
}

// Wrap returns limiter, skipped for exempt requests.
//
// A decorator rather than a middleware in the chain, because a middleware cannot
// tell the NEXT one not to run. Composing here lets one definition of "exempt"
// govern whichever limiter the composition root chose, with the limiter itself
// unchanged and unaware.
//
// A nil limiter returns a pass-through, so the caller does not have to branch on
// whether a limiter was configured at all.
func (ref *RateLimitExemptions) Wrap(limiter Middleware) Middleware {
	if limiter == nil {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		limited := limiter(next)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ref.Exempt(r) {
				next.ServeHTTP(w, r)

				return
			}

			limited.ServeHTTP(w, r)
		})
	}
}

// Excluded reports how many networks are exempt, for the startup log.
func (ref *RateLimitExemptions) Excluded() int {
	if ref == nil {
		return 0
	}

	return len(ref.excluded)
}

// BypassPrefixes reports the exempt path prefixes, for the startup log.
func (ref *RateLimitExemptions) BypassPrefixes() []string {
	if ref == nil {
		return nil
	}

	return slices.Clone(ref.bypassPrefixes)
}

// ipExcluded reports whether the resolved client address falls in any excluded
// network.
//
// An address that will not parse is NOT excluded. The resolver only ever returns
// something it parsed, so this is unreachable in practice -- and if it ever
// becomes reachable, over-limiting is the direction to fail in.
func (ref *RateLimitExemptions) ipExcluded(ip string) bool {
	if len(ref.excluded) == 0 {
		return false
	}

	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}

	addr = addr.Unmap()

	return slices.ContainsFunc(ref.excluded, func(p netip.Prefix) bool { return p.Contains(addr) })
}

// bypassed reports whether the path is answered without any limiter.
func bypassed(path string, prefixes []string) bool {
	return slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(path, p) })
}
