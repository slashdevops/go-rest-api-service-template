package driving

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// Policies is the driving port consumed by the HTTP policies handler.
type Policies interface {
	List(ctx context.Context, input *domain.ListPoliciesInput) (*domain.ListPoliciesOutput, error)
	ListByRoleID(ctx context.Context, roleID uuid.UUID, input *domain.ListPoliciesInput) (*domain.ListPoliciesOutput, error)

	Create(ctx context.Context, input *domain.CreatePolicyInput) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Policy, error)
	UpdateByID(ctx context.Context, input *domain.UpdatePolicyInput) error
	DeleteByID(ctx context.Context, input *domain.DeletePolicyInput) error

	LinkRoles(ctx context.Context, input *domain.LinkRolesToPolicyInput) error
	UnlinkRoles(ctx context.Context, input *domain.UnlinkRolesFromPolicyInput) error
}
