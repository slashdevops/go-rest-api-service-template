package config

import (
	"slices"
	"strings"
	"time"
)

// Rate-limit bounds. Each is a refusal at startup rather than a value clamped
// silently: a limiter running with numbers the operator did not write is worse
// than one that refuses to start, because nothing afterwards says so.
const (
	ValidRateLimitMinReloadInterval = 1 * time.Second
	ValidRateLimitMaxReloadInterval = 10 * time.Minute

	ValidRateLimitMinStoreTimeout = 5 * time.Millisecond
	ValidRateLimitMaxStoreTimeout = 5 * time.Second

	ValidRateLimitMinBucketSweepInterval = 10 * time.Second
	ValidRateLimitMaxBucketSweepInterval = 1 * time.Hour
)

const (
	// DefaultRateLimitEnabled is TRUE.
	//
	// Rate limiting is on unless an operator turns it off. There is no second
	// limiter to fall back to and no fallback budget: the budgets live in the
	// rate_limits table, seeded with a default, and this switch decides only
	// whether they are enforced.
	//
	// It replaced ratelimit.rules.enabled, which defaulted to FALSE and chose
	// between two limiters. That arrangement produced two bugs on its own: the
	// exemptions lived in the limiter that did not run by default, so /health
	// was rate limited; and http.server.ip.rate.limiter.enabled=false did not
	// disable anything, because the branch that read it was unreachable
	// whenever rules were on.
	DefaultRateLimitEnabled = true

	// DefaultRateLimitReloadInterval bounds how stale a rule written on ANOTHER
	// replica may be here. A write on the serving replica applies immediately,
	// so this is the cross-replica horizon, not the edit-to-effect latency an
	// operator experiences.
	DefaultRateLimitReloadInterval = 30 * time.Second

	// DefaultRateLimitStoreTimeout bounds one round trip to the shared counter.
	//
	// It matters more than it looks: the HTTP server sets no ReadTimeout or
	// WriteTimeout deliberately, so a request context often carries no deadline
	// of its own. Without this an unresponsive Valkey holds a request open for
	// as long as the connection allows. Kept at the same order as
	// cache.max.query.timeout (70ms) for the same reason.
	DefaultRateLimitStoreTimeout = 70 * time.Millisecond

	// DefaultRateLimitStoreBreakerThreshold is how many CONSECUTIVE failures
	// stop the shared counter being called.
	//
	// Five rather than one: a single failed round trip is a blip, and opening on
	// it would turn a dropped packet into a period of degraded limiting. Five in
	// a row is an outage.
	DefaultRateLimitStoreBreakerThreshold = 5

	// DefaultRateLimitStoreBreakerCooldown is how long the store is left alone
	// before one request is allowed to test it.
	//
	// Five seconds: long enough that a restarting Valkey is not hammered while
	// it comes up, short enough that recovery is not noticed by an operator.
	DefaultRateLimitStoreBreakerCooldown = 5 * time.Second

	// DefaultRateLimitStoreFailMode is "closed": refuse when the shared counter
	// cannot answer.
	//
	// An unknown budget is not an empty one. The alternative, "local", is
	// bounded -- N replicas means N x the rate -- and is a deliberate choice,
	// never a silent default.
	DefaultRateLimitStoreFailMode = RateLimitFailModeClosed

	// DefaultRateLimitBucketSweepInterval drops per-replica buckets nobody has
	// touched. An ip-scoped rule has one bucket per client address, so without a
	// sweep a scan is an unbounded allocation.
	DefaultRateLimitBucketSweepInterval = 5 * time.Minute

	// DefaultRateLimitBucketIdleAfter is how long a bucket must be untouched
	// before the sweep may drop it. Never shorter than the bucket's own window,
	// which the sweeper enforces regardless -- dropping a bucket sooner hands a
	// full budget back to a caller who is still spending.
	DefaultRateLimitBucketIdleAfter = 10 * time.Minute
)

// Fail modes for the shared counter.
const (
	RateLimitFailModeClosed = "closed"
	RateLimitFailModeLocal  = "local"
)

// DefaultRateLimitBypassPrefixes are answered without any rule lookup.
//
// With fail-closed a shared-store outage would otherwise 429 the readiness
// probe, and the limiter would turn a cache outage into an eviction from the
// load balancer. This is config rather than a hardcoded list so a deployment
// that mounts the API under a different prefix can still say so.
var DefaultRateLimitBypassPrefixes = SliceStringVar{"/health", "/version"}

// DefaultRateLimitExcludedIPs is empty. See [RateLimitConfig.ExcludedIPs].
var DefaultRateLimitExcludedIPs = SliceStringVar{}

type RateLimitConfig struct {
	StoreFailMode Field[string]

	// StoreBreaker* stop a failing shared counter being asked again on every
	// request. They change no OUTCOME -- store.fail.mode still decides what a
	// fault means -- only the waiting.
	StoreBreakerThreshold Field[int]
	StoreBreakerCooldown  Field[time.Duration]
	ReloadInterval        Field[time.Duration]
	StoreTimeout          Field[time.Duration]

	BucketSweepInterval Field[time.Duration]
	BucketIdleAfter     Field[time.Duration]

	// ExcludedIPs never see a limit.
	//
	// Config and not a database row on purpose: this is the escape hatch an
	// operator reaches for when the limiter ITSELF is the incident, and it must
	// not depend on the database being reachable, or on a rule being loadable,
	// to work.
	ExcludedIPs Field[SliceStringVar]

	// BypassPrefixes are answered before any rule lookup.
	BypassPrefixes Field[SliceStringVar]

	// Enabled is the master switch. Off means no rate limiting anywhere -- not
	// "fall back to something else", because there is nothing else.
	Enabled Field[bool]
}

func NewRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		Enabled: NewField("ratelimit.enabled", "RATELIMIT_ENABLED",
			"Enforce rate limits. On by default. Budgets come from the rate_limits table, which is seeded with a per-IP default; off means no limiting at all, since there is no second limiter to fall back to",
			DefaultRateLimitEnabled),
		ReloadInterval: NewField("ratelimit.reload.interval", "RATELIMIT_RELOAD_INTERVAL",
			"How often the rule set is reloaded, and therefore how stale a rule written on another replica may be. A write on the serving replica applies immediately",
			DefaultRateLimitReloadInterval),
		StoreBreakerThreshold: NewField("ratelimit.store.breaker.threshold", "RATELIMIT_STORE_BREAKER_THRESHOLD",
			"Consecutive shared-counter failures after which the store is not called at all. 0 disables the breaker, which means paying a failed round trip on every request during an outage",
			DefaultRateLimitStoreBreakerThreshold),
		StoreBreakerCooldown: NewField("ratelimit.store.breaker.cooldown", "RATELIMIT_STORE_BREAKER_COOLDOWN",
			"How long the shared counter is left alone once the breaker opens, before one request is allowed through to test it",
			DefaultRateLimitStoreBreakerCooldown),
		StoreTimeout: NewField("ratelimit.store.timeout", "RATELIMIT_STORE_TIMEOUT",
			"Bound on one round trip to the shared counter. The HTTP server sets no read or write timeout, so without this an unresponsive store holds a request open",
			DefaultRateLimitStoreTimeout),
		StoreFailMode: NewField("ratelimit.store.fail.mode", "RATELIMIT_STORE_FAIL_MODE",
			"What happens when the shared counter cannot answer. [closed] refuses the request; [local] falls back to the per-replica limiter, which is bounded but means N replicas allow N times the rate",
			DefaultRateLimitStoreFailMode),
		BucketSweepInterval: NewField("ratelimit.bucket.sweep.interval", "RATELIMIT_BUCKET_SWEEP_INTERVAL",
			"How often idle per-replica buckets are dropped. An ip-scoped rule has one bucket per client address, so without a sweep a scan is an unbounded allocation",
			DefaultRateLimitBucketSweepInterval),
		BucketIdleAfter: NewField("ratelimit.bucket.idle.after", "RATELIMIT_BUCKET_IDLE_AFTER",
			"How long a per-replica bucket must be untouched before the sweep may drop it. Never applied sooner than the bucket's own window",
			DefaultRateLimitBucketIdleAfter),
		ExcludedIPs: NewField("ratelimit.excluded.ips", "RATELIMIT_EXCLUDED_IPS",
			"Client addresses that are never limited. Config rather than a database rule, so it works when the limiter itself is the incident. Example: --ratelimit.excluded.ips=10.0.0.1 --ratelimit.excluded.ips=10.0.0.2",
			DefaultRateLimitExcludedIPs),
		BypassPrefixes: NewField("ratelimit.bypass.prefixes", "RATELIMIT_BYPASS_PREFIXES",
			"Path prefixes answered without any rule lookup. Health and version live here: with fail-closed a store outage must not 429 the readiness probe",
			DefaultRateLimitBypassPrefixes),
	}
}

func (c *RateLimitConfig) ParseEnvVars() {
	c.Enabled.Value = GetEnv(c.Enabled.EnVarName, c.Enabled.Value)
	c.ReloadInterval.Value = GetEnv(c.ReloadInterval.EnVarName, c.ReloadInterval.Value)
	c.StoreTimeout.Value = GetEnv(c.StoreTimeout.EnVarName, c.StoreTimeout.Value)
	c.StoreFailMode.Value = GetEnv(c.StoreFailMode.EnVarName, c.StoreFailMode.Value)
	c.StoreBreakerThreshold.Value = GetEnv(c.StoreBreakerThreshold.EnVarName, c.StoreBreakerThreshold.Value)
	c.StoreBreakerCooldown.Value = GetEnv(c.StoreBreakerCooldown.EnVarName, c.StoreBreakerCooldown.Value)
	c.BucketSweepInterval.Value = GetEnv(c.BucketSweepInterval.EnVarName, c.BucketSweepInterval.Value)
	c.BucketIdleAfter.Value = GetEnv(c.BucketIdleAfter.EnVarName, c.BucketIdleAfter.Value)
	c.ExcludedIPs.Value = GetEnv(c.ExcludedIPs.EnVarName, c.ExcludedIPs.Value)
	c.BypassPrefixes.Value = GetEnv(c.BypassPrefixes.EnVarName, c.BypassPrefixes.Value)
}

func (c *RateLimitConfig) Validate() error {
	if c.ReloadInterval.Value < ValidRateLimitMinReloadInterval || c.ReloadInterval.Value > ValidRateLimitMaxReloadInterval {
		return &InvalidConfigurationError{
			Field:   c.ReloadInterval.FlagName,
			Value:   c.ReloadInterval.Value.String(),
			Message: "reload interval must be between " + ValidRateLimitMinReloadInterval.String() + " and " + ValidRateLimitMaxReloadInterval.String(),
		}
	}

	if c.StoreTimeout.Value < ValidRateLimitMinStoreTimeout || c.StoreTimeout.Value > ValidRateLimitMaxStoreTimeout {
		return &InvalidConfigurationError{
			Field:   c.StoreTimeout.FlagName,
			Value:   c.StoreTimeout.Value.String(),
			Message: "store timeout must be between " + ValidRateLimitMinStoreTimeout.String() + " and " + ValidRateLimitMaxStoreTimeout.String(),
		}
	}

	if !slices.Contains([]string{RateLimitFailModeClosed, RateLimitFailModeLocal}, c.StoreFailMode.Value) {
		return &InvalidConfigurationError{
			Field:   c.StoreFailMode.FlagName,
			Value:   c.StoreFailMode.Value,
			Message: "fail mode must be one of [" + RateLimitFailModeClosed + "|" + RateLimitFailModeLocal + "]",
		}
	}

	if c.BucketSweepInterval.Value < ValidRateLimitMinBucketSweepInterval || c.BucketSweepInterval.Value > ValidRateLimitMaxBucketSweepInterval {
		return &InvalidConfigurationError{
			Field:   c.BucketSweepInterval.FlagName,
			Value:   c.BucketSweepInterval.Value.String(),
			Message: "bucket sweep interval must be between " + ValidRateLimitMinBucketSweepInterval.String() + " and " + ValidRateLimitMaxBucketSweepInterval.String(),
		}
	}

	// A bucket dropped before its own window has elapsed hands a full budget
	// back to a caller who is still spending it. The sweeper refuses to do that
	// whatever this says, but a configuration that ASKS for it is a
	// misunderstanding worth correcting at startup rather than silently
	// ignoring.
	if c.BucketIdleAfter.Value < c.BucketSweepInterval.Value {
		return &InvalidConfigurationError{
			Field:   c.BucketIdleAfter.FlagName,
			Value:   c.BucketIdleAfter.Value.String(),
			Message: "idle-after must be at least the sweep interval (" + c.BucketSweepInterval.Value.String() + "); a shorter value asks the sweeper to drop buckets it will refuse to drop",
		}
	}

	return nil
}

// ExcludedIPsList returns the configured exclusions, trimmed.
func (c *RateLimitConfig) ExcludedIPsList() []string {
	return trimmedList(c.ExcludedIPs.Value)
}

// BypassPrefixesList returns the configured bypass prefixes, trimmed.
func (c *RateLimitConfig) BypassPrefixesList() []string {
	return trimmedList(c.BypassPrefixes.Value)
}

func trimmedList(in SliceStringVar) []string {
	out := make([]string, 0, len(in))

	for _, v := range in {
		if t := strings.TrimSpace(v); t != "" {
			out = append(out, t)
		}
	}

	return out
}
