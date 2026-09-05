package payload

import (
	"time"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// ResourceResponse represents a resource response.
//
//	@Description	API resource permission definition for authorization control
type ResourceResponse struct {
	CreatedAt   time.Time `json:"created_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`              // Timestamp when the resource was created
	UpdatedAt   time.Time `json:"updated_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`              // Timestamp when the resource was last updated
	System      *bool     `json:"system,omitempty,omitzero" example:"false"`                                          // Indicates if this is a system-managed resource (cannot be deleted)
	Name        string    `json:"name,omitempty" example:"Read Users" format:"string"`                                // Human-readable name of the permission
	Description string    `json:"description,omitempty" example:"Allows reading of user data" format:"string"`        // Detailed description of what this permission grants
	Action      string    `json:"action,omitempty" example:"GET" format:"string" enums:"GET,POST,PUT,DELETE,PATCH"`   // HTTP method or action type
	Resource    string    `json:"resource,omitempty" example:"/api/v1/users" format:"string"`                         // API resource path or identifier
	ID          uuid.UUID `json:"id,omitempty,omitzero" example:"019b4b0d-a682-7e48-b818-261829e39f76" format:"uuid"` // Unique identifier for the resource permission
}

// ListResourcesResponse represents a list of resources.
//
//	@Description	Paginated list of API resource permission definitions
type ListResourcesResponse struct {
	Items     []ResourceResponse `json:"items"`     // Array of resource permission definitions
	Paginator domain.Paginator   `json:"paginator"` // Pagination information including total count and page details
}
