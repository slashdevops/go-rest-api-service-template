package payload

import "github.com/slashdevops/go-rest-api-service-template/internal/core/domain"

// Health-related types are domain-shaped (health-state info, not HTTP
// framing) and live in internal/core/domain. The payload package re-exports
// them as type aliases so existing handler/app/swagger references
// continue to work unchanged.

type (
	Status            = domain.Status
	Check             = domain.Check
	Health            = domain.Health
	ComponentHealth   = domain.ComponentHealth
	DatabasePoolStats = domain.DatabasePoolStats
	StartupMetrics    = domain.StartupMetrics
	DetailedHealth    = domain.DetailedHealth
)

const (
	StatusUp   = domain.StatusUp
	StatusDown = domain.StatusDown
)
