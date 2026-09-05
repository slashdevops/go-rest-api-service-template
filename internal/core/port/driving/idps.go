package driving

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// IDPs is the driving port consumed by the HTTP IDPs handler.
type IDPs interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.IDP, error)
	GetByName(ctx context.Context, name string) (*domain.IDP, error)
	GetAvailableIDPs(ctx context.Context) (*domain.SelectIDPAvailableOutput, error)

	Create(ctx context.Context, input *domain.CreateIDPInput) error
	UpdateByID(ctx context.Context, input *domain.UpdateIDPInput) error
	DeleteByID(ctx context.Context, input *domain.DeleteIDPInput) error

	List(ctx context.Context, input *domain.ListIDPsInput) (*domain.ListIDPsOutput, error)
}
