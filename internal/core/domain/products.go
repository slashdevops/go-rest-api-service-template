package domain

import (
	"reflect"
	"time"

	"uuid"
)

const (
	ProductNameMinLength        = 2
	ProductNameMaxLength        = 255
	ProductDescriptionMinLength = 2
	ProductDescriptionMaxLength = 1024

	ProductsFound                      = "Products found"
	ProductsProductCreatedSuccessfully = "Product created successfully"
	ProductsProductUpdatedSuccessfully = "Product updated successfully"
	ProductsProductDeletedSuccessfully = "Product deleted successfully"
)

var (
	// ProductsFilterFields is a list of valid fields for filtering products.
	//
	// `price` and `currency` are deliberately absent. The layered
	// implementation this entity was ported from listed both here and in
	// ProductsSortFields, but the products table has never had either column —
	// so `?filter=price gt 10` parsed successfully and then produced SQL
	// referencing a column that does not exist. The lists now name only what
	// the table actually stores.
	ProductsFilterFields = []string{FieldID, FieldName, FieldCreatedAt, FieldUpdatedAt}

	// ProductsSortFields is a list of valid fields for sorting products.
	ProductsSortFields = []string{FieldID, FieldName, FieldCreatedAt, FieldUpdatedAt}

	// ProductsPartialFields is a list of valid fields for partial responses.
	ProductsPartialFields = []string{
		FieldID, FieldName, FieldDescription, FieldCreatedAt, FieldUpdatedAt,
	}
)

// Product is a project-scoped catalogue entry.
//
// json tags are carried on the domain type so payload.ProductResponse can be a
// type alias to it rather than a parallel struct that has to be kept in step.
// SerialID is excluded with json:"-".
//
//	@Description	Response containing a product
type Product struct {
	UpdatedAt   time.Time `json:"updated_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`    // Timestamp updated
	CreatedAt   time.Time `json:"created_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`    // Timestamp created
	Name        string    `json:"name,omitempty" example:"Product Name" format:"string"`                    // Product name, unique within its project
	Description string    `json:"description,omitempty" example:"This is a product" format:"string"`        // Description
	Project     Project   `json:"project,omitzero" format:"uuid"`                                           // Project that owns this product
	SerialID    int64     `json:"-"`                                                                        // Internal sequential identifier
	ID          uuid.UUID `json:"id,omitzero" example:"01980434-b7ff-7abe-a45d-7311bc7011f5" format:"uuid"` // Unique identifier
}

type InsertProductInput struct {
	Name        string
	Description string
	ProjectID   uuid.UUID
	// UserID is the caller. It is not stored on the row: it is the subject of
	// the membership check the repository applies, so a policy that grants
	// /projects/*/products cannot reach a project the caller is not in. OPA
	// authorises the path; this authorises the tenant.
	UserID uuid.UUID
	ID     uuid.UUID
}

func (ref *InsertProductInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ValidateUUID(ref.ID, 7, FieldID))
	errs.Add(ValidateUUID(ref.ProjectID, 7, FieldProjectID))
	errs.Add(ValidateUUID(ref.UserID, 7, FieldUserID))

	nameOptions := StringValidationOptions{
		MinLength: ProductNameMinLength, MaxLength: ProductNameMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldName,
	}
	if _, err := ValidateString(ref.Name, nameOptions); err != nil {
		errs.Add(err)
	}

	descriptionOptions := StringValidationOptions{
		MinLength: ProductDescriptionMinLength, MaxLength: ProductDescriptionMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldDescription,
	}
	if _, err := ValidateString(ref.Description, descriptionOptions); err != nil {
		errs.Add(err)
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type CreateProductInput = InsertProductInput

type UpdateProductInput struct {
	Name        *string
	Description *string
	ID          uuid.UUID
	ProjectID   uuid.UUID
	// UserID is the caller; see InsertProductInput.UserID.
	UserID uuid.UUID
}

func (ref *UpdateProductInput) Validate() error {
	var errs ValidationErrors

	if reflect.DeepEqual(ref, &UpdateProductInput{}) {
		errs.Add(&ValidationError{
			Field:   FieldGeneral,
			Message: "at least one field must be updated",
			Code:    "NO_FIELDS_TO_UPDATE",
		})
	}

	errs.Add(ValidateUUID(ref.ID, 7, FieldID))
	errs.Add(ValidateUUID(ref.ProjectID, 7, FieldProjectID))
	errs.Add(ValidateUUID(ref.UserID, 7, FieldUserID))

	if ref.Name != nil {
		nameOptions := StringValidationOptions{
			MinLength: ProductNameMinLength, MaxLength: ProductNameMaxLength,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
			NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldName,
		}
		if _, err := ValidateString(*ref.Name, nameOptions); err != nil {
			errs.Add(err)
		}
	}

	if ref.Description != nil {
		descriptionOptions := StringValidationOptions{
			MinLength: ProductDescriptionMinLength, MaxLength: ProductDescriptionMaxLength,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
			NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldDescription,
		}
		if _, err := ValidateString(*ref.Description, descriptionOptions); err != nil {
			errs.Add(err)
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type DeleteProductInput struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	// UserID is the caller; see InsertProductInput.UserID.
	UserID uuid.UUID
}

func (ref *DeleteProductInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ValidateUUID(ref.ID, 7, FieldID))
	errs.Add(ValidateUUID(ref.ProjectID, 7, FieldProjectID))
	errs.Add(ValidateUUID(ref.UserID, 7, FieldUserID))

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type SelectProductsInput struct {
	Sort      string
	Filter    string
	Fields    string
	Paginator Paginator
}

func (ref *SelectProductsInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ref.Paginator.Validate())

	if ref.Sort != "" {
		if _, err := ProductsSortParser.Parse(ref.Sort); err != nil {
			errs.Add(&ValidationError{Field: FieldSort, Message: err.Error(), Code: "INVALID_SORT_FIELD"})
		}
	}

	if ref.Filter != "" {
		if _, err := ProductsFilterParser.Parse(ref.Filter); err != nil {
			errs.Add(&ValidationError{Field: FieldFilter, Message: err.Error(), Code: "INVALID_FILTER_FIELD"})
		}
	}

	if ref.Fields != "" {
		if _, err := ProductsFieldsParser.Parse(ref.Fields); err != nil {
			errs.Add(&ValidationError{Field: FieldFields, Message: err.Error(), Code: "INVALID_FIELD"})
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type ListProductsInput = SelectProductsInput

type SelectProductsOutput struct {
	Items     []Product
	Paginator Paginator
}

type ListProductsOutput = SelectProductsOutput
