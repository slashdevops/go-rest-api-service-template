package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/slashdevops/httpx"
	"github.com/slashdevops/mailer"
	"github.com/valkey-io/valkey-go"

	"github.com/slashdevops/c3e"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/cachevalkey"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/cipheraes"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/notifieremail"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/oauthidp"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/policyopa"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/policyopa/rego"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/throttlememory"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/tokenjwt"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/cache"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/throttle"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/usecase"
)

// cacheConnWriteTimeout bounds a single Valkey command at the connection.
// See the ConnWriteTimeout comment in initCacheClient for why it is stated
// rather than inherited, and why it is this size.
const cacheConnWriteTimeout = 2 * time.Second

// cacheTLSConfig builds the TLS configuration for the cache client, or nil when
// TLS is disabled.
//
// TLS 1.3 minimum, matching the inbound server. The CA file is optional: a
// server with a publicly trusted certificate verifies against the host trust
// store, which is the common case for a managed cache.
func (a *App) cacheTLSConfig() (*tls.Config, error) {
	cfg := a.configs.Cache

	if !cfg.TLSEnabled.Value {
		return nil, nil
	}

	//nolint:gosec // InsecureSkipVerify is opt-in, rejected by Validate unless
	// TLS is on, and documented as testing-only. Refusing to implement it would
	// push operators to a worse workaround.
	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: cfg.TLSInsecureSkipVerify.Value,
	}

	if ca := cfg.TLSCAFile.Value; ca != "" {
		pem, err := os.ReadFile(ca)
		if err != nil {
			return nil, fmt.Errorf("reading cache TLS CA file %q: %w", ca, err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			// AppendCertsFromPEM reports failure by returning false and nothing
			// else, so a malformed file would otherwise fall through to a
			// handshake error that names neither the file nor the reason.
			return nil, fmt.Errorf("cache TLS CA file %q contains no usable certificate", ca)
		}

		tlsCfg.RootCAs = pool
	}

	if cfg.TLSCertFile.Value != "" {
		pair, err := tls.LoadX509KeyPair(cfg.TLSCertFile.Value, cfg.TLSKeyFile.Value)
		if err != nil {
			return nil, fmt.Errorf("loading cache TLS client keypair: %w", err)
		}

		tlsCfg.Certificates = []tls.Certificate{pair}
	}

	return tlsCfg, nil
}

// initServices initializes all service components of the application
func (a *App) initServices(ctx context.Context) error {
	a.services = &Services{}

	slog.Info("initializing services")

	// Create the common cache service to be used by other services
	var cacheService cache.Cache

	if a.configs.Cache.Enabled.Value {
		slog.Info("initializing cache client")
		cacheClient, err := a.initCacheClient()
		if err != nil {
			return fmt.Errorf("failed to initialize cache client: %w", err)
		}
		// Retained so Shutdown can close the pool and the health check can
		// PING; neither is reachable through the cache.Cache port.
		a.cacheClient = cacheClient

		cacheManager, err := c3e.NewCacheManager(cacheClient, a.configs.Cache.EnableOnClient.Value)
		if err != nil {
			return fmt.Errorf("failed to create cache manager: %w", err)
		}

		// json is the only encoder config.CacheConfig.Validate accepts, so this
		// is the only mapping reachable. Kept as a switch so adding a second
		// encoder is a change here and nowhere else.
		var encoderType c3e.CacheEncoderType

		switch domain.CacheEncoderType(strings.ToLower(a.configs.Cache.EncoderType.Value)) {
		case domain.CacheEncoderTypeJSON:
			encoderType = c3e.CacheEncoderTypeJSON
		default:
			encoderType = c3e.CacheEncoderTypeJSON
		}

		// Build the instruments before the manager: c3e reports the outcome of
		// every read through these hooks, which is the only place hit, stale,
		// miss and timeout can be told apart. Losing telemetry must not lose
		// the cache, so a failure here is a warning and Hooks() degrades to a
		// no-op on the nil value.
		cacheInstruments, err := cachevalkey.NewInstruments(a.telemetry.Metrics.Meter)
		if err != nil {
			slog.Warn("cache metrics unavailable, continuing without cache instrumentation", "error", err)
		}

		safeCacheManager, err := c3e.NewSafeCacheManager(cacheManager, c3e.SafeCacheManagerConfig{
			HardTTL:       a.configs.Cache.EntitiesHardTTL.Value,
			SoftTTL:       a.configs.Cache.EntitiesSoftTTL.Value,
			JitterPercent: a.configs.Cache.TTLJitterPercent.Value,
			EncoderType:   encoderType,
			QueryTimeout:  a.configs.Cache.MaxQueryTimeout.Value,
			Logger:        slog.Default().With("component", "cache"),
			Hooks:         cacheInstruments.Hooks(),
		})
		if err != nil {
			return fmt.Errorf("failed to create safe cache manager: %w", err)
		}
		cacheService = cachevalkey.New(safeCacheManager, cachevalkey.Config{
			InvalidateTimeout: a.configs.Cache.InvalidateTimeout.Value,
		})
	}

	// Load and cache JWT and symmetric keys
	slog.Info("loading authentication keys")
	keys, err := a.loadAuthKeys()
	if err != nil {
		return err
	}
	a.authKeys = keys

	// Initialize http client for API calls
	slog.Info("initializing HTTP client")
	// Already built, before the mail service, which also needs it.
	httpClient := a.httpClient

	// Initialize auth services
	mailService := a.mailServer // Initialize mail service first
	if err := a.initAuthServices(httpClient, keys, mailService, cacheService); err != nil {
		return err
	}

	slog.Info("services initialized successfully")
	return nil
}

// initCacheClient initializes the cache client based on configuration
func (a *App) initCacheClient() (valkey.Client, error) {
	switch a.configs.Cache.ServerKind.Value {
	case "valkey":
		valkeyConfig := valkey.ClientOption{
			InitAddress: a.configs.Cache.ServerAddresses.Value,
			Username:    a.configs.Cache.ServerUsername.Value,
			Password:    a.configs.Cache.ServerPassword.Value,
			SelectDB:    a.configs.Cache.ServerDB.Value,
			ClientName:  appName,
			// Disable auto-retry for read commands to prevent queueing when cache is down
			// This ensures immediate fallback to database instead of waiting for retries
			// this is necessary to ensure low latency during cache outages and the flag parameter
			// cache.max.query.timeout is respected
			DisableRetry: true,

			// The per-connection read/write deadline, and the only bound on a
			// command that c3e does not wrap in its own timeout — Set and
			// Invalidate run on the caller's context, and the HTTP server
			// deliberately sets no ReadTimeout or WriteTimeout, so a request
			// context has no deadline of its own.
			//
			// Left unset this defaults to max(TCPKeepAlive, KeepAlive) * 10,
			// roughly 10s, which is derived from keepalive tuning rather than
			// from anything about this workload. State it explicitly and keep
			// it in the same order of magnitude as the read budget: an
			// unresponsive Valkey should surface as a cache miss quickly, not
			// hold a write request open.
			ConnWriteTimeout: cacheConnWriteTimeout,
		}

		tlsCfg, err := a.cacheTLSConfig()
		if err != nil {
			return nil, err
		}

		valkeyConfig.TLSConfig = tlsCfg

		// Without TLS the AUTH password is sent in the clear on every
		// connection, and so is everything cached — including the bcrypt
		// password hash carried on *domain.User. Say so once at startup rather
		// than leaving it to be discovered from a packet capture.
		if tlsCfg == nil {
			slog.Warn(
				"cache connection is not encrypted; cached password hashes and the cache password cross the network in the clear",
				"enable_with", "cache.tls.enabled=true",
				"password_set", a.configs.Cache.ServerPassword.Value != "",
			)
		}

		return valkey.NewClient(valkeyConfig)
	default:
		return nil, nil
	}
}

// loadAuthKeys reads the JWT and symmetric keys from files
// Keys are cached in the App struct to avoid repeated file reads
func (a *App) loadAuthKeys() (*authKeys, error) {
	slog.Debug("loading JWT private key", "file", a.configs.Authn.PrivateKeyFile.Value.Name())
	// Read JWT private key
	jwtPrivateKey, err := os.ReadFile(a.configs.Authn.PrivateKeyFile.Value.Name())
	if err != nil {
		return nil, fmt.Errorf("error reading JWT private key file: %w", err)
	}

	slog.Debug("loading JWT public key", "file", a.configs.Authn.PublicKeyFile.Value.Name())
	// Read JWT public key
	jwtPublicKey, err := os.ReadFile(a.configs.Authn.PublicKeyFile.Value.Name())
	if err != nil {
		return nil, fmt.Errorf("error reading JWT public key file: %w", err)
	}

	// Additional verification keys. Absent unless a rotation is in progress.
	additionalPaths := a.configs.Authn.AdditionalPublicKeyFilesList()
	jwtAdditionalPublicKeys := make([][]byte, 0, len(additionalPaths))

	for _, path := range additionalPaths {
		slog.Debug("loading additional JWT verification key", "file", path)

		key, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("error reading additional JWT public key file %q: %w", path, err)
		}

		jwtAdditionalPublicKeys = append(jwtAdditionalPublicKeys, key)
	}

	slog.Debug("loading symmetric key", "file", a.configs.Authn.SymmetricKeyFile.Value.Name())
	// Read symmetric key
	symmetricHexKey, err := os.ReadFile(a.configs.Authn.SymmetricKeyFile.Value.Name())
	if err != nil {
		return nil, fmt.Errorf("error reading symmetric key file: %w", err)
	}

	// Process symmetric key
	symmetricHexKeyCleaned := strings.TrimRight(string(symmetricHexKey), "\n\r")
	symmetricKey, err := hex.DecodeString(symmetricHexKeyCleaned)
	if err != nil {
		return nil, fmt.Errorf("error decoding symmetric key: %w", err)
	}

	return &authKeys{
		jwtPrivateKey:           jwtPrivateKey,
		jwtPublicKey:            jwtPublicKey,
		jwtAdditionalPublicKeys: jwtAdditionalPublicKeys,
		symmetricKey:            symmetricKey,
	}, nil
}

// initHTTPClient initializes the HTTP client for API calls
func (a *App) initHTTPClient() *http.Client {
	slog.Info(
		"building HTTP client",
		"configured_timeout", a.configs.HTTPClient.Timeout.Value,
		"max_retries", a.configs.HTTPClient.MaxRetries.Value,
		"retry_strategy", a.configs.HTTPClient.RetryStrategy.Value,
	)

	client := httpx.NewClientBuilder().
		WithMaxIdleConns(a.configs.HTTPClient.MaxIdleConns.Value).
		WithMaxIdleConnsPerHost(a.configs.HTTPClient.MaxIdleConnsPerHost.Value).
		WithIdleConnTimeout(a.configs.HTTPClient.IdleConnTimeout.Value).
		WithTLSHandshakeTimeout(a.configs.HTTPClient.TLSHandshakeTimeout.Value).
		WithExpectContinueTimeout(a.configs.HTTPClient.ExpectContinueTimeout.Value).
		WithTimeout(a.configs.HTTPClient.Timeout.Value).
		WithDisableKeepAlive(a.configs.HTTPClient.DisableKeepAlives.Value).
		WithMaxRetries(a.configs.HTTPClient.MaxRetries.Value).
		WithRetryStrategyAsString(a.configs.HTTPClient.RetryStrategy.Value).
		WithLogger(slog.Default()).
		Build()

	slog.Info("HTTP client built", "actual_timeout", client.Timeout)

	return client
}

// initAuthServices initializes the authentication and authorization services
func (a *App) initAuthServices(
	httpClient *http.Client,
	keys *authKeys,
	mailService *mailer.MailService,
	cacheService cache.Cache,
) error {
	cipherAdapter, err := cipheraes.New(keys.symmetricKey)
	if err != nil {
		return fmt.Errorf("error creating cipher adapter: %w", err)
	}

	// The issuer is passed in because Verify requires iss and aud to equal it.
	// Both claims are written on every token this service mints and, until now,
	// were read by nothing: a token minted for another issuer — another
	// deployment sharing this key — was accepted on its signature alone.
	tokenSigner, err := tokenjwt.New(tokenjwt.Config{
		PrivateKey:           keys.jwtPrivateKey,
		PublicKey:            keys.jwtPublicKey,
		AdditionalPublicKeys: keys.jwtAdditionalPublicKeys,
		Issuer:               a.configs.Authn.Issuer.Value,
	})
	if err != nil {
		return fmt.Errorf("error creating token signer: %w", err)
	}

	// Which key signs, and which may verify, is invisible from a request and is
	// the only way to see where a rotation has reached. An overlap is worth a
	// warning rather than an info: it is a transitional state that someone has
	// to finish, and a keyset left holding a retired key indefinitely is a key
	// that never actually got retired.
	if verifyKeyIDs := tokenSigner.VerifyKeyIDs(); len(verifyKeyIDs) > 1 {
		slog.Warn("more than one JWT verification key is trusted; a signing key rotation is in progress and should be completed",
			"signingKeyID", tokenSigner.SigningKeyID(), "verifyKeyIDs", verifyKeyIDs)
	} else {
		slog.Info("JWT signing key loaded", "signingKeyID", tokenSigner.SigningKeyID())
	}

	// Held for the HTTP middleware, which verifies through this same signer.
	a.tokenSigner = tokenSigner

	// since this is required for other service must create first
	limitsPrivateKey, limitsPublicKey, err := a.loadResourceLimitsSigningKeys(keys)
	if err != nil {
		return err
	}

	a.services.ResourcesLimits, err = usecase.NewResourcesLimitsService(usecase.ResourcesLimitsServiceConf{
		Repository: a.repositories.ResourcesLimits,
		PrivateKey: limitsPrivateKey,
		PublicKey:  limitsPublicKey,
		OT:         a.telemetry,
	})
	if err != nil {
		return fmt.Errorf("error creating resources limits service: %w", err)
	}

	// Users service
	// needed by Authn service
	a.services.Users, err = usecase.NewUsersService(usecase.UsersServiceConf{
		Repository:      a.repositories.Users,
		CacheService:    cacheService,
		ResourcesLimits: a.services.ResourcesLimits,
		OT:              a.telemetry,
	})
	if err != nil {
		return fmt.Errorf("error creating users service: %w", err)
	}

	// Authz service
	policyEngine, err := policyopa.New(rego.RegoQuery, rego.RegoPolicy)
	if err != nil {
		return fmt.Errorf("error creating policy engine: %w", err)
	}
	a.services.Authz, err = usecase.NewAuthzService(usecase.AuthzServiceConf{
		UserService:  a.services.Users,
		PolicyEngine: policyEngine,
		OT:           a.telemetry,
	})
	if err != nil {
		return fmt.Errorf("error creating authz service: %w", err)
	}

	// Authn service
	emailNotifier := notifieremail.New(notifieremail.Config{
		Queue:                       mailService,
		Meter:                       a.telemetry.Metrics.Meter,
		SenderName:                  a.configs.Mail.SenderName.Value,
		SenderEmail:                 a.configs.Mail.SenderAddress.Value,
		UserVerificationWebEndpoint: a.configs.Authn.UserVerificationWebEndpoint.Value,
		UserResetPasswordEndpoint:   a.configs.Authn.UserResetPasswordEndpoint.Value,
	})
	// Bounds guessing against a single account, which the per-IP limiter cannot:
	// spread the same guesses over enough addresses and each one stays under its
	// own limit. Owned by the App so its eviction goroutine is stopped on
	// shutdown; left nil when disabled, which the use-case treats as no throttle.
	if a.configs.Authn.LoginThrottleEnabled.Value {
		slog.Warn(
			"login throttle enabled",
			"max_attempts", a.configs.Authn.LoginThrottleMaxAttempts.Value,
			"window", a.configs.Authn.LoginThrottleWindow.Value,
			"idle_after", a.configs.Authn.LoginThrottleIdleAfter.Value,
		)

		a.loginThrottle = throttlememory.New(throttlememory.Conf{
			MaxAttempts: a.configs.Authn.LoginThrottleMaxAttempts.Value,
			Window:      a.configs.Authn.LoginThrottleWindow.Value,
			IdleAfter:   a.configs.Authn.LoginThrottleIdleAfter.Value,
		})
	} else {
		slog.Warn("login throttle disabled; password guessing against a single account is bounded only by the per-IP rate limiter")
	}

	// Neither posture is visible from a request and they fail in opposite
	// directions: without rotation a stolen refresh token works until it
	// expires, and with it a client that discards the token it is handed is
	// locked out on its next refresh. Say which one is running.
	if a.configs.Authn.RefreshTokenRotationEnabled.Value {
		slog.Info("refresh token rotation enabled; every refresh revokes the token it consumed",
			"grace", a.configs.Authn.RefreshTokenRotationGrace.Value)
	} else {
		slog.Warn("refresh token rotation disabled; a stolen refresh token stays usable for its whole life and its reuse cannot be detected",
			"refreshTokenLifetime", "a runtime setting: GET /auth/token_lifetimes")
	}

	// Before the authn service, which takes it as a dependency: a logout has to
	// add the token it revoked to the local set before it answers.
	if err := a.initRevokedAccessTokens(); err != nil {
		return err
	}

	// Before the authn service too: it signs with whatever the mirror holds,
	// read at issuance. The row itself is loaded in Run, synchronously and
	// fatally, before the server accepts a request.
	if err := a.initTokenLifetimes(); err != nil {
		return err
	}

	// After the repositories and before the HTTP server: the middleware chain
	// takes the mirror as a dependency, so it has to exist by the time the
	// routes are registered.
	if err := a.initRateLimitRules(); err != nil {
		return err
	}

	// After initRateLimitRules, which is what supplies the mirror. The service
	// works without one -- a rule can be written before enforcement is switched
	// on, which is the sane order to roll this out in.
	a.services.RateLimits, err = usecase.NewRateLimitsService(usecase.RateLimitsServiceConf{
		Repository:          a.repositories.RateLimits,
		ResourcesRepository: a.repositories.Resources,
		RuleSet:             a.rateLimitRuleSetOrNil(),
		Notifier:            a.rateLimitNotifierOrNil(),
		OT:                  a.telemetry,
	})
	if err != nil {
		return err
	}

	a.services.Authn, err = usecase.NewAuthnService(usecase.AuthnServiceConf{
		UserService:               a.services.Users,
		CacheService:              cacheService,
		LoginThrottle:             a.loginThrottleOrNil(),
		RevokedTokens:             a.repositories.RevokedTokens,
		RevokedAccessTokens:       a.revokedAccessTokensOrNil(),
		Notifier:                  emailNotifier,
		TokenSigner:               tokenSigner,
		Issuer:                    a.configs.Authn.Issuer.Value,
		TokenLifetimes:            a.services.TokenLifetimesMirror,
		RefreshRotationEnabled:    a.configs.Authn.RefreshTokenRotationEnabled.Value,
		RefreshRotationGrace:      a.configs.Authn.RefreshTokenRotationGrace.Value,
		UserVerificationTokenTTL:  a.configs.Authn.UserVerificationTokenTTL.Value,
		UserResetPasswordTokenTTL: a.configs.Authn.UserResetPasswordTokenTTL.Value,
		OT:                        a.telemetry,
	})
	if err != nil {
		return fmt.Errorf("error creating authn service: %w", err)
	}

	a.services.IDPTypes, err = usecase.NewIDPTypesService(usecase.IDPTypesServiceConf{
		Repository: a.repositories.IDPTypes,
		OT:         a.telemetry,
	})
	if err != nil {
		return fmt.Errorf("error creating idp types service: %w", err)
	}

	a.services.IDPs, err = usecase.NewIDPsService(usecase.IDPsServiceConf{
		Repository:      a.repositories.IDPs,
		CacheService:    cacheService,
		ResourcesLimits: a.services.ResourcesLimits,
		Cipher:          cipherAdapter,
		OT:              a.telemetry,
	})
	if err != nil {
		return fmt.Errorf("error creating idps service: %w", err)
	}

	a.services.AuthnIDPs, err = usecase.NewAuthnIDPsService(usecase.AuthnIDPsServiceConf{
		AuthnService:  a.services.Authn,
		IDPsService:   a.services.IDPs,
		TokenSigner:   tokenSigner,
		OAuth:         oauthidp.New(),
		RevokedTokens: a.repositories.RevokedTokens,
		Issuer:        a.configs.Authn.Issuer.Value,
		OT:            a.telemetry,
	})
	if err != nil {
		return fmt.Errorf("error creating authn idps service: %w", err)
	}

	// Health service
	a.services.Health, err = usecase.NewHealthService(usecase.HealthServiceConf{
		Repository: a.repositories.Health,
		OT:         a.telemetry,
	})
	if err != nil {
		return fmt.Errorf("error creating health service: %w", err)
	}

	// Projects service
	a.services.Projects, err = usecase.NewProjectsService(usecase.ProjectsServiceConf{
		Repository:      a.repositories.Projects,
		CacheService:    cacheService,
		ResourcesLimits: a.services.ResourcesLimits,
		OT:              a.telemetry,
	})
	if err != nil {
		return fmt.Errorf("error creating projects service: %w", err)
	}

	// Products service. Deliberately without a CacheService: its reads are
	// tenant-scoped in SQL, which leaves no cache key that is both tenant-safe
	// and invalidatable (docs/architecture/caching.md).
	a.services.Products, err = usecase.NewProductsService(usecase.ProductsServiceConf{
		Repository:      a.repositories.Products,
		ResourcesLimits: a.services.ResourcesLimits,
		OT:              a.telemetry,
	})
	if err != nil {
		return fmt.Errorf("error creating products service: %w", err)
	}

	// Resources service
	a.services.Resources, err = usecase.NewResourcesService(usecase.ResourcesServiceConf{
		Repository:   a.repositories.Resources,
		CacheService: cacheService,
		OT:           a.telemetry,
	})
	if err != nil {
		return fmt.Errorf("error creating resources service: %w", err)
	}

	// Policies service
	a.services.Policies, err = usecase.NewPoliciesService(usecase.PoliciesServiceConf{
		Repository:       a.repositories.Policies,
		ResourcesService: a.services.Resources,
		CacheService:     cacheService,
		OT:               a.telemetry,
	})
	if err != nil {
		return fmt.Errorf("error creating policies service: %w", err)
	}

	// Roles service
	a.services.Roles, err = usecase.NewRolesService(usecase.RolesServiceConf{
		Repository:   a.repositories.Roles,
		CacheService: cacheService,
		OT:           a.telemetry,
	})
	if err != nil {
		return fmt.Errorf("error creating roles service: %w", err)
	}

	return nil
}

// loginThrottleOrNil returns the login throttle as the port interface, or a
// genuinely nil interface when there is none.
//
// Assigning a nil *throttlememory.Throttle straight to a throttle.Throttle
// field would produce an interface that is NOT nil — it carries a type with a
// nil value — so the use-case's `!= nil` guard would pass and then call a method
// on nothing. This is the one place that conversion happens, so it is the one
// place that has to get it right.
func (a *App) loginThrottleOrNil() throttle.Throttle {
	if a.loginThrottle == nil {
		return nil
	}

	return a.loginThrottle
}
