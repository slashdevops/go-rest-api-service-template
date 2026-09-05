package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/ratelimit"
)

// countingLimiter records every charge, so a test can assert HOW MANY buckets a
// request touched -- not only whether it was allowed. The double-charge bug is
// invisible to an allow/deny assertion.
type countingLimiter struct {
	err   error
	keys  []string
	allow bool
	mu    sync.Mutex
}

func newCountingLimiter(allow bool) *countingLimiter {
	return &countingLimiter{allow: allow}
}

func (c *countingLimiter) Allow(_ context.Context, key string, _ ratelimit.Budget, _ int) (ratelimit.Decision, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.err != nil {
		return ratelimit.Decision{}, c.err
	}

	c.keys = append(c.keys, key)

	if !c.allow {
		return ratelimit.Decision{Allowed: false, Remaining: 0, RetryAfter: 2 * time.Second}, nil
	}

	return ratelimit.Decision{Allowed: true, Remaining: 7}, nil
}

func (c *countingLimiter) Close() error { return nil }

func (c *countingLimiter) charges() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]string(nil), c.keys...)
}

// staticRules is a mirror that always answers the same way.
type staticRules struct {
	rules []domain.RateLimit
	known bool
}

func (s staticRules) Resolve(req domain.RateLimitRequest) ([]domain.RateLimitMatch, bool) {
	if !s.known {
		return nil, false
	}

	return domain.ResolveRateLimits(s.rules, req), true
}

func mwRule(name string, scope domain.RateLimitScope, windows ...domain.RateLimitWindow) domain.RateLimit {
	if len(windows) == 0 {
		windows = []domain.RateLimitWindow{{ID: uuid.NewV7(), Requests: 10, Period: time.Second}}
	}

	return domain.RateLimit{
		ID: uuid.NewV7(), Name: name,
		TargetKind: domain.RateLimitTargetKindGlobal, Target: "*",
		Methods: []string{"*"}, Scope: scope,
		Audience: domain.RateLimitAudienceAny,
		Strategy: domain.RateLimitStrategyTokenBucket,
		Windows:  windows,
	}
}

func newResolver(t *testing.T) *middleware.ClientIPResolver {
	t.Helper()

	r, err := middleware.NewClientIPResolver(nil)
	if err != nil {
		t.Fatalf("NewClientIPResolver: %v", err)
	}

	return r
}

func rlServe(t *testing.T, conf middleware.RateLimitConfig, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()

	middleware.RateLimit(conf)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, r)

	return rec
}

func rlRequest(method, target string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.RemoteAddr = "203.0.113.9:1234"

	return r
}

// THE double-charge trap. Both stages see every matched rule, so without the
// stage filter an ip rule is charged pre-auth AND post-auth -- halving every ip
// limit, silently, and only on routes that have a post-auth chain.
func TestAnIPRuleIsChargedOnlyInThePreAuthStage(t *testing.T) {
	t.Parallel()

	rules := staticRules{known: true, rules: []domain.RateLimit{mwRule("ip", domain.RateLimitScopeIP)}}

	pre := newCountingLimiter(true)
	rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: pre, Resolver: newResolver(t), Stage: middleware.RateLimitStagePreAuth,
	}, rlRequest(http.MethodGet, "/models"))

	if got := len(pre.charges()); got != 1 {
		t.Fatalf("pre-auth charged %d buckets, want 1", got)
	}

	post := newCountingLimiter(true)
	rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: post, Resolver: newResolver(t), Stage: middleware.RateLimitStagePostAuth,
	}, rlRequest(http.MethodGet, "/models"))

	if got := len(post.charges()); got != 0 {
		t.Fatalf("post-auth charged %d buckets for an ip-scoped rule; it is charged pre-auth, "+
			"so charging it again halves every ip limit", got)
	}
}

// The mirror of the above: a user rule must not be charged pre-auth, where there
// are no claims to key it on.
func TestAUserRuleIsChargedOnlyInThePostAuthStage(t *testing.T) {
	t.Parallel()

	rules := staticRules{known: true, rules: []domain.RateLimit{mwRule("user", domain.RateLimitScopeUser)}}

	pre := newCountingLimiter(true)
	rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: pre, Resolver: newResolver(t), Stage: middleware.RateLimitStagePreAuth,
	}, rlRequest(http.MethodGet, "/models"))

	if got := len(pre.charges()); got != 0 {
		t.Fatalf("pre-auth charged %d buckets for a user-scoped rule; there are no claims there", got)
	}

	post := newCountingLimiter(true)

	r := rlRequest(http.MethodGet, "/models")
	r = r.WithContext(context.WithValue(r.Context(), middleware.JwtClaims, map[string]any{"sub": "alice"}))

	rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: post, Resolver: newResolver(t), Stage: middleware.RateLimitStagePostAuth,
	}, r)

	if got := post.charges(); len(got) != 1 {
		t.Fatalf("post-auth charged %d buckets, want 1", len(got))
	}
}

// A rule keyed on something the request does not carry must be SKIPPED, not
// bucketed under a placeholder -- that would be one shared budget wearing a
// per-user label.
func TestARuleIsSkippedWhenItsScopeKeyIsAbsent(t *testing.T) {
	t.Parallel()

	rules := staticRules{known: true, rules: []domain.RateLimit{mwRule("user", domain.RateLimitScopeUser)}}
	lim := newCountingLimiter(true)

	// Post-auth stage, but no claims -- the route did not authenticate.
	rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: lim, Resolver: newResolver(t), Stage: middleware.RateLimitStagePostAuth,
	}, rlRequest(http.MethodGet, "/models"))

	if got := lim.charges(); len(got) != 0 {
		t.Fatalf("charged %v; a user rule on an unauthenticated request must be skipped, "+
			"or every anonymous caller shares one bucket", got)
	}
}

func TestARefusalIs429WithRetryAfter(t *testing.T) {
	t.Parallel()

	rules := staticRules{known: true, rules: []domain.RateLimit{mwRule("ip", domain.RateLimitScopeIP)}}

	rec := rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: newCountingLimiter(false), Resolver: newResolver(t),
		Stage: middleware.RateLimitStagePreAuth,
	}, rlRequest(http.MethodGet, "/models"))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}

	if got := rec.Header().Get("Retry-After"); got == "" || got == "0" {
		t.Fatalf("Retry-After = %q; zero or absent tells a client to spin", got)
	}

	if got := rec.Header().Get("RateLimit-Limit"); got == "" {
		t.Fatal("RateLimit-Limit should be published so a client can pace itself")
	}
}

// A store fault must not read as an admitted request. Fail-closed refuses, with
// a DIFFERENT code -- "slow down" and "the rate limiter is broken" need
// different responses from whoever sees them.
func TestAStoreFaultFailsClosedWithItsOwnCode(t *testing.T) {
	t.Parallel()

	shared := newCountingLimiter(true)
	shared.err = errors.New("valkey is unreachable")

	rec := rlServe(t, middleware.RateLimitConfig{
		Rules:    staticRules{known: true, rules: []domain.RateLimit{mwRule("ip", domain.RateLimitScopeIP)}},
		Shared:   shared,
		Resolver: newResolver(t),
		Stage:    middleware.RateLimitStagePreAuth,
		FailMode: middleware.RateLimitFailClosed,
	}, rlRequest(http.MethodGet, "/models"))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 — an unknown budget is not an empty one", rec.Code)
	}

	if body := rec.Body.String(); !contains(body, "RATE_LIMIT_UNAVAILABLE") {
		t.Fatalf("a store fault must carry its own code, not the ordinary budget code: %s", body)
	}
}

// Fail-local is the deliberate alternative: bounded by the per-replica limiter
// rather than refusing outright.
func TestAStoreFaultInLocalModeFallsThrough(t *testing.T) {
	t.Parallel()

	shared := newCountingLimiter(true)
	shared.err = errors.New("valkey is unreachable")

	rec := rlServe(t, middleware.RateLimitConfig{
		Rules:    staticRules{known: true, rules: []domain.RateLimit{mwRule("ip", domain.RateLimitScopeIP)}},
		Shared:   shared,
		Local:    newCountingLimiter(true),
		Resolver: newResolver(t),
		Stage:    middleware.RateLimitStagePreAuth,
		FailMode: middleware.RateLimitFailLocal,
	}, rlRequest(http.MethodGet, "/models"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — fail-local falls back to the per-replica limiter", rec.Code)
	}
}

// An unloaded rule set charges NOTHING and says so loudly.
//
// It used to fall back to the flag budget, which was the floor. There is no
// floor now: budgets live in the rate_limits table and nowhere else, and the
// first load is synchronous and fatal at startup, so a serving replica always
// has rules. This path is therefore unreachable -- and the test exists to pin
// what happens if that invariant ever breaks.
//
// Charging nothing rather than refusing is deliberate. A can't-happen path that
// answers 429 turns an internal inconsistency into an outage, and the rule set
// being absent is not evidence that this particular caller is abusive.
func TestAnUnknownRuleSetChargesNothing(t *testing.T) {
	t.Parallel()

	lim := newCountingLimiter(true)

	rec := rlServe(t, middleware.RateLimitConfig{
		Rules: staticRules{known: false}, Local: lim, Resolver: newResolver(t),
		Stage: middleware.RateLimitStagePreAuth,
	}, rlRequest(http.MethodGet, "/models"))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: an unloaded rule set must not refuse the request", rec.Code)
	}

	if got := lim.charges(); len(got) != 0 {
		t.Fatalf("charged %v against an unloaded rule set; there is no fallback budget to charge", got)
	}
}

// A rule carrying several windows charges all of them, shortest first: spending
// a minute's budget on requests the second window would have refused drains the
// long window at a rate nobody configured.
func TestEveryWindowIsChargedShortestFirst(t *testing.T) {
	t.Parallel()

	rule := mwRule(
		"multi", domain.RateLimitScopeIP,
		domain.RateLimitWindow{ID: uuid.NewV7(), Requests: 1000, Period: time.Hour},
		domain.RateLimitWindow{ID: uuid.NewV7(), Requests: 10, Period: time.Second},
		domain.RateLimitWindow{ID: uuid.NewV7(), Requests: 300, Period: time.Minute},
	)

	lim := newCountingLimiter(true)

	rlServe(t, middleware.RateLimitConfig{
		Rules: staticRules{known: true, rules: []domain.RateLimit{rule}},
		Local: lim, Resolver: newResolver(t), Stage: middleware.RateLimitStagePreAuth,
	}, rlRequest(http.MethodGet, "/models"))

	charges := lim.charges()
	if len(charges) != 3 {
		t.Fatalf("charged %d buckets, want 3 — every window applies", len(charges))
	}

	sorted := rule.SortedWindows()
	for i, w := range sorted {
		// The window's PARAMETERS, not its id: a bucket keyed on the id was
		// reset by every edit, because PUT remints the window set.
		want := rule.ID.String() + ":" + w.Period.String() + ":" +
			strconv.Itoa(w.Requests) + ":" + strconv.Itoa(w.Burst) + ":ip:203.0.113.9"
		if charges[i] != want {
			t.Fatalf("charge %d was %q, want %q — windows must be charged shortest period first", i, charges[i], want)
		}
	}
}

// Two rules must never share a bucket, or editing one reaches into the other's
// budget.
//
// The rules here have the SAME scope and differ only in which route they target
// -- written with two different scopes, as this test was at first, the scope
// value alone keeps the keys apart and the test asserts nothing.
//
// What actually guarantees the separation is the WINDOW id: a v7 uuid belonging
// to exactly one rule. The rule id in the key is legibility, not mechanism, and
// removing it leaves this test passing -- verified.
func TestTwoRulesOnTheSameScopeGetDifferentBuckets(t *testing.T) {
	t.Parallel()

	models := mwRule("models", domain.RateLimitScopeIP)
	models.TargetKind = domain.RateLimitTargetKindEndpoint
	models.Target = "/models"

	projects := mwRule("projects", domain.RateLimitScopeIP)
	projects.TargetKind = domain.RateLimitTargetKindEndpoint
	projects.Target = "/projects"

	rules := staticRules{known: true, rules: []domain.RateLimit{models, projects}}
	lim := newCountingLimiter(true)

	conf := middleware.RateLimitConfig{
		Rules: rules, Local: lim, Resolver: newResolver(t), Stage: middleware.RateLimitStagePreAuth,
	}

	// Same client, same scope value, two different routes.
	rlServe(t, conf, rlRequest(http.MethodGet, "/models"))
	rlServe(t, conf, rlRequest(http.MethodGet, "/projects"))

	charges := lim.charges()
	if len(charges) != 2 {
		t.Fatalf("charged %d buckets, want 2", len(charges))
	}

	if charges[0] == charges[1] {
		t.Fatalf("two ip-scoped rules on different routes shared the bucket %q; "+
			"the same client would spend one budget across both, and editing one rule "+
			"would reach into the other", charges[0])
	}
}

// Two scopes both apply to one request, and each gets its own bucket.
func TestDifferentScopesGetDifferentBuckets(t *testing.T) {
	t.Parallel()

	a := mwRule("a", domain.RateLimitScopeIP)
	b := mwRule("b", domain.RateLimitScopeGlobal)

	lim := newCountingLimiter(true)

	rlServe(t, middleware.RateLimitConfig{
		Rules: staticRules{known: true, rules: []domain.RateLimit{a, b}},
		Local: lim, Resolver: newResolver(t), Stage: middleware.RateLimitStagePreAuth,
	}, rlRequest(http.MethodGet, "/models"))

	charges := lim.charges()
	if len(charges) != 2 {
		t.Fatalf("charged %d, want 2 (one per scope)", len(charges))
	}

	if charges[0] == charges[1] {
		t.Fatalf("both scopes charged the same bucket %q", charges[0])
	}
}

// The per-replica limiter runs FIRST and its refusal is final -- that is what
// smooths the shared counter's fixed window. If the shared store were consulted
// on a locally-refused request it would spend budget for a request nobody
// served.
func TestALocalRefusalDoesNotReachTheSharedStore(t *testing.T) {
	t.Parallel()

	shared := newCountingLimiter(true)

	rec := rlServe(t, middleware.RateLimitConfig{
		Rules:    staticRules{known: true, rules: []domain.RateLimit{mwRule("ip", domain.RateLimitScopeIP)}},
		Local:    newCountingLimiter(false),
		Shared:   shared,
		Resolver: newResolver(t),
		Stage:    middleware.RateLimitStagePreAuth,
	}, rlRequest(http.MethodGet, "/models"))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}

	if got := shared.charges(); len(got) != 0 {
		t.Fatalf("the shared store was consulted %d times after a local refusal; "+
			"that spends a shared budget on a request nobody served", len(got))
	}
}

func TestRemainingIsOnlyPublishedWhenTheLimiterCanSayIt(t *testing.T) {
	t.Parallel()

	rules := staticRules{known: true, rules: []domain.RateLimit{mwRule("ip", domain.RateLimitScopeIP)}}

	// countingLimiter reports 7.
	rec := rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: newCountingLimiter(true), Resolver: newResolver(t),
		Stage: middleware.RateLimitStagePreAuth,
	}, rlRequest(http.MethodGet, "/models"))

	if got := rec.Header().Get("RateLimit-Remaining"); got != "7" {
		t.Fatalf("RateLimit-Remaining = %q, want 7", got)
	}

	// A limiter that cannot say reports -1, and -1 must not be published: a
	// client would read it as a number.
	unknown := &unknownRemaining{}

	rec = rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: unknown, Resolver: newResolver(t),
		Stage: middleware.RateLimitStagePreAuth,
	}, rlRequest(http.MethodGet, "/models"))

	if got := rec.Header().Get("RateLimit-Remaining"); got != "" {
		t.Fatalf("RateLimit-Remaining = %q; -1 means 'cannot say' and must not be published", got)
	}
}

type unknownRemaining struct{}

func (unknownRemaining) Allow(context.Context, string, ratelimit.Budget, int) (ratelimit.Decision, error) {
	return ratelimit.Decision{Allowed: true, Remaining: -1}, nil
}

func (unknownRemaining) Close() error { return nil }

// A ServeMux pattern carries the method and may carry a host. A rule targets a
// path, so without normalisation every endpoint rule would fail to match.
// The regression that unit tests missed and a live run found.
//
// The API mux is mounted on an outer router as a subtree, so r.Pattern is
// already set to the MOUNT POINT ("/api/v1/") when the pre-auth stage runs.
// Trusting it there makes every request look like the mount, so no endpoint or
// prefix rule can ever match and the global rule silently wins. Measured
// against the running service before the fix: a 5/minute rule on /models had no
// effect, and the response carried the global rule's headers.
func TestPreAuthIgnoresAnOuterMuxPattern(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.Handle("GET /models", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	tight := mwRule("models tight", domain.RateLimitScopeIP,
		domain.RateLimitWindow{ID: uuid.NewV7(), Requests: 5, Period: time.Minute, Burst: 5})
	tight.TargetKind = domain.RateLimitTargetKindEndpoint
	tight.Target = "/models"
	tight.Methods = []string{"GET"}

	global := mwRule("global", domain.RateLimitScopeIP)

	lim := newCountingLimiter(true)

	r := rlRequest(http.MethodGet, "/models")
	// What the outer router leaves behind.
	r.Pattern = "/api/v1/"

	rlServe(t, middleware.RateLimitConfig{
		Rules: staticRules{known: true, rules: []domain.RateLimit{global, tight}},
		Local: lim, Resolver: newResolver(t), Router: mux,
		Stage: middleware.RateLimitStagePreAuth,
	}, r)

	charges := lim.charges()
	if len(charges) != 1 {
		t.Fatalf("charged %d buckets, want 1", len(charges))
	}

	if !strings.HasPrefix(charges[0], tight.ID.String()) {
		t.Fatalf("the endpoint rule did not win: charged %q, want the bucket of %q. "+
			"The pre-auth stage must ask the inner mux rather than trust r.Pattern, "+
			"which the outer subtree mount has already set to \"/api/v1/\"", charges[0], tight.Name)
	}
}

func TestPatternNormalisationStripsTheMethod(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.Handle("POST /projects/{project_id}/generate", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	rule := mwRule("generate", domain.RateLimitScopeIP)
	rule.TargetKind = domain.RateLimitTargetKindEndpoint
	rule.Target = "/projects/{project_id}/generate"
	rule.Methods = []string{"POST"}

	lim := newCountingLimiter(true)

	rlServe(t, middleware.RateLimitConfig{
		Rules: staticRules{known: true, rules: []domain.RateLimit{rule}},
		Local: lim, Resolver: newResolver(t), Router: mux,
		Stage: middleware.RateLimitStagePreAuth,
	}, rlRequest(http.MethodPost, "/projects/01a03000-0000-7000-8000-000000000001/generate"))

	if got := lim.charges(); len(got) != 1 {
		t.Fatalf("charged %d buckets; the mux pattern is \"POST /projects/{project_id}/generate\" "+
			"and a rule targets the path, so without stripping the method nothing matches", len(got))
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(haystack) > 0 && indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}

	return -1
}

// The metrics have to record, and record the labels that make the two real
// questions answerable: which rule is refusing, and did switching a strategy
// change anything. A metric registered but never incremented is worse than
// none -- a flat line reads as "nothing is happening".
func TestTheLimiterRecordsItsDecisions(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test")

	m, err := middleware.NewRateLimitMetrics(meter, "")
	if err != nil {
		t.Fatalf("NewRateLimitMetrics: %v", err)
	}

	rules := staticRules{known: true, rules: []domain.RateLimit{mwRule("ip rule", domain.RateLimitScopeIP)}}

	// One allowed, then one refused.
	rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: newCountingLimiter(true), Resolver: newResolver(t),
		Stage: middleware.RateLimitStagePreAuth, Metrics: m,
	}, rlRequest(http.MethodGet, "/models"))

	rlServe(t, middleware.RateLimitConfig{
		Rules: rules, Local: newCountingLimiter(false), Resolver: newResolver(t),
		Stage: middleware.RateLimitStagePreAuth, Metrics: m,
	}, rlRequest(http.MethodGet, "/models"))

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	decisions := findSum(t, &rm, "rate_limit_decisions_total")
	if len(decisions) != 2 {
		t.Fatalf("expected an allowed and a refused series, got %d", len(decisions))
	}

	seen := map[string]int64{}

	for _, dp := range decisions {
		outcome, _ := dp.Attributes.Value(attribute.Key("decision"))
		rule, _ := dp.Attributes.Value(attribute.Key("rule"))
		strategy, _ := dp.Attributes.Value(attribute.Key("strategy"))

		if rule.AsString() != "ip rule" {
			t.Fatalf("the rule label should carry the rule NAME an operator wrote, got %q", rule.AsString())
		}

		if strategy.AsString() != "token_bucket" {
			t.Fatalf("the strategy label is what makes 'did switching this change anything' answerable, got %q", strategy.AsString())
		}

		seen[outcome.AsString()] = dp.Value
	}

	if seen["allowed"] != 1 || seen["refused"] != 1 {
		t.Fatalf("expected one allowed and one refused, got %v", seen)
	}
}

// store_up is a GAUGE and the thing to alert on. With fail-local a sustained
// fault silently enforces N x the limit, and a fault RATE that is high but
// steady looks like a plateau rather than an outage.
func TestAStoreFaultDropsTheStoreUpGauge(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test")

	m, err := middleware.NewRateLimitMetrics(meter, "")
	if err != nil {
		t.Fatalf("NewRateLimitMetrics: %v", err)
	}

	up := func() int64 {
		t.Helper()

		var rm metricdata.ResourceMetrics
		if err := reader.Collect(t.Context(), &rm); err != nil {
			t.Fatalf("collect: %v", err)
		}

		pts := findGauge(t, &rm, "rate_limit_store_up")
		if len(pts) != 1 {
			t.Fatalf("expected one store_up series, got %d", len(pts))
		}

		return pts[0].Value
	}

	if got := up(); got != 1 {
		t.Fatalf("store_up starts at %d, want 1 — starting at 0 would fire an alert on every boot", got)
	}

	shared := newCountingLimiter(true)
	shared.err = errors.New("valkey is unreachable")

	rlServe(t, middleware.RateLimitConfig{
		Rules:  staticRules{known: true, rules: []domain.RateLimit{mwRule("ip rule", domain.RateLimitScopeIP)}},
		Shared: shared, Resolver: newResolver(t),
		Stage: middleware.RateLimitStagePreAuth, Metrics: m,
	}, rlRequest(http.MethodGet, "/models"))

	if got := up(); got != 0 {
		t.Fatalf("store_up = %d after a fault, want 0", got)
	}
}

// A nil Metrics must not panic. The middleware has to be constructible in a
// test without standing up a meter, or the metric is one nobody has watched
// work.
func TestNilMetricsDoesNotPanic(t *testing.T) {
	t.Parallel()

	rlServe(t, middleware.RateLimitConfig{
		Rules: staticRules{known: true, rules: []domain.RateLimit{mwRule("ip", domain.RateLimitScopeIP)}},
		Local: newCountingLimiter(true), Resolver: newResolver(t),
		Stage: middleware.RateLimitStagePreAuth, Metrics: nil,
	}, rlRequest(http.MethodGet, "/models"))
}

func findSum(t *testing.T, rm *metricdata.ResourceMetrics, name string) []metricdata.DataPoint[int64] {
	t.Helper()

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}

			if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
				return sum.DataPoints
			}
		}
	}

	t.Fatalf("metric %q was not recorded at all", name)

	return nil
}

func findGauge(t *testing.T, rm *metricdata.ResourceMetrics, name string) []metricdata.DataPoint[int64] {
	t.Helper()

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}

			if g, ok := m.Data.(metricdata.Gauge[int64]); ok {
				return g.DataPoints
			}
		}
	}

	t.Fatalf("metric %q was not recorded at all", name)

	return nil
}
