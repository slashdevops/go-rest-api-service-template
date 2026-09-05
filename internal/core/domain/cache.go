package domain

import "time"

type CacheEncoderType string

// json is the only supported cache encoder.
//
// gob was supported and was briefly the default. It is gone because it cannot
// represent the difference between a nil pointer and a pointer to a zero value:
// it omits any field holding its type's zero value and flattens a pointer to
// the value behind it, so *bool(false) is transmitted as nothing and decodes as
// nil. This package carries 40 *bool and 52 *string fields — Admin, Disabled,
// System, LocalAccount — and handlers return them verbatim, so under gob an
// endpoint answered "admin": null on a cache hit and "admin": false on a miss.
//
// Registering types made gob able to *encode* those values; it never made it
// lossless. A startup warning was tried first and was not enough: a setting
// that silently drops data should be impossible to select, not discouraged.
// Anything else is now rejected by [CacheConfig.Validate] before the service
// starts.
const (
	CacheEncoderTypeJSON CacheEncoderType = "json"
)

func (cet CacheEncoderType) IsValid() bool {
	return cet == CacheEncoderTypeJSON
}

func (cet CacheEncoderType) String() string {
	return string(cet)
}

const (
	// A Valkey server ships with `databases 16`, so the valid SELECT indexes
	// are 0..15 — 16 is one past the end and the client refuses to connect
	// with "DB index is out of range". This bound said 16 and so accepted a
	// value that could only fail at startup.
	ValidCacheMaxDatabaseNumber = 15
	ValidCacheMinDatabaseNumber = 0

	ValidCacheServerKind = "valkey"
	ValidCacheMaxPort    = 65535
	ValidCacheMinPort    = 0

	// The upper bound is c3e's own MaxSafeCacheManagerQueryTimeout. It has to
	// be, because NewSafeCacheManager rejects anything above it and the
	// service then fails to boot on a value this package just declared valid —
	// with a raw library string rather than an InvalidConfigurationError
	// naming the flag. Anything Validate accepts must start.
	ValidCacheMaxQueryTimeout = 500 * time.Millisecond
	ValidCacheMinQueryTimeout = 10 * time.Millisecond

	// Invalidation gets a far larger budget than a read, and needs one: a
	// cascade walks the dependency graph breadth-first with a round trip per
	// node, where a read is a single GET. It is also not interchangeable with
	// the read budget in kind — a read that misses its deadline falls through
	// to the database and the request is still correct, whereas an
	// invalidation that misses its deadline leaves stale entries behind.
	ValidCacheMaxInvalidateTimeout = 30 * time.Second
	ValidCacheMinInvalidateTimeout = 100 * time.Millisecond
	ValidCacheMaxEntitiesHardTTL   = 72 * time.Hour
	ValidCacheMinEntitiesHardTTL   = 1 * time.Hour

	// The 95% of ValidCacheMaxEntitiesHardTTL
	ValidCacheMaxEntitiesSoftTTL = ValidCacheMaxEntitiesHardTTL - (ValidCacheMaxEntitiesHardTTL / 20)
	ValidCacheMinEntitiesSoftTTL = 1 * time.Minute

	ValidCacheMinTTLJitterPercent = 0.0
	ValidCacheMaxTTLJitterPercent = 0.99
)
