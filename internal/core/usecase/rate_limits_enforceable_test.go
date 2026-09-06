package usecase

import (
	"testing"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// A rule the mirror cannot enforce must not merely fail to apply -- it must not
// SHADOW a rule that can.
//
// ResolveRateLimits picks one winner per scope on specificity. Let a malformed
// rule into it and that rule can win its scope over a broader working one, so a
// bad row switches off a limit that was in force rather than just adding
// nothing. Filtering before resolution is what makes "skip it and fall through
// to the next tier" true rather than aspirational.
func TestAnUnenforceableRuleDoesNotShadowAWorkingOne(t *testing.T) {
	t.Parallel()

	// The endpoint rule outranks the global one, so if it survives it wins.
	broken := testRateLimitRule("aaa-broken-endpoint")
	broken.TargetKind = domain.RateLimitTargetKindEndpoint
	broken.Target = "/models"
	broken.Windows = nil

	working := testRateLimitRule("zzz-working-global")

	kept := EnforceableRateLimits([]domain.RateLimit{broken, working})

	matches := domain.ResolveRateLimits(kept, domain.RateLimitRequest{
		Method: "GET", Pattern: "/models",
	})

	if len(matches) != 1 {
		t.Fatalf("got %d matches, want 1", len(matches))
	}

	if matches[0].Rule.Name != "zzz-working-global" {
		t.Fatalf("the winning rule is %q; the malformed endpoint rule outranks the global one on "+
			"specificity, so letting it into resolution disables a limit rather than adding none",
			matches[0].Rule.Name)
	}
}

// A strategy no limiter can be built from is as unenforceable as a missing
// window. It cannot arrive through the API -- validation rejects it -- so it
// comes from a direct write or a partially applied change, which is exactly
// when nothing else is watching.
func TestARuleWithAnUnbuildableStrategyIsDropped(t *testing.T) {
	t.Parallel()

	broken := testRateLimitRule("broken")
	broken.Strategy = domain.RateLimitStrategy("sliding_window")

	kept := EnforceableRateLimits([]domain.RateLimit{broken, testRateLimitRule("good")})

	if len(kept) != 1 || kept[0].Name != "good" {
		t.Fatalf("kept %d rules (%v); a rule naming a strategy that cannot be built enforces nothing "+
			"and must not reach the limiter", len(kept), names(kept))
	}
}

// Disabled is an operator's deliberate choice, not a fault: dropped, but not
// warned about, and not counted as enforceable.
func TestEnforceableDropsDisabledRules(t *testing.T) {
	t.Parallel()

	off := testRateLimitRule("off")
	off.Enabled = new(false)

	kept := EnforceableRateLimits([]domain.RateLimit{off, testRateLimitRule("on")})

	if len(kept) != 1 || kept[0].Name != "on" {
		t.Fatalf("kept %v, want only the enabled rule", names(kept))
	}
}

// Reload used to keep disabled rules while Apply dropped them, so
// rate_limit_rules_size -- documented as "enabled rules currently mirrored" --
// counted different things depending on which path had last run.
func TestReloadAndApplyKeepTheSameRules(t *testing.T) {
	t.Parallel()

	off := testRateLimitRule("off")
	off.Enabled = new(false)

	noWindow := testRateLimitRule("no-window")
	noWindow.Windows = nil

	rules := []domain.RateLimit{off, noWindow, testRateLimitRule("good")}

	repo := &fakeRateLimits{rules: rules}
	m := newTestRulesMirror(t, repo)

	if err := m.Reload(t.Context()); err != nil {
		t.Fatalf("reload: %v", err)
	}

	afterReload := names(m.Rules())

	m.Apply(rules)

	afterApply := names(m.Rules())

	if len(afterReload) != 1 || afterReload[0] != "good" {
		t.Fatalf("after Reload the mirror holds %v, want only the enforceable rule", afterReload)
	}

	if len(afterApply) != len(afterReload) || afterApply[0] != afterReload[0] {
		t.Fatalf("Reload kept %v and Apply kept %v; the two paths must agree, or rate_limit_rules_size "+
			"means something different depending on which one ran last", afterReload, afterApply)
	}
}

// A rule with a window whose period is zero has no budget either, and would ask
// the limiter for a rate of "N per no time at all".
func TestEnforceableKeepsAWellFormedRule(t *testing.T) {
	t.Parallel()

	good := testRateLimitRule("good", domain.RateLimitWindow{Requests: 5, Period: time.Minute, Burst: 5})

	if kept := EnforceableRateLimits([]domain.RateLimit{good}); len(kept) != 1 {
		t.Fatalf("a well-formed rule was dropped; the filter must not refuse everything, "+
			"or every test above passes for the wrong reason (kept %v)", names(kept))
	}
}

func names(rules []domain.RateLimit) []string {
	out := make([]string, 0, len(rules))
	for i := range rules {
		out = append(out, rules[i].Name)
	}

	return out
}
