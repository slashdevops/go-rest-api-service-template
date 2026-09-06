package usecase

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// RateLimitRulesConfig configures [RateLimitRules].
type RateLimitRulesConfig struct {
	Repository repository.RateLimits
	OT         *o11y.OpenTelemetry

	MetricsPrefix string

	// ReloadInterval is how often the rule set is rebuilt, and therefore how
	// stale a rule written on ANOTHER replica may be here. A write on THIS
	// replica applies immediately -- see [RateLimitRules.Apply].
	ReloadInterval time.Duration
}

// RateLimitRules holds the rule set in memory and answers "which rules apply to
// this request?" without touching the database.
//
// # Why a mirror and not a query
//
// This runs on EVERY request, including unauthenticated ones, and including the
// ones a rate limiter exists to shed. A database round trip there would mean the
// limiter's cost scales with the traffic it is meant to bound -- so a flood
// would be amplified by the thing meant to stop it, which is the opposite of the
// point.
//
// The set is small by construction: it is one row per RULE an operator wrote,
// tens at most, not one per caller. It is nothing like the bucket state, which
// is per (rule, window, scope value) and lives in the limiter.
//
// # Failure is "keep the last good copy", never "empty"
//
// A failed reload leaves the previous set in place and logs loudly. Clearing it
// would mean a database blip silently removes every limit -- and the symptom
// (traffic flows freely) looks exactly like health.
//
// Before the FIRST successful load there is no previous copy, and [Rules]
// returns nil. The caller must read that as "no rules known", not "no rules
// configured". There is no fallback budget behind it, so the difference is
// between "we do not know" and "we know, and nothing applies".
type RateLimitRules struct {
	repository repository.RateLimits

	// rules is replaced wholesale on reload, never mutated. Readers take one
	// atomic load and then hold an immutable slice, so resolution needs no lock
	// on the request path.
	rules atomic.Pointer[[]domain.RateLimit]

	lastReload     atomic.Int64
	reloadFailures atomic.Int64
	loaded         atomic.Bool

	reloadInterval time.Duration

	ot *o11y.OpenTelemetry
}

// NewRateLimitRules creates the mirror. It does NOT load: the first load happens
// in [RateLimitRules.Reload], so a database that is briefly unavailable at
// startup delays the rules rather than refusing to start.
func NewRateLimitRules(conf RateLimitRulesConfig) (*RateLimitRules, error) {
	if conf.Repository == nil {
		return nil, &domain.InvalidInputError{Message: "rate limit rules: repository is nil"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "rate limit rules: OpenTelemetry configuration is nil"}
	}

	if conf.ReloadInterval <= 0 {
		return nil, &domain.InvalidInputError{Message: "rate limit rules: reload interval must be positive"}
	}

	ref := &RateLimitRules{
		repository:     conf.Repository,
		reloadInterval: conf.ReloadInterval,
		ot:             conf.OT,
	}

	if err := ref.registerMetrics(conf.MetricsPrefix); err != nil {
		return nil, err
	}

	return ref, nil
}

func (ref *RateLimitRules) registerMetrics(prefix string) error {
	if prefix != "" {
		prefix += "_"
	}

	// Staleness, not a reload counter. A counter answers "did reloads happen";
	// this answers "is what we are enforcing current", which is the question an
	// operator actually has -- and the one a stopped ticker gets wrong silently.
	if _, err := ref.ot.Metrics.Meter.Float64ObservableGauge(
		prefix+"rate_limit_rules_staleness_seconds",
		metric.WithDescription("Seconds since the rate-limit rule set was last loaded"),
		metric.WithUnit("s"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			s := ref.Staleness()
			if s == staleForever {
				// Never loaded. Reporting 0 would look perfectly fresh, which is
				// the opposite of the truth.
				o.Observe(-1)

				return nil
			}

			o.Observe(s.Seconds())

			return nil
		}),
	); err != nil {
		return err
	}

	if _, err := ref.ot.Metrics.Meter.Int64ObservableGauge(
		prefix+"rate_limit_rules_size",
		metric.WithDescription("Number of enabled rate-limit rules currently mirrored"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(int64(ref.Size()))

			return nil
		}),
	); err != nil {
		return err
	}

	if _, err := ref.ot.Metrics.Meter.Int64ObservableCounter(
		prefix+"rate_limit_rules_reload_failures_total",
		metric.WithDescription("Total failed rate-limit rule reloads"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(ref.reloadFailures.Load())

			return nil
		}),
	); err != nil {
		return err
	}

	return nil
}

// staleForever is what Staleness reports before the first successful load. It is
// deliberately the maximum duration rather than zero: an alert on staleness must
// fire when the set has NEVER loaded, which is the worst case, not the best.
const staleForever = time.Duration(1<<63 - 1)

// Rules returns the current rule set, or nil if none has ever loaded.
//
// nil means "not known", NOT "none configured". A caller that treats the two the
// same removes every limit on the first failed startup load.
func (ref *RateLimitRules) Rules() []domain.RateLimit {
	// The nil pointer IS the "never loaded" signal -- every store is paired with
	// loaded.Store(true), so checking the flag here as well changed nothing and
	// could not fail when removed. `loaded` remains as the public accessor and
	// the metric, where it means something a pointer cannot express.
	//
	// Note the distinction this preserves: a SUCCESSFUL reload that finds zero
	// rules stores an empty non-nil slice, so it reports "known, none apply"
	// and the caller falls back to the flags. Only "never loaded" is nil.
	p := ref.rules.Load()
	if p == nil {
		return nil
	}

	return *p
}

// Resolve answers which rules apply to a request.
//
// Returns (matches, known). When known is false the caller MUST fall back to the
// this rather than treating an empty match list as "nothing applies" --
// those are different states and only one of them is safe.
func (ref *RateLimitRules) Resolve(req domain.RateLimitRequest) ([]domain.RateLimitMatch, bool) {
	rules := ref.Rules()
	if rules == nil {
		return nil, false
	}

	return domain.ResolveRateLimits(rules, req), true
}

// Reload rebuilds the set from the repository.
//
// On failure the previous set is KEPT and the error returned. The caller logs;
// this does not clear.
func (ref *RateLimitRules) Reload(ctx context.Context) error {
	rules, err := ref.repository.SelectAll(ctx)
	if err != nil {
		ref.reloadFailures.Add(1)

		return err
	}

	ref.rules.Store(new(EnforceableRateLimits(rules)))
	ref.lastReload.Store(time.Now().UnixNano())
	ref.loaded.Store(true)

	return nil
}

// Apply installs a rule set directly, without a query.
//
// This is what makes a write visible to the replica that served it BEFORE the
// response is written. Without it an operator saves a rule, immediately tests
// it, sees the old behaviour, and concludes the feature is broken -- for up to
// one reload interval.
//
// It does not mark the set fresher than it is: lastReload is not touched, so
// staleness still measures time since the last real load and an alert on it
// cannot be silenced by a stream of writes.
func (ref *RateLimitRules) Apply(rules []domain.RateLimit) {
	ref.rules.Store(new(EnforceableRateLimits(rules)))
	ref.loaded.Store(true)
}

// Size reports how many rules are mirrored.
func (ref *RateLimitRules) Size() int {
	p := ref.rules.Load()
	if p == nil {
		return 0
	}

	return len(*p)
}

// Staleness reports how long since the last successful load, or [staleForever]
// if there has never been one.
func (ref *RateLimitRules) Staleness() time.Duration {
	last := ref.lastReload.Load()
	if last == 0 {
		return staleForever
	}

	return time.Since(time.Unix(0, last))
}

// Loaded reports whether a successful load has ever happened.
func (ref *RateLimitRules) Loaded() bool { return ref.loaded.Load() }

// Run loads once immediately, then reloads on a ticker until ctx is done.
//
// The immediate load matters: waiting a full interval for the first one would
// leave every request unlimited for that long after every restart,
// which is a rolling deploy's worth of unlimited traffic.
func (ref *RateLimitRules) Run(ctx context.Context) {
	if err := ref.Reload(ctx); err != nil {
		slog.Error(
			"could not load the rate-limit rule set; nothing is being rate limited until a reload succeeds",
			"error", err,
		)
	}

	ticker := time.NewTicker(ref.reloadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Debug("stopping the rate-limit rule mirror", "cause", context.Cause(ctx))

			return
		case <-ticker.C:
			if err := ref.Reload(ctx); err != nil {
				// Loud, because every symptom is invisible: the mirror keeps
				// answering from a set that is quietly getting older, and a rule
				// written on another replica is never seen here.
				slog.Error(
					"could not reload the rate-limit rule set; serving from the previous copy",
					"error", err,
					"staleness", ref.Staleness(),
					"size", ref.Size(),
				)
			}
		}
	}
}

// RateLimitRuleSet is the half of [RateLimitRules] the CRUD service needs: make
// what I just wrote visible here, now, and tell me what is in force.
//
// An interface so the service depends on the operation rather than the mirror,
// and so "no mirror wired" is expressible as a nil.
//
// Resolve is here so the effective-rule endpoint answers from the set being
// ENFORCED rather than from a fresh query. The two are not the same set -- the
// mirror has dropped what it cannot enforce and is up to one reload interval
// behind another replica's writes -- and an endpoint whose whole purpose is
// "why is this not limited?" answering from the other one is worse than not
// having it.
type RateLimitRuleSet interface {
	Apply(rules []domain.RateLimit)
	Reload(ctx context.Context) error
	Resolve(req domain.RateLimitRequest) ([]domain.RateLimitMatch, bool)
}

var _ RateLimitRuleSet = (*RateLimitRules)(nil)

// EnforceableRateLimits keeps only the rules this service can actually enforce,
// logging one warning per rule it drops.
//
// # Why the mirror filters, and resolution does not
//
// [domain.ResolveRateLimits] picks a winner per scope. If an unenforceable rule
// were allowed into it, that rule could WIN its scope on specificity and shadow
// a broader rule that does carry a budget -- so a malformed row would not merely
// fail to apply, it would switch off a working limit that was otherwise in
// force. Filtering before resolution is what makes "skip it and fall through to
// the next tier" true rather than aspirational.
//
// Every caller that resolves must filter with THIS function. Two call sites that
// filter differently is how the effective-rule endpoint came to disagree with
// what was being enforced.
//
// Disabled rules are dropped silently -- that is an operator's deliberate
// choice, not a fault -- which also makes rate_limit_rules_size mean what its
// description says.
func EnforceableRateLimits(rules []domain.RateLimit) []domain.RateLimit {
	kept := make([]domain.RateLimit, 0, len(rules))

	for i := range rules {
		if !rules[i].IsEnabled() {
			continue
		}

		if problem := rules[i].EnforceabilityProblem(); problem != "" {
			// Warned on every reload, deliberately. A rule that silently does
			// nothing is the failure this whole feature is meant to prevent, and
			// the operator who wrote it is not watching the log at the moment it
			// first loads. The set is tens of rules and broken ones are rare, so
			// the repetition is bounded.
			slog.Warn(
				"rate-limit rule cannot be enforced and is being ignored",
				"rate_limit_id", rules[i].ID.String(),
				"name", rules[i].Name,
				"scope", string(rules[i].Scope),
				"audience", string(rules[i].Audience),
				"problem", problem,
				"consequence", "this rule is enforcing nothing; requests it names fall through to the next matching rule, or to no limit at all",
			)

			continue
		}

		kept = append(kept, rules[i])
	}

	return kept
}
