package domain

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"uuid"
)

func validCreateInput() CreateRateLimitInput {
	return CreateRateLimitInput{
		ID:          uuid.NewV7(),
		Name:        "generate",
		Description: "one expensive call, real money",
		TargetKind:  RateLimitTargetKindEndpoint,
		Target:      "/projects/{project_id}/generate",
		Methods:     []string{"POST"},
		Scope:       RateLimitScopeProject,
		Audience:    RateLimitAudienceAuth,
		Strategy:    RateLimitStrategyLeakyBucket,
		Windows:     []RateLimitWindow{{Requests: 10, Period: time.Minute, Burst: 1}},
	}
}

func TestCreateRateLimitInputAcceptsAValidRule(t *testing.T) {
	t.Parallel()

	in := validCreateInput()
	if err := in.Validate(); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
}

// A validator that refuses everything would pass every rejection test below, so
// each case here changes ONE field of an input already proven to pass.
func TestCreateRateLimitInputRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mutate func(*CreateRateLimitInput)
		name   string
		field  string
	}{
		{name: "unknown_strategy", field: "strategy", mutate: func(i *CreateRateLimitInput) { i.Strategy = "sliding_window" }},
		{name: "empty_strategy", field: "strategy", mutate: func(i *CreateRateLimitInput) { i.Strategy = "" }},
		{name: "unknown_scope", field: "scope", mutate: func(i *CreateRateLimitInput) { i.Scope = "organisation" }},
		{name: "unknown_audience", field: "audience", mutate: func(i *CreateRateLimitInput) { i.Audience = "admin" }},
		{name: "unknown_target_kind", field: "target_kind", mutate: func(i *CreateRateLimitInput) { i.TargetKind = "tag" }},
		{name: "global_kind_with_a_real_target", field: "target", mutate: func(i *CreateRateLimitInput) {
			i.TargetKind = RateLimitTargetKindGlobal
		}},
		{name: "endpoint_kind_targeting_star", field: "target", mutate: func(i *CreateRateLimitInput) { i.Target = "*" }},
		{name: "prefix_without_trailing_slash", field: "target", mutate: func(i *CreateRateLimitInput) {
			i.TargetKind = RateLimitTargetKindPrefix
			i.Target = "/projects"
		}},
		{name: "star_mixed_with_named_methods", field: "methods", mutate: func(i *CreateRateLimitInput) {
			i.Methods = []string{"*", "GET"}
		}},
		{name: "empty_methods", field: "methods", mutate: func(i *CreateRateLimitInput) { i.Methods = nil }},
		{name: "duplicate_method", field: "methods", mutate: func(i *CreateRateLimitInput) { i.Methods = []string{"GET", "GET"} }},
		{name: "lowercase_method", field: "methods", mutate: func(i *CreateRateLimitInput) { i.Methods = []string{"post"} }},
		{name: "no_windows", field: "windows", mutate: func(i *CreateRateLimitInput) { i.Windows = nil }},
		{name: "duplicate_period", field: "windows", mutate: func(i *CreateRateLimitInput) {
			i.Windows = []RateLimitWindow{{Requests: 1, Period: time.Second}, {Requests: 2, Period: time.Second}}
		}},
		{name: "zero_requests", field: "windows", mutate: func(i *CreateRateLimitInput) {
			i.Windows = []RateLimitWindow{{Requests: 0, Period: time.Second}}
		}},
		{name: "sub_second_period", field: "windows", mutate: func(i *CreateRateLimitInput) {
			i.Windows = []RateLimitWindow{{Requests: 1, Period: 500 * time.Millisecond}}
		}},
		{name: "period_over_a_day", field: "windows", mutate: func(i *CreateRateLimitInput) {
			i.Windows = []RateLimitWindow{{Requests: 1, Period: 25 * time.Hour}}
		}},
		{name: "negative_burst", field: "windows", mutate: func(i *CreateRateLimitInput) {
			i.Windows = []RateLimitWindow{{Requests: 1, Period: time.Second, Burst: -1}}
		}},
		{name: "guest_scoped_to_user", field: "scope", mutate: func(i *CreateRateLimitInput) {
			i.Audience = RateLimitAudienceGuest
			i.Scope = RateLimitScopeUser
		}},
		{name: "too_many_windows", field: "windows", mutate: func(i *CreateRateLimitInput) {
			w := make([]RateLimitWindow, 0, RateLimitMaxWindows+1)
			for n := range RateLimitMaxWindows + 1 {
				w = append(w, RateLimitWindow{Requests: 1, Period: time.Duration(n+1) * time.Second})
			}
			i.Windows = w
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := validCreateInput()
			tt.mutate(&in)

			err := in.Validate()
			if err == nil {
				t.Fatalf("expected a rejection, got none")
			}

			// Naming the field is the point: a validator that rejected for the
			// wrong reason would pass a bare "expected an error" assertion.
			if !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("rejection did not mention %q: %v", tt.field, err)
			}
		})
	}
}

// The message has to say what to use instead. An error that says only "invalid"
// leaves the caller guessing, and guessing is what produced the migration that
// asked providers for a model called "-unknown".
func TestUnknownStrategyErrorListsTheValidOnes(t *testing.T) {
	t.Parallel()

	err := IsValidRateLimitStrategy("sliding_window")
	if err == nil {
		t.Fatal("expected a rejection")
	}

	for _, s := range RateLimitStrategies() {
		if !strings.Contains(err.Error(), string(s)) {
			t.Fatalf("error does not name the valid strategy %q: %v", s, err)
		}
	}
}

// The wire values, the database CHECK constraint and ratelimiter.ParseStrategy
// must agree. If the library renames a strategy, every stored row becomes
// unbuildable -- silently, because nothing validates the string before use.
func TestRateLimitStrategiesMatchTheMigrationCheckConstraint(t *testing.T) {
	t.Parallel()

	b, err := os.ReadFile("../../../database/migrations/00015_rate_limits.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	m := regexp.MustCompile(`chk_rate_limits_strategy CHECK \(strategy IN \(([^)]*)\)\)`).FindSubmatch(b)
	if m == nil {
		t.Fatal("chk_rate_limits_strategy not found in the migration")
	}

	inDB := regexp.MustCompile(`'([a-z_]+)'`).FindAllStringSubmatch(string(m[1]), -1)

	got := make([]string, 0, len(inDB))
	for _, s := range inDB {
		got = append(got, s[1])
	}

	want := make([]string, 0, len(RateLimitStrategies()))
	for _, s := range RateLimitStrategies() {
		want = append(want, string(s))
	}

	slices.Sort(got)
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Fatalf("migration CHECK has %v, domain has %v", got, want)
	}
}

// Same contract for the other three enums: the CHECK and the domain must not
// drift apart, in either direction.
func TestRateLimitEnumsMatchTheMigrationCheckConstraints(t *testing.T) {
	t.Parallel()

	b, err := os.ReadFile("../../../database/migrations/00015_rate_limits.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	scopes := make([]string, 0)
	for _, s := range RateLimitScopes() {
		scopes = append(scopes, string(s))
	}

	audiences := make([]string, 0)
	for _, a := range RateLimitAudiences() {
		audiences = append(audiences, string(a))
	}

	kinds := make([]string, 0)
	for _, k := range RateLimitTargetKinds() {
		kinds = append(kinds, string(k))
	}

	tests := []struct {
		constraint string
		column     string
		want       []string
	}{
		{constraint: "chk_rate_limits_scope", column: "scope", want: scopes},
		{constraint: "chk_rate_limits_audience", column: "audience", want: audiences},
		{constraint: "chk_rate_limits_target_kind", column: "target_kind", want: kinds},
	}

	for _, tt := range tests {
		t.Run(tt.constraint, func(t *testing.T) {
			t.Parallel()

			re := regexp.MustCompile(tt.constraint + ` CHECK \(` + tt.column + ` IN \(([^)]*)\)\)`)

			m := re.FindSubmatch(b)
			if m == nil {
				t.Fatalf("%s not found in the migration", tt.constraint)
			}

			got := make([]string, 0)
			for _, s := range regexp.MustCompile(`'([a-z_]+)'`).FindAllStringSubmatch(string(m[1]), -1) {
				got = append(got, s[1])
			}

			want := slices.Clone(tt.want)
			slices.Sort(got)
			slices.Sort(want)

			if !slices.Equal(got, want) {
				t.Fatalf("migration has %v, domain has %v", got, want)
			}
		})
	}
}

func TestEffectiveBurstDefaultsToRequests(t *testing.T) {
	t.Parallel()

	// Stored as 0 rather than expanded on write, so a later edit to Requests
	// carries the burst with it instead of freezing at the old value.
	w := RateLimitWindow{Requests: 100}
	if got := w.EffectiveBurst(); got != 100 {
		t.Fatalf("burst 0 should mean requests (100), got %d", got)
	}

	w.Burst = 7
	if got := w.EffectiveBurst(); got != 7 {
		t.Fatalf("explicit burst should win, got %d", got)
	}
}

func TestHeadIsCoveredByAGetRule(t *testing.T) {
	t.Parallel()

	rule := RateLimit{Methods: []string{"GET"}}

	if !rule.MatchesMethod("HEAD") {
		t.Fatal("HEAD is served by the GET route, so a GET rule must cover it; otherwise HEAD slips past every rule written for that endpoint")
	}

	if rule.MatchesMethod("POST") {
		t.Fatal("a GET rule must not cover POST")
	}

	// The wildcard covers HEAD by covering everything, which is a different
	// path through MatchesMethod and would hide a broken GET/HEAD branch.
	any := RateLimit{Methods: []string{"*"}}
	if !any.MatchesMethod("HEAD") || !any.MatchesMethod("POST") {
		t.Fatal("* must cover every verb")
	}
}

func TestSortedWindowsPutsTheShortestPeriodFirst(t *testing.T) {
	t.Parallel()

	rule := RateLimit{Windows: []RateLimitWindow{
		{Requests: 1000, Period: time.Hour},
		{Requests: 10, Period: time.Second},
		{Requests: 300, Period: time.Minute},
	}}

	got := rule.SortedWindows()

	want := []time.Duration{time.Second, time.Minute, time.Hour}
	for i, w := range got {
		if w.Period != want[i] {
			t.Fatalf("window %d: got %s, want %s — evaluation order is load-bearing, see SortedWindows", i, w.Period, want[i])
		}
	}

	// SortedWindows must not reorder the rule's own slice: the mirror shares
	// rules between goroutines, and an in-place sort would be a data race.
	if rule.Windows[0].Period != time.Hour {
		t.Fatal("SortedWindows mutated the receiver's slice")
	}
}

func TestMatchesAudience(t *testing.T) {
	t.Parallel()

	tests := []struct {
		audience      RateLimitAudience
		name          string
		authenticated bool
		want          bool
	}{
		{name: "any_matches_a_guest", audience: RateLimitAudienceAny, authenticated: false, want: true},
		{name: "any_matches_a_user", audience: RateLimitAudienceAny, authenticated: true, want: true},
		{name: "guest_matches_a_guest", audience: RateLimitAudienceGuest, authenticated: false, want: true},
		{name: "guest_does_not_match_a_user", audience: RateLimitAudienceGuest, authenticated: true, want: false},
		{name: "auth_matches_a_user", audience: RateLimitAudienceAuth, authenticated: true, want: true},
		{name: "auth_does_not_match_a_guest", audience: RateLimitAudienceAuth, authenticated: false, want: false},
		{name: "an_impossible_audience_matches_nothing", audience: "admin", authenticated: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rule := RateLimit{Audience: tt.audience}
			if got := rule.MatchesAudience(tt.authenticated); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsEnabledTreatsNilAsTheDatabaseDefault(t *testing.T) {
	t.Parallel()

	if !(&RateLimit{}).IsEnabled() {
		t.Fatal("a nil Enabled must read as the column default, which is TRUE")
	}

	if (&RateLimit{Enabled: new(false)}).IsEnabled() {
		t.Fatal("an explicit false must disable the rule")
	}
}

func TestValidRateLimitTargetAcceptsRouteTemplatesNotPolicyResources(t *testing.T) {
	t.Parallel()

	// The whole reason this is not IsValidResource: a policy resource has its
	// ids already substituted, a rule target is the template.
	valid := []string{
		"*",
		"/projects/",
		"/projects/{project_id}/generate",
		"/health",
		"/projects/{project_id}/embeddings/{embedding_id}",
	}
	for _, target := range valid {
		if err := IsValidRateLimitTarget(target); err != nil {
			t.Fatalf("%q should be a valid target: %v", target, err)
		}
	}

	invalid := []string{
		"",
		"projects/",                         // no leading slash
		"/Projects/",                        // uppercase
		"/projects/{Project_ID}",            // uppercase in the placeholder
		"/projects/{project_id}/generate/*", // policy wildcard, not a template
		"https://example.com/projects",
	}
	for _, target := range invalid {
		if err := IsValidRateLimitTarget(target); err == nil {
			t.Fatalf("%q should be rejected as a target", target)
		}
	}
}
