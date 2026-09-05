package domain

import (
	"time"

	"uuid"
)

// IDPTypeName represents the name of an identity provider type.
type IDPTypeName string

const (
	// IDPTypeNameGoogle is the identity provider type for Google.
	IDPTypeNameGoogle IDPTypeName = "Google"

	// IDPTypeNameGithub is the identity provider type for Github.
	IDPTypeNameGithub IDPTypeName = "Github"
)

// String returns the string representation of the IDPTypeName.
func (n IDPTypeName) String() string {
	return string(n)
}

var (
	// IDPTypesFilterFields is a list of valid fields for filtering idp types.
	IDPTypesFilterFields = []string{FieldID, FieldName, FieldSystem, FieldCreatedAt, FieldUpdatedAt}

	// IDPTypesSortFields is a list of valid fields for sorting idp types.
	IDPTypesSortFields = []string{FieldID, FieldName, FieldSystem, FieldCreatedAt, FieldUpdatedAt}

	// IDPTypesPartialFields is a list of valid fields for partial responses.
	IDPTypesPartialFields = []string{FieldID, FieldName, FieldDescription, FieldSystem, FieldCreatedAt, FieldUpdatedAt}
)

// IDPTypes is the identity-provider-type entity.
type IDPTypes struct {
	CreatedAt      time.Time
	UpdatedAt      time.Time
	System         *bool
	Name           string
	Description    string
	UserInfoAPIURL string
	Scopes         []string
	SerialID       int64
	ID             uuid.UUID
}

type SelectIDPTypesInput struct {
	Sort      string
	Filter    string
	Fields    string
	Paginator Paginator
}

func (ref *SelectIDPTypesInput) Validate() error {
	var errs ValidationErrors

	// Validate paginator
	errs.Add(ref.Paginator.Validate())

	// Validate sort expression
	errs.Add(ValidateSortExpression(ref.Sort, FieldSort))

	// Validate filter expression
	errs.Add(ValidateFilterExpression(ref.Filter, FieldFilter))

	// Validate fields expression
	errs.Add(ValidateFieldsExpression(ref.Fields, FieldFields))

	// Additional business logic validation for sort fields
	if ref.Sort != "" {
		_, err := IDPTypesSortParser.Parse(ref.Sort)
		if err != nil {
			errs.AddError(FieldSort, err.Error(), "INVALID_SORT_FIELD")
		}
	}

	// Additional business logic validation for filter fields
	if ref.Filter != "" {
		_, err := IDPTypesFilterParser.Parse(ref.Filter)
		if err != nil {
			errs.AddError(FieldFilter, err.Error(), "INVALID_FILTER_FIELD")
		}
	}

	// Additional business logic validation for fields
	if ref.Fields != "" {
		_, err := IDPTypesFieldsParser.Parse(ref.Fields)
		if err != nil {
			errs.AddError(FieldFields, err.Error(), "INVALID_FIELD")
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type ListIDPTypesInput = SelectIDPTypesInput

type SelectIDPTypesOutput struct {
	Items     []IDPTypes
	Paginator Paginator
}

type ListIDPTypesOutput = SelectIDPTypesOutput

// IDPTypeDecoder is a partial-projection of an identity provider type used by
// IDP-specific decoders (e.g. Google/GitHub user-info endpoints).
type IDPTypeDecoder struct {
	Name           string
	UserInfoAPIURL string
	Scopes         []string
	ID             uuid.UUID
}
