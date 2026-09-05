package driving

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// Projects is the driving port consumed by the HTTP projects handler.
type Projects interface {
	GetByIDByUserID(ctx context.Context, id, userID uuid.UUID) (*domain.Project, error)
	Create(ctx context.Context, input *domain.CreateProjectInput) error
	UpdateByID(ctx context.Context, input *domain.UpdateProjectInput) error
	DeleteByID(ctx context.Context, input *domain.DeleteProjectInput) error
	List(ctx context.Context, input *domain.ListProjectsInput) (*domain.ListProjectsOutput, error)
	ListByUserID(ctx context.Context, input *domain.ListProjectsInput) (*domain.ListProjectsOutput, error)

	LinkUsers(ctx context.Context, input *domain.LinkUsersToProjectInput) error
	UnlinkUsers(ctx context.Context, input *domain.UnlinkUsersFromProjectInput) error
}
