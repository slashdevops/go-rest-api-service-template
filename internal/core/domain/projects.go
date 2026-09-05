package domain

import (
	"fmt"
	"time"

	"uuid"
)

const (
	ProjectNameMinLength        = 2
	ProjectNameMaxLength        = 70
	ProjectDescriptionMinLength = 2
	ProjectDescriptionMaxLength = 1024

	ProjectsProjectCreatedSuccessfully = "Project created successfully"
	ProjectsProjectUpdatedSuccessfully = "Project updated successfully"
	ProjectsProjectDeletedSuccessfully = "Project deleted successfully"

	ProjectsUsersLinkedSuccessfully   = "Users linked to project successfully"
	ProjectsUsersUnlinkedSuccessfully = "Users unlinked from project successfully"
)

var (
	// ProjectFilterFields is a list of valid fields for filtering models.
	ProjectFilterFields = []string{FieldID, FieldName, FieldDisabled, FieldCreatedAt, FieldUpdatedAt}

	// ProjectSortFields is a list of valid fields for sorting models.
	ProjectSortFields = []string{FieldID, FieldName, FieldDisabled, FieldCreatedAt, FieldUpdatedAt}

	// ProjectPartialFields is a list of valid fields for partial responses.
	ProjectPartialFields = []string{FieldID, FieldName, FieldDescription, FieldDisabled, FieldCreatedAt, FieldUpdatedAt}
)

// Project is the project workspace entity.
//
// Note (D-011/D-012): json tags carried on the entity so that
// payload.ProjectResponse can be a type alias to this. The HTTP wire
// shape is preserved; SerialID is excluded with json:"-".
//
//	@Description	Project represents a project.
type Project struct {
	CreatedAt time.Time `json:"created_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"` // Timestamp when the project was created
	UpdatedAt time.Time `json:"updated_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"` // Timestamp when the project was last updated

	// Disabled indicates if the project is disabled.
	// Pointer to distinguish between false and unset values in partial responses.
	Disabled *bool `json:"disabled,omitzero" example:"false"` // Indicates if the project is currently disabled
	System   *bool `json:"system,omitzero" example:"false"`   // Indicates if this is a system-managed project (cannot be deleted)

	Name        string `json:"name,omitempty" example:"My Project" format:"string"`                     // Project display name
	Description string `json:"description,omitempty" example:"This is my main project" format:"string"` // Detailed description of the project's purpose

	SerialID int64     `json:"-"`                                                                                  // Internal sequential identifier, not serialized
	ID       uuid.UUID `json:"id,omitempty,omitzero" example:"019b4b0d-a682-7e34-a20c-c71a7147d7e7" format:"uuid"` // Unique identifier for the project
}

type InsertProjectInput struct {
	Name        string
	Description string
	ID          uuid.UUID
	UserID      uuid.UUID
	Disabled    bool
	System      bool
}

func (ref *InsertProjectInput) Validate() error {
	var validationErrors ValidationErrors

	if err := ValidateUUID(ref.ID, 7, FieldID); err != nil {
		if ve, ok := err.(*ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	}

	if err := ValidateUUID(ref.UserID, 7, FieldUserID); err != nil {
		if ve, ok := err.(*ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	}

	normalizedName, err := ValidateString(ref.Name, StringValidationOptions{
		MinLength:        ProjectNameMinLength,
		MaxLength:        ProjectNameMaxLength,
		TrimWhitespace:   true,
		AllowEmpty:       false,
		NoControlChars:   true,
		NoHTMLTags:       true,
		NoScriptTags:     true,
		NoNullBytes:      true,
		NormalizeUnicode: true,
		FieldName:        FieldName,
	})
	if err != nil {
		if ve, ok := err.(*ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	} else {
		ref.Name = normalizedName
	}

	normalizedDescription, err := ValidateString(ref.Description, StringValidationOptions{
		MinLength:        ProjectDescriptionMinLength,
		MaxLength:        ProjectDescriptionMaxLength,
		TrimWhitespace:   true,
		AllowEmpty:       false,
		NoControlChars:   true,
		NoHTMLTags:       true,
		NoScriptTags:     true,
		NoNullBytes:      true,
		NormalizeUnicode: true,
		FieldName:        FieldDescription,
	})
	if err != nil {
		if ve, ok := err.(*ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	} else {
		ref.Description = normalizedDescription
	}

	if validationErrors.HasErrors() {
		return &validationErrors
	}

	return nil
}

type CreateProjectInput = InsertProjectInput

type UpdateProjectInput struct {
	Name        *string
	Description *string
	Disabled    *bool
	ID          uuid.UUID
	UserID      uuid.UUID
}

func (ref *UpdateProjectInput) Validate() error {
	var validationErrors ValidationErrors

	if err := ValidateUUID(ref.ID, 7, FieldID); err != nil {
		if ve, ok := err.(*ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	}

	if err := ValidateUUID(ref.UserID, 7, FieldUserID); err != nil {
		if ve, ok := err.(*ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	}

	if ref.Name != nil {
		normalizedName, err := ValidateString(*ref.Name, StringValidationOptions{
			MinLength:        ProjectNameMinLength,
			MaxLength:        ProjectNameMaxLength,
			TrimWhitespace:   true,
			AllowEmpty:       false,
			NoControlChars:   true,
			NoHTMLTags:       true,
			NoScriptTags:     true,
			NoNullBytes:      true,
			NormalizeUnicode: true,
			FieldName:        FieldName,
		})
		if err != nil {
			if ve, ok := err.(*ValidationError); ok {
				validationErrors.Errors = append(validationErrors.Errors, *ve)
			}
		} else {
			*ref.Name = normalizedName
		}
	}

	if ref.Description != nil {
		normalizedDescription, err := ValidateString(*ref.Description, StringValidationOptions{
			MinLength:        ProjectDescriptionMinLength,
			MaxLength:        ProjectDescriptionMaxLength,
			TrimWhitespace:   true,
			AllowEmpty:       false,
			NoControlChars:   true,
			NoHTMLTags:       true,
			NoScriptTags:     true,
			NoNullBytes:      true,
			NormalizeUnicode: true,
			FieldName:        FieldDescription,
		})
		if err != nil {
			if ve, ok := err.(*ValidationError); ok {
				validationErrors.Errors = append(validationErrors.Errors, *ve)
			}
		} else {
			*ref.Description = normalizedDescription
		}
	}

	if validationErrors.HasErrors() {
		return &validationErrors
	}

	return nil
}

type DeleteProjectInput struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

func (ref *DeleteProjectInput) Validate() error {
	var validationErrors ValidationErrors
	if err := ValidateUUID(ref.ID, 7, FieldID); err != nil {
		if ve, ok := err.(*ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	}

	if err := ValidateUUID(ref.UserID, 7, FieldUserID); err != nil {
		if ve, ok := err.(*ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	}

	if validationErrors.HasErrors() {
		return &validationErrors
	}

	return nil
}

type SelectProjectsInput struct {
	Sort      string
	Filter    string
	Fields    string
	Paginator Paginator
	UserID    uuid.UUID
}

func (ref *SelectProjectsInput) Validate() error {
	var validationErrors ValidationErrors

	if err := ValidateUUID(ref.UserID, 7, FieldUserID); err != nil {
		if ve, ok := err.(*ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	}

	if err := ref.Paginator.Validate(); err != nil {
		if ve, ok := err.(*ValidationErrors); ok {
			validationErrors.Errors = append(validationErrors.Errors, ve.Errors...)
		} else {
			validationErrors.AddError("paginator", err.Error(), "INVALID_PAGINATION")
		}
	}

	if err := ValidateSortExpression(ref.Sort, FieldSort); err != nil {
		if ve, ok := err.(*ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	}

	if ref.Sort != "" {
		if _, err := ProjectSortParser.Parse(ref.Sort); err != nil {
			validationErrors.AddError(FieldSort, err.Error(), "INVALID_SORT")
		}
	}

	if err := ValidateFilterExpression(ref.Filter, FieldFilter); err != nil {
		if ve, ok := err.(*ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	}

	if ref.Filter != "" {
		if _, err := ProjectFilterParser.Parse(ref.Filter); err != nil {
			validationErrors.AddError(FieldFilter, err.Error(), "INVALID_FILTER")
		}
	}

	if err := ValidateFieldsExpression(ref.Fields, FieldFields); err != nil {
		if ve, ok := err.(*ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	}

	if ref.Fields != "" {
		if _, err := ProjectFieldsParser.Parse(ref.Fields); err != nil {
			validationErrors.AddError(FieldFields, err.Error(), "INVALID_FIELDS")
		}
	}

	if validationErrors.HasErrors() {
		return &validationErrors
	}

	return nil
}

type ListProjectsInput = SelectProjectsInput

type SelectProjectsOutput struct {
	Items     []Project
	Paginator Paginator
}

type ListProjectsOutput = SelectProjectsOutput

type LinkUsersToProjectInput struct {
	UserIDs   []uuid.UUID
	ProjectID uuid.UUID
}

func (req *LinkUsersToProjectInput) Validate() error {
	var validationErrors ValidationErrors

	if err := ValidateUUID(req.ProjectID, 7, FieldProjectID); err != nil {
		if ve, ok := err.(*ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	}

	if len(req.UserIDs) < 1 {
		validationErrors.AddError("user_ids", "at least one user ID is required", "REQUIRED")
		return &validationErrors
	}

	for i, userID := range req.UserIDs {
		if err := ValidateUUID(userID, 7, fmt.Sprintf("user_ids[%d]", i)); err != nil {
			if ve, ok := err.(*ValidationError); ok {
				validationErrors.Errors = append(validationErrors.Errors, *ve)
			}
		}
	}

	if validationErrors.HasErrors() {
		return &validationErrors
	}

	return nil
}

type UnlinkUsersFromProjectInput = LinkUsersToProjectInput
