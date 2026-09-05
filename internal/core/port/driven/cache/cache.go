// Package cache defines the driven port that use-cases consume to read
// and invalidate cached values. The implementation lives in
// internal/adapter/driven/cachevalkey (which wraps github.com/slashdevops/c3e).
//
// nil is a valid implementation: passing a nil Cache disables caching
// entirely. Use-cases must handle the nil case (most do; the existing
// `if ref.cacheService == nil` branches in service/*.go are the model).
//
// # The cache may never fail a request
//
// A cache is an optimisation. Every use-case that consults one has a
// source of truth behind it, so a fault in the cache layer must cost
// latency and nothing else. [GetTyped] enforces that: if the fetcher
// produced a value and the cache still returned an error — a failed
// encode, a payload that no longer decodes after an encoder or schema
// change, a connection that dropped mid-write — the fetched value is
// returned and the cache error is dropped.
//
// This is not theoretical tidiness. The service caches the
// authorization permissions map on the path of every authenticated
// request, and it once shipped with an encoder that could not encode
// that map at all. Without this guarantee a serialization problem
// surfaces as 500 "authorization service unavailable" for every user,
// which is exactly how it shipped.
//
// The one error that is *not* swallowed is the fetcher's own. An error
// from the source of truth is returned unwrapped, so a
// domain.*NotFoundError still reaches the handler as a 404 rather than
// being flattened into a 500.
package cache

import (
	"context"
	"errors"
	"reflect"
	"sync"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/cache.go -source=cache.go Cache

// errUncacheableNil is returned to the cache implementation — never to a
// caller — when a fetcher yields a nil pointer. See [GetTyped] for why the
// value is refused rather than stored.
var errUncacheableNil = errors.New("cache: fetcher returned a nil pointer; value not cached")

// Identifier identifies a cached value and (when used in a Fetcher's
// returned dependency list) the cached values that should be invalidated
// when this entry is invalidated.
type Identifier struct {
	Type string // entity type, e.g. "user", "project"
	ID   string // entity id, typically a UUID
}

// String returns "type:id".
func (id Identifier) String() string {
	return id.Type + ":" + id.ID
}

// Fetcher retrieves the source-of-truth value when the cache misses.
// The returned dependency list lets the cache invalidate this entry
// when any of its dependencies are invalidated.
type Fetcher func(ctx context.Context) (data any, dependencies []Identifier, err error)

// Cache is the driven port consumed by use-cases.
type Cache interface {
	// Get returns the cached value for identifier into dest. On miss it
	// invokes fetcher, caches the result, then writes it to dest.
	Get(ctx context.Context, identifier Identifier, dest any, fetcher Fetcher) error

	// Invalidate removes the cached value for identifier (and any value
	// whose stored dependency list includes identifier).
	Invalidate(ctx context.Context, identifier Identifier) error
}

// GetTyped is a generic helper around Cache.Get that handles the
// untyped fetcher wrapping, and is the only supported way to read
// through the cache — it carries the guarantees the raw port cannot.
//
// If c is nil, fetcher is invoked directly (cache disabled).
//
// A cache-layer error never reaches the caller when the fetcher
// succeeded; see the package comment. An error from fetcher itself is
// returned unwrapped so callers can still match on its type.
func GetTyped[T any](
	ctx context.Context,
	c Cache,
	identifier Identifier,
	fetcher func(ctx context.Context) (T, []Identifier, error),
) (T, error) {
	if c == nil {
		v, _, err := fetcher(ctx)
		return v, err
	}

	// What the fetcher produced, if it ran at all during this call. The
	// cache implementation may invoke the wrapper below from a detached
	// stale-while-revalidate goroutine that outlives Cache.Get, so this
	// is read and written under a mutex rather than as a plain field —
	// the unsynchronised version of exactly this is a data race the
	// adapter shipped with once already.
	var outcome struct {
		mu  sync.Mutex
		val T
		err error
		ran bool
	}

	untyped := func(ctx context.Context) (any, []Identifier, error) {
		v, dependencies, err := fetcher(ctx)

		outcome.mu.Lock()
		outcome.val, outcome.err, outcome.ran = v, err, true
		outcome.mu.Unlock()

		if err != nil {
			return nil, nil, err
		}

		// A nil result is returned to the caller but never stored.
		//
		// This began as a guard against gob, which panics on a nil
		// pointer rather than returning an error — and on the
		// stale-while-revalidate path that panic runs in a detached
		// goroutine with no recover, so it took the process down rather
		// than the request. gob is gone, and json would store a nil
		// happily as null.
		//
		// It stays because storing it is the wrong behaviour anyway:
		// caching a typed nil is negative caching, which this service
		// deliberately does not do. Removing this would switch that on
		// silently for any fetcher that starts returning (nil, nil, nil).
		if isNilPointer(v) {
			return nil, nil, errUncacheableNil
		}

		return v, dependencies, nil
	}

	var result T
	err := c.Get(ctx, identifier, &result, untyped)
	if err == nil {
		return result, nil
	}

	outcome.mu.Lock()
	val, fetchErr, ran := outcome.val, outcome.err, outcome.ran
	outcome.mu.Unlock()

	switch {
	case ran && fetchErr == nil:
		// The source of truth answered and the cache is what failed.
		// Serve the value we already have and let the adapter's metrics
		// and the implementation's logs carry the fault.
		return val, nil

	case ran:
		// The source of truth itself failed. Return its error unwrapped
		// so handlers keep mapping it to the right status code.
		return result, fetchErr

	default:
		// The fetcher never ran for this call: either the cached payload
		// no longer decodes, or another caller's in-flight fetch failed
		// and this one was folded into it. Go to the source directly.
		v, _, err := fetcher(ctx)
		return v, err
	}
}

// isNilPointer reports whether v is a nil pointer. Nil maps and slices
// encode cleanly as empty ones, so they are deliberately not covered —
// a narrower guard changes less behaviour.
func isNilPointer(v any) bool {
	if v == nil {
		return true
	}

	switch rv := reflect.ValueOf(v); rv.Kind() {
	case reflect.Pointer, reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}
