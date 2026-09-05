package app

import (
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/repositorypg"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/handler"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/usecase"
)

// Repositories holds all repository instances for data access layer.
// Repositories are responsible for interacting with the database and
// should not contain business logic.
//
// Initialization: Repositories are initialized first after the database
// connection is established, as they depend only on the database pool.
//
// For testing: Mock repository implementations can be injected via
// AppBuilder.WithRepositories().
type Repositories struct {
	Health          *repositorypg.HealthRepository
	Users           *repositorypg.UsersRepository
	Projects        *repositorypg.ProjectsRepository
	Products        *repositorypg.ProductsRepository
	Policies        *repositorypg.PoliciesRepository
	Resources       *repositorypg.ResourcesRepository
	Roles           *repositorypg.RolesRepository
	RevokedTokens   *repositorypg.RevokedTokensRepository
	IDPTypes        *repositorypg.IDPTypesRepository
	IDPs            *repositorypg.IDPsRepository
	ResourcesLimits *repositorypg.ResourcesLimitsRepository
	RateLimits      *repositorypg.RateLimitsRepository
}

// Services holds all service instances for business logic layer.
// Services contain the core business logic and orchestrate operations
// across multiple repositories.
//
// Initialization: Services are initialized after repositories and depend on:
//   - Repositories for data access
//   - Mail service for notifications
//   - Cache service for performance
//   - Cryptographic keys for security operations
//
// Dependencies between services:
//   - ResourcesLimits must be initialized first (used by other services)
//   - Users must be initialized before Authn
//   - Authz depends on Users service
//
// For testing: Mock service implementations can be injected via
// AppBuilder.WithServices().
type Services struct {
	Health          *usecase.HealthService
	Users           *usecase.UsersService
	Projects        *usecase.ProjectsService
	Products        *usecase.ProductsService
	Policies        *usecase.PoliciesService
	Resources       *usecase.ResourcesService
	Roles           *usecase.RolesService
	Authz           *usecase.AuthzService
	Authn           *usecase.AuthnService
	AuthnIDPs       *usecase.AuthnIDPsService
	IDPTypes        *usecase.IDPTypesService
	IDPs            *usecase.IDPsService
	ResourcesLimits *usecase.ResourcesLimitsService
	RateLimits      *usecase.RateLimitsService

	// RevokedAccessTokens is the in-memory denylist the auth middleware
	// consults on every authenticated request. nil when
	// authn.access.token.revocation.enabled is false, which is how the check
	// is switched off — see middleware.checkToken.
	RevokedAccessTokens *usecase.RevokedAccessTokens

	// RateLimitRules is the in-memory rule set the limiter middleware resolves
	// against. nil when ratelimit.enabled is false, which is how rule
	// enforcement is switched off — the flag limiter then runs alone.
	RateLimitRules *usecase.RateLimitRules
}

// Handlers holds all HTTP handler instances for the presentation layer.
// Handlers are responsible for HTTP request/response handling and should
// delegate business logic to services.
//
// Initialization: Handlers are initialized last and depend only on services
// and telemetry for observability.
//
// For testing: Mock handler implementations can be injected via
// AppBuilder.WithHandlers().
type Handlers struct {
	RateLimits      *handler.RateLimitsHandler
	Version         *handler.VersionHandler
	Health          *handler.HealthHandler
	Users           *handler.UsersHandler
	Projects        *handler.ProjectsHandler
	Products        *handler.ProductsHandler
	Policies        *handler.PoliciesHandler
	Resources       *handler.ResourcesHandler
	Roles           *handler.RolesHandler
	Swagger         *handler.SwaggerHandler
	Authn           *handler.AuthnHandler
	AuthnIDPs       *handler.AuthnIDPsHandler
	IDPTypes        *handler.IDPTypesHandler
	IDPs            *handler.IDPsHandler
	ResourcesLimits *handler.ResourcesLimitsHandler
	Me              *handler.MeHandler
}
