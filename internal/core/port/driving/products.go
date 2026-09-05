package driving

import (
	"context"

	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// Products is the driving port consumed by the HTTP products handler.
type Products interface {
	CreateByProjectID(ctx context.Context, input *domain.CreateProductInput) error
	UpdateByIDByProjectID(ctx context.Context, input *domain.UpdateProductInput) error
	DeleteByIDByProjectID(ctx context.Context, input *domain.DeleteProductInput) error
	GetByIDByProjectID(ctx context.Context, id, projectID, userID uuid.UUID) (*domain.Product, error)
	ListByProjectID(ctx context.Context, projectID, userID uuid.UUID, input *domain.ListProductsInput) (*domain.ListProductsOutput, error)
	List(ctx context.Context, input *domain.ListProductsInput) (*domain.ListProductsOutput, error)
}
