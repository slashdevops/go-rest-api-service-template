package driving

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// IDPTypes is the driving port consumed by the HTTP IDP-types handler.
type IDPTypes interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.IDPTypes, error)
	GetByName(ctx context.Context, name string) (*domain.IDPTypes, error)
	List(ctx context.Context, input *domain.ListIDPTypesInput) (*domain.ListIDPTypesOutput, error)
}
