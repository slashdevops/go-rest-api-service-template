package driving

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// Resources is the driving port consumed by the HTTP resources handler.
type Resources interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Resource, error)
	List(ctx context.Context, input *domain.ListResourcesInput) (*domain.ListResourcesOutput, error)

	ListMatches(ctx context.Context, action, resource string, input *domain.ListResourcesInput) (*domain.ListResourcesOutput, error)
}
