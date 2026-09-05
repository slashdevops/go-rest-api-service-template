package repository

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/policies.go -source=policies.go Policies

// Policies is the driven persistence port for policy entities.
type Policies interface {
	Insert(ctx context.Context, input *domain.CreatePolicyInput) error
	UpdateByID(ctx context.Context, input *domain.UpdatePolicyInput) error
	DeleteByID(ctx context.Context, input *domain.DeletePolicyInput) error

	Select(ctx context.Context, input *domain.SelectPoliciesInput) (*domain.SelectPoliciesOutput, error)
	SelectByID(ctx context.Context, id uuid.UUID) (*domain.Policy, error)
	SelectByRoleID(ctx context.Context, roleID uuid.UUID, input *domain.SelectPoliciesInput) (*domain.SelectPoliciesOutput, error)

	LinkRoles(ctx context.Context, input *domain.LinkRolesToPolicyInput) error
	UnlinkRoles(ctx context.Context, input *domain.UnlinkRolesFromPolicyInput) error
}
