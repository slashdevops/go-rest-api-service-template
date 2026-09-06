package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slashdevops/mailer"
	"github.com/valkey-io/valkey-go"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/ratelimitmemory"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/throttlememory"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/server"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/changenotify"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/ratelimit"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/token"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

const (
	appName    = "go-rest-api-service-template"
	apiVersion = "v1"
)

var apiPrefix = fmt.Sprintf("api/%s", apiVersion)

// App represents the application and manages its lifecycle.
// It follows a layered architecture:
//
// Initialization order (dependencies flow downward):
//  1. Configuration - Load from flags and environment
//  2. Telemetry - Initialize observability (tracing, metrics)
//  3. Database - Setup connection pool and run migrations
//  4. Repositories - Data access layer
//  5. Mail Service - Email notification system
//  6. Services - Business logic layer (depends on repositories and mail)
//  7. Handlers - HTTP request handlers (depends on services)
//  8. HTTP Server - Setup routes and middleware
//
// For testing, use NewAppBuilder to inject mock dependencies.
type App struct {
	// A ratelimit.Limiter, not the concrete Valkey adapter: it is wrapped in a
	// circuit breaker, so the concrete type is no longer what is stored.
	rateLimitShared ratelimit.Limiter

	// rateLimitNotifier propagates a rule write to the other replicas. nil
	// without a cache, which is a supported deployment -- the reload ticker is
	// the floor and this only removes the wait.
	rateLimitNotifier ratelimit.ChangeNotifier

	// tokenLifetimesNotifier propagates a PUT /auth/token_lifetimes to the
	// other replicas. Same shape and same "nil without a cache" as the
	// rate-limit one.
	tokenLifetimesNotifier changenotify.Notifier

	// changeNotifyValkey is the subscriber client every change notifier shares.
	// Separate from cacheClient because a subscribed connection accepts
	// nothing else; owned here so shutdown closes it once.
	changeNotifyValkey valkey.Client

	// The JWT signer, held so the HTTP middleware's validators verify tokens
	// through the same routine the use-cases do. The middleware used to carry
	// its own implementation, and the two disagreed about what to check.
	tokenSigner token.Signer

	// Valkey client backing the cache (nil when cache.enabled is false).
	// Held here for two reasons the cache.Cache port cannot serve: the
	// connection pool and its background goroutines have to be closed on
	// shutdown, and the health check needs to PING the server directly.
	cacheClient valkey.Client

	// Configuration loaded from flags and environment
	configs *Configs

	// Core infrastructure components
	telemetry *o11y.OpenTelemetry // Observability (tracing, metrics)
	dbPool    *pgxpool.Pool       // Database connection pool

	// httpClient is the ONE outbound client, shared by every integration that
	// leaves this process -- the IdPs, and the mail API
	// sender. Built once so a timeout or retry policy changed in config reaches
	// all of them, rather than each growing its own and drifting.
	httpClient *http.Client

	// HTTP servers
	httpServer  *server.HTTPServer  // Main API server
	mailServer  *mailer.MailService // Email service with worker pool
	pprofServer *http.Server        // Profiling server (pprof)

	// Per-IP request rate limiter (nil when disabled). Owns a background
	// eviction goroutine that is stopped in Shutdown via Close.
	loginThrottle *throttlememory.Throttle

	// The rule-driven limiters. rateLimitLocal is always present when rules are
	// enforced — it is the limiter when cache.enabled is false, and the layer in
	// front of the shared counter when it is not. rateLimitShared is nil without
	// a cache client, and every rule is then per replica.
	rateLimitLocal   *ratelimitmemory.Adapter
	rateLimitMetrics *middleware.RateLimitMetrics

	// Application layers (organized by architectural layer)
	repositories *Repositories // Data access layer
	services     *Services     // Business logic layer
	handlers     *Handlers     // HTTP presentation layer

	// Cached cryptographic keys (loaded once from files)
	authKeys *authKeys // JWT and symmetric keys for authentication

	// Metrics and observability
	startupMetrics *StartupMetrics // Startup performance metrics

	// Lifecycle management
	shutdownCh chan struct{} // Channel to signal shutdown

	// replicaID names this process in the change messages it publishes.
	replicaID string

	shutdownOnce sync.Once // Ensures shutdown only happens once
}

// authKeys holds the cryptographic keys used for authentication
// This struct caches keys to avoid repeated file reads
type authKeys struct {
	jwtPrivateKey []byte // EC (P-256) private key that signs tokens
	jwtPublicKey  []byte // EC (P-256) public half of jwtPrivateKey

	// Keys that may verify a token but never sign one. Empty in the ordinary
	// case; non-empty only while a signing key is being rotated, so that a new
	// key can verify before it signs and an old one after it stops.
	jwtAdditionalPublicKeys [][]byte

	symmetricKey []byte // Symmetric key for encrypting sensitive data
}

// NewApp creates a new application instance
// This is a convenience wrapper around NewAppBuilder for backward compatibility
func NewApp(ctx context.Context) (*App, error) {
	return NewAppBuilder(ctx).Build()
}

// Run starts the application and blocks until ctx is cancelled, a SIGINT
// or SIGTERM is received, or Shutdown is invoked. The signal that
// triggered termination (when applicable) is recorded via context.Cause.
func (a *App) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a.reconcileResourceUsage(ctx)
	a.startRevokedTokensSweeper(ctx)

	// Fatal on failure: see startRevokedAccessTokensMirror. A process that
	// cannot build the denylist must not serve as though nothing is revoked.
	if err := a.startRevokedAccessTokensMirror(ctx); err != nil {
		return err
	}

	// Fatal, and it must be BEFORE the server starts serving. The rule set is
	// the only source of budgets now, so a replica that begins accepting
	// requests without it is a replica limiting nothing.
	if err := a.startRateLimitRulesMirror(ctx); err != nil {
		return err
	}

	// Fatal, before serving, for the same reason: the row is the only source
	// of token lifetimes, and a login served before it loaded would have
	// nothing to sign with.
	if err := a.startTokenLifetimesMirror(ctx); err != nil {
		return err
	}

	// Start HTTP server
	go a.httpServer.Start()
	go a.mailServer.Start()

	// Start pprof server if enabled
	if a.configs.HTTPServer.PprofEnabled.Value {
		go a.startPprofServer()
	}

	// Wait for shutdown signal
	select {
	case <-ctx.Done():
		slog.Info("received shutdown signal", "cause", context.Cause(ctx))
	case <-a.shutdownCh:
		slog.Info("shutdown requested")
	}

	return a.Shutdown()
}

// Shutdown gracefully shuts down the application
func (a *App) Shutdown() error {
	var shutdownErr error

	a.shutdownOnce.Do(func() {
		// 1. Shutdown HTTP server with a timeout context for graceful shutdown
		slog.Info("shutting down HTTP server")

		// Setup a timeout context for shutdown operations
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Stop the HTTP server
		a.httpServer.Stop()

		// Wait for server to shut down completely with timeout
		select {
		case <-a.httpServer.Wait():
			slog.Info("HTTP server shut down successfully")
		case <-ctx.Done():
			slog.Warn("HTTP server shutdown timed out")
		}

		// The rule limiters hold no goroutine of their own -- the sweeper and the
		// mirror both stop with the run context -- but Close is called anyway so
		// that a future implementation which does hold one is stopped without
		// anybody having to remember this file exists.
		if a.rateLimitLocal != nil {
			if err := a.rateLimitLocal.Close(); err != nil {
				slog.Error("error stopping the per-replica rate limiter", "error", err)
			}
		}

		if a.rateLimitShared != nil {
			// Deliberately does NOT close the Valkey client: it is shared with
			// the cache and owned by initCacheClient. Closing it here would take
			// the cache down with it.
			if err := a.rateLimitShared.Close(); err != nil {
				slog.Error("error stopping the shared rate limiter", "error", err)
			}
		}

		// The notifier DOES own its client -- it was built separately, because a
		// subscribed connection accepts nothing else -- so closing it here is
		// closing something this component owns, not the shared cache client.
		if a.rateLimitNotifier != nil {
			if err := a.rateLimitNotifier.Close(); err != nil {
				slog.Error("error stopping the rate-limit change notifier", "error", err)
			}
		}

		if a.tokenLifetimesNotifier != nil {
			if err := a.tokenLifetimesNotifier.Close(); err != nil {
				slog.Error("error stopping the token lifetimes change notifier", "error", err)
			}
		}

		// Both notifiers share this client; each Close above is idempotent on
		// it, and this is the one that owns it.
		if a.changeNotifyValkey != nil {
			a.changeNotifyValkey.Close()
		}

		// Stop the login throttle's background eviction goroutine
		if a.loginThrottle != nil {
			slog.Info("stopping login throttle")
			if err := a.loginThrottle.Close(); err != nil {
				slog.Error("error stopping login throttle", "error", err)
			}
		}

		// 2. Shutdown pprof server if running
		if a.pprofServer != nil {
			slog.Info("shutting down pprof server")
			if err := a.pprofServer.Shutdown(context.Background()); err != nil {
				slog.Error("error shutting down pprof server", "error", err)
			}
		}

		// 3. Close the cache client. After the HTTP server has drained, so no
		// request can still be reading through it, and before the database so
		// a late cache miss can still be served. Nothing used to close this:
		// the connection pool, its background goroutines and — with
		// cache.enable.on.client — the client-side tracking subscription were
		// all left running at exit.
		if a.cacheClient != nil {
			slog.Info("closing cache client")
			a.cacheClient.Close()
		}

		// 4. Close database connection
		slog.Info("closing database connection")
		a.dbPool.Close()

		// Stop the mail service
		slog.Info("stopping mail service")
		a.mailServer.Stop()

		// 5. Shutdown telemetry
		slog.Info("shutting down telemetry")
		a.telemetry.Shutdown()

		close(a.shutdownCh)
	})

	return shutdownErr
}
