package payload

import (
	"reflect"

	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// ProductResponse is a type alias of domain.Product — they have the same shape
// on the wire (D-012). Handlers that construct payload.ProductResponse{...}
// keep working unchanged.
type ProductResponse = domain.Product

// CreateProductRequest represents the inputs necessary to create a new product.
//
//	@Description	Request payload for creating a new product within a project
type CreateProductRequest struct {
	Name        string    `json:"name" example:"Product Name" format:"string" validate:"required" minLength:"2" maxLength:"255"`              // Product name, unique within its project
	Description string    `json:"description" example:"This is a product" format:"string" validate:"required" minLength:"2" maxLength:"1024"` // Detailed description of the product
	ID          uuid.UUID `json:"id,omitempty" example:"01980434-b7ff-7abe-a45d-7311bc7011f5" format:"uuid" validate:"optional"`              // Optional custom product ID (auto-generated if not provided)
}

func (req *CreateProductRequest) Validate() error {
	var validationErrors domain.ValidationErrors

	if err := domain.ValidateUUID(req.ID, 7, domain.FieldID); err != nil {
		if ve, ok := err.(*domain.ValidationError); ok {
			validationErrors.Errors = append(validationErrors.Errors, *ve)
		}
	}

	normalizedName, err := domain.ValidateString(req.Name, domain.StringValidationOptions{
		MinLength:        domain.ProductNameMinLength,
		MaxLength:        domain.ProductNameMaxLength,
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
		MinLength:        domain.ProductDescriptionMinLength,
		MaxLength:        domain.ProductDescriptionMaxLength,
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

// UpdateProductRequest represents the inputs necessary to update a product.
//
//	@Description	Request payload for updating an existing product (all fields optional)
type UpdateProductRequest struct {
	Name        *string `json:"name,omitempty" example:"Updated Product Name" format:"string" validate:"optional" minLength:"2" maxLength:"255"`        // Updated product name
	Description *string `json:"description,omitempty" example:"Updated description" format:"string" validate:"optional" minLength:"2" maxLength:"1024"` // Updated product description
}

func (req *UpdateProductRequest) Validate() error {
	if reflect.DeepEqual(req, &UpdateProductRequest{}) {
		return &domain.ValidationError{
			Field:   domain.FieldRequest,
			Message: "at least one field must be provided for update",
			Code:    "REQUIRED_FIELD",
		}
	}

	var validationErrors domain.ValidationErrors

	if req.Name != nil {
		normalizedName, err := domain.ValidateString(*req.Name, domain.StringValidationOptions{
			MinLength:        domain.ProductNameMinLength,
			MaxLength:        domain.ProductNameMaxLength,
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
			MinLength:        domain.ProductDescriptionMinLength,
			MaxLength:        domain.ProductDescriptionMaxLength,
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

// ListProductsResponse represents a list of products.
//
//	@Description	Paginated list of products
type ListProductsResponse struct {
	Items     []ProductResponse `json:"items"`     // Array of products
	Paginator domain.Paginator  `json:"paginator"` // Pagination information including page tokens
}
