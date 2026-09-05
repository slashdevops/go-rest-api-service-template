package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/changenotifyvalkey"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/ratelimitbreaker"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/ratelimitmemory"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/ratelimitvalkey"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/config"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/ratelimit"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/usecase"
)

// initRateLimitRules builds the rule mirror and the two limiters.
//
// The FIRST load is synchronous and fatal -- see [App.loadRateLimitRules].
//
// It used not to be, because an unloaded rule set fell back to the flag
// defaults. There is no fallback now: budgets live in the rate_limits table and
// nowhere else, so a replica that starts without them would serve unlimited
// traffic. Postgres is already a hard startup dependency (the service exits if
// it cannot ping it), so refusing to start without a loadable rule set adds no
// new class of failure -- and it buys the invariant the rest of this package
// relies on: IF THE SERVICE IS SERVING, IT HAS RULES.
func (a *App) initRateLimitRules() error {
	if !a.configs.RateLimit.Enabled.Value {
		// Explicit, because from the outside "no limiting" and "a very high
		// limit" look identical until someone tries to abuse the API.
		slog.Warn(
			"rate limiting is DISABLED; no request is limited by anything",
			"why", a.configs.RateLimit.Enabled.FlagName+"=false",
			"consequence", "there is no second limiter and no fallback budget; nothing bounds any caller",
		)

		return nil
	}

	if a.repositories.RateLimits == nil {
		return fmt.Errorf("rate-limit rules are enabled but there is no rate limits repository")
	}

	mirror, err := usecase.NewRateLimitRules(usecase.RateLimitRulesConfig{
		Repository:     a.repositories.RateLimits,
		OT:             a.telemetry,
		ReloadInterval: a.configs.RateLimit.ReloadInterval.Value,
	})
	if err != nil {
		return fmt.Errorf("error creating the rate-limit rule mirror: %w", err)
	}

	a.services.RateLimitRules = mirror

	metrics, err := middleware.NewRateLimitMetrics(a.telemetry.Metrics.Meter, "")
	if err != nil {
		return fmt.Errorf("error registering rate-limit metrics: %w", err)
	}

	a.rateLimitMetrics = metrics

	// The per-replica limiter always exists. It is the limiter when
	// cache.enabled=false, and the layer in front of the shared counter when it
	// is on -- the token bucket that smooths the shared counter's fixed window.
	a.rateLimitLocal = ratelimitmemory.New()

	if a.cacheClient != nil {
		shared, err := ratelimitvalkey.New(ratelimitvalkey.Config{
			Client:  a.cacheClient,
			Timeout: a.configs.RateLimit.StoreTimeout.Value,
		})
		if err != nil {
			return fmt.Errorf("error creating the shared rate-limit counter: %w", err)
		}

		// Wrapped in a breaker so an outage costs one failed round trip per
		// cooldown rather than one per request. It changes no OUTCOME --
		// store.fail.mode still decides what a fault means, and the breaker
		// returns an ERROR while open, never "allowed" -- only the waiting.
		a.rateLimitShared = ratelimitbreaker.New(ratelimitbreaker.Config{
			Limiter:   shared,
			Threshold: a.configs.RateLimit.StoreBreakerThreshold.Value,
			Cooldown:  a.configs.RateLimit.StoreBreakerCooldown.Value,
			// No metric callback: an open breaker returns an ERROR, so the
			// middleware's ordinary store-fault path still drives
			// rate_limit_store_up to 0 and counts the fault. A second signal
			// here would be one more thing to correlate for no new information.
		})
		// A SECOND client, deliberately. valkey-go puts a connection into
		// subscribe mode, where it accepts nothing but further subscription
		// commands -- sharing the counter's client would stall every INCR the
		// limiter makes. It is shared with every other change notifier, which
		// is fine: each Receive takes a dedicated connection from the client.
		subClient, err := a.changeNotifyClient()
		if err != nil {
			return fmt.Errorf("error creating the rate-limit notification client: %w", err)
		}

		if subClient != nil {
			notifier, err := changenotifyvalkey.NewNotifier(changenotifyvalkey.NotifierConfig{
				Client:     subClient,
				Channel:    changenotifyvalkey.RateLimitRulesChannel,
				Subject:    "rate-limit rules",
				InstanceID: a.instanceID(),
			})
			if err != nil {
				return fmt.Errorf("error creating the rate-limit notifier: %w", err)
			}

			a.rateLimitNotifier = notifier
		}
	} else {
		// A global- or user-scoped rule is only meaningful across replicas if
		// there is a shared counter. Without one every rule is per replica, so
		// N replicas allow N times what the rule says. That is a supported
		// deployment, not a broken one -- but it must be stated, because the
		// rule in the database will read as a total.
		slog.Warn(
			"no shared counter for rate limits; every rule is enforced per replica",
			"why", "cache.enabled is false",
			"consequence", "N replicas allow N times the configured rate",
		)
	}

	// The "off" path logs; so must this one. An asymmetry there means the only
	// way to tell which limiter is running is to infer it from the absence of a
	// line, and the two fail in opposite directions.
	slog.Info(
		"rate-limit rules are being enforced",
		"reload_interval", a.configs.RateLimit.ReloadInterval.Value,
		"store", sharedStoreName(a.rateLimitShared != nil),
		"fail_mode", a.configs.RateLimit.StoreFailMode.Value,
		"excluded_ips", len(a.configs.RateLimit.ExcludedIPsList()),
		"bypass_prefixes", a.configs.RateLimit.BypassPrefixesList(),
	)

	return nil
}

func sharedStoreName(shared bool) string {
	if shared {
		return "valkey (shared across replicas)"
	}

	return "per replica (cache.enabled is false)"
}

// startRateLimitRulesMirror loads the rules and keeps them loaded.
//
// A failed first load is logged, not fatal. See [App.initRateLimitRules].
func (a *App) startRateLimitRulesMirror(ctx context.Context) error {
	if a.services == nil || a.services.RateLimitRules == nil {
		return nil
	}

	// The FIRST load is synchronous and fatal.
	//
	// It used to be a bare `go Run(ctx)`, which raced the HTTP server's own
	// goroutine: for as long as the first query took, the server was accepting
	// requests against an unloaded rule set. That was survivable only because
	// an unloaded set fell back to the flag budget. There is no fallback now --
	// budgets live in the rate_limits table and nowhere else -- so the same race
	// would serve unlimited traffic instead.
	//
	// Refusing to start is the right failure. Postgres is already a hard
	// startup dependency (the service exits if it cannot ping it) and the
	// migrations that create rate_limits have already run by here, so a failure
	// at this point is genuinely exceptional. What it buys is the invariant the
	// limiter now relies on: if the service is serving, it has rules.
	if err := a.services.RateLimitRules.Reload(ctx); err != nil {
		return fmt.Errorf("could not load the rate-limit rule set: %w", err)
	}

	// The ticker keeps it fresh from here; the first load is already done.
	go a.services.RateLimitRules.Run(ctx)

	// And a write on ANOTHER replica arrives at once rather than waiting out the
	// interval. The ticker is still the floor: everything here may fail, and the
	// only consequence is a change taking up to ratelimit.reload.interval, which
	// is exactly the behaviour before any of this existed.
	if a.rateLimitNotifier != nil {
		go a.watchRateLimitRuleChanges(ctx)
	}

	if a.rateLimitLocal != nil {
		go a.sweepRateLimitBuckets(ctx)
	}

	return nil
}

// sweepRateLimitBuckets drops per-replica buckets nobody has touched.
//
// An ip-scoped rule has one bucket per client address, so without this a scan or
// a botnet is an unbounded allocation -- and the growth is invisible until it is
// an OOM.
func (a *App) sweepRateLimitBuckets(ctx context.Context) {
	ticker := time.NewTicker(a.configs.RateLimit.BucketSweepInterval.Value)
	defer ticker.Stop()

	idle := a.configs.RateLimit.BucketIdleAfter.Value

	for {
		select {
		case <-ctx.Done():
			slog.Debug("stopping the rate-limit bucket sweeper", "cause", context.Cause(ctx))

			return
		case <-ticker.C:
			if removed := a.rateLimitLocal.Sweep(idle); removed > 0 {
				slog.Debug("swept idle rate-limit buckets", "removed", removed, "remaining", a.rateLimitLocal.Size())
			}
		}
	}
}

// rateLimitMiddleware builds one stage of the limiter, or nil when rules are
// not being enforced.
//
// Returning nil rather than a pass-through middleware is deliberate: the caller
// appends it to a chain, and a nil there is a compile-visible mistake where a
// silently-inert middleware is not.
func (a *App) rateLimitMiddleware(stage middleware.RateLimitStage, router *http.ServeMux, resolver *middleware.ClientIPResolver) middleware.Middleware {
	if a.services == nil || a.services.RateLimitRules == nil {
		return nil
	}

	failMode := middleware.RateLimitFailClosed
	if a.configs.RateLimit.StoreFailMode.Value == config.RateLimitFailModeLocal {
		failMode = middleware.RateLimitFailLocal
	}

	conf := middleware.RateLimitConfig{
		Rules:    a.services.RateLimitRules,
		Metrics:  a.rateLimitMetrics,
		Local:    a.rateLimitLocal,
		Shared:   a.sharedRateLimiterOrNil(),
		Resolver: resolver,
		Stage:    stage,
		FailMode: failMode,
	}

	// Only the pre-auth stage needs the router: it runs before the mux has
	// matched, so it has to ask for the pattern. Post-auth reads r.Pattern.
	if stage == middleware.RateLimitStagePreAuth {
		conf.Router = router
	}

	return middleware.RateLimit(conf)
}

// rateLimitExemptions builds the health/version bypass and the excluded-IP
// allowlist.
//
// Both used to live inside middleware.RateLimit, which ran only when rules were
// enabled -- and they were off by default -- so in the shipped posture /health
// answered 429 under load (measured 1 x 200 then 7 x 429 against a 1 req/s
// limiter) and ratelimit.excluded.ips was inert. There is one limiter now, but
// the gate stays where it is: it must apply when the limiter is disabled too,
// and an exempt request must SKIP the limiter rather than be allowed by it.
func (a *App) rateLimitExemptions(resolver *middleware.ClientIPResolver) (*middleware.RateLimitExemptions, error) {
	return middleware.NewRateLimitExemptions(
		resolver,
		a.configs.RateLimit.ExcludedIPsList(),
		a.configs.RateLimit.BypassPrefixesList(),
	)
}

// sharedRateLimiterOrNil returns the Valkey counter, or a nil INTERFACE when
// there is none.
//
// Returning the concrete *ratelimitvalkey.Adapter would hand the middleware a
// non-nil interface wrapping a nil pointer -- the classic Go trap, where
// `!= nil` is true and the first call panics.
func (a *App) sharedRateLimiterOrNil() ratelimit.Limiter {
	if a.rateLimitShared == nil {
		return nil
	}

	return a.rateLimitShared
}

// rateLimitRuleSetOrNil returns the mirror for the CRUD service, or a nil
// INTERFACE when rules are not enforced.
//
// Returning the concrete *usecase.RateLimitRules would hand the service a
// non-nil interface wrapping a nil pointer -- the classic Go trap, where
// `!= nil` is true and the first call panics.
func (a *App) rateLimitRuleSetOrNil() usecase.RateLimitRuleSet {
	if a.services == nil || a.services.RateLimitRules == nil {
		return nil
	}

	return a.services.RateLimitRules
}

// watchRateLimitRuleChanges reloads the mirror when another replica writes.
//
// It reloads rather than applying anything from the message: the payload is a
// SIGNAL, so a duplicate costs one query and a lost one costs a delay, where a
// payload carrying rules would make delivery order load-bearing across a
// reconnect that offers no order.
func (a *App) watchRateLimitRuleChanges(ctx context.Context) {
	err := a.rateLimitNotifier.Watch(ctx, func() {
		if err := a.services.RateLimitRules.Reload(ctx); err != nil {
			slog.Warn(
				"notified of a rate-limit rule change but the reload failed",
				"error", err,
				"consequence", "this replica keeps the previous set until the next scheduled reload",
			)

			return
		}

		slog.Debug("rate-limit rules reloaded after a change on another replica")
	})
	if err != nil {
		slog.Error("the rate-limit change watcher stopped", "error", err)
	}
}

// rateLimitNotifierOrNil returns the notifier, or a nil INTERFACE when there is
// none.
//
// Returning the concrete *ratelimitvalkey.Notifier would hand the service a
// non-nil interface wrapping a nil pointer -- the classic Go trap, where
// `!= nil` is true and the first call panics.
func (a *App) rateLimitNotifierOrNil() ratelimit.ChangeNotifier {
	if a.rateLimitNotifier == nil {
		return nil
	}

	return a.rateLimitNotifier
}
