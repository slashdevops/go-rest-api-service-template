package repository

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/roles.go -source=roles.go Roles

// Roles is the driven persistence port for role entities.
type Roles interface {
	Insert(ctx context.Context, input *domain.InsertRoleInput) error
	UpdateByID(ctx context.Context, input *domain.UpdateRoleInput) error
	DeleteByID(ctx context.Context, input *domain.DeleteRoleInput) error
	SelectByID(ctx context.Context, id uuid.UUID) (*domain.Role, error)

	Select(ctx context.Context, input *domain.SelectRolesInput) (*domain.SelectRolesOutput, error)
	SelectByUserID(ctx context.Context, userID uuid.UUID, input *domain.SelectRolesInput) (*domain.SelectRolesOutput, error)
	SelectByPolicyID(ctx context.Context, policyID uuid.UUID, input *domain.SelectRolesInput) (*domain.SelectRolesOutput, error)

	LinkPolicies(ctx context.Context, input *domain.LinkPoliciesToRoleInput) error
	UnlinkPolicies(ctx context.Context, input *domain.UnlinkPoliciesFromRoleInput) error

	LinkUsers(ctx context.Context, input *domain.LinkUsersToRoleInput) error
	UnlinkUsers(ctx context.Context, input *domain.UnlinkUsersFromRoleInput) error
}
