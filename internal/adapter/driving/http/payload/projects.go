package payload

import (
	"fmt"
	"reflect"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// ProjectResponse is a type alias of domain.Project — they have the
// same shape on the wire (D-012). Handlers that construct
// payload.ProjectResponse{...} keep working unchanged.
type ProjectResponse = domain.Project

// CreateProjectRequest represents the inputs necessary to create a new project.
//
//	@Description	Request payload for creating a new project workspace
type CreateProjectRequest struct {
	Name        string    `json:"name" example:"My New Project" format:"string" validate:"required" minLength:"2" maxLength:"100"`                             // Project display name
	Description string    `json:"description" example:"A workspace for team collaboration" format:"string" validate:"required" minLength:"10" maxLength:"500"` // Detailed description of the project's purpose
	ID          uuid.UUID `json:"id,omitempty" example:"019b4b0d-a682-7dea-9751-9b2bb20b0132" format:"uuid" validate:"optional"`                               // Optional custom project ID (auto-generated if not provided)
}

func (req *CreateProjectRequest) Validate() error {
	var validationErrors domain.ValidationErrors

	if err := domain.ValidateUUID(req.ID, 7, domain.FieldID); err != nil {
		if ve, ok := err.(*domain.ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	}

	normalizedName, err := domain.ValidateString(req.Name, domain.StringValidationOptions{
		MinLength:        domain.ProjectNameMinLength,
		MaxLength:        domain.ProjectNameMaxLength,
		TrimWhitespace:   true,
		AllowEmpty:       false,
		NoControlChars:   true,
		NoHTMLTags:       true,
		NoScriptTags:     true,
		NoNullBytes:      true,
		NormalizeUnicode: true,
		FieldName:        domain.FieldName,
	})
	if err != nil {
		if ve, ok := err.(*domain.ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	} else {
		req.Name = normalizedName
	}

	normalizedDescription, err := domain.ValidateString(req.Description, domain.StringValidationOptions{
		MinLength:        domain.ProjectDescriptionMinLength,
		MaxLength:        domain.ProjectDescriptionMaxLength,
		TrimWhitespace:   true,
		AllowEmpty:       false,
		NoControlChars:   true,
		NoHTMLTags:       true,
		NoScriptTags:     true,
		NoNullBytes:      true,
		NormalizeUnicode: true,
		FieldName:        domain.FieldDescription,
	})
	if err != nil {
		if ve, ok := err.(*domain.ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	} else {
		req.Description = normalizedDescription
	}

	if validationErrors.HasErrors() {
		return &validationErrors
	}

	return nil
}

// UpdateProjectRequest represents the inputs necessary to update a project.
//
//	@Description	Request payload for updating an existing project (all fields optional)
type UpdateProjectRequest struct {
	Name        *string `json:"name,omitempty" example:"Updated Project Name" format:"string" validate:"optional" minLength:"2" maxLength:"100"`                // Updated project name
	Description *string `json:"description,omitempty" example:"Updated project description" format:"string" validate:"optional" minLength:"10" maxLength:"500"` // Updated project description
	Disabled    *bool   `json:"disabled,omitempty" example:"false" validate:"optional"`                                                                         // Set to true to disable the project, false to enable
}

func (req *UpdateProjectRequest) Validate() error {
	if reflect.DeepEqual(req, &UpdateProjectRequest{}) {
		return &domain.ValidationError{
			Field:   domain.FieldRequest,
			Message: "at least one field must be provided for update",
			Code:    "REQUIRED_FIELD",
		}
	}

	var validationErrors domain.ValidationErrors

	if req.Name != nil {
		normalizedName, err := domain.ValidateString(*req.Name, domain.StringValidationOptions{
			MinLength:        domain.ProjectNameMinLength,
			MaxLength:        domain.ProjectNameMaxLength,
			TrimWhitespace:   true,
			AllowEmpty:       false,
			NoControlChars:   true,
			NoHTMLTags:       true,
			NoScriptTags:     true,
			NoNullBytes:      true,
			NormalizeUnicode: true,
			FieldName:        domain.FieldName,
		})
		if err != nil {
			if ve, ok := err.(*domain.ValidationError); ok {
				validationErrors.Errors = append(validationErrors.Errors, *ve)
			}
		} else {
			*req.Name = normalizedName
		}
	}

	if req.Description != nil {
		normalizedDescription, err := domain.ValidateString(*req.Description, domain.StringValidationOptions{
			MinLength:        domain.ProjectDescriptionMinLength,
			MaxLength:        domain.ProjectDescriptionMaxLength,
			TrimWhitespace:   true,
			AllowEmpty:       false,
			NoControlChars:   true,
			NoHTMLTags:       true,
			NoScriptTags:     true,
			NoNullBytes:      true,
			NormalizeUnicode: true,
			FieldName:        domain.FieldDescription,
		})
		if err != nil {
			if ve, ok := err.(*domain.ValidationError); ok {
				validationErrors.Errors = append(validationErrors.Errors, *ve)
			}
		} else {
			*req.Description = normalizedDescription
		}
	}

	if validationErrors.HasErrors() {
		return &validationErrors
	}

	return nil
}

// ListProjectsResponse represents a list of projects.
//
//	@Description	Paginated list of project workspaces
type ListProjectsResponse struct {
	Items     []ProjectResponse `json:"items"`     // Array of project configurations
	Paginator domain.Paginator  `json:"paginator"` // Pagination information including total count and page details
}

// LinkUsersToProjectRequest represents the input for the LinkUserToProject method.
//
//	@Description	Request payload for adding users to a project workspace
type LinkUsersToProjectRequest struct {
	UserIDs []uuid.UUID `json:"user_ids" format:"array" example:"019b4b0d-a682-7e34-a20c-c71a7147d7e7,019b4b0d-a682-7e38-b235-3dfcb59f4d9e"` // Array of user IDs to add to the project
}

func (req *LinkUsersToProjectRequest) Validate() error {
	var validationErrors domain.ValidationErrors

	if len(req.UserIDs) < 1 {
		validationErrors.AddError("user_ids", "at least one user ID is required", "REQUIRED")
		return &validationErrors
	}

	for i, userID := range req.UserIDs {
		if err := domain.ValidateUUID(userID, 7, fmt.Sprintf("user_ids[%d]", i)); err != nil {
			if ve, ok := err.(*domain.ValidationError); ok {
				validationErrors.Errors = append(validationErrors.Errors, *ve)
			}
		}
	}

	if validationErrors.HasErrors() {
		return &validationErrors
	}

	return nil
}

// UnlinkUsersFromProjectRequest represents the input for the UnlinkUserFromProject method.
//
//	@Description	Request payload for removing users from a project workspace
type UnlinkUsersFromProjectRequest = LinkUsersToProjectRequest
