// Package cachevalkey is the driven adapter that implements the
// internal/core/port/driven/cache.Cache port using the existing
// c3e.SafeCacheManager (github.com/slashdevops/c3e, a Valkey-backed cache).
//
// Once the cache stack changes (e.g. swap Valkey for Redis or memcached),
// only this adapter needs to change; the use-cases under internal/core/
// continue to depend on cache.Cache.
//
// # What this package does and does not own
//
// The adapter is deliberately thin: it translates identifiers and fetchers
// between the port's vocabulary and c3e's, and nothing else. Two things it
// might look like it should own live elsewhere on purpose.
//
// Observability is reported by c3e itself through [c3e.Hooks], wired in the
// composition root from [Instruments.Hooks]. The adapter cannot see whether a
// read hit, went stale or missed, and the version of this file that tried to
// infer it from outside had a data race for its trouble — see [Instruments].
//
// Fail-open behaviour belongs to [cache.GetTyped], which has the caller's
// concrete type and can return a fetched value when the cache faults. The
// adapter surfaces faults as errors and lets the port decide; it never
// silently returns a zero value, which is what an earlier "cache disabled"
// branch here did.
//
// # Encoding
//
// json is the only encoder. gob was supported once and is gone: it cannot tell
// a nil pointer from a pointer to a zero value, so it silently dropped every
// optional field holding false or "". See [domain.CacheEncoderTypeJSON].
package cachevalkey

import (
	"context"
	"errors"
	"time"

	"github.com/slashdevops/c3e"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/cache"
)

// errNoCacheManager is returned when the adapter was built without a manager.
// [cache.GetTyped] treats any cache-layer error as a reason to consult the
// source of truth directly, so this degrades to "no caching" rather than to
// wrong data. Constructing the adapter this way is a wiring mistake — the
// composition root leaves the cache.Cache interface nil when caching is
// disabled — but it must not be a silent one.
var errNoCacheManager = errors.New("cachevalkey: no cache manager configured")

// Config carries the policy the adapter applies on top of c3e.
type Config struct {
	// InvalidateTimeout bounds one invalidation cascade. c3e applies its
	// QueryTimeout to reads only — Set and Invalidate run on the caller's
	// context, and the HTTP server deliberately sets no ReadTimeout or
	// WriteTimeout, so a request context has no deadline of its own. Without
	// this, a degraded Valkey adds unbounded latency to every write that
	// invalidates something.
	InvalidateTimeout time.Duration
}

// Adapter wraps a *c3e.SafeCacheManager and implements cache.Cache.
type Adapter struct {
	inner *c3e.SafeCacheManager
	cfg   Config
}

// New returns an adapter that delegates to inner.
//
// To disable caching, leave the cache.Cache interface nil rather than passing
// a nil manager here: [cache.GetTyped] checks for that and skips the cache
// entirely. A nil inner is treated as a misconfiguration and every operation
// reports errNoCacheManager.
//
// Metrics are not wired here. Build [Instruments] first and pass
// [Instruments.Hooks] to c3e.NewSafeCacheManager, so the outcome of each read
// is reported by the code that knows it.
func New(inner *c3e.SafeCacheManager, cfg Config) *Adapter {
	return &Adapter{inner: inner, cfg: cfg}
}

// Get implements cache.Cache.
func (a *Adapter) Get(ctx context.Context, id cache.Identifier, dest any, fetcher cache.Fetcher) error {
	if a == nil || a.inner == nil {
		return errNoCacheManager
	}

	c3eFetcher := func(ctx context.Context) (any, []c3e.CacheIdentifier, error) {
		data, dependencies, err := fetcher(ctx)
		if err != nil {
			return nil, nil, err
		}

		return data, toC3EIdentifiers(dependencies), nil
	}

	return a.inner.Get(ctx, toC3EIdentifier(id), dest, c3eFetcher)
}

// Invalidate implements cache.Cache.
//
// The cascade runs on a detached, separately bounded context. Detached because
// a cascade abandoned halfway is worse than one that never started: the
// database write has already committed, so every entry the walk did not reach
// stays cached and wrong until its hard TTL. A client that hangs up mid-request
// must not cause that. Bounded because the caller's context frequently has no
// deadline at all, and the cascade is a breadth-first walk with a round trip
// per node — it is the one cache operation that can take arbitrarily long
// against a degraded server.
//
// Request-scoped values (the trace span in particular) survive
// context.WithoutCancel, so the cascade still appears under the request that
// caused it.
func (a *Adapter) Invalidate(ctx context.Context, id cache.Identifier) error {
	if a == nil || a.inner == nil {
		return errNoCacheManager
	}

	if a.cfg.InvalidateTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), a.cfg.InvalidateTimeout)
		defer cancel()
	}

	return a.inner.Invalidate(ctx, toC3EIdentifier(id))
}

func toC3EIdentifier(id cache.Identifier) c3e.CacheIdentifier {
	return c3e.CacheIdentifier{Type: id.Type, ID: id.ID}
}

func toC3EIdentifiers(ids []cache.Identifier) []c3e.CacheIdentifier {
	if len(ids) == 0 {
		return nil
	}

	out := make([]c3e.CacheIdentifier, len(ids))
	for i, id := range ids {
		out[i] = toC3EIdentifier(id)
	}

	return out
}
