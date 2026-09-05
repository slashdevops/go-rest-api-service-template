package payload

import (
	"fmt"
	"reflect"
	"regexp"
	"time"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// PolicyResponse represents a policy response.
//
//	@Description	Response containing policy details with authorization rules
type PolicyResponse struct {
	CreatedAt       time.Time        `json:"created_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`                                  // Timestamp when the policy was created
	UpdatedAt       time.Time        `json:"updated_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`                                  // Timestamp when the policy was last updated
	System          *bool            `json:"system,omitempty,omitzero" example:"false"`                                                              // Indicates if this is a system-managed policy (cannot be modified)
	Name            string           `json:"name,omitempty" example:"Project Viewer" format:"string"`                                                // Policy display name
	Description     string           `json:"description,omitempty" example:"Allows read access to project resources" format:"string"`                // Detailed description of the policy's purpose
	AllowedAction   string           `json:"allowed_action,omitempty" example:"GET" format:"string" enum:"GET,POST,PUT,PATCH,DELETE,OPTIONS,HEAD,*"` // HTTP method allowed by this policy
	AllowedResource string           `json:"allowed_resource,omitempty" example:"/projects/*/tokens" format:"string"`                                // Resource path pattern that this policy grants access to
	Resource        ResourceResponse `json:"resource,omitzero"`                                                                                      // Resource that this policy applies to
	ID              uuid.UUID        `json:"id,omitempty,omitzero" example:"019b4b0d-a682-7e34-a20c-c71a7147d7e7" format:"uuid"`                     // Unique identifier for the policy
}

// LinkRolesToPolicyRequest links roles to a policy.
//
//	@Description	Request payload for associating multiple roles with a policy
type LinkRolesToPolicyRequest struct {
	RoleIDs []uuid.UUID `json:"role_ids" format:"array" example:"019b4b0d-a682-7e34-a20c-c71a7147d7e7,019b4b0d-a682-7e38-b235-3dfcb59f4d9e"` // Array of role IDs to associate with the policy
}

func (ref *LinkRolesToPolicyRequest) Validate() error {
	var errs domain.ValidationErrors

	if reflect.DeepEqual(ref, &LinkRolesToPolicyRequest{}) {
		errs.AddError(domain.FieldRequest, "at least one field must be updated", "REQUIRED")
		return &errs
	}

	if len(ref.RoleIDs) == 0 {
		errs.AddError(domain.FieldRoleIDs, "must be at least one role ID", "REQUIRED")
	}

	if ref.RoleIDs != nil {
		for i, roleID := range ref.RoleIDs {
			if err := domain.ValidateUUID(roleID, 7, fmt.Sprintf("%s[%d]", domain.FieldRoleIDs, i)); err != nil {
				errs.Add(err)
			}
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// UnlinkRolesFromPolicyRequest unlinks roles from a policy.
//
//	@Description	Request payload for removing multiple roles from a policy
type UnlinkRolesFromPolicyRequest = LinkRolesToPolicyRequest

// ListPoliciesResponse represents a paginated list of policies.
//
//	@Description	Paginated list of authorization policies with access rules
type ListPoliciesResponse struct {
	Items     []PolicyResponse `json:"items"`     // Array of policy configurations with authorization rules
	Paginator domain.Paginator `json:"paginator"` // Pagination information including total count and page details
}

// CreatePolicyRequest represents a request to create a policy.
//
//	@Description	Request payload for creating a new authorization policy
type CreatePolicyRequest struct {
	Name            string    `json:"name" example:"Project Tokens Manager" format:"string" validate:"required" minLength:"2" maxLength:"255"`                                             // Policy display name
	Description     string    `json:"description,omitempty" example:"Allows managing API tokens for specific projects" format:"string" validate:"optional" minLength:"2" maxLength:"1024"` // Detailed description of the policy's purpose and scope
	AllowedAction   string    `json:"allowed_action,omitempty" example:"POST" format:"string" validate:"required" enum:"GET,POST,PUT,PATCH,DELETE,OPTIONS,HEAD,*"`                         // HTTP method allowed by this policy
	AllowedResource string    `json:"allowed_resource,omitempty" example:"/projects/*/tokens" format:"string" validate:"required"`                                                         // Resource path pattern (supports wildcards and UUIDs)
	ID              uuid.UUID `json:"id,omitempty,omitzero" example:"019b4b0d-a682-7e38-b235-3dfcb59f4d9e" format:"uuid" validate:"optional"`                                              // Optional custom policy ID (auto-generated if not provided)
}

func (ref *CreatePolicyRequest) Validate() error {
	var errs domain.ValidationErrors

	if reflect.DeepEqual(ref, &CreatePolicyRequest{}) {
		errs.AddError(domain.FieldRequest, "at least one field must be updated", "REQUIRED")
		return &errs
	}

	errs.Add(domain.ValidateUUID(ref.ID, 7, domain.FieldID))

	nameOptions := domain.StringValidationOptions{
		MinLength: domain.PolicyNameMinLength, MaxLength: domain.PolicyNameMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: domain.FieldName,
	}
	if _, err := domain.ValidateString(ref.Name, nameOptions); err != nil {
		errs.Add(err)
	}

	descriptionOptions := domain.StringValidationOptions{
		MinLength: domain.PolicyDescriptionMinLength, MaxLength: domain.PolicyDescriptionMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: domain.FieldDescription,
	}
	if _, err := domain.ValidateString(ref.Description, descriptionOptions); err != nil {
		errs.Add(err)
	}

	if ref.AllowedAction == "" {
		errs.AddError(domain.FieldAllowedAction, "cannot be empty", "REQUIRED")
	} else if !regexp.MustCompile(domain.ValidActionsRegex).MatchString(ref.AllowedAction) {
		errs.AddError(domain.FieldAllowedAction, "invalid action format, must be one of "+domain.GetValidActions()+" in Uppercase", "INVALID_FORMAT")
	}

	if ref.AllowedResource == "" {
		errs.AddError(domain.FieldAllowedResource, "cannot be empty", "REQUIRED")
	} else if !regexp.MustCompile(domain.ValidResourceRegex).MatchString(ref.AllowedResource) {
		errs.AddError(domain.FieldAllowedResource, "invalid resource format", "INVALID_FORMAT")
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// UpdatePolicyRequest represents a request to update a policy.
//
//	@Description	Request payload for updating an existing authorization policy (all fields optional)
type UpdatePolicyRequest struct {
	Name            *string `json:"name,omitempty" example:"Updated Policy Name" format:"string" validate:"optional" minLength:"2" maxLength:"255"`                // Updated policy name
	Description     *string `json:"description,omitempty" example:"Updated policy description" format:"string" validate:"optional" minLength:"2" maxLength:"1024"` // Updated description
	AllowedAction   *string `json:"allowed_action,omitempty" example:"PUT" format:"string" validate:"optional" enum:"GET,POST,PUT,PATCH,DELETE,OPTIONS,HEAD,*"`    // Updated HTTP method
	AllowedResource *string `json:"allowed_resource,omitempty" example:"/projects/*/tokens" format:"string" validate:"optional"`                                   // Updated resource path pattern
}

func (ref *UpdatePolicyRequest) Validate() error {
	var errs domain.ValidationErrors

	if reflect.DeepEqual(ref, &UpdatePolicyRequest{}) {
		errs.AddError(domain.FieldRequest, "at least one field must be updated", "REQUIRED")
		return &errs
	}

	if ref.Name != nil {
		nameOptions := domain.StringValidationOptions{
			MinLength: domain.PolicyNameMinLength, MaxLength: domain.PolicyNameMaxLength,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
			NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: domain.FieldName,
		}
		if _, err := domain.ValidateString(*ref.Name, nameOptions); err != nil {
			errs.Add(err)
		}
	}

	if ref.Description != nil {
		descriptionOptions := domain.StringValidationOptions{
			MinLength: domain.PolicyDescriptionMinLength, MaxLength: domain.PolicyDescriptionMaxLength,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
			NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: domain.FieldDescription,
		}
		if _, err := domain.ValidateString(*ref.Description, descriptionOptions); err != nil {
			errs.Add(err)
		}
	}

	if ref.AllowedAction != nil {
		if *ref.AllowedAction == "" {
			errs.AddError(domain.FieldAllowedAction, "cannot be empty", "REQUIRED")
		} else if !regexp.MustCompile(domain.ValidActionsRegex).MatchString(*ref.AllowedAction) {
			errs.AddError(domain.FieldAllowedAction, "invalid action format, must be one of "+domain.GetValidActions()+" in Uppercase", "INVALID_FORMAT")
		}
	}

	if ref.AllowedResource != nil {
		if *ref.AllowedResource == "" {
			errs.AddError(domain.FieldAllowedResource, "cannot be empty", "REQUIRED")
		} else if !regexp.MustCompile(domain.ValidResourceRegex).MatchString(*ref.AllowedResource) {
			errs.AddError(domain.FieldAllowedResource, "invalid resource format", "INVALID_FORMAT")
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}
