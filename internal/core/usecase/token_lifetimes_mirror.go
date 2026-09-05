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

// TokenLifetimesConfig configures [TokenLifetimes].
type TokenLifetimesConfig struct {
	Repository repository.TokenLifetimes
	OT         *o11y.OpenTelemetry

	MetricsPrefix string

	// ReloadInterval is how often the row is re-read, and therefore how stale
	// a change made on ANOTHER replica may be here when the change signal is
	// lost or absent. A change made on THIS replica applies before its PUT
	// answers -- see [TokenLifetimesService.applyLocally].
	ReloadInterval time.Duration
}

// TokenLifetimes holds the one row of authn_token_lifetimes in memory and
// answers "how long should the token I am about to sign live?" without
// touching the database.
//
// # Why a mirror and not a query
//
// The value is read on every login and every refresh. Neither is a hot path
// the way the rate limiter's is, and a query there would not be ruinous -- but
// it would put the database on the path of the two calls that exist to hand
// out credentials, and a database blip would then refuse logins over a number
// that changes a few times a year. Reading memory keeps issuance as available
// as the signer.
//
// # Why not cache.Cache
//
// The cache port is optional (cache.enabled=false is supported) and fail-open
// by contract: a fault answers with whatever the fetcher returns, or nothing.
// A lifetime is needed on every login and must have the service's own
// availability -- the same argument that put the token denylist in Postgres
// rather than Valkey.
//
// # Failure is "keep the last good copy", never "use the defaults"
//
// A failed reload leaves the previous value in place and logs loudly. There
// is no fallback constant: before the FIRST successful load [Current] has
// nothing to return, and the composition root makes that first load
// synchronous and fatal so a serving replica always has a value. The
// invariant is the rate limiter's: if the service is serving, it has
// lifetimes.
type TokenLifetimes struct {
	repository repository.TokenLifetimes
	ot         *o11y.OpenTelemetry

	// current is replaced wholesale on reload, never mutated. Readers take one
	// atomic load and hold an immutable copy, so issuance needs no lock.
	current atomic.Pointer[domain.TokenLifetimes]

	lastReload     atomic.Int64
	reloadFailures atomic.Int64

	reloadInterval time.Duration
}

// NewTokenLifetimes creates the mirror. It does NOT load: the composition root
// calls [TokenLifetimes.Reload] synchronously and refuses to start on failure.
func NewTokenLifetimes(conf TokenLifetimesConfig) (*TokenLifetimes, error) {
	if conf.Repository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "token lifetimes: repository is nil"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "token lifetimes: OpenTelemetry configuration is nil"}
	}

	if conf.ReloadInterval <= 0 {
		return nil, &domain.InvalidInputError{Message: "token lifetimes: reload interval must be positive"}
	}

	ref := &TokenLifetimes{
		repository:     conf.Repository,
		reloadInterval: conf.ReloadInterval,
		ot:             conf.OT,
	}

	if err := ref.registerMetrics(conf.MetricsPrefix); err != nil {
		return nil, err
	}

	return ref, nil
}

func (ref *TokenLifetimes) registerMetrics(prefix string) error {
	if prefix != "" {
		prefix += "_"
	}

	// Staleness, not a reload counter: the question an operator has is "is
	// what this replica issues current", and a stopped ticker gets that wrong
	// silently.
	if _, err := ref.ot.Metrics.Meter.Float64ObservableGauge(
		prefix+"authn_token_lifetimes_staleness_seconds",
		metric.WithDescription("Seconds since the token lifetimes were last loaded from the store; -1 before the first load"),
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

	// The values themselves, so an alert about the revocation mirror can
	// compare its staleness with the lifetime it has to keep up with, rather
	// than with a number copied into the rule when the lifetime was a flag.
	if _, err := ref.ot.Metrics.Meter.Float64ObservableGauge(
		prefix+"authn_access_token_lifetime_seconds",
		metric.WithDescription("Lifetime of the access tokens this replica is issuing"),
		metric.WithUnit("s"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			if v, ok := ref.Loaded(); ok {
				o.Observe(v.AccessTokenDuration.Seconds())
			}

			return nil
		}),
	); err != nil {
		return err
	}

	if _, err := ref.ot.Metrics.Meter.Float64ObservableGauge(
		prefix+"authn_refresh_token_lifetime_seconds",
		metric.WithDescription("Lifetime of the refresh tokens this replica is issuing"),
		metric.WithUnit("s"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			if v, ok := ref.Loaded(); ok {
				o.Observe(v.RefreshTokenDuration.Seconds())
			}

			return nil
		}),
	); err != nil {
		return err
	}

	if _, err := ref.ot.Metrics.Meter.Int64ObservableCounter(
		prefix+"authn_token_lifetimes_reload_failures_total",
		metric.WithDescription("Total failed token-lifetime reloads; the previous value is kept"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(ref.reloadFailures.Load())

			return nil
		}),
	); err != nil {
		return err
	}

	return nil
}

// Current returns the lifetimes to issue with right now.
//
// It panics before the first successful load, deliberately. The composition
// root loads synchronously before the server accepts a request, so reaching
// this unloaded is a wiring bug -- and the alternative, returning zero
// durations, would sign tokens that expire at the instant they are issued and
// report nothing.
func (ref *TokenLifetimes) Current() domain.TokenLifetimes {
	p := ref.current.Load()
	if p == nil {
		panic("token lifetimes: Current called before the first load; the composition root must Reload before serving")
	}

	return *p
}

// Loaded returns the value and true once a load has succeeded, or false before.
// The non-panicking form, for metrics and health.
func (ref *TokenLifetimes) Loaded() (domain.TokenLifetimes, bool) {
	p := ref.current.Load()
	if p == nil {
		return domain.TokenLifetimes{}, false
	}

	return *p, true
}

// Reload re-reads the row. On failure the previous value is KEPT and the error
// returned; the caller logs, this does not clear.
//
// The row is validated on the way in. A value the validator refuses -- which
// only a hand-edited row or a bounds change can produce -- is treated as a
// failed reload rather than installed: the CHECK constraints should make it
// unreachable, and if they did not, issuing tokens with it is the wrong answer.
func (ref *TokenLifetimes) Reload(ctx context.Context) error {
	row, err := ref.repository.Get(ctx)
	if err != nil {
		ref.reloadFailures.Add(1)

		return err
	}

	if err := row.Validate(); err != nil {
		ref.reloadFailures.Add(1)

		return err
	}

	ref.current.Store(row)
	ref.lastReload.Store(time.Now().UnixNano())

	return nil
}

// Staleness reports how long since the last successful load, or [staleForever]
// if there has never been one.
func (ref *TokenLifetimes) Staleness() time.Duration {
	last := ref.lastReload.Load()
	if last == 0 {
		return staleForever
	}

	return time.Since(time.Unix(0, last))
}

// Run reloads on a ticker until ctx is done. The first load is the caller's,
// synchronous and fatal; this only keeps it fresh.
func (ref *TokenLifetimes) Run(ctx context.Context) {
	ticker := time.NewTicker(ref.reloadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Debug("stopping the token lifetimes mirror", "cause", context.Cause(ctx))

			return
		case <-ticker.C:
			if err := ref.Reload(ctx); err != nil {
				// Loud, because every symptom is invisible: tokens keep being
				// issued, from a value that is quietly getting older, and a
				// change made on another replica is never seen here.
				slog.Error("could not reload the token lifetimes; issuing from the previous value",
					"error", err,
					"staleness", ref.Staleness(),
				)
			}
		}
	}
}

// TokenLifetimesProvider is the half of [TokenLifetimes] the authn service
// needs: what do I sign with, now.
//
// An interface so issuance depends on the question rather than on the mirror,
// and so a test can hand the authn service a fixed value.
type TokenLifetimesProvider interface {
	Current() domain.TokenLifetimes
}

// TokenLifetimesSet is the half the CRUD service needs: make what I just wrote
// visible here, now.
type TokenLifetimesSet interface {
	Reload(ctx context.Context) error
}

var (
	_ TokenLifetimesProvider = (*TokenLifetimes)(nil)
	_ TokenLifetimesSet      = (*TokenLifetimes)(nil)
)

// FixedTokenLifetimes is a [TokenLifetimesProvider] that always answers the
// same value. For tests, and for nothing else: a fixed lifetime in production
// is exactly the startup flag this mirror replaced.
type FixedTokenLifetimes domain.TokenLifetimes

// Current implements [TokenLifetimesProvider].
func (f FixedTokenLifetimes) Current() domain.TokenLifetimes { return domain.TokenLifetimes(f) }
