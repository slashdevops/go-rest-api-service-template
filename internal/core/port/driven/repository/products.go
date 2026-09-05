package repository

import (
	"context"

	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/products.go -source=products.go Products

// Products is the driven persistence port for product entities.
type Products interface {
	InsertByProjectID(ctx context.Context, input *domain.InsertProductInput) error
	UpdateByIDByProjectID(ctx context.Context, input *domain.UpdateProductInput) error
	DeleteByIDByProjectID(ctx context.Context, input *domain.DeleteProductInput) error
	SelectByIDByProjectID(ctx context.Context, id, projectID, userID uuid.UUID) (*domain.Product, error)
	SelectByProjectID(ctx context.Context, projectID, userID uuid.UUID, input *domain.SelectProductsInput) (*domain.SelectProductsOutput, error)
	Select(ctx context.Context, input *domain.SelectProductsInput) (*domain.SelectProductsOutput, error)
}
