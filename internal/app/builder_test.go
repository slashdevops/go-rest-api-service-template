package app

import (
	"context"
	"testing"
)

// TestNewAppBuilder_BasicConstruction tests that AppBuilder can create an App instance
func TestNewAppBuilder_BasicConstruction(t *testing.T) {
	t.Run("creates_builder_with_context", func(t *testing.T) {
		ctx := context.Background()
		builder := NewAppBuilder(ctx)

		if builder == nil {
			t.Fatal("expected builder to be created, got nil")
		}

		if builder.ctx != ctx {
			t.Error("expected builder context to match provided context")
		}
	})
}

// TestAppBuilder_ConfigurationMethods tests the builder's fluent API
func TestAppBuilder_ConfigurationMethods(t *testing.T) {
	t.Run("skip_database", func(t *testing.T) {
		builder := NewAppBuilder(context.Background())
		builder.SkipDatabase()

		if !builder.skipDatabase {
			t.Error("expected skipDatabase to be true")
		}
	})

	t.Run("skip_telemetry", func(t *testing.T) {
		builder := NewAppBuilder(context.Background())
		builder.SkipTelemetry()

		if !builder.skipTelemetry {
			t.Error("expected skipTelemetry to be true")
		}
	})

	t.Run("skip_mail", func(t *testing.T) {
		builder := NewAppBuilder(context.Background())
		builder.SkipMail()

		if !builder.skipMail {
			t.Error("expected skipMail to be true")
		}
	})

	t.Run("fluent_api_chaining", func(t *testing.T) {
		builder := NewAppBuilder(context.Background()).
			SkipDatabase().
			SkipTelemetry().
			SkipMail()

		if !builder.skipDatabase || !builder.skipTelemetry || !builder.skipMail {
			t.Error("expected all skip flags to be true when chained")
		}
	})
}

// TestTestAppBuilder_MinimalConfig tests the test helper functions
func TestTestAppBuilder_MinimalConfig(t *testing.T) {
	t.Run("creates_test_builder", func(t *testing.T) {
		testBuilder := NewTestApp(t)

		if testBuilder == nil {
			t.Fatal("expected test builder to be created, got nil")
		}

		if testBuilder.builder == nil {
			t.Error("expected internal builder to be initialized")
		}
	})

	t.Run("minimal_config_sets_test_defaults", func(t *testing.T) {
		testBuilder := NewTestApp(t).WithMinimalConfig()

		if testBuilder.builder.configs == nil {
			t.Fatal("expected configs to be set")
		}

		configs := testBuilder.builder.configs
		if configs.Database.MigrationEnable.Value != false {
			t.Error("expected migrations to be disabled in test config")
		}

		if configs.HTTPServer.Port.Value != 0 {
			t.Error("expected HTTP port to be 0 (random) in test config")
		}

		if configs.Cache.Enabled.Value != false {
			t.Error("expected cache to be disabled in test config")
		}
	})
}

// TestAppBuilder_DependencyInjection tests that dependencies can be injected
func TestAppBuilder_DependencyInjection(t *testing.T) {
	t.Run("inject_repositories", func(t *testing.T) {
		mockRepos := &Repositories{}
		builder := NewAppBuilder(context.Background()).
			WithRepositories(mockRepos)

		if builder.repositories != mockRepos {
			t.Error("expected repositories to be set")
		}
	})

	t.Run("inject_services", func(t *testing.T) {
		mockServices := &Services{}
		builder := NewAppBuilder(context.Background()).
			WithServices(mockServices)

		if builder.services != mockServices {
			t.Error("expected services to be set")
		}
	})

	t.Run("inject_handlers", func(t *testing.T) {
		mockHandlers := &Handlers{}
		builder := NewAppBuilder(context.Background()).
			WithHandlers(mockHandlers)

		if builder.handlers != mockHandlers {
			t.Error("expected handlers to be set")
		}
	})
}
