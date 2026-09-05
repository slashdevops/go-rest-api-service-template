package repository

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/projects.go -source=projects.go Projects

// Projects is the driven persistence port for project entities.
type Projects interface {
	Insert(ctx context.Context, input *domain.InsertProjectInput) error
	UpdateByID(ctx context.Context, input *domain.UpdateProjectInput) error
	DeleteByID(ctx context.Context, input *domain.DeleteProjectInput) error
	SelectByIDByUserID(ctx context.Context, id, userID uuid.UUID) (*domain.Project, error)
	SelectByUserID(ctx context.Context, input *domain.SelectProjectsInput) (*domain.SelectProjectsOutput, error)
	Select(ctx context.Context, input *domain.SelectProjectsInput) (*domain.SelectProjectsOutput, error)

	LinkUsers(ctx context.Context, input *domain.LinkUsersToProjectInput) error
	UnlinkUsers(ctx context.Context, input *domain.UnlinkUsersFromProjectInput) error
}
