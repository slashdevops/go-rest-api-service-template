package driving

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../mocks/handler/rate_limits.go -source=rate_limits.go RateLimits

// RateLimits is the driving port consumed by the HTTP rate-limits handler.
type RateLimits interface {
	List(ctx context.Context, input *domain.SelectRateLimitsInput) (*domain.ListRateLimitsOutput, error)

	Create(ctx context.Context, input *domain.CreateRateLimitInput) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.RateLimit, error)
	UpdateByID(ctx context.Context, input *domain.UpdateRateLimitInput) error
	DeleteByID(ctx context.Context, input *domain.DeleteRateLimitInput) error

	// Effective answers which rules apply to a (method, endpoint) pair. It
	// exists because a precedence ladder is the thing operators get wrong, and
	// the only honest way to answer "why is this not limited?" is to resolve it
	// with the same function the middleware uses.
	Effective(ctx context.Context, req domain.RateLimitRequest) ([]domain.RateLimitMatch, error)

	// Enforcing reports whether rate limiting is switched on at all. Distinct
	// from "are there rules": with ratelimit.enabled=false the rules are still
	// real, listable and editable, and enforcing nothing.
	Enforcing() bool
}
