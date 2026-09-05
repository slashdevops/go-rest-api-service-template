package repository

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/resources.go -source=resources.go Resources

// Resources is the driven persistence port for resource entities.
type Resources interface {
	SelectByID(ctx context.Context, id uuid.UUID) (*domain.Resource, error)
	Select(ctx context.Context, input *domain.SelectResourcesInput) (*domain.SelectResourcesOutput, error)
}
