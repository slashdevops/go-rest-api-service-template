//go:build unit

package middleware_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// audienceRule is mwRule with an explicit audience, which mwRule always sets to
// "any".
func audienceRule(name string, scope domain.RateLimitScope, audience domain.RateLimitAudience) domain.RateLimit {
	rule := mwRule(name, scope)
	rule.Audience = audience

	return rule
}

func authedRequest(method, target string) *http.Request {
	r := rlRequest(method, target)

	return r.WithContext(context.WithValue(r.Context(), middleware.JwtClaims, map[string]any{"sub": "alice"}))
}

// An ip rule with audience=auth was enforced in NEITHER stage.
//
// The stage filter routed it pre-auth because its SCOPE is ip; the audience
// check rejected it there because nobody is authenticated before
// CheckAccessToken has run. Post-auth the audience matched and the scope filter
// dropped it instead. The rule was accepted by the API, listed in the UI,
// returned by the ladder -- and charged nowhere.
//
// Reverting appliesInThisStage to branch on scope alone fails this.
func TestAnAuthAudienceIPRuleIsChargedPostAuth(t *testing.T) {
	t.Parallel()

	rules := staticRules{known: true, rules: []domain.RateLimit{
		audienceRule("ip-auth", domain.RateLimitScopeIP, domain.RateLimitAudienceAuth),
	}}

	pre := newCountingLimiter(true)
	rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: pre, Resolver: newResolver(t), Stage: middleware.RateLimitStagePreAuth,
	}, rlRequest(http.MethodGet, "/models"))

	if got := len(pre.charges()); got != 0 {
		t.Fatalf("pre-auth charged %d buckets for an auth-audience rule; nobody is authenticated there", got)
	}

	post := newCountingLimiter(true)
	rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: post, Resolver: newResolver(t), Stage: middleware.RateLimitStagePostAuth,
	}, authedRequest(http.MethodGet, "/models"))

	if got := len(post.charges()); got != 1 {
		t.Fatalf("post-auth charged %d buckets for an ip+auth rule, want 1; "+
			"charged in neither stage, the rule silently enforces nothing", got)
	}
}

// The same for global, the other pre-auth scope.
func TestAnAuthAudienceGlobalRuleIsChargedPostAuth(t *testing.T) {
	t.Parallel()

	rules := staticRules{known: true, rules: []domain.RateLimit{
		audienceRule("global-auth", domain.RateLimitScopeGlobal, domain.RateLimitAudienceAuth),
	}}

	post := newCountingLimiter(true)
	rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: post, Resolver: newResolver(t), Stage: middleware.RateLimitStagePostAuth,
	}, authedRequest(http.MethodGet, "/models"))

	if got := len(post.charges()); got != 1 {
		t.Fatalf("post-auth charged %d buckets for a global+auth rule, want 1", got)
	}
}

// The fix must not reintroduce the double charge the stage filter exists to
// prevent: an ip rule with the DEFAULT audience is still charged once, pre-auth.
func TestTheAudienceFixDoesNotDoubleChargeAnAnyAudienceIPRule(t *testing.T) {
	t.Parallel()

	rules := staticRules{known: true, rules: []domain.RateLimit{
		audienceRule("ip-any", domain.RateLimitScopeIP, domain.RateLimitAudienceAny),
	}}

	post := newCountingLimiter(true)
	rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: post, Resolver: newResolver(t), Stage: middleware.RateLimitStagePostAuth,
	}, authedRequest(http.MethodGet, "/models"))

	if got := len(post.charges()); got != 0 {
		t.Fatalf("post-auth charged %d buckets for an ip+any rule; it is charged pre-auth, "+
			"so charging it again halves every ip limit", got)
	}
}

// A guest rule keeps being charged pre-auth, where the caller is anonymous.
func TestAGuestAudienceIPRuleIsStillChargedPreAuth(t *testing.T) {
	t.Parallel()

	rules := staticRules{known: true, rules: []domain.RateLimit{
		audienceRule("ip-guest", domain.RateLimitScopeIP, domain.RateLimitAudienceGuest),
	}}

	pre := newCountingLimiter(true)
	rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: pre, Resolver: newResolver(t), Stage: middleware.RateLimitStagePreAuth,
	}, rlRequest(http.MethodGet, "/models"))

	if got := len(pre.charges()); got != 1 {
		t.Fatalf("pre-auth charged %d buckets for an ip+guest rule, want 1", got)
	}
}
