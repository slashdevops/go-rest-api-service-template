package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slashdevops/mailer"

	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// AppBuilder provides a flexible way to construct an App instance
// with optional dependency injection for testing
type AppBuilder struct {
	ctx context.Context

	// Optional overrides for testing
	configs      *Configs
	telemetry    *o11y.OpenTelemetry
	dbPool       *pgxpool.Pool
	mailServer   *mailer.MailService
	repositories *Repositories
	services     *Services
	handlers     *Handlers

	// Flags to control initialization
	skipDatabase   bool
	skipMigrations bool
	skipTelemetry  bool
	skipMail       bool
}

// NewAppBuilder creates a new AppBuilder with default settings
func NewAppBuilder(ctx context.Context) *AppBuilder {
	return &AppBuilder{
		ctx: ctx,
	}
}

// WithConfigs sets the configuration to use
func (b *AppBuilder) WithConfigs(configs *Configs) *AppBuilder {
	b.configs = configs
	return b
}

// WithTelemetry injects a telemetry instance (useful for testing)
func (b *AppBuilder) WithTelemetry(telemetry *o11y.OpenTelemetry) *AppBuilder {
	b.telemetry = telemetry
	return b
}

// WithDatabase injects a database pool (useful for testing)
func (b *AppBuilder) WithDatabase(dbPool *pgxpool.Pool) *AppBuilder {
	b.dbPool = dbPool
	return b
}

// WithMailServer injects a mail server (useful for testing)
func (b *AppBuilder) WithMailServer(mailServer *mailer.MailService) *AppBuilder {
	b.mailServer = mailServer
	return b
}

// WithRepositories injects repositories (useful for testing)
func (b *AppBuilder) WithRepositories(repositories *Repositories) *AppBuilder {
	b.repositories = repositories
	return b
}

// WithServices injects services (useful for testing)
func (b *AppBuilder) WithServices(services *Services) *AppBuilder {
	b.services = services
	return b
}

// WithHandlers injects handlers (useful for testing)
func (b *AppBuilder) WithHandlers(handlers *Handlers) *AppBuilder {
	b.handlers = handlers
	return b
}

// SkipDatabase skips database initialization (useful for testing)
func (b *AppBuilder) SkipDatabase() *AppBuilder {
	b.skipDatabase = true
	return b
}

// SkipMigrations skips database migrations (useful for testing)
func (b *AppBuilder) SkipMigrations() *AppBuilder {
	b.skipMigrations = true
	return b
}

// SkipTelemetry skips telemetry initialization (useful for testing)
func (b *AppBuilder) SkipTelemetry() *AppBuilder {
	b.skipTelemetry = true
	return b
}

// SkipMail skips mail service initialization (useful for testing)
func (b *AppBuilder) SkipMail() *AppBuilder {
	b.skipMail = true
	return b
}

// Build constructs the App instance with the configured options
func (b *AppBuilder) Build() (*App, error) {
	startTime := time.Now()
	app := &App{
		shutdownCh: make(chan struct{}),
		startupMetrics: &StartupMetrics{
			startTime: startTime,
		},
	}

	var err error
	metrics := newInitMetrics()

	// Load or use provided configs
	if b.configs == nil {
		configStart := time.Now()
		app.configs, err = LoadConfigs()
		if err != nil {
			return nil, err
		}
		app.startupMetrics.ConfigLoadDuration = time.Since(configStart)
		metrics.recordPhase("config_load", app.startupMetrics.ConfigLoadDuration)
	} else {
		app.configs = b.configs
	}

	// Initialize telemetry
	if !b.skipTelemetry {
		if b.telemetry == nil {
			telemetryStart := time.Now()
			if err := app.initTelemetry(b.ctx); err != nil {
				return nil, err
			}
			app.startupMetrics.TelemetryDuration = time.Since(telemetryStart)
			metrics.recordPhase("telemetry", app.startupMetrics.TelemetryDuration)
		} else {
			app.telemetry = b.telemetry
		}
	}

	// Initialize database
	if !b.skipDatabase {
		if b.dbPool == nil {
			dbStart := time.Now()
			if err := app.initDatabase(b.ctx); err != nil {
				return nil, err
			}
			app.startupMetrics.DatabaseDuration = time.Since(dbStart)
			metrics.recordPhase("database", app.startupMetrics.DatabaseDuration)
			// Log database pool statistics
			app.logDatabasePoolStats()
		} else {
			app.dbPool = b.dbPool
		}
	}

	// Initialize repositories
	if b.repositories == nil {
		reposStart := time.Now()
		if err := app.initRepositories(); err != nil {
			return nil, err
		}
		app.startupMetrics.RepositoriesDuration = time.Since(reposStart)
		metrics.recordPhase("repositories", app.startupMetrics.RepositoriesDuration)
	} else {
		app.repositories = b.repositories
	}

	// With the pool up and before anything serves: a deployment that kept the
	// seeded administrator's password does not get to serve at all.
	if err := app.checkSeededAdmin(b.ctx); err != nil {
		return nil, err
	}

	// The HTTP client comes before the mail service, which needs it for the API
	// sender, and before services, which need it for the IdPs.
	app.httpClient = app.initHTTPClient()

	// Initialize mail service
	if !b.skipMail {
		if b.mailServer == nil {
			mailStart := time.Now()
			if err := app.initMailService(b.ctx); err != nil {
				return nil, err
			}
			app.startupMetrics.MailServiceDuration = time.Since(mailStart)
			metrics.recordPhase("mail_service", app.startupMetrics.MailServiceDuration)
		} else {
			app.mailServer = b.mailServer
		}
	}

	// Initialize services
	if b.services == nil {
		servicesStart := time.Now()
		if err := app.initServices(b.ctx); err != nil {
			return nil, err
		}
		app.startupMetrics.ServicesDuration = time.Since(servicesStart)
		metrics.recordPhase("services", app.startupMetrics.ServicesDuration)
	} else {
		app.services = b.services
	}

	// Wire up the health provider (after services are initialized to avoid circular dependency)
	if app.services.Health != nil {
		app.services.Health.SetAppHealthProvider(app)
	}

	// Initialize handlers
	if b.handlers == nil {
		handlersStart := time.Now()
		if err := app.initHandlers(); err != nil {
			return nil, err
		}
		app.startupMetrics.HandlersDuration = time.Since(handlersStart)
		metrics.recordPhase("handlers", app.startupMetrics.HandlersDuration)
	} else {
		app.handlers = b.handlers
	}

	// Initialize HTTP server
	httpServerStart := time.Now()
	if err := app.initHTTPServer(b.ctx); err != nil {
		return nil, err
	}
	app.startupMetrics.HTTPServerDuration = time.Since(httpServerStart)
	metrics.recordPhase("http_server", app.startupMetrics.HTTPServerDuration)

	// Record total startup time and log summary
	app.startupMetrics.TotalDuration = time.Since(startTime)
	metrics.logSummary()

	return app, nil
}
