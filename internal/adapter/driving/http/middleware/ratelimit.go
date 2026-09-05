package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/ratelimit"
)

// RateLimitStage selects which half of the request lifecycle a limiter runs in.
//
// The split is forced by where each piece of information exists:
//
//   - PRE-AUTH runs in the common chain, which wraps the mux, so r.Pattern is
//     NOT yet set and no claims exist. It resolves the route itself and can only
//     enforce ip- and global-scoped rules -- but it runs before any
//     authentication work, which is what makes it useful against a flood.
//   - POST-AUTH runs inside a route's chain, after the mux matched and after the
//     token was verified. r.Pattern and the claims are both available, so it
//     enforces user-, token- and project-scoped rules.
//
// Running everything post-auth would put JWT verification and an authz lookup in
// front of the limiter -- so the work a flood costs would be done before the
// limiter could refuse it, which is the opposite of the point.
type RateLimitStage int

const (
	RateLimitStagePreAuth RateLimitStage = iota
	RateLimitStagePostAuth
)

// RateLimitFailMode is what happens when the shared store cannot answer.
type RateLimitFailMode int

const (
	// RateLimitFailClosed refuses the request. The default, and the safe
	// reading: an unknown budget is not an empty one.
	RateLimitFailClosed RateLimitFailMode = iota

	// RateLimitFailLocal falls back to the per-replica limiter. Bounded, but N
	// replicas means N x the configured rate -- so it is a deliberate choice an
	// operator makes, never a silent default.
	RateLimitFailLocal
)

// RateLimitRuleResolver is the half of the mirror this middleware needs.
//
// The bool is NOT redundant with an empty slice. false means "the rule set is
// not known" -- nothing has ever loaded -- and the caller must fall back to the
// nothing at all. An empty slice with true means "known, and no rule
// applies", which is a different thing entirely.
type RateLimitRuleResolver interface {
	Resolve(req domain.RateLimitRequest) ([]domain.RateLimitMatch, bool)
}

// RateLimitConfig configures [RateLimit].
type RateLimitConfig struct {
	Rules    RateLimitRuleResolver
	Shared   ratelimit.Limiter
	Local    ratelimit.Limiter
	Resolver *ClientIPResolver

	// Router resolves a request to its route pattern in the pre-auth stage,
	// where r.Pattern is not yet set. Ignored post-auth.
	Router *http.ServeMux

	// Metrics is optional; nil disables recording. Optional because the
	// middleware has to be constructible in a test without standing up a meter,
	// and a metric that only exists in production is one nobody has watched
	// work.
	Metrics *RateLimitMetrics

	Stage    RateLimitStage
	FailMode RateLimitFailMode
}

// RateLimitMetrics records what the limiter did.
//
// The labels are chosen so the two questions an operator actually has are
// answerable without a deploy: "which rule is refusing traffic?" and "did
// switching that rule's strategy change anything?". rule and strategy are
// therefore labels, not just log fields.
//
// rule is the rule NAME, not its id. A name is what an operator wrote and what
// they will search for; an id sends them to the database first. Names are
// unique (unique_rate_limit_name), so cardinality is bounded by the number of
// rules, which is tens.
type RateLimitMetrics struct {
	// Decisions counts allow/deny per rule, scope and strategy.
	Decisions metric.Int64Counter

	// StoreFaults counts times the shared counter could not answer. Separate
	// from a denial: one means "slow down", the other means "nobody is being
	// limited correctly", and they need different responses.
	StoreFaults metric.Int64Counter

	// RuleFaults counts rules that could not be enforced, by name.
	//
	// A SEPARATE instrument from StoreFaults, because the two have different
	// owners and different fixes: a store fault is an infrastructure incident,
	// a rule fault is one malformed row that an operator corrects with an API
	// call. Counting them together sends every rule fault to whoever is on call
	// for Valkey, and buries it there.
	//
	// Any non-zero value is actionable -- the rule named is enforcing nothing.
	RuleFaults metric.Int64Counter

	// storeUp is 1 while the shared counter last answered, 0 after a fault.
	//
	// A GAUGE, and alerting on it rather than on the fault rate is the point:
	// with fail-local a sustained fault silently enforces N x the limit, and a
	// rate that is high-but-steady looks like a plateau rather than an outage.
	storeUp atomic.Int64
}

// NewRateLimitMetrics registers the limiter's instruments.
func NewRateLimitMetrics(meter metric.Meter, prefix string) (*RateLimitMetrics, error) {
	if prefix != "" {
		prefix += "_"
	}

	ref := &RateLimitMetrics{}
	ref.storeUp.Store(1)

	var err error

	ref.Decisions, err = meter.Int64Counter(
		prefix+"rate_limit_decisions_total",
		metric.WithDescription("Rate-limit decisions, by rule, scope, strategy and outcome"),
	)
	if err != nil {
		return nil, err
	}

	ref.StoreFaults, err = meter.Int64Counter(
		prefix+"rate_limit_store_faults_total",
		metric.WithDescription("Times the shared rate-limit counter could not answer"),
	)
	if err != nil {
		return nil, err
	}

	ref.RuleFaults, err = meter.Int64Counter(
		prefix+"rate_limit_rule_faults_total",
		metric.WithDescription("Times a rate-limit rule could not be enforced because the rule itself is malformed"),
	)
	if err != nil {
		return nil, err
	}

	if _, err := meter.Int64ObservableGauge(
		prefix+"rate_limit_store_up",
		metric.WithDescription("1 while the shared rate-limit counter is answering, 0 after a fault"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(ref.storeUp.Load())

			return nil
		}),
	); err != nil {
		return nil, err
	}

	return ref, nil
}

func (m *RateLimitMetrics) recordDecision(ctx context.Context, b bucket, allowed bool) {
	if m == nil {
		return
	}

	outcome := "allowed"
	if !allowed {
		outcome = "refused"
	}

	m.Decisions.Add(ctx, 1, metric.WithAttributes(
		attribute.String("rule", b.rule),
		attribute.String("scope", b.scope),
		attribute.String("strategy", b.strategy),
		attribute.String("decision", outcome),
	))
}

func (m *RateLimitMetrics) recordStoreFault(ctx context.Context, b bucket, failMode string) {
	if m == nil {
		return
	}

	m.storeUp.Store(0)
	m.StoreFaults.Add(ctx, 1, metric.WithAttributes(
		attribute.String("rule", b.rule),
		attribute.String("scope", b.scope),
		attribute.String("fail_mode", failMode),
	))
}

// recordRuleFault counts a rule that could not be enforced.
//
// It deliberately does NOT touch storeUp: the shared counter is not implicated
// by a rule this replica could not build a limiter from, and reporting it as
// down sends the alert to the wrong team about the wrong subsystem.
func (m *RateLimitMetrics) recordRuleFault(ctx context.Context, b bucket) {
	if m == nil {
		return
	}

	m.RuleFaults.Add(ctx, 1, metric.WithAttributes(
		attribute.String("rule", b.rule),
		attribute.String("scope", b.scope),
		attribute.String("strategy", b.strategy),
	))
}

func (m *RateLimitMetrics) recordStoreOK() {
	if m == nil {
		return
	}

	m.storeUp.Store(1)
}

// StoreUp reports whether the shared counter answered the last time it was
// asked. It is the same value as the rate_limit_store_up gauge.
//
// Exposed so health can report the store's state without probing it a second
// time. A separate ping would be a different question -- "can a fresh
// connection reach Valkey" rather than "is the limiter able to count" -- and
// the two are free to disagree at exactly the moment the answer matters, with
// the breaker open being the obvious case.
//
// A limiter that has not yet been asked anything reports up, matching the
// gauge's own initial value: nothing has failed.
func (m *RateLimitMetrics) StoreUp() bool {
	if m == nil {
		return true
	}

	return m.storeUp.Load() == 1
}

// rateLimitCode is the response code for a budget refusal.
const rateLimitCode = "RATE_LIMIT_EXCEEDED"

// rateLimitStoreFaultCode is the code for a refusal caused by the STORE, not by
// a spent budget.
//
// A separate code because the two need different responses from whoever sees
// them: one means "slow down", the other means "the rate limiter is broken and
// nobody is being limited correctly". A single code makes the second invisible
// inside the noise of the first.
const rateLimitStoreFaultCode = "RATE_LIMIT_UNAVAILABLE"

// storeFaultRetryAfter bounds the Retry-After on a store-fault refusal.
//
// It is short and fixed. A fault has no budget to derive a wait from, and
// echoing a rule's window would tell a caller to wait a minute for a condition
// that usually clears in seconds.
const storeFaultRetryAfter = 5 * time.Second

// RateLimit enforces the mirrored rules.
func RateLimit(conf RateLimitConfig) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Exemptions are NOT checked here. They live in
			// [RateLimitExemptions], ahead of the choice between this limiter
			// and IPRateLimiter, because they used to live here and therefore
			// did not exist in the posture that ships by default -- where
			// /health answered 429 and ratelimit.excluded.ips did nothing.
			ip := conf.Resolver.ClientIP(r)

			pattern := conf.pattern(r)
			if pattern == "" {
				// No route matched. The handler will answer 404; limiting it
				// against a rule that cannot exist would be arbitrary, and the
				// ip-scoped global rule below still covers the flood case.
				pattern = r.URL.Path
			}

			claims, authenticated := r.Context().Value(JwtClaims).(map[string]any)

			matches, known := conf.Rules.Resolve(domain.RateLimitRequest{
				Method:        r.Method,
				Pattern:       pattern,
				Authenticated: authenticated,
			})

			budgets := conf.budgetsFor(matches, known, r, claims, ip)
			if len(budgets) == 0 {
				next.ServeHTTP(w, r)

				return
			}

			if !conf.charge(w, r, budgets) {
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ruleFaultError marks an error as coming from the per-replica limiter rather
// than from the shared counter.
//
// The distinction is the whole point. Both surface as "could not charge this
// bucket", and treating them alike made a malformed rule report itself as a
// Valkey outage: rate_limit_store_up went to zero, RateLimitStoreDown fired,
// and under fail-closed every request the rule matched was refused with
// RATE_LIMIT_UNAVAILABLE. Nothing anywhere named the rule.
type ruleFaultError struct {
	err  error
	key  string
	rule string
}

func (e *ruleFaultError) Error() string {
	return "rate limit rule " + e.rule + " (bucket " + e.key + ") cannot be enforced: " + e.err.Error()
}

func (e *ruleFaultError) Unwrap() error { return e.err }

// bucket is one budget to charge, with the key it is charged against.
type bucket struct {
	key      string
	rule     string
	scope    string
	strategy string
	budget   ratelimit.Budget
}

// charge spends one request against every budget, SHORTEST WINDOW FIRST, and
// writes the refusal if any of them says no.
//
// Order is load-bearing. A rule carrying 10/s and 300/min must consult the
// second window first: spending the minute's budget on requests the second
// would have refused drains the long window at a rate nobody configured, and the
// boundary eventually reported is the wrong one.
func (conf RateLimitConfig) charge(w http.ResponseWriter, r *http.Request, buckets []bucket) bool {
	for _, b := range buckets {
		decision, err := conf.spend(r.Context(), b)
		if err != nil {
			if fault, ok := errors.AsType[*ruleFaultError](err); ok {
				conf.Metrics.recordRuleFault(r.Context(), b)

				// A broken RULE, not a broken store. Three things follow, and
				// each was wrong before:
				//
				// rate_limit_store_up is NOT touched. Driving it to zero pages
				// whoever is on call for Valkey, about a Valkey that is fine,
				// and hides a real store outage behind a bad row.
				//
				// The fail mode is NOT consulted. It answers "what should an
				// unreachable counter mean", and this counter is reachable.
				// Under fail-closed it would have turned one malformed row into
				// a 429 for every request the rule matched -- an outage of the
				// endpoint the operator was trying to protect.
				//
				// The bucket is skipped, so the request falls through to the
				// other rules that matched and to the flag floor. That is the
				// same answer the mirror gives a rule it cannot enforce; this
				// path exists for the window between a bad write and the reload
				// that drops it.
				slog.Error(
					"rate-limit rule cannot be enforced and is being skipped for this request",
					"error", fault.err,
					"rule", fault.rule,
					"scope", b.scope,
					"strategy", b.strategy,
					"what", "the per-replica limiter could not be built from this rule; the shared store is NOT implicated",
					"consequence", "this rule is enforcing nothing until it is corrected or the mirror reloads",
				)

				continue
			}

			conf.Metrics.recordStoreFault(r.Context(), b, conf.failModeName())

			// The store could not answer. NOT "allowed": an unknown budget is
			// not an empty one, and reporting it as allowed removes the limit
			// exactly when the system is least healthy.
			slog.Warn(
				"rate limit store fault",
				"error", err,
				"rule", b.rule,
				"scope", b.scope,
				"fail_mode", conf.failModeName(),
				"what", "the shared counter could not answer; this request is not being limited by the configured rule",
			)

			if conf.FailMode == RateLimitFailLocal {
				continue
			}

			w.Header().Set("Retry-After", strconv.Itoa(secondsCeil(storeFaultRetryAfter)))
			respond.WriteJSONMessageWithCode(w, r, http.StatusTooManyRequests, rateLimitStoreFaultCode,
				"rate limiting is temporarily unavailable, please retry shortly")

			return false
		}

		conf.Metrics.recordStoreOK()
		conf.Metrics.recordDecision(r.Context(), b, decision.Allowed)

		writeRateLimitHeaders(w, b, decision)

		if !decision.Allowed {
			retryAfter := max(secondsCeil(decision.RetryAfter), 1)
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			respond.WriteJSONMessageWithCode(w, r, http.StatusTooManyRequests, rateLimitCode, "too many requests")

			return false
		}
	}

	return true
}

// spend charges one request, using the shared store when there is one and
// falling back to the per-replica limiter when the store faults in local mode.
func (conf RateLimitConfig) spend(ctx context.Context, b bucket) (ratelimit.Decision, error) {
	// The per-replica limiter runs FIRST, always, and its refusal is final.
	//
	// This is what smooths the shared counter's fixed window: the counter admits
	// up to 2N across a boundary, and the token bucket in front of it does not.
	// Two layers, and each covers the other's weakness -- removing either makes
	// the survivor's trade worse.
	local := ratelimit.Decision{Allowed: true, Remaining: -1}

	if conf.Local != nil {
		d, err := conf.Local.Allow(ctx, b.key, b.budget, 1)
		if err != nil {
			// Wrapped so the caller can tell this apart from the shared store
			// failing. The per-replica limiter does no I/O -- the only way it
			// refuses to answer is a rule it cannot build a limiter from -- so
			// reporting it as a store fault names the wrong subsystem, and the
			// two want opposite responses.
			return ratelimit.Decision{}, &ruleFaultError{key: b.key, rule: b.rule, err: err}
		}

		if !d.Allowed {
			return d, nil
		}

		local = d
	}

	// With no shared store -- cache.enabled=false, a supported deployment --
	// the local decision IS the decision. Returning a synthesised one instead
	// discards whatever the local limiter reported, so RateLimit-Remaining
	// would read as "cannot say" on every request in that deployment.
	if conf.Shared == nil {
		return local, nil
	}

	return conf.Shared.Allow(ctx, b.key, b.budget, 1)
}

// budgetsFor turns the matched rules into the buckets to charge.
func (conf RateLimitConfig) budgetsFor(
	matches []domain.RateLimitMatch,
	known bool,
	r *http.Request,
	claims map[string]any,
	ip string,
) []bucket {
	if !known {
		// The rule set has never loaded, and there is no fallback budget to
		// apply -- budgets live in the database now, and the first load is
		// synchronous and fatal at startup, so a serving replica always has
		// rules. Reaching here means that invariant broke.
		//
		// It is logged rather than refused: a can't-happen path that answers
		// 429 turns an internal inconsistency into an outage, and the rule set
		// being absent is not evidence that this particular caller is abusive.
		slog.Error(
			"rate-limit rule set is not loaded; this request is not being limited",
			"what", "the mirror reports no known rule set on a serving replica",
			"why", "the first load is fatal at startup, so this should be unreachable",
			"consequence", "requests are not being limited until a reload succeeds",
		)

		return nil
	}

	buckets := make([]bucket, 0, len(matches))

	for _, m := range matches {
		if !conf.appliesInThisStage(m.Rule) {
			continue
		}

		scopeValue, ok := scopeKey(m.Rule.Scope, r, claims, ip)
		if !ok {
			// The rule is keyed on something this request does not carry -- a
			// user rule on an unauthenticated request, a project rule off a
			// project route. Skipping is right: bucketing them all under a
			// placeholder would be one shared budget wearing a per-user label.
			continue
		}

		for _, wdw := range m.Rule.SortedWindows() {
			// The window's PARAMETERS distinguish the bucket, not its id.
			//
			// This used to be the window id, and that silently reset every
			// budget on every edit: PUT replaces a rule's window set wholesale,
			// minting fresh uuids, so renaming a rule -- or changing its
			// description -- gave every caller their full allowance back.
			// Measured against the running service: spend 4 of 6, edit the
			// DESCRIPTION, and the next 4 requests were all admitted.
			//
			// That is the trap PocketBase falls into, arriving by a different
			// route. The adapter's own guard could not catch it, because that
			// guard keeps a bucket alive for an unchanged budget under the same
			// KEY -- and the key was what changed.
			//
			// Keying on the parameters makes the documented intent true: a
			// bucket is rebuilt when, and only when, a rule's numbers actually
			// change. The rule id stays for legibility, so a key in a log or in
			// Valkey names its rule without a second lookup.
			buckets = append(buckets, bucket{
				key:      m.Rule.ID.String() + ":" + windowKey(wdw) + ":" + scopeValue,
				rule:     m.Rule.Name,
				scope:    string(m.Rule.Scope),
				strategy: string(m.Rule.Strategy),
				budget: ratelimit.Budget{
					Requests: wdw.Requests,
					Period:   wdw.Period,
					Burst:    wdw.Burst,
					Strategy: string(m.Rule.Strategy),
				},
			})
		}
	}

	return buckets
}

// appliesInThisStage keeps a rule from being charged twice, and keeps an
// auth-audience rule from being charged never.
//
// Both stages see every matched rule, so without the scope half an ip-scoped
// rule would be charged pre-auth AND post-auth -- halving every ip limit,
// silently, and only on routes that have a post-auth chain.
//
// # The audience half, which was missing
//
// Scope alone sent every ip and global rule to the pre-auth stage. But the
// pre-auth stage runs before CheckAccessToken, so nobody is authenticated there
// and MatchesAudience rejects every auth rule. Post-auth the audience matched
// and the SCOPE was rejected instead. The two halves never met: an ip or global
// rule with audience=auth was accepted by the API, listed in the UI, resolved
// by the ladder -- and enforced in neither stage.
//
// So an auth rule is charged post-auth whatever its scope, which is what
// "auth rules are only ever evaluated post-auth, where identity exists" meant.
// That reintroduces no double-charging, because the pre-auth stage cannot match
// an auth rule in the first place: guest and any rules still route by scope,
// and an ip+any rule is charged pre-auth exactly as before.
//
// ip and global are still usable post-auth because the client address is
// resolved in both stages -- an ip+auth rule buckets authenticated callers by
// address, which is a limit an operator can reasonably want and previously
// could write but not get.
func (conf RateLimitConfig) appliesInThisStage(rule *domain.RateLimit) bool {
	if rule.Audience == domain.RateLimitAudienceAuth {
		return conf.Stage == RateLimitStagePostAuth
	}

	if conf.Stage == RateLimitStagePreAuth {
		return rule.Scope.DecidedPreAuth()
	}

	return !rule.Scope.DecidedPreAuth()
}

// pattern resolves the route template for this request.
//
// The branch is on the STAGE, not on whether r.Pattern happens to be set, and
// that distinction is load-bearing.
//
// The API mux is mounted on an outer router as a subtree ("/api/v1/"), so by the
// time the pre-auth stage runs r.Pattern is ALREADY set -- to the mount point,
// not to the route. Trusting it there makes every request look like "/api/v1/",
// so no endpoint or prefix rule can ever match and the global rule silently
// wins every time. Measured: a 5/minute rule on /models had no effect and the
// response carried the global rule's headers.
func (conf RateLimitConfig) pattern(r *http.Request) string {
	if conf.Stage == RateLimitStagePostAuth {
		// The inner mux has matched by now, so this is the route template.
		return normalisePattern(r.Pattern)
	}

	if conf.Router == nil {
		return ""
	}

	// Pre-auth: ask the inner mux. Handler reports the pattern without invoking
	// anything.
	_, p := conf.Router.Handler(r)

	return normalisePattern(p)
}

// normalisePattern strips the method and host that ServeMux includes in a
// pattern, leaving the path template a rule targets.
//
// "POST /projects/{project_id}/generate" -> "/projects/{project_id}/generate".
// Without this every endpoint rule would fail to match, because a rule's target
// is a path and the mux's pattern is not.
func normalisePattern(p string) string {
	if p == "" {
		return ""
	}

	if i := strings.LastIndex(p, " "); i >= 0 {
		p = p[i+1:]
	}

	// A pattern may carry a host: "example.com/models". A rule targets a path.
	if i := strings.Index(p, "/"); i > 0 {
		p = p[i:]
	}

	// ServeMux writes a subtree pattern as a trailing "/..."; a rule's prefix
	// target is written without it.
	p = strings.TrimSuffix(p, "{$}")

	return p
}

// scopeKey derives the value a bucket is keyed on, or false when the request
// does not carry it.
func scopeKey(scope domain.RateLimitScope, r *http.Request, claims map[string]any, ip string) (string, bool) {
	switch scope {
	case domain.RateLimitScopeIP:
		return "ip:" + ip, ip != ""

	case domain.RateLimitScopeGlobal:
		// One bucket for the whole deployment. Not per replica: the shared
		// counter is what makes this meaningful, and with cache.enabled=false a
		// global rule is per replica and the docs say so.
		return "global", true

	case domain.RateLimitScopeUser:
		sub, ok := claims["sub"].(string)

		return "user:" + sub, ok && sub != ""

	case domain.RateLimitScopeToken:
		jti, ok := claims["jti"].(string)

		return "token:" + jti, ok && jti != ""

	case domain.RateLimitScopeProject:
		id := r.PathValue("project_id")

		return "project:" + id, id != ""

	default:
		return "", false
	}
}

// writeRateLimitHeaders publishes the budget so a client can pace itself instead
// of discovering the limit by hitting it.
// windowKey renders the part of a bucket key that must change when, and only
// when, the budget does.
//
// Period first because it is what makes two windows of one rule distinct -- a
// rule may not carry two windows with the same period, so the period alone
// separates them. Requests and burst follow so that changing either rebuilds
// the bucket, which is the half of "rebuilt only on a real parameter change"
// that stops an operator tightening a limit and having it apply from a full
// bucket.
func windowKey(w domain.RateLimitWindow) string {
	return w.Period.String() + ":" + strconv.Itoa(w.Requests) + ":" + strconv.Itoa(w.Burst)
}

func writeRateLimitHeaders(w http.ResponseWriter, b bucket, d ratelimit.Decision) {
	limit := b.budget.Burst
	if limit <= 0 {
		limit = b.budget.Requests
	}

	w.Header().Set("RateLimit-Limit", strconv.Itoa(limit))

	// Remaining is only written when the limiter can actually say. The
	// in-process one reports -1, and publishing that as a header would be a
	// number clients would believe.
	if d.Remaining >= 0 {
		w.Header().Set("RateLimit-Remaining", strconv.Itoa(d.Remaining))
	}

	if d.RetryAfter > 0 {
		w.Header().Set("RateLimit-Reset", strconv.Itoa(secondsCeil(d.RetryAfter)))
	}
}

func (conf RateLimitConfig) failModeName() string {
	if conf.FailMode == RateLimitFailLocal {
		return "local"
	}

	return "closed"
}
