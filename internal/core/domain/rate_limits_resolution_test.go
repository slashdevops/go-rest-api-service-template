package domain

import (
	"strings"
	"testing"
	"time"
)

func rule(name string, kind RateLimitTargetKind, target string, methods []string, scope RateLimitScope) RateLimit {
	return RateLimit{
		Name: name, TargetKind: kind, Target: target, Methods: methods,
		Scope: scope, Audience: RateLimitAudienceAny,
		Strategy: RateLimitStrategyTokenBucket,
		Windows:  []RateLimitWindow{{Requests: 10, Period: time.Second}},
	}
}

func names(ms []RateLimitMatch) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Rule.Name)
	}

	return out
}

func TestResolvePrefersTheMostSpecificRuleWithinAScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		want  string
		rules []RateLimit
	}{
		{
			name: "endpoint_beats_prefix",
			want: "endpoint",
			rules: []RateLimit{
				rule("prefix", RateLimitTargetKindPrefix, "/projects/", []string{"*"}, RateLimitScopeIP),
				rule("endpoint", RateLimitTargetKindEndpoint, "/projects/{project_id}/generate", []string{"*"}, RateLimitScopeIP),
			},
		},
		{
			name: "prefix_beats_global",
			want: "prefix",
			rules: []RateLimit{
				rule("global", RateLimitTargetKindGlobal, "*", []string{"*"}, RateLimitScopeIP),
				rule("prefix", RateLimitTargetKindPrefix, "/projects/", []string{"*"}, RateLimitScopeIP),
			},
		},
		{
			// The names are chosen so that a TIE would pick the wildcard: the
			// tie-break is alphabetical, and "aaa" sorts before "zzz". Named
			// the other way round, this test passes even when the two tiers are
			// made equal -- verified, it did.
			name: "a_named_verb_beats_the_wildcard_at_the_same_kind",
			want: "zzz_named",
			rules: []RateLimit{
				rule("aaa_wildcard", RateLimitTargetKindEndpoint, "/projects/{project_id}/generate", []string{"*"}, RateLimitScopeIP),
				rule("zzz_named", RateLimitTargetKindEndpoint, "/projects/{project_id}/generate", []string{"POST"}, RateLimitScopeIP),
			},
		},
		{
			// The rung that is easy to get wrong: a named verb does NOT lift a
			// prefix rule above an endpoint rule. Kind dominates verb.
			name: "an_endpoint_wildcard_still_beats_a_prefix_named_verb",
			want: "endpoint_wildcard",
			rules: []RateLimit{
				rule("prefix_named", RateLimitTargetKindPrefix, "/projects/", []string{"POST"}, RateLimitScopeIP),
				rule("endpoint_wildcard", RateLimitTargetKindEndpoint, "/projects/{project_id}/generate", []string{"*"}, RateLimitScopeIP),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ResolveRateLimits(tt.rules, RateLimitRequest{
				Method: "POST", Pattern: "/projects/{project_id}/generate", Authenticated: true,
			})

			if len(got) != 1 {
				t.Fatalf("expected exactly one ip-scoped match, got %v", names(got))
			}

			if got[0].Rule.Name != tt.want {
				t.Fatalf("got %q, want %q", got[0].Rule.Name, tt.want)
			}
		})
	}
}

// The heart of the design: scopes do not compete. An IP rule bounds a source, a
// project rule bounds a tenant; collapsing to one winner would silently drop
// whichever bound the operator was not thinking about.
func TestResolveReturnsOneRulePerScopeNotOneOverall(t *testing.T) {
	t.Parallel()

	rules := []RateLimit{
		rule("ip_global", RateLimitTargetKindGlobal, "*", []string{"*"}, RateLimitScopeIP),
		rule("project_endpoint", RateLimitTargetKindEndpoint, "/projects/{project_id}/generate", []string{"POST"}, RateLimitScopeProject),
		rule("user_prefix", RateLimitTargetKindPrefix, "/projects/", []string{"*"}, RateLimitScopeUser),
	}

	got := ResolveRateLimits(rules, RateLimitRequest{
		Method: "POST", Pattern: "/projects/{project_id}/generate", Authenticated: true,
	})

	if len(got) != 3 {
		t.Fatalf("all three scopes apply, got %v", names(got))
	}

	// Most specific first.
	if got[0].Rule.Name != "project_endpoint" {
		t.Fatalf("the endpoint rule should sort first, got %v", names(got))
	}

	if got[len(got)-1].Rule.Name != "ip_global" {
		t.Fatalf("the global rule should sort last, got %v", names(got))
	}
}

func TestResolveSkipsRulesThatDoNotApply(t *testing.T) {
	t.Parallel()

	disabled := rule("disabled", RateLimitTargetKindGlobal, "*", []string{"*"}, RateLimitScopeIP)
	disabled.Enabled = new(false)

	guestOnly := rule("guest_only", RateLimitTargetKindGlobal, "*", []string{"*"}, RateLimitScopeUser)
	guestOnly.Audience = RateLimitAudienceGuest

	wrongVerb := rule("wrong_verb", RateLimitTargetKindEndpoint, "/projects/{project_id}/generate", []string{"DELETE"}, RateLimitScopeProject)

	wrongPath := rule("wrong_path", RateLimitTargetKindEndpoint, "/models", []string{"*"}, RateLimitScopeToken)

	got := ResolveRateLimits(
		[]RateLimit{disabled, guestOnly, wrongVerb, wrongPath},
		RateLimitRequest{Method: "POST", Pattern: "/projects/{project_id}/generate", Authenticated: true},
	)

	if len(got) != 0 {
		t.Fatalf("none of these should apply, got %v", names(got))
	}
}

// A prefix must not match a sibling that merely shares a string prefix.
// "/projects/" is required to end in a slash precisely so that this holds.
func TestPrefixDoesNotMatchASiblingPath(t *testing.T) {
	t.Parallel()

	rules := []RateLimit{rule("projects", RateLimitTargetKindPrefix, "/projects/", []string{"*"}, RateLimitScopeIP)}

	got := ResolveRateLimits(rules, RateLimitRequest{Method: "GET", Pattern: "/projects_archive/{id}"})
	if len(got) != 0 {
		t.Fatalf("/projects/ must not match /projects_archive, got %v", names(got))
	}

	got = ResolveRateLimits(rules, RateLimitRequest{Method: "GET", Pattern: "/projects/{project_id}"})
	if len(got) != 1 {
		t.Fatalf("/projects/ must match /projects/{project_id}, got %v", names(got))
	}
}

// An arbitrary winner is acceptable; a winner that CHANGES between calls is not.
// A limit that flaps for no visible reason is close to undiagnosable.
func TestResolveIsStableWhenTwoRulesTie(t *testing.T) {
	t.Parallel()

	a := rule("aaa", RateLimitTargetKindEndpoint, "/models", []string{"GET"}, RateLimitScopeIP)
	b := rule("bbb", RateLimitTargetKindEndpoint, "/models", []string{"GET"}, RateLimitScopeIP)

	req := RateLimitRequest{Method: "GET", Pattern: "/models"}

	first := ResolveRateLimits([]RateLimit{a, b}, req)
	second := ResolveRateLimits([]RateLimit{b, a}, req)

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected one match each, got %v and %v", names(first), names(second))
	}

	if first[0].Rule.Name != second[0].Rule.Name {
		t.Fatalf("input order changed the winner: %q vs %q", first[0].Rule.Name, second[0].Rule.Name)
	}
}

func TestResolveExplainsWhy(t *testing.T) {
	t.Parallel()

	got := ResolveRateLimits(
		[]RateLimit{rule("global", RateLimitTargetKindGlobal, "*", []string{"*"}, RateLimitScopeIP)},
		RateLimitRequest{Method: "GET", Pattern: "/models"},
	)

	if len(got) != 1 {
		t.Fatalf("expected one match, got %d", len(got))
	}

	// The endpoint exists to answer "why not the other rule", so an empty or
	// generic explanation is a failure, not a cosmetic gap.
	if !strings.Contains(got[0].Why, "global") {
		t.Fatalf("explanation should say the rule matched globally, got %q", got[0].Why)
	}
}

func TestResolveCoversHeadThroughTheGetRule(t *testing.T) {
	t.Parallel()

	got := ResolveRateLimits(
		[]RateLimit{rule("get_models", RateLimitTargetKindEndpoint, "/models", []string{"GET"}, RateLimitScopeIP)},
		RateLimitRequest{Method: "HEAD", Pattern: "/models"},
	)

	if len(got) != 1 {
		t.Fatal("a HEAD request must be covered by the GET rule for that route, or it slips past every limit on the endpoint")
	}
}

func TestResolveHandlesNoRulesAtAll(t *testing.T) {
	t.Parallel()

	// Not a curiosity: this is the state after an operator deletes the seeded
	// default, and it must resolve to nothing so the caller falls back to the
	// flags rather than to an arbitrary rule.
	if got := ResolveRateLimits(nil, RateLimitRequest{Method: "GET", Pattern: "/models"}); len(got) != 0 {
		t.Fatalf("no rules must resolve to no matches, got %v", names(got))
	}
}
