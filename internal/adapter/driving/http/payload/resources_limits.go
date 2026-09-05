package payload

import (
	"time"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// ResourcesLimitsResponse represent the limits on various resources within the system.
//
// NOTE for maintainers: usage, soft_limit and hard_limit deliberately carry no
// `omitempty`. Zero is a real answer for each — "nothing used yet", and a hard
// limit of zero means the scope may create nothing at all — and omitting them
// sent the client `undefined` where the answer was `0`. Keep any explanation
// away from the field lines themselves: swag turns the comment immediately
// above a field into that field's published description, so a note there ends
// up in swagger.json as the API contract.
//
//	@Description	Resource limit configuration defining usage constraints and current consumption
type ResourcesLimitsResponse struct {
	CreatedAt    time.Time `json:"created_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`                                                    // Timestamp when the limit was created
	UpdatedAt    time.Time `json:"updated_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`                                                    // Timestamp when the limit was last updated
	ScopeType    string    `json:"scope_type,omitempty" example:"user" enums:"system,user,project"`                                                          // Scope level: system (global), user (per user), or project (per project)
	ResourceType string    `json:"resource_type,omitempty" example:"projects" enums:"users,projects,pa_tokens,models,generate_config,embedding_config,idps"` // Type of resource being limited
	Usage        int       `json:"usage" example:"5" minimum:"0"`                                                                                            // Current number of resources in use
	SoftLimit    int       `json:"soft_limit" example:"10" minimum:"0"`                                                                                      // Soft limit threshold (warning level)
	HardLimit    int       `json:"hard_limit" example:"20" minimum:"0"`                                                                                      // Hard limit threshold (maximum allowed)
	ID           uuid.UUID `json:"id,omitempty,omitzero" example:"019b4b0d-a682-7e38-b235-3dfcb59f4d9e" format:"uuid"`                                       // Unique identifier for this resource limit
	ScopeID      uuid.UUID `json:"scope_id,omitempty" example:"019b4b0d-a682-7e3c-aca0-dd93b3229ff7" format:"uuid"`                                          // ID of the scope entity (null for system-wide limits)
}

// ResourceUsageStatusResponse is one resource's ceiling and consumption.
//
//	@Description	A single resource's limit and how much of it is used
type ResourceUsageStatusResponse struct {
	ResourceType string `json:"resource_type" example:"projects" enums:"users,projects,pa_tokens,models,generate_config,embedding_config,idps"` // Type of resource being limited
	Usage        int    `json:"usage" example:"3" minimum:"0"`                                                                                  // How many currently exist in this scope
	SoftLimit    int    `json:"soft_limit" example:"10"`                                                                                        // Warning threshold. -1 means no limit is configured
	HardLimit    int    `json:"hard_limit" example:"12"`                                                                                        // Creation is refused at or above this. -1 means no limit is configured
	CanCreate    bool   `json:"can_create" example:"true"`                                                                                      // Whether another one may be created right now
	SoftReached  bool   `json:"soft_limit_reached" example:"false"`                                                                             // Whether usage has reached the warning threshold
	Tampered     bool   `json:"tamper_detected" example:"false"`                                                                                // The stored counter failed its integrity check; creation is refused until it is reconciled
}

// ResourcesLimitsStatusResponse is every limit that applies to one scope.
//
//	@Description	Limits and consumption for a single scope, such as the calling user or one project
type ResourcesLimitsStatusResponse struct {
	ScopeType string                        `json:"scope_type" example:"user" enums:"system,user,project"`                 // Scope these limits belong to
	Resources []ResourceUsageStatusResponse `json:"resources"`                                                             // One entry per resource type this scope governs
	ScopeID   uuid.UUID                     `json:"scope_id" example:"019b4b0d-a682-7e3c-aca0-dd93b3229ff7" format:"uuid"` // The scope's identifier
}

// ListResourcesLimitsResponse represents the response for listing resource limits.
//
//	@Description	Paginated list of resource limit configurations across different scopes and resource types
type ListResourcesLimitsResponse struct {
	Items     []ResourcesLimitsResponse `json:"items"`     // Array of resource limit configurations
	Paginator domain.Paginator          `json:"paginator"` // Pagination information including total count and page details
}
