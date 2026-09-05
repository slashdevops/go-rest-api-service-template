package payload

import (
	"reflect"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// UpdateMeRequest represents the input for the UpdateUser method.
//
//	@Description	Request payload for updating the authenticated user's profile information (all fields optional)
type UpdateMeRequest struct {
	FirstName *string `json:"first_name,omitempty" example:"John" format:"string" validate:"optional" minLength:"2" maxLength:"50"`              // Updated first name
	LastName  *string `json:"last_name,omitempty" example:"Doe" format:"string" validate:"optional" minLength:"2" maxLength:"50"`                // Updated last name
	Password  *string `json:"password,omitempty" example:"NewSecureP@ssw0rd" format:"password" validate:"optional" minLength:"8" maxLength:"72"` // New password (must meet security requirements)
}

func (req *UpdateMeRequest) Validate() error {
	if reflect.DeepEqual(req, &UpdateMeRequest{}) {
		return &domain.InvalidUserUpdateError{Message: "at least one field must be updated"}
	}

	var errs domain.ValidationErrors

	// Validate first name if provided
	if req.FirstName != nil {
		if normalizedFirstName, err := domain.ValidateString(*req.FirstName, domain.StringValidationOptions{
			MinLength:        domain.ValidUserFirstNameMinLength,
			MaxLength:        domain.ValidUserFirstNameMaxLength,
			TrimWhitespace:   true,
			AllowEmpty:       false,
			NoControlChars:   true,
			NoHTMLTags:       true,
			NoNullBytes:      true,
			NormalizeUnicode: true,
			FieldName:        domain.FieldFirstName,
		}); err != nil {
			errs.Add(err)
		} else {
			req.FirstName = &normalizedFirstName
		}
	}

	// Validate last name if provided
	if req.LastName != nil {
		if normalizedLastName, err := domain.ValidateString(*req.LastName, domain.StringValidationOptions{
			MinLength:        domain.ValidUserLastNameMinLength,
			MaxLength:        domain.ValidUserLastNameMaxLength,
			TrimWhitespace:   true,
			AllowEmpty:       false,
			NoControlChars:   true,
			NoHTMLTags:       true,
			NoNullBytes:      true,
			NormalizeUnicode: true,
			FieldName:        domain.FieldLastName,
		}); err != nil {
			errs.Add(err)
		} else {
			req.LastName = &normalizedLastName
		}
	}

	// Validate password if provided
	if req.Password != nil {
		errs.Add(domain.ValidatePassword(*req.Password, domain.FieldPassword))
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}
