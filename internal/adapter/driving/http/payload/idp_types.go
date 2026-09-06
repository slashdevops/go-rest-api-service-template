package payload

import (
	"time"

	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// IDPTypesAvailable represents the available identity provider types.
//
//	@Description	Available identity provider types that can be configured in the system
type IDPTypesAvailable struct {
	Name        string    `json:"name,omitzero" example:"Google" format:"string"`                           // Display name of the identity provider type (e.g., Google, Github)
	Description string    `json:"description,omitzero" example:"Google OAuth2.0 Provider" format:"string"`  // Detailed description of the identity provider type
	ID          uuid.UUID `json:"id,omitzero" example:"019b4b0d-a682-7e2c-86fa-05e7237c0c5b" format:"uuid"` // Unique identifier for the identity provider type
}

// IDPTypesResponse represents the type of identity providers.
//
//	@Description	Complete identity provider type information including OAuth scopes and API endpoints
type IDPTypesResponse struct {
	CreatedAt      time.Time `json:"created_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`                          // Timestamp when the identity provider type was created
	UpdatedAt      time.Time `json:"updated_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`                          // Timestamp when the identity provider type was last updated
	System         *bool     `json:"system,omitzero" example:"true"`                                                                 // Indicates if this is a system-managed identity provider type (cannot be deleted)
	Name           string    `json:"name,omitzero" example:"Google" format:"string"`                                                 // Display name of the identity provider type (e.g., Google, Github, Microsoft)
	Description    string    `json:"description,omitzero" example:"Google OAuth2.0 Identity Provider" format:"string"`               // Detailed description of the identity provider type and its capabilities
	UserInfoAPIURL string    `json:"user_info_api_url,omitzero" example:"https://api.github.com/user" format:"uri"`                  // User-info endpoint, stated only for the github kind; an oidc kind discovers it
	Kind           string    `json:"kind,omitzero" example:"oidc" format:"string" enum:"oidc,github"`                                // How the provider is spoken to: oidc (discovery, PKCE, ID token) or github (plain OAuth2)
	IssuerHint     string    `json:"issuer_hint,omitzero" example:"https://login.microsoftonline.com/<tenant-id>/v2.0" format:"uri"` // What to put in an instance's issuer_url; empty for the github kind
	Scopes         []string  `json:"scopes,omitzero" example:"openid,email,profile" format:"csv"`                                    // OAuth scopes required for this identity provider type
	SerialID       int64     `json:"serial_id,omitzero" example:"1"`                                                                 // Sequential identifier for ordering
	ID             uuid.UUID `json:"id,omitzero" example:"019b4b0d-a682-7e30-8b33-650caa6446c7" format:"uuid"`                       // Unique identifier for the identity provider type
}

// ListIDPTypesResponse represents the response of IDP Types.
//
//	@Description	Paginated list of identity provider types available in the system
type ListIDPTypesResponse struct {
	Items     []IDPTypesResponse `json:"items"`     // Array of identity provider type configurations
	Paginator domain.Paginator   `json:"paginator"` // Pagination information including total count and page details
}
