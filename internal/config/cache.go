package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

var DefaultCacheServerAddresses = SliceStringVar{"localhost:6379"}

const (
	DefaultCacheServerKind        = "valkey"
	DefaultCacheServerUsername    = ""
	DefaultCacheServerPassword    = ""
	DefaultCacheServerDB          = 0
	DefaultCacheMaxQueryTimeout   = 70 * time.Millisecond
	DefaultCacheInvalidateTimeout = 5 * time.Second
	DefaultCacheEnabled           = true
	DefaultCacheHardEntitiesTTL   = 12 * time.Hour
	DefaultCacheSoftEntitiesTTL   = 8 * time.Hour
	DefaultCacheTTLJitterPercent  = 0.1 // 10%
	DefaultCacheEnableOnClient    = true

	// Cache TLS is off by default, which matches every other transport default
	// in this service and keeps `make start-dev-env` working — the dev Valkey
	// has no certificate. It is off, not absent: the connection carries bcrypt
	// password hashes (SelectByID and SelectByEmail both scan password_hash into
	// the *domain.User that gets cached) and the Valkey AUTH password itself, so
	// an unencrypted link puts credentials on the wire. The service warns at
	// startup when that is the case.
	DefaultCacheTLSEnabled            = false
	DefaultCacheTLSInsecureSkipVerify = false

	// DefaultCacheGeneratedAnswersEnabled caches the model's answer for a fully
	// rendered prompt.
	//
	// On by default because generation is the most expensive thing this service
	// does — bounded by the outbound client timeout and retried up to
	// http.client.max.retries times — and a knowledge base is asked the same
	// questions repeatedly. It is sound because generation is not
	// deterministic: there is no single correct answer a caller could have
	// relied on, so replaying one changes nothing they could depend on.
	//
	// The visible consequence is that byte-identical prompts return
	// byte-identical answers within the TTL. A deployment that wants variation
	// per call — a non-zero temperature offered as a feature — should turn this
	// off.
	DefaultCacheGeneratedAnswersEnabled = true
	// DefaultCacheEncoderType is json, which is now also the only option — see
	// domain.CacheEncoderTypeJSON for why gob was removed rather than merely
	// deprecated.
	DefaultCacheEncoderType = domain.CacheEncoderTypeJSON
)

type CacheConfig struct {
	ServerKind        Field[string]
	ServerAddresses   Field[SliceStringVar]
	ServerUsername    Field[string]
	ServerPassword    Field[string]
	ServerDB          Field[int]
	MaxQueryTimeout   Field[time.Duration]
	InvalidateTimeout Field[time.Duration]
	EntitiesHardTTL   Field[time.Duration]
	EntitiesSoftTTL   Field[time.Duration]
	TTLJitterPercent  Field[float64]
	EnableOnClient    Field[bool]

	// TLS file fields are paths, not FileVar. FileVar cannot represent "not
	// set" — its zero value is a *os.File around fd 0 — and all three of these
	// are optional: a server with a publicly trusted certificate needs no CA
	// file, and client certificates are only needed for mutual TLS.
	TLSCAFile             Field[string]
	TLSCertFile           Field[string]
	TLSKeyFile            Field[string]
	TLSEnabled            Field[bool]
	TLSInsecureSkipVerify Field[bool]
	GeneratedAnswers      Field[bool]
	EncoderType           Field[string]
	Enabled               Field[bool]
}

func NewCacheConfig() *CacheConfig {
	return &CacheConfig{
		ServerKind:        NewField("cache.server.kind", "CACHE_SERVER_KIND", "Cache Kind. Possible values ["+domain.ValidCacheServerKind+"]", DefaultCacheServerKind),
		ServerAddresses:   NewField("cache.server.addresses", "CACHE_SERVER_ADDRESSES", "Cache Server Addresses. List of host:port, Example: --cache.server.addresses=host1:port1 --cache.server.addresses=host2:port2", DefaultCacheServerAddresses),
		ServerUsername:    NewField("cache.server.username", "CACHE_SERVER_USERNAME", "Cache Server Username", DefaultCacheServerUsername),
		ServerPassword:    NewField("cache.server.password", "CACHE_SERVER_PASSWORD", "Cache Server Password", DefaultCacheServerPassword),
		ServerDB:          NewField("cache.server.db", "CACHE_SERVER_DB", "Cache Server DB number", DefaultCacheServerDB),
		MaxQueryTimeout:   NewField("cache.max.query.timeout", "CACHE_MAX_QUERY_TIMEOUT", "Maximum timeout for a cache read before falling back to the database", DefaultCacheMaxQueryTimeout),
		InvalidateTimeout: NewField("cache.invalidate.timeout", "CACHE_INVALIDATE_TIMEOUT", "Maximum time a cache invalidation cascade may take", DefaultCacheInvalidateTimeout),
		EntitiesHardTTL:   NewField("cache.entities.hard.ttl", "CACHE_ENTITIES_HARD_TTL", "Hard TTL for the cache entities", DefaultCacheHardEntitiesTTL),
		EntitiesSoftTTL:   NewField("cache.entities.soft.ttl", "CACHE_ENTITIES_SOFT_TTL", "Soft TTL for the cache entities", DefaultCacheSoftEntitiesTTL),
		TTLJitterPercent:  NewField("cache.ttl.jitter.percent", "CACHE_TTL_JITTER_PERCENT", "TTL Jitter Percent for cache entries", DefaultCacheTTLJitterPercent),
		EnableOnClient:    NewField("cache.client.enabled", "CACHE_CLIENT_ENABLED", "Enable cache on client side", DefaultCacheEnableOnClient),

		TLSEnabled:            NewField("cache.tls.enabled", "CACHE_TLS_ENABLED", "Encrypt the connection to the cache server. The connection carries password hashes and the cache AUTH password", DefaultCacheTLSEnabled),
		TLSCAFile:             NewField("cache.tls.ca.file", "CACHE_TLS_CA_FILE", "PEM file holding the CA that signed the cache server certificate. Leave empty to use the host trust store", ""),
		TLSCertFile:           NewField("cache.tls.cert.file", "CACHE_TLS_CERT_FILE", "Client certificate for mutual TLS to the cache server. Requires cache.tls.key.file", ""),
		TLSKeyFile:            NewField("cache.tls.key.file", "CACHE_TLS_KEY_FILE", "Private key for cache.tls.cert.file", ""),
		TLSInsecureSkipVerify: NewField("cache.tls.insecure.skip.verify", "CACHE_TLS_INSECURE_SKIP_VERIFY", "Do not verify the cache server certificate. Encrypts but does not authenticate, so it does not stop an interception. Testing only", DefaultCacheTLSInsecureSkipVerify),
		GeneratedAnswers:      NewField("cache.generated.answers.enabled", "CACHE_GENERATED_ANSWERS_ENABLED", "Cache the generated answer for an identical rendered prompt. Disable if callers rely on per-call variation", DefaultCacheGeneratedAnswersEnabled),
		EncoderType:           NewField("cache.encoder.type", "CACHE_ENCODER_TYPE", "Cache Encoder Type. Possible values [json]", DefaultCacheEncoderType.String()),
		Enabled:               NewField("cache.enabled", "CACHE_ENABLED", "Enable cache layer", DefaultCacheEnabled),
	}
}

func (c *CacheConfig) ParseEnvVars() {
	c.ServerKind.Value = GetEnv(c.ServerKind.EnVarName, c.ServerKind.Value)
	c.ServerAddresses.Value = GetEnv(c.ServerAddresses.EnVarName, c.ServerAddresses.Value)
	c.ServerUsername.Value = GetEnv(c.ServerUsername.EnVarName, c.ServerUsername.Value)
	c.ServerPassword.Value = GetEnv(c.ServerPassword.EnVarName, c.ServerPassword.Value)
	c.ServerDB.Value = GetEnv(c.ServerDB.EnVarName, c.ServerDB.Value)
	c.MaxQueryTimeout.Value = GetEnv(c.MaxQueryTimeout.EnVarName, c.MaxQueryTimeout.Value)
	c.InvalidateTimeout.Value = GetEnv(c.InvalidateTimeout.EnVarName, c.InvalidateTimeout.Value)
	c.EntitiesHardTTL.Value = GetEnv(c.EntitiesHardTTL.EnVarName, c.EntitiesHardTTL.Value)
	c.EntitiesSoftTTL.Value = GetEnv(c.EntitiesSoftTTL.EnVarName, c.EntitiesSoftTTL.Value)
	c.TTLJitterPercent.Value = GetEnv(c.TTLJitterPercent.EnVarName, c.TTLJitterPercent.Value)
	c.EnableOnClient.Value = GetEnv(c.EnableOnClient.EnVarName, c.EnableOnClient.Value)
	c.TLSEnabled.Value = GetEnv(c.TLSEnabled.EnVarName, c.TLSEnabled.Value)
	c.TLSCAFile.Value = GetEnv(c.TLSCAFile.EnVarName, c.TLSCAFile.Value)
	c.TLSCertFile.Value = GetEnv(c.TLSCertFile.EnVarName, c.TLSCertFile.Value)
	c.TLSKeyFile.Value = GetEnv(c.TLSKeyFile.EnVarName, c.TLSKeyFile.Value)
	c.TLSInsecureSkipVerify.Value = GetEnv(c.TLSInsecureSkipVerify.EnVarName, c.TLSInsecureSkipVerify.Value)
	c.GeneratedAnswers.Value = GetEnv(c.GeneratedAnswers.EnVarName, c.GeneratedAnswers.Value)
	c.EncoderType.Value = GetEnv(c.EncoderType.EnVarName, c.EncoderType.Value)
	c.Enabled.Value = GetEnv(c.Enabled.EnVarName, c.Enabled.Value)
}

func (c *CacheConfig) Validate() error {
	if !slices.Contains(strings.Split(domain.ValidCacheServerKind, "|"), c.ServerKind.Value) {
		return &InvalidConfigurationError{
			Field:   "cache.server.kind",
			Value:   c.ServerKind.Value,
			Message: "invalid cache kind, must be one of [" + domain.ValidCacheServerKind + "]",
		}
	}

	if len(c.ServerAddresses.Value) == 0 {
		return &InvalidConfigurationError{
			Field:   "cache.server.addresses",
			Value:   c.ServerAddresses.Value.String(),
			Message: "invalid cache server addresses, must be a list of host:port",
		}
	}

	if len(c.ServerAddresses.Value) > 0 {
		for _, addr := range c.ServerAddresses.Value {
			parts := strings.Split(addr, ":")

			if len(parts) != 2 {
				return &InvalidConfigurationError{
					Field:   "cache.server.addresses",
					Value:   c.ServerAddresses.Value.String(),
					Message: "invalid cache address, must be in the format host:port",
				}
			}

			port, err := strconv.Atoi(parts[1])
			if err != nil {
				return &InvalidConfigurationError{
					Field:   "cache.server.addresses",
					Value:   c.ServerAddresses.Value.String(),
					Message: "invalid cache address port, must be a number",
				}
			}

			if port < domain.ValidCacheMinPort || port > domain.ValidCacheMaxPort {
				return &InvalidConfigurationError{
					Field:   "cache.server.addresses",
					Value:   c.ServerAddresses.Value.String(),
					Message: fmt.Sprintf("invalid cache address port, must be between %d and %d", domain.ValidCacheMinPort, domain.ValidCacheMaxPort),
				}
			}

			if len(parts[0]) < 3 {
				return &InvalidConfigurationError{
					Field:   "cache.server.addresses",
					Value:   c.ServerAddresses.Value.String(),
					Message: "invalid cache address, must be at least 3 characters",
				}
			}
		}
	}

	if c.ServerDB.Value < domain.ValidCacheMinDatabaseNumber || c.ServerDB.Value > domain.ValidCacheMaxDatabaseNumber {
		return &InvalidConfigurationError{
			Field:   "cache.server.db",
			Value:   fmt.Sprintf("%d", c.ServerDB.Value),
			Message: fmt.Sprintf("invalid cache db number, must be between %d and %d", domain.ValidCacheMinDatabaseNumber, domain.ValidCacheMaxDatabaseNumber),
		}
	}

	if c.MaxQueryTimeout.Value < domain.ValidCacheMinQueryTimeout || c.MaxQueryTimeout.Value > domain.ValidCacheMaxQueryTimeout {
		return &InvalidConfigurationError{
			Field:   "cache.max.query.timeout",
			Value:   c.MaxQueryTimeout.Value.String(),
			Message: fmt.Sprintf("invalid cache query max timeout, must be between %s and %s", time.Duration(domain.ValidCacheMinQueryTimeout), time.Duration(domain.ValidCacheMaxQueryTimeout)),
		}
	}

	if c.InvalidateTimeout.Value < domain.ValidCacheMinInvalidateTimeout || c.InvalidateTimeout.Value > domain.ValidCacheMaxInvalidateTimeout {
		return &InvalidConfigurationError{
			Field:   "cache.invalidate.timeout",
			Value:   c.InvalidateTimeout.Value.String(),
			Message: fmt.Sprintf("invalid cache invalidate timeout, must be between %s and %s", time.Duration(domain.ValidCacheMinInvalidateTimeout), time.Duration(domain.ValidCacheMaxInvalidateTimeout)),
		}
	}

	if c.EntitiesHardTTL.Value < domain.ValidCacheMinEntitiesHardTTL || c.EntitiesHardTTL.Value > domain.ValidCacheMaxEntitiesHardTTL {
		return &InvalidConfigurationError{
			Field:   "cache.entities.hard.ttl",
			Value:   c.EntitiesHardTTL.Value.String(),
			Message: fmt.Sprintf("invalid cache entities ttl, must be between %s and %s", time.Duration(domain.ValidCacheMinEntitiesHardTTL), time.Duration(domain.ValidCacheMaxEntitiesHardTTL)),
		}
	}

	if c.EntitiesSoftTTL.Value < domain.ValidCacheMinEntitiesSoftTTL || c.EntitiesSoftTTL.Value > domain.ValidCacheMaxEntitiesSoftTTL {
		return &InvalidConfigurationError{
			Field:   "cache.entities.soft.ttl",
			Value:   c.EntitiesSoftTTL.Value.String(),
			Message: fmt.Sprintf("invalid cache entities soft ttl, must be between %s and %s", time.Duration(domain.ValidCacheMinEntitiesSoftTTL), time.Duration(domain.ValidCacheMaxEntitiesSoftTTL)),
		}
	}

	// The soft TTL is when an entry becomes stale, the hard TTL is when it
	// expires, so a soft TTL past the hard one describes an entry that can
	// never go stale. Both bounds above pass independently for such a pair —
	// only comparing them catches it, and without this the service boots as
	// far as c3e's own check and then dies on "SoftTTL must be between 0 and
	// HardTTL", which names neither flag.
	if c.EntitiesSoftTTL.Value > c.EntitiesHardTTL.Value {
		return &InvalidConfigurationError{
			Field:   "cache.entities.soft.ttl",
			Value:   c.EntitiesSoftTTL.Value.String(),
			Message: "invalid cache entities soft ttl, must not exceed cache.entities.hard.ttl (" + c.EntitiesHardTTL.Value.String() + ")",
		}
	}

	if c.TTLJitterPercent.Value < domain.ValidCacheMinTTLJitterPercent || c.TTLJitterPercent.Value > domain.ValidCacheMaxTTLJitterPercent {
		return &InvalidConfigurationError{
			Field:   "cache.ttl.jitter.percent",
			Value:   fmt.Sprintf("%f", c.TTLJitterPercent.Value),
			Message: fmt.Sprintf("invalid cache ttl jitter percent, must be between %f and %f", domain.ValidCacheMinTTLJitterPercent, domain.ValidCacheMaxTTLJitterPercent),
		}
	}

	// TLS files are only meaningful when TLS is on. Accepting them silently
	// while the connection stays in cleartext is how an operator ends up
	// believing a link is encrypted because they configured a CA for it.
	if !c.TLSEnabled.Value {
		for field, value := range map[string]string{
			"cache.tls.ca.file":   c.TLSCAFile.Value,
			"cache.tls.cert.file": c.TLSCertFile.Value,
			"cache.tls.key.file":  c.TLSKeyFile.Value,
		} {
			if value != "" {
				return &InvalidConfigurationError{
					Field:   field,
					Value:   value,
					Message: "TLS files are configured but cache.tls.enabled is false; the connection would still be cleartext",
				}
			}
		}

		if c.TLSInsecureSkipVerify.Value {
			return &InvalidConfigurationError{
				Field:   "cache.tls.insecure.skip.verify",
				Value:   "true",
				Message: "cache.tls.insecure.skip.verify has no meaning while cache.tls.enabled is false",
			}
		}
	}

	if c.TLSEnabled.Value {
		// A certificate without its key, or the reverse, cannot produce a
		// keypair — better to say so than to fail inside the TLS handshake.
		if (c.TLSCertFile.Value == "") != (c.TLSKeyFile.Value == "") {
			return &InvalidConfigurationError{
				Field:   "cache.tls.cert.file",
				Value:   c.TLSCertFile.Value,
				Message: "cache.tls.cert.file and cache.tls.key.file must be set together",
			}
		}

		if c.TLSInsecureSkipVerify.Value && c.TLSCAFile.Value != "" {
			return &InvalidConfigurationError{
				Field:   "cache.tls.insecure.skip.verify",
				Value:   "true",
				Message: "a CA file is configured but verification is disabled; one of the two is a mistake",
			}
		}

		for field, value := range map[string]string{
			"cache.tls.ca.file":   c.TLSCAFile.Value,
			"cache.tls.cert.file": c.TLSCertFile.Value,
			"cache.tls.key.file":  c.TLSKeyFile.Value,
		} {
			if value == "" {
				continue
			}

			if _, err := os.Stat(value); err != nil {
				return &InvalidConfigurationError{
					Field:   field,
					Value:   value,
					Message: "TLS file cannot be read: " + err.Error(),
				}
			}
		}
	}

	encoderType := domain.CacheEncoderType(c.EncoderType.Value)
	if !encoderType.IsValid() {
		return &InvalidConfigurationError{
			Field:   "cache.encoder.type",
			Value:   c.EncoderType.Value,
			Message: "invalid cache encoder type, must be [json]; gob was removed because it cannot distinguish a nil pointer from a pointer to a zero value",
		}
	}

	return nil
}
