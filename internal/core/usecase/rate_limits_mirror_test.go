package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// fakeRateLimits implements only SelectAll. Every other method panics, which is
// the point: the mirror must never reach the database on the request path, and
// a panic says so loudly where a nil return would not.
type fakeRateLimits struct {
	err   error
	rules []domain.RateLimit
	calls int
	mu    sync.Mutex
}

func (f *fakeRateLimits) SelectAll(context.Context) ([]domain.RateLimit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++

	if f.err != nil {
		return nil, f.err
	}

	return f.rules, nil
}

func (f *fakeRateLimits) set(rules []domain.RateLimit, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.rules, f.err = rules, err
}

func (f *fakeRateLimits) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls
}

func (f *fakeRateLimits) Insert(context.Context, *domain.CreateRateLimitInput) error {
	panic("the mirror must not write")
}

func (f *fakeRateLimits) UpdateByID(context.Context, *domain.UpdateRateLimitInput) error {
	panic("the mirror must not write")
}

func (f *fakeRateLimits) DeleteByID(context.Context, *domain.DeleteRateLimitInput) error {
	panic("the mirror must not write")
}

func (f *fakeRateLimits) Select(context.Context, *domain.SelectRateLimitsInput) (*domain.SelectRateLimitsOutput, error) {
	panic("the mirror must not paginate; it needs every rule")
}

func (f *fakeRateLimits) SelectByID(context.Context, uuid.UUID) (*domain.RateLimit, error) {
	panic("the mirror must not query per rule")
}

var _ repository.RateLimits = (*fakeRateLimits)(nil)

func testRateLimitRule(name string, windows ...domain.RateLimitWindow) domain.RateLimit {
	if len(windows) == 0 {
		windows = []domain.RateLimitWindow{{Requests: 10, Period: time.Second}}
	}

	return domain.RateLimit{
		ID: uuid.NewV7(), Name: name,
		TargetKind: domain.RateLimitTargetKindGlobal, Target: "*",
		Methods: []string{"*"}, Scope: domain.RateLimitScopeIP,
		Audience: domain.RateLimitAudienceAny, Strategy: domain.RateLimitStrategyTokenBucket,
		Windows: windows,
	}
}

func newTestRulesMirror(t *testing.T, repo repository.RateLimits) *RateLimitRules {
	t.Helper()

	ctx := t.Context()
	ot := &o11y.OpenTelemetry{
		Traces:  o11y.NewOpenTelemetryTracer(ctx, &o11y.OpenTelemetryTracerConfig{Name: "test"}),
		Metrics: o11y.NewOpenTelemetryMeter(ctx, &o11y.OpenTelemetryMeterConfig{Name: "test"}),
	}

	m, err := NewRateLimitRules(RateLimitRulesConfig{
		Repository: repo, OT: ot, ReloadInterval: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRateLimitRules: %v", err)
	}

	return m
}

// nil is "not known", NOT "none configured". A caller that conflates the two
// removes every limit on the first failed startup load -- and the symptom,
// traffic flowing freely, looks exactly like health.
func TestBeforeTheFirstLoadResolveReportsNotKnown(t *testing.T) {
	t.Parallel()

	m := newTestRulesMirror(t, &fakeRateLimits{})

	matches, known := m.Resolve(domain.RateLimitRequest{Method: "GET", Pattern: "/models"})
	if known {
		t.Fatal("nothing has loaded, so the mirror must report the set as unknown; " +
			"reporting 'known, no matches' removes every limit")
	}

	if matches != nil {
		t.Fatal("no matches should come back with an unknown set")
	}

	if m.Loaded() {
		t.Fatal("Loaded must be false before the first successful load")
	}
}

// The distinction the nil-versus-empty design exists for. An operator who
// deletes every rule must get "known, none apply" -- which falls back to the
// flags -- and NOT "unknown", which is the failure state. Conflating them makes
// deleting the last rule indistinguishable from the database being down.
func TestASuccessfulReloadOfZeroRulesIsKnownNotUnknown(t *testing.T) {
	t.Parallel()

	m := newTestRulesMirror(t, &fakeRateLimits{rules: nil})

	if err := m.Reload(t.Context()); err != nil {
		t.Fatalf("reload: %v", err)
	}

	matches, known := m.Resolve(domain.RateLimitRequest{Method: "GET", Pattern: "/models"})
	if !known {
		t.Fatal("a successful reload that found no rules is KNOWN and empty, not unknown; " +
			"otherwise deleting the last rule looks identical to the database being down")
	}

	if len(matches) != 0 {
		t.Fatalf("no rules were loaded, so nothing can match; got %d", len(matches))
	}

	if !m.Loaded() {
		t.Fatal("Loaded must be true after a successful reload, even of zero rules")
	}
}

// The single most important property here. A failed reload must leave the
// previous set in place: clearing it means a database blip silently removes
// every limit.
func TestAFailedReloadKeepsTheLastGoodCopy(t *testing.T) {
	t.Parallel()

	repo := &fakeRateLimits{rules: []domain.RateLimit{testRateLimitRule("first"), testRateLimitRule("second")}}
	m := newTestRulesMirror(t, repo)

	if err := m.Reload(t.Context()); err != nil {
		t.Fatalf("first reload: %v", err)
	}

	if got := m.Size(); got != 2 {
		t.Fatalf("Size = %d, want 2", got)
	}

	repo.set(nil, errors.New("database is down"))

	if err := m.Reload(t.Context()); err == nil {
		t.Fatal("a failing repository must produce an error")
	}

	if got := m.Size(); got != 2 {
		t.Fatalf("Size = %d after a failed reload, want 2 — the previous copy must be kept, "+
			"or a database blip silently removes every limit", got)
	}

	if _, known := m.Resolve(domain.RateLimitRequest{Method: "GET", Pattern: "/models"}); !known {
		t.Fatal("the set is still known after a failed reload; it is stale, not absent")
	}
}

// Staleness must NOT advance on a failed reload -- otherwise an alert on it can
// never fire, because every failure would look like a fresh load.
func TestAFailedReloadDoesNotRefreshStaleness(t *testing.T) {
	t.Parallel()

	repo := &fakeRateLimits{rules: []domain.RateLimit{testRateLimitRule("first")}}
	m := newTestRulesMirror(t, repo)

	if err := m.Reload(t.Context()); err != nil {
		t.Fatalf("first reload: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	before := m.Staleness()

	repo.set(nil, errors.New("down"))

	if err := m.Reload(t.Context()); err == nil {
		t.Fatal("expected an error")
	}

	if after := m.Staleness(); after < before {
		t.Fatalf("staleness went backwards on a failed reload (%s -> %s); "+
			"an alert on staleness would then never fire", before, after)
	}
}

// Before the first load staleness must be the maximum, not zero. Zero looks
// perfectly fresh, which is the exact opposite of never having loaded.
func TestRuleStalenessBeforeTheFirstLoadIsNotZero(t *testing.T) {
	t.Parallel()

	m := newTestRulesMirror(t, &fakeRateLimits{})

	if got := m.Staleness(); got != staleForever {
		t.Fatalf("Staleness = %s before any load, want the maximum duration — "+
			"zero reads as perfectly fresh", got)
	}
}

// A rule with no window has no budget. Enforcing it would refuse EVERY request
// to the endpoint it names, so it is skipped and logged rather than applied.
func TestARuleWithNoWindowIsSkippedNotEnforced(t *testing.T) {
	t.Parallel()

	broken := testRateLimitRule("broken")
	broken.Windows = nil

	repo := &fakeRateLimits{rules: []domain.RateLimit{broken, testRateLimitRule("good")}}
	m := newTestRulesMirror(t, repo)

	if err := m.Reload(t.Context()); err != nil {
		t.Fatalf("reload: %v", err)
	}

	if got := m.Size(); got != 1 {
		t.Fatalf("Size = %d, want 1 — a rule with no window must be skipped, "+
			"because enforcing a zero budget refuses every request to its target", got)
	}

	for _, r := range m.Rules() {
		if r.Name == "broken" {
			t.Fatal("the window-less rule reached the enforced set")
		}
	}
}

// The operator who saves a rule and immediately tests it must see it. Without an
// immediate local apply they see the old behaviour for up to one reload
// interval and conclude the feature is broken.
func TestApplyMakesAWriteVisibleWithoutAQuery(t *testing.T) {
	t.Parallel()

	repo := &fakeRateLimits{rules: []domain.RateLimit{testRateLimitRule("old")}}
	m := newTestRulesMirror(t, repo)

	if err := m.Reload(t.Context()); err != nil {
		t.Fatalf("reload: %v", err)
	}

	before := repo.callCount()

	m.Apply([]domain.RateLimit{testRateLimitRule("old"), testRateLimitRule("new")})

	if got := repo.callCount(); got != before {
		t.Fatalf("Apply made %d repository calls; it must not query", got-before)
	}

	if got := m.Size(); got != 2 {
		t.Fatalf("Size = %d after Apply, want 2", got)
	}
}

// Apply must not make the set look fresher than it is, or a stream of writes
// would silence a staleness alert while reloads were in fact failing.
func TestApplyDoesNotRefreshStaleness(t *testing.T) {
	t.Parallel()

	repo := &fakeRateLimits{rules: []domain.RateLimit{testRateLimitRule("a")}}
	m := newTestRulesMirror(t, repo)

	if err := m.Reload(t.Context()); err != nil {
		t.Fatalf("reload: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	before := m.Staleness()

	m.Apply([]domain.RateLimit{testRateLimitRule("a"), testRateLimitRule("b")})

	if after := m.Staleness(); after < before {
		t.Fatalf("Apply reset staleness (%s -> %s); a stream of writes would then "+
			"silence an alert while reloads were failing", before, after)
	}
}

// Apply drops disabled rules. Reload gets that from the WHERE clause; Apply is
// handed whatever the caller has, so it has to filter for itself.
func TestApplyDropsDisabledRules(t *testing.T) {
	t.Parallel()

	m := newTestRulesMirror(t, &fakeRateLimits{})

	off := testRateLimitRule("disabled")
	off.Enabled = new(false)

	m.Apply([]domain.RateLimit{testRateLimitRule("on"), off})

	if got := m.Size(); got != 1 {
		t.Fatalf("Size = %d, want 1 — a disabled rule must not be enforced", got)
	}
}

// Run must load IMMEDIATELY, not after one interval. Waiting would leave every
// request on the flag defaults for that long after every restart -- a rolling
// deploy's worth.
func TestRunLoadsImmediately(t *testing.T) {
	t.Parallel()

	repo := &fakeRateLimits{rules: []domain.RateLimit{testRateLimitRule("a")}}
	m := newTestRulesMirror(t, repo)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go m.Run(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.Loaded() {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatal("Run did not load within 2s; the reload interval is 10s, so it is waiting for the first tick " +
		"and every request would run on the flag defaults until then")
}

// A startup load that fails must not stop Run: the ticker has to keep trying,
// or a database that was briefly down at boot leaves the replica ruleless
// forever.
func TestRunSurvivesAFailedStartupLoad(t *testing.T) {
	t.Parallel()

	repo := &fakeRateLimits{err: errors.New("down at boot")}
	m := newTestRulesMirror(t, repo)

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	m.Run(ctx)

	if m.Loaded() {
		t.Fatal("nothing should have loaded")
	}

	if repo.callCount() == 0 {
		t.Fatal("Run must attempt a load even when it will fail")
	}
}

func TestResolveUsesTheMirroredRules(t *testing.T) {
	t.Parallel()

	specific := testRateLimitRule("generate")
	specific.TargetKind = domain.RateLimitTargetKindEndpoint
	specific.Target = "/projects/{project_id}/generate"
	specific.Methods = []string{"POST"}
	specific.Scope = domain.RateLimitScopeProject

	repo := &fakeRateLimits{rules: []domain.RateLimit{testRateLimitRule("global"), specific}}
	m := newTestRulesMirror(t, repo)

	if err := m.Reload(t.Context()); err != nil {
		t.Fatalf("reload: %v", err)
	}

	matches, known := m.Resolve(domain.RateLimitRequest{
		Method: "POST", Pattern: "/projects/{project_id}/generate", Authenticated: true,
	})
	if !known {
		t.Fatal("the set has loaded, so it is known")
	}

	if len(matches) != 2 {
		t.Fatalf("both the project and ip scopes apply, got %d", len(matches))
	}

	if matches[0].Rule.Name != "generate" {
		t.Fatalf("the endpoint rule should sort first, got %q", matches[0].Rule.Name)
	}
}

// The request path takes one atomic load and then holds an immutable slice, so
// a reload racing a resolution must not be a data race.
func TestConcurrentResolveAndReloadIsSafe(t *testing.T) {
	t.Parallel()

	repo := &fakeRateLimits{rules: []domain.RateLimit{testRateLimitRule("a"), testRateLimitRule("b")}}
	m := newTestRulesMirror(t, repo)

	if err := m.Reload(t.Context()); err != nil {
		t.Fatalf("reload: %v", err)
	}

	var wg sync.WaitGroup

	for range 8 {
		wg.Go(func() {
			for range 200 {
				m.Resolve(domain.RateLimitRequest{Method: "GET", Pattern: "/models"})
			}
		})
	}

	wg.Go(func() {
		for range 50 {
			//nolint:errcheck // the assertion is that -race finds nothing.
			_ = m.Reload(context.Background())
			m.Apply([]domain.RateLimit{testRateLimitRule("c")})
		}
	})

	wg.Wait()
}

func TestNewRejectsAnIncompleteConfig(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	ot := &o11y.OpenTelemetry{
		Traces:  o11y.NewOpenTelemetryTracer(ctx, &o11y.OpenTelemetryTracerConfig{Name: "test"}),
		Metrics: o11y.NewOpenTelemetryMeter(ctx, &o11y.OpenTelemetryMeterConfig{Name: "test"}),
	}

	tests := []struct {
		conf RateLimitRulesConfig
		name string
	}{
		{name: "nil_repository", conf: RateLimitRulesConfig{OT: ot, ReloadInterval: time.Second}},
		{name: "nil_ot", conf: RateLimitRulesConfig{Repository: &fakeRateLimits{}, ReloadInterval: time.Second}},
		{name: "zero_interval", conf: RateLimitRulesConfig{Repository: &fakeRateLimits{}, OT: ot}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewRateLimitRules(tt.conf); err == nil {
				t.Fatal("expected a rejection")
			}
		})
	}
}
