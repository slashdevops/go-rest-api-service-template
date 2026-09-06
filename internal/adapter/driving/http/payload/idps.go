package payload

import (
	"reflect"
	"time"

	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// IDPAvailableResponse represents an available identity provider.
//
//	@Description	Available identity provider information including type and branding details
type IDPAvailableResponse struct {
	Name        string            `json:"name,omitzero" example:"Google" format:"string"`                           // Display name of the identity provider
	Description string            `json:"description,omitzero" example:"Google Identity Provider" format:"string"`  // Detailed description
	Logo        string            `json:"logo,omitzero" example:"https://example.com/logo.png" format:"uri"`        // URL to the identity provider's logo image
	IDPType     IDPTypesAvailable `json:"idp_type,omitzero"`                                                        // Type information for this identity provider
	ID          uuid.UUID         `json:"id,omitzero" example:"019b4b0d-a682-7d88-9ce1-12d63815e879" format:"uuid"` // Unique identifier
	// Whether a sign-in from an identity nobody has linked yet creates an
	// account. The login page offers "register with" only when this is true.
	AutoProvision bool `json:"auto_provision" example:"true" format:"boolean"`
}

// IDPResponse represents an identity provider configuration.
//
//	@Description	Complete identity provider configuration including OAuth credentials and redirect URLs
type IDPResponse struct {
	CreatedAt     time.Time        `json:"created_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`                                                       // Timestamp when the identity provider was created
	UpdatedAt     time.Time        `json:"updated_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`                                                       // Timestamp when the identity provider was last updated
	Name          string           `json:"name,omitzero" example:"Google" format:"string"`                                                                              // Display name
	Description   string           `json:"description,omitzero" example:"Google Identity Provider" format:"string"`                                                     // Description
	CallbackURL   string           `json:"callback_url,omitzero" example:"https://app.example.com/auth/idp/019b4b0d-a682-7e19-a524-866cfffef121/callback" format:"uri"` // The redirect_uri registered with the provider: the frontend's callback route URL
	Logo          string           `json:"logo,omitzero" example:"https://example.com/logo.png" format:"uri"`                                                           // Logo URL
	ClientID      string           `json:"client_id,omitzero" example:"367082405970-example" format:"string"`                                                           // OAuth client ID
	IssuerURL     string           `json:"issuer_url,omitzero" example:"https://login.microsoftonline.com/00000000-0000-0000-0000-000000000000/v2.0" format:"uri"`      // OpenID Connect issuer; empty for the github kind
	Enabled       *bool            `json:"enabled,omitempty" example:"true"`                                                                                            // Offered on the login page and accepted at the callback
	AutoProvision *bool            `json:"auto_provision,omitempty" example:"true"`                                                                                     // A first sign-in with a provider-verified email may create an account
	IDPType       IDPTypesResponse `json:"idp_type,omitzero"`                                                                                                           // Type
	ID            uuid.UUID        `json:"id,omitzero" example:"019b4b0d-a682-7e11-90b0-c94f29b8483a" format:"uuid"`                                                    // Unique identifier
}

// CreateIDPRequest represents the input for the CreateIDP method.
//
//	@Description	Request payload for creating a new identity provider configuration
type CreateIDPRequest struct {
	Enabled       *bool     `json:"enabled,omitempty" example:"true" validate:"optional"`                                                                                        // Defaults to true
	AutoProvision *bool     `json:"auto_provision,omitempty" example:"true" validate:"optional"`                                                                                 // Defaults to true
	Name          string    `json:"name" example:"Google" format:"string" validate:"required" minLength:"2" maxLength:"100"`                                                     // Display name
	Description   string    `json:"description" example:"Google OAuth2 Identity Provider" format:"string" validate:"required" minLength:"10" maxLength:"500"`                    // Description
	CallbackURL   string    `json:"callback_url" example:"http://localhost:8080/api/v1/auth/idp/019b4b0d-a682-7e19-a524-866cfffef121/callback" format:"uri" validate:"required"` // OAuth callback URL
	Logo          string    `json:"logo,omitempty" example:"https://example.com/logo.png" format:"uri" minLength:"10" maxLength:"1024"`                                          // Logo URL
	ClientID      string    `json:"client_id" example:"367082405970-example" format:"string" validate:"required" minLength:"2" maxLength:"255"`                                  // OAuth client ID
	ClientSecret  string    `json:"client_secret" example:"GOCSPX-example_secret_key" format:"string" validate:"required" minLength:"2" maxLength:"255"`                         // OAuth client secret
	IssuerURL     string    `json:"issuer_url,omitempty" example:"https://accounts.google.com" format:"uri" validate:"optional"`                                                 // Required for an oidc kind; the value the ID token's iss must equal
	ID            uuid.UUID `json:"id,omitempty" example:"019b4b0d-a682-7e19-a524-866cfffef121" format:"uuid" validate:"optional"`                                               // Optional custom ID
	IDPTypeID     uuid.UUID `json:"idp_type_id" example:"019b4b0d-a682-7e1d-bd83-3864c7d5aa43" format:"uuid" validate:"required"`                                                // Identity provider type ID
}

func (ref *CreateIDPRequest) Validate() error {
	var errs domain.ValidationErrors

	if err := domain.ValidateUUID(ref.IDPTypeID, 7, domain.FieldIDPTypeID); err != nil {
		errs.Add(err)
	}

	for field, val := range map[string]string{
		domain.FieldName:         ref.Name,
		domain.FieldDescription:  ref.Description,
		domain.FieldCallbackURL:  ref.CallbackURL,
		domain.FieldClientID:     ref.ClientID,
		domain.FieldClientSecret: ref.ClientSecret,
	} {
		if val == "" {
			errs.Add(&domain.ValidationError{Field: field, Message: field + " is required", Code: "REQUIRED"})
		}
	}

	if errs.HasErrors() {
		return &errs
	}
	return nil
}

// UpdateIDPRequest represents the input for the UpdateIDP method.
//
//	@Description	Request payload for updating an existing identity provider configuration (all fields optional)
type UpdateIDPRequest struct {
	IDPTypeID     *uuid.UUID `json:"idp_type_id,omitempty" example:"019b4b0d-a682-7e21-9f5c-725b8be59cd5" format:"uuid" validate:"optional"`                              // Updated IDP type ID
	Name          *string    `json:"name,omitempty" example:"Google Updated" format:"string" validate:"optional" minLength:"2" maxLength:"100"`                           // Updated display name
	Description   *string    `json:"description,omitempty" example:"Updated Google Identity Provider" format:"string" validate:"optional" minLength:"10" maxLength:"500"` // Updated description
	CallbackURL   *string    `json:"callback_url,omitempty" example:"https://example.com/callback" format:"uri" validate:"optional"`                                      // Updated callback URL
	Logo          *string    `json:"logo,omitempty" example:"https://example.com/logo-new.png" format:"uri" validate:"optional" minLength:"10" maxLength:"1024"`          // Updated logo URL
	ClientID      *string    `json:"client_id,omitempty" example:"367082405970-new" format:"string" validate:"optional" minLength:"2" maxLength:"255"`                    // Updated OAuth client ID
	ClientSecret  *string    `json:"client_secret,omitempty" example:"GOCSPX-new_secret" format:"string" validate:"optional" minLength:"2" maxLength:"255"`               // Updated OAuth client secret
	IssuerURL     *string    `json:"issuer_url,omitempty" example:"https://accounts.google.com" format:"uri" validate:"optional"`                                         // Updated issuer; an empty string clears it
	Enabled       *bool      `json:"enabled,omitempty" example:"false" validate:"optional"`                                                                               // Updated enabled flag
	AutoProvision *bool      `json:"auto_provision,omitempty" example:"false" validate:"optional"`                                                                        // Updated auto-provision flag
}

func (req *UpdateIDPRequest) Validate() error {
	if reflect.DeepEqual(req, &UpdateIDPRequest{}) {
		return &domain.InvalidIDPUpdateError{Message: "at least one field must be provided for update"}
	}

	var errs domain.ValidationErrors

	if req.IDPTypeID != nil {
		if err := domain.ValidateUUID(*req.IDPTypeID, 7, domain.FieldIDPTypeID); err != nil {
			errs.Add(err)
		}
	}

	stringChecks := []struct {
		field      string
		val        *string
		minLen     int
		maxLen     int
		noHTMLTags bool
	}{
		{domain.FieldName, req.Name, domain.ValidIDPNameMinLength, domain.ValidIDPNameMaxLength, true},
		{domain.FieldDescription, req.Description, domain.ValidIDPDescriptionMinLength, domain.ValidIDPDescriptionMaxLength, true},
		{domain.FieldLogo, req.Logo, domain.ValidIDPLogoMinLength, domain.ValidIDPLogoMaxLength, true},
		{domain.FieldClientID, req.ClientID, domain.ValidIDPClientIDMinLength, domain.ValidIDPClientIDMaxLength, true},
		{domain.FieldClientSecret, req.ClientSecret, domain.ValidIDPClientSecretMinLength, domain.ValidIDPClientSecretMaxLength, true},
		{domain.FieldCallbackURL, req.CallbackURL, 5, 255, true},
	}

	for _, c := range stringChecks {
		if c.val == nil {
			continue
		}
		if normalized, err := domain.ValidateString(*c.val, domain.StringValidationOptions{
			MinLength: c.minLen, MaxLength: c.maxLen,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: c.noHTMLTags,
			NoNullBytes: true, NormalizeUnicode: true, FieldName: c.field,
		}); err != nil {
			errs.Add(err)
		} else {
			*c.val = normalized
		}
	}

	if errs.HasErrors() {
		return &errs
	}
	return nil
}

// ListIDPsResponse represents a list of IDPs.
//
//	@Description	Paginated list of identity provider configurations
type ListIDPsResponse struct {
	Items     []IDPResponse    `json:"items"`     // Array of IDP configurations
	Paginator domain.Paginator `json:"paginator"` // Pagination information
}

// ListIDPAvailableResponse represents a list of available IDPs.
//
//	@Description	List of identity providers available for configuration in the system
type ListIDPAvailableResponse struct {
	Items []IDPAvailableResponse `json:"items"` // Array of available IDP options
}

// IDPLoginResponse represents the response for the IDP login request.
//
//	@Description	Response containing OAuth redirect information for initiating login flow
type IDPLoginResponse struct {
	RedirectURL  string    `json:"redirect_url" format:"uri"`
	RedirectCode int       `json:"redirect_code" example:"302"`
	IDPID        uuid.UUID `json:"idp_id" example:"019b4b0d-a682-7e25-b4c8-afa26aa7d1dc" format:"uuid"`
}

// IDPRegisterResponse represents the response for the IDP registration request.
//
//	@Description	Response containing OAuth redirect information for initiating registration flow
type IDPRegisterResponse struct {
	RedirectURL  string    `json:"redirect_url" format:"uri"`
	RedirectCode int       `json:"redirect_code" example:"302"`
	IDPID        uuid.UUID `json:"idp_id" example:"019b4b0d-a682-7e29-b6e2-a4cc6db87ea8" format:"uuid"`
}

// IDPCallbackResponse is what the provider callback answers with.
//
// For a login or a register it carries the same session a password login
// returns; the frontend server stores it in its own cookies. For a link it
// carries the account the identity was attached to and no session -- the
// caller was already signed in.
//
//	@Description	The outcome of a provider callback: a session for login and register, the linked account for link
type IDPCallbackResponse struct {
	Login    *LoginUserResponse `json:"login,omitempty"`                                                                  // The session, for login and register
	LinkedTo *uuid.UUID         `json:"linked_to,omitempty" example:"019b4b0d-a682-7e34-a20c-c71a7147d7e7" format:"uuid"` // The account, for link
	Event    string             `json:"event" example:"login" format:"string" enum:"login,register,link"`                 // Which event the state was minted for
}

// UserIdentityResponse is one provider identity linked to the caller's account.
//
//	@Description	A provider identity linked to the account
type UserIdentityResponse struct {
	LinkedAt    time.Time `json:"linked_at" example:"2026-09-05T10:12:00Z" format:"date-time"`         // When the link was made
	IDPName     string    `json:"idp_name" example:"Company Entra ID" format:"string"`                 // The provider row's name
	IDPTypeName string    `json:"idp_type_name" example:"EntraID" format:"string"`                     // The provider's type
	Email       string    `json:"email" example:"jane@example.com" format:"email"`                     // The email the provider reported when the link was made
	IDPID       uuid.UUID `json:"idp_id" example:"019b4b0d-a682-7e19-a524-866cfffef121" format:"uuid"` // The provider row
}

// ListUserIdentitiesResponse lists the caller's linked identities.
//
//	@Description	The provider identities linked to the account
type ListUserIdentitiesResponse struct {
	Items []UserIdentityResponse `json:"items"`
}
