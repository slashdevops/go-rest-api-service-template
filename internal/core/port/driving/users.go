package driving

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// Users is the driving port consumed by the HTTP users handler (and
// the /me handler, which views the current user).
type Users interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)

	Create(ctx context.Context, input *domain.CreateUserInput) error
	UpdateByID(ctx context.Context, input *domain.UpdateUserInput) error
	DeleteByID(ctx context.Context, input *domain.DeleteUserInput) error

	List(ctx context.Context, input *domain.ListUsersInput) (*domain.ListUsersOutput, error)
	ListByRoleID(ctx context.Context, roleID uuid.UUID, input *domain.ListUsersInput) (*domain.ListUsersOutput, error)
	ListByProjectID(ctx context.Context, projectID uuid.UUID, input *domain.ListUsersInput) (*domain.ListUsersOutput, error)

	SelectAuthz(ctx context.Context, userID uuid.UUID) (map[string]any, error)

	LinkRoles(ctx context.Context, input *domain.LinkRolesToUserInput) error
	UnlinkRoles(ctx context.Context, input *domain.UnlinkRolesFromUsersInput) error

	LinkProjects(ctx context.Context, input *domain.LinkProjectsToUserInput) error
	UnlinkProjects(ctx context.Context, input *domain.UnlinkProjectsFromUserInput) error
}
