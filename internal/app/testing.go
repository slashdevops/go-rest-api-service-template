package app

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slashdevops/mailer"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// TestAppBuilder provides a convenient way to build App instances for testing
type TestAppBuilder struct {
	builder *AppBuilder
	t       *testing.T
}

// NewTestApp creates a new TestAppBuilder for testing purposes
func NewTestApp(t *testing.T) *TestAppBuilder {
	return &TestAppBuilder{
		builder: NewAppBuilder(context.Background()),
		t:       t,
	}
}

// WithMinimalConfig creates a minimal configuration for testing
func (tb *TestAppBuilder) WithMinimalConfig() *TestAppBuilder {
	configs := &Configs{
		Log:        config.NewLogConfig(),
		HTTPServer: config.NewHTTPServerConfig(),
		HTTPClient: config.NewHTTPClientConfig(),
		Database:   config.NewDatabaseConfig(),
		Cache:      config.NewCacheConfig(),
		Telemetry:  config.NewOpenTelemetryConfig("test-app", "0.0.0-test"),
		Authn:      config.NewAuthConfig(),
		Mail:       config.NewMailConfig(),
	}

	// Set test defaults
	configs.Database.MigrationEnable.Value = false
	configs.HTTPServer.Port.Value = 0 // Random port for testing
	configs.Cache.Enabled.Value = false

	tb.builder.WithConfigs(configs)
	return tb
}

// WithMockDatabase injects a mock database pool
func (tb *TestAppBuilder) WithMockDatabase(dbPool *pgxpool.Pool) *TestAppBuilder {
	tb.builder.WithDatabase(dbPool)
	return tb
}

// WithMockTelemetry injects a mock telemetry instance
func (tb *TestAppBuilder) WithMockTelemetry(telemetry *o11y.OpenTelemetry) *TestAppBuilder {
	tb.builder.WithTelemetry(telemetry)
	return tb
}

// WithMockMailServer injects a mock mail server
func (tb *TestAppBuilder) WithMockMailServer(mailServer *mailer.MailService) *TestAppBuilder {
	tb.builder.WithMailServer(mailServer)
	return tb
}

// WithMockRepositories injects mock repositories
func (tb *TestAppBuilder) WithMockRepositories(repositories *Repositories) *TestAppBuilder {
	tb.builder.WithRepositories(repositories)
	return tb
}

// WithMockServices injects mock services
func (tb *TestAppBuilder) WithMockServices(services *Services) *TestAppBuilder {
	tb.builder.WithServices(services)
	return tb
}

// WithMockHandlers injects mock handlers
func (tb *TestAppBuilder) WithMockHandlers(handlers *Handlers) *TestAppBuilder {
	tb.builder.WithHandlers(handlers)
	return tb
}

// SkipDatabase skips database initialization
func (tb *TestAppBuilder) SkipDatabase() *TestAppBuilder {
	tb.builder.SkipDatabase()
	return tb
}

// SkipTelemetry skips telemetry initialization
func (tb *TestAppBuilder) SkipTelemetry() *TestAppBuilder {
	tb.builder.SkipTelemetry()
	return tb
}

// SkipMail skips mail service initialization
func (tb *TestAppBuilder) SkipMail() *TestAppBuilder {
	tb.builder.SkipMail()
	return tb
}

// Build constructs the test App instance
func (tb *TestAppBuilder) Build() *App {
	app, err := tb.builder.Build()
	if err != nil {
		tb.t.Fatalf("failed to build test app: %v", err)
	}
	return app
}

// BuildWithError constructs the App and returns any error
func (tb *TestAppBuilder) BuildWithError() (*App, error) {
	return tb.builder.Build()
}

// NewMinimalTestApp creates a minimal test app with all external dependencies mocked
// This is useful for unit testing handlers and services in isolation
func NewMinimalTestApp(t *testing.T) *App {
	return NewTestApp(t).
		WithMinimalConfig().
		SkipDatabase().
		SkipTelemetry().
		SkipMail().
		Build()
}
