package driving

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../mocks/handler/roles.go -source=roles.go Roles

// Roles is the driving port consumed by the HTTP roles handler.
type Roles interface {
	List(ctx context.Context, input *domain.ListRolesInput) (*domain.ListRolesOutput, error)
	ListByUserID(ctx context.Context, userID uuid.UUID, input *domain.ListRolesInput) (*domain.ListRolesOutput, error)
	ListByPolicyID(ctx context.Context, policyID uuid.UUID, input *domain.ListRolesInput) (*domain.ListRolesOutput, error)

	Create(ctx context.Context, input *domain.CreateRoleInput) error

	GetByID(ctx context.Context, id uuid.UUID) (*domain.Role, error)
	UpdateByID(ctx context.Context, input *domain.UpdateRoleInput) error
	DeleteByID(ctx context.Context, input *domain.DeleteRoleInput) error

	LinkPolicies(ctx context.Context, input *domain.LinkPoliciesToRoleInput) error
	UnlinkPolicies(ctx context.Context, input *domain.UnlinkPoliciesFromRoleInput) error

	LinkUsers(ctx context.Context, input *domain.LinkUsersToRoleInput) error
	UnlinkUsers(ctx context.Context, input *domain.UnlinkUsersFromRoleInput) error
}
