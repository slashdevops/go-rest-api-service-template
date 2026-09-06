package app

import (
	"fmt"
	"log/slog"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/repositorypg"
)

// initRepositories initializes all repositories
func (a *App) initRepositories() error {
	a.repositories = &Repositories{}

	slog.Info("initializing repositories")

	var err error

	a.repositories.Health, err = repositorypg.NewHealthRepository(
		repositorypg.HealthRepositoryConfig{
			DB:             a.dbPool,
			MaxPingTimeout: a.configs.Database.MaxPingTimeout.Value,
			OT:             a.telemetry,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create health repository: %w", err)
	}

	// since this is required for other service must create first
	a.repositories.ResourcesLimits, err = repositorypg.NewResourcesLimitsRepository(
		repositorypg.ResourcesLimitsRepositoryConfig{
			DB:              a.dbPool,
			MaxPingTimeout:  a.configs.Database.MaxPingTimeout.Value,
			MaxQueryTimeout: a.configs.Database.MaxQueryTimeout.Value,
			OT:              a.telemetry,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create resources limits repository: %w", err)
	}

	a.repositories.Users, err = repositorypg.NewUsersRepository(
		repositorypg.UsersRepositoryConfig{
			DB:              a.dbPool,
			MaxPingTimeout:  a.configs.Database.MaxPingTimeout.Value,
			MaxQueryTimeout: a.configs.Database.MaxQueryTimeout.Value,
			OT:              a.telemetry,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create users repository: %w", err)
	}

	a.repositories.Projects, err = repositorypg.NewProjectsRepository(
		repositorypg.ProjectsRepositoryConfig{
			DB:              a.dbPool,
			MaxPingTimeout:  a.configs.Database.MaxPingTimeout.Value,
			MaxQueryTimeout: a.configs.Database.MaxQueryTimeout.Value,
			OT:              a.telemetry,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create projects repository: %w", err)
	}

	a.repositories.Products, err = repositorypg.NewProductsRepository(
		repositorypg.ProductsRepositoryConfig{
			DB:              a.dbPool,
			MaxPingTimeout:  a.configs.Database.MaxPingTimeout.Value,
			MaxQueryTimeout: a.configs.Database.MaxQueryTimeout.Value,
			OT:              a.telemetry,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create products repository: %w", err)
	}

	a.repositories.Policies, err = repositorypg.NewPoliciesRepository(
		repositorypg.PoliciesRepositoryConfig{
			DB:              a.dbPool,
			MaxPingTimeout:  a.configs.Database.MaxPingTimeout.Value,
			MaxQueryTimeout: a.configs.Database.MaxQueryTimeout.Value,
			OT:              a.telemetry,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create policies repository: %w", err)
	}

	a.repositories.Resources, err = repositorypg.NewResourcesRepository(
		repositorypg.ResourcesRepositoryConfig{
			DB:              a.dbPool,
			MaxPingTimeout:  a.configs.Database.MaxPingTimeout.Value,
			MaxQueryTimeout: a.configs.Database.MaxQueryTimeout.Value,
			OT:              a.telemetry,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create resources repository: %w", err)
	}

	a.repositories.Roles, err = repositorypg.NewRolesRepository(
		repositorypg.RolesRepositoryConfig{
			DB:              a.dbPool,
			MaxPingTimeout:  a.configs.Database.MaxPingTimeout.Value,
			MaxQueryTimeout: a.configs.Database.MaxQueryTimeout.Value,
			OT:              a.telemetry,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create roles repository: %w", err)
	}

	a.repositories.RevokedTokens, err = repositorypg.NewRevokedTokensRepository(
		repositorypg.RevokedTokensRepositoryConfig{
			DB:              a.dbPool,
			MaxPingTimeout:  a.configs.Database.MaxPingTimeout.Value,
			MaxQueryTimeout: a.configs.Database.MaxQueryTimeout.Value,
			OT:              a.telemetry,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create revoked tokens repository: %w", err)
	}

	a.repositories.UsersIdentities, err = repositorypg.NewUsersIdentitiesRepository(
		repositorypg.UsersIdentitiesRepositoryConfig{
			DB:              a.dbPool,
			MaxPingTimeout:  a.configs.Database.MaxPingTimeout.Value,
			MaxQueryTimeout: a.configs.Database.MaxQueryTimeout.Value,
			OT:              a.telemetry,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create users identities repository: %w", err)
	}

	a.repositories.RateLimits, err = repositorypg.NewRateLimitsRepository(
		repositorypg.RateLimitsRepositoryConfig{
			DB:              a.dbPool,
			MaxPingTimeout:  a.configs.Database.MaxPingTimeout.Value,
			MaxQueryTimeout: a.configs.Database.MaxQueryTimeout.Value,
			OT:              a.telemetry,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create rate limits repository: %w", err)
	}

	a.repositories.TokenLifetimes, err = repositorypg.NewTokenLifetimesRepository(
		repositorypg.TokenLifetimesRepositoryConfig{
			DB:              a.dbPool,
			MaxPingTimeout:  a.configs.Database.MaxPingTimeout.Value,
			MaxQueryTimeout: a.configs.Database.MaxQueryTimeout.Value,
			OT:              a.telemetry,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create token lifetimes repository: %w", err)
	}

	a.repositories.IDPTypes, err = repositorypg.NewIDPTypesRepository(
		repositorypg.IDPTypesRepositoryConfig{
			DB:              a.dbPool,
			MaxPingTimeout:  a.configs.Database.MaxPingTimeout.Value,
			MaxQueryTimeout: a.configs.Database.MaxQueryTimeout.Value,
			OT:              a.telemetry,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create IDP types repository: %w", err)
	}

	a.repositories.IDPs, err = repositorypg.NewIDPsRepository(
		repositorypg.IDPsRepositoryConfig{
			DB:              a.dbPool,
			MaxPingTimeout:  a.configs.Database.MaxPingTimeout.Value,
			MaxQueryTimeout: a.configs.Database.MaxQueryTimeout.Value,
			OT:              a.telemetry,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create IDPs repository: %w", err)
	}

	slog.Info("repositories initialized successfully")
	return nil
}
