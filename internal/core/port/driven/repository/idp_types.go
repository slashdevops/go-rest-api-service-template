package repository

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/idp_types.go -source=idp_types.go IDPTypes

// IDPTypes is the driven persistence port for IDP-type entities.
type IDPTypes interface {
	SelectByID(ctx context.Context, id uuid.UUID) (*domain.IDPTypes, error)
	SelectByName(ctx context.Context, name string) (*domain.IDPTypes, error)
	Select(ctx context.Context, input *domain.SelectIDPTypesInput) (*domain.ListIDPTypesOutput, error)
}
