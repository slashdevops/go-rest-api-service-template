package domain

import (
	"slices"
	"strings"
)

// RateLimitRequest is everything the ladder needs to know about a request.
// Deliberately not an *http.Request: resolution is pure, and internal/core may
// not import a transport package.
type RateLimitRequest struct {
	// Method is the verb as registered, uppercase.
	Method string

	// Pattern is the ROUTE TEMPLATE the mux matched -- "/projects/{project_id}/generate",
	// braces included -- NOT the concrete path. Matching on the concrete path
	// would need a rule per project id.
	Pattern string

	// Authenticated selects which audiences apply.
	Authenticated bool
}

// RateLimitMatch is one rule that applies, with the tier it won on.
type RateLimitMatch struct {
	Rule *RateLimit

	// Why is prose, and prose on purpose. The ladder is the thing operators get
	// wrong, and a tier number answers "which rung" when the question they are
	// actually asking is "why not the other rule".
	Why string

	// Specificity is the tier, higher wins. Exported for tests and for the
	// effective endpoint's ordering; not part of the API contract.
	Specificity int
}

// Rate-limit specificity tiers. Each target kind is crossed with named-verb vs
// any-verb, because naming a verb is more specific than not, at every kind.
//
// The values are ordinal only -- nothing depends on the gaps.
const (
	rateLimitTierGlobalAny     = 1
	rateLimitTierGlobalMethod  = 2
	rateLimitTierPrefixAny     = 3
	rateLimitTierPrefixMethod  = 4
	rateLimitTierEndpointAny   = 5
	rateLimitTierEndpointNamed = 6
)

// ResolveRateLimits returns the rules that apply to a request: AT MOST ONE PER
// SCOPE, each the most specific of its scope.
//
// One per scope, not one overall, is the whole design. An ip-scoped rule and a
// project-scoped rule both apply to the same request and neither substitutes for
// the other -- the first bounds a source, the second bounds a tenant. Collapsing
// to a single winner would silently drop whichever bound the operator was not
// thinking about when they wrote the more specific rule.
//
// Within a scope, ties are broken by specificity and then by name, so the result
// is stable across reloads. An unstable order here would make the effective
// endpoint disagree with itself between calls.
func ResolveRateLimits(rules []RateLimit, req RateLimitRequest) []RateLimitMatch {
	best := make(map[RateLimitScope]RateLimitMatch, len(validRateLimitScopes))

	for i := range rules {
		rule := &rules[i]

		if !rule.IsEnabled() {
			continue
		}

		if !rule.MatchesAudience(req.Authenticated) {
			continue
		}

		if !rule.MatchesMethod(req.Method) {
			continue
		}

		tier, why, ok := rateLimitTier(rule, req.Pattern)
		if !ok {
			continue
		}

		current, seen := best[rule.Scope]
		if seen && !rateLimitBeats(tier, rule, current) {
			continue
		}

		best[rule.Scope] = RateLimitMatch{Rule: rule, Specificity: tier, Why: why}
	}

	out := make([]RateLimitMatch, 0, len(best))
	for _, m := range best {
		out = append(out, m)
	}

	// Most specific first, then by scope, then by name: a total order, so two
	// calls with the same rules always answer identically.
	slices.SortFunc(out, func(a, b RateLimitMatch) int {
		if a.Specificity != b.Specificity {
			return b.Specificity - a.Specificity
		}

		if c := strings.Compare(string(a.Rule.Scope), string(b.Rule.Scope)); c != 0 {
			return c
		}

		return strings.Compare(a.Rule.Name, b.Rule.Name)
	})

	return out
}

// rateLimitBeats reports whether a candidate at tier should replace the current
// winner in its scope.
func rateLimitBeats(tier int, candidate *RateLimit, current RateLimitMatch) bool {
	if tier != current.Specificity {
		return tier > current.Specificity
	}

	// Same tier is a genuine ambiguity -- two rules the operator wrote that are
	// equally specific. Name order is arbitrary but STABLE, which is the
	// property that matters: an arbitrary winner that changes between reloads
	// would make a limit appear to flap for no visible reason.
	return strings.Compare(candidate.Name, current.Rule.Name) < 0
}

// rateLimitTier reports which rung a rule occupies for this pattern, and why.
func rateLimitTier(rule *RateLimit, pattern string) (int, string, bool) {
	named := !rule.AnyMethod()

	switch rule.TargetKind {
	case RateLimitTargetKindEndpoint:
		if rule.Target != pattern {
			return 0, "", false
		}

		if named {
			return rateLimitTierEndpointNamed, "exact endpoint and a named verb — the most specific match there is", true
		}

		return rateLimitTierEndpointAny, "exact endpoint, any verb — one budget shared across every method on this route", true

	case RateLimitTargetKindPrefix:
		if !strings.HasPrefix(pattern, rule.Target) {
			return 0, "", false
		}

		if named {
			return rateLimitTierPrefixMethod, "prefix " + rule.Target + " and a named verb — no endpoint rule matched", true
		}

		return rateLimitTierPrefixAny, "prefix " + rule.Target + ", any verb", true

	case RateLimitTargetKindGlobal:
		if named {
			return rateLimitTierGlobalMethod, "global, named verb — no endpoint or prefix rule matched", true
		}

		return rateLimitTierGlobalAny, "global — no more specific rule matched in this scope", true

	default:
		// A kind outside the three cannot have passed Validate or the CHECK
		// constraint. Matching nothing disables the rule, which is the reading
		// that fails toward the flags rather than toward an arbitrary tier.
		return 0, "", false
	}
}
