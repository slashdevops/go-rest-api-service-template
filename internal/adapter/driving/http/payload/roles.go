package payload

import (
	"fmt"
	"reflect"
	"time"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// RoleResponse represents a role.
//
//	@Description	RoleResponse represents a role.
type RoleResponse struct {
	CreatedAt   time.Time `json:"created_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`               // Timestamp when the role was created
	UpdatedAt   time.Time `json:"updated_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`               // Timestamp when the role was last updated
	System      *bool     `json:"system" example:"false"`                                                              // Indicates if this is a system-managed role (cannot be deleted)
	AutoAssign  *bool     `json:"auto_assign" example:"false"`                                                         // Indicates if this role is automatically assigned to new users
	Name        string    `json:"name,omitempty" example:"Admin" format:"string"`                                      // Role display name
	Description string    `json:"description,omitempty" example:"Administrator role with full access" format:"string"` // Detailed description of the role's permissions and purpose
	ID          uuid.UUID `json:"id,omitempty,omitzero" example:"019b4b0d-a682-7033-b6af-5e7f9a689613" format:"uuid"`  // Unique identifier for the role
}

// CreateRoleRequest represents the input for the CreateRole method.
//
//	@Description	Request payload for creating a new role with permissions
type CreateRoleRequest struct {
	Name        string    `json:"name" example:"Editor" format:"string" validate:"required" minLength:"2" maxLength:"100"`                      // Role display name
	Description string    `json:"description" example:"Content editor role" format:"string" validate:"required" minLength:"10" maxLength:"500"` // Description of the role's responsibilities and scope
	ID          uuid.UUID `json:"id,omitempty" example:"019b4b0d-a682-7f0b-a592-7bc362bae397" format:"uuid" validate:"optional"`                // Optional custom role ID
}

func (req *CreateRoleRequest) Validate() error {
	var errs domain.ValidationErrors

	if err := domain.ValidateUUID(req.ID, 7, domain.FieldID); err != nil {
		errs.Add(err)
	}

	nameOptions := domain.StringValidationOptions{
		MinLength: domain.RoleNameMinLength, MaxLength: domain.RoleNameMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: domain.FieldName,
	}
	if _, err := domain.ValidateString(req.Name, nameOptions); err != nil {
		errs.Add(err)
	}

	descriptionOptions := domain.StringValidationOptions{
		MinLength: domain.RoleDescriptionMinLength, MaxLength: domain.RoleDescriptionMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: domain.FieldDescription,
	}
	if _, err := domain.ValidateString(req.Description, descriptionOptions); err != nil {
		errs.Add(err)
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// UpdateRoleRequest represents the input for the UpdateRole method.
//
//	@Description	Request payload for updating an existing role (all fields optional)
type UpdateRoleRequest struct {
	Name        *string `json:"name,omitempty" example:"Updated Editor" format:"string" validate:"optional" minLength:"2" maxLength:"100"`                   // Updated role name
	Description *string `json:"description,omitempty" example:"Updated role description" format:"string" validate:"optional" minLength:"10" maxLength:"500"` // Updated role description
}

func (req *UpdateRoleRequest) Validate() error {
	var errs domain.ValidationErrors

	if reflect.DeepEqual(req, &UpdateRoleRequest{}) {
		errs.Add(&domain.ValidationError{Field: domain.FieldRequest, Message: "at least one field must be provided for update", Code: "REQUIRED"})
		return &errs
	}

	if req.Name != nil {
		nameOptions := domain.StringValidationOptions{
			MinLength: domain.RoleNameMinLength, MaxLength: domain.RoleNameMaxLength,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
			NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: domain.FieldName,
		}
		if _, err := domain.ValidateString(*req.Name, nameOptions); err != nil {
			errs.Add(err)
		}
	}

	if req.Description != nil {
		descriptionOptions := domain.StringValidationOptions{
			MinLength: domain.RoleDescriptionMinLength, MaxLength: domain.RoleDescriptionMaxLength,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
			NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: domain.FieldDescription,
		}
		if _, err := domain.ValidateString(*req.Description, descriptionOptions); err != nil {
			errs.Add(err)
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// ListRolesResponse represents a list of roles.
//
//	@Description	Paginated list of roles with their permissions and configurations
type ListRolesResponse struct {
	Items     []RoleResponse   `json:"items"`     // Array of role configurations
	Paginator domain.Paginator `json:"paginator"` // Pagination information including total count and page details
}

// LinkUsersToRoleRequest input values for linking users to a role.
//
//	@Description	Request payload for assigning multiple users to a role
type LinkUsersToRoleRequest struct {
	UserIDs []uuid.UUID `json:"user_ids" format:"array" validate:"required" example:"019b4b0d-a682-7e34-a20c-c71a7147d7e7,019b4b0d-a682-7e38-b235-3dfcb59f4d9e"` // Array of user IDs to assign to the role
}

func (req *LinkUsersToRoleRequest) Validate() error {
	var errs domain.ValidationErrors

	if len(req.UserIDs) < 1 {
		errs.Add(&domain.ValidationError{Field: domain.FieldUserIDs, Message: "must be at least one user ID", Code: "REQUIRED"})
	}

	for i, userID := range req.UserIDs {
		if err := domain.ValidateUUID(userID, 7, fmt.Sprintf("%s[%d]", domain.FieldUserIDs, i)); err != nil {
			errs.Add(err)
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// UnlinkUsersFromRoleRequest input values for unlinking users from a role.
//
//	@Description	Request payload for removing multiple users from a role
type UnlinkUsersFromRoleRequest = LinkUsersToRoleRequest

// LinkPoliciesToRoleRequest input values for linking policies to a role.
//
//	@Description	Request payload for attaching multiple policies to a role
type LinkPoliciesToRoleRequest struct {
	PolicyIDs []uuid.UUID `json:"policy_ids" format:"array" validate:"required" example:"019b4b0d-a682-7e34-a20c-c71a7147d7e7,019b4b0d-a682-7e38-b235-3dfcb59f4d9e"` // Array of policy IDs to attach to the role
}

func (req *LinkPoliciesToRoleRequest) Validate() error {
	var errs domain.ValidationErrors

	if len(req.PolicyIDs) < 1 {
		errs.Add(&domain.ValidationError{Field: domain.FieldPolicyIDs, Message: "must be at least one policy ID", Code: "REQUIRED"})
	}

	for i, policyID := range req.PolicyIDs {
		if err := domain.ValidateUUID(policyID, 7, fmt.Sprintf("%s[%d]", domain.FieldPolicyIDs, i)); err != nil {
			errs.Add(err)
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// UnlinkPoliciesFromRoleRequest input values for unlinking policies from a role.
//
//	@Description	UnlinkPoliciesFromRoleRequest input values for unlinking policies from a role.
type UnlinkPoliciesFromRoleRequest = LinkPoliciesToRoleRequest
