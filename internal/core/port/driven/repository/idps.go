package repository

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/idps.go -source=idps.go IDPs

// IDPs is the driven persistence port for IDP entities.
type IDPs interface {
	Insert(ctx context.Context, input *domain.InsertIDPInput) error
	UpdateByID(ctx context.Context, input *domain.UpdateIDPInput) error
	DeleteByID(ctx context.Context, input *domain.DeleteIDPInput) error

	SelectByID(ctx context.Context, id uuid.UUID) (*domain.IDP, error)
	SelectByName(ctx context.Context, name string) (*domain.IDP, error)
	Select(ctx context.Context, input *domain.SelectIDPsInput) (*domain.ListIDPsOutput, error)
}
