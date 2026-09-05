package repository

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/rate_limits.go -source=rate_limits.go RateLimits

// RateLimits is the driven persistence port for rate-limit rules.
//
// SelectAll is not a convenience over Select: it is what the in-memory mirror
// reloads from, and it deliberately takes no paginator. The mirror must hold
// EVERY enabled rule or resolution silently answers with a subset -- a rule that
// exists, is enabled, and is not enforced because it fell off page two.
type RateLimits interface {
	Insert(ctx context.Context, input *domain.CreateRateLimitInput) error
	UpdateByID(ctx context.Context, input *domain.UpdateRateLimitInput) error
	DeleteByID(ctx context.Context, input *domain.DeleteRateLimitInput) error

	Select(ctx context.Context, input *domain.SelectRateLimitsInput) (*domain.SelectRateLimitsOutput, error)
	SelectByID(ctx context.Context, id uuid.UUID) (*domain.RateLimit, error)

	// SelectAll returns every enabled rule with its windows, for the mirror.
	SelectAll(ctx context.Context) ([]domain.RateLimit, error)
}
