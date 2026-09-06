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

	// IDPTypeNameEntraID is Microsoft Entra ID, one tenant per provider row.
	IDPTypeNameEntraID IDPTypeName = "EntraID"

	// IDPTypeNameOkta is Okta, one authorization server per provider row.
	IDPTypeNameOkta IDPTypeName = "Okta"
)

// IDPTypeKind is HOW the adapter talks to a provider. It used to be inferred
// from the name with a switch in the adapter, so a provider the adapter did
// not know about could be created and could never sign anybody in.
type IDPTypeKind string

const (
	// IDPTypeKindOIDC is OpenID Connect with discovery, PKCE, a nonce and an ID
	// token verified against the discovered JWKS. Google, Entra ID, Okta, and
	// any other compliant provider.
	IDPTypeKindOIDC IDPTypeKind = "oidc"

	// IDPTypeKindGithub is plain OAuth2 against GitHub's fixed endpoints.
	// GitHub has no OpenID Connect for users, so the identity is the numeric
	// user id and the email the primary VERIFIED address from /user/emails.
	IDPTypeKindGithub IDPTypeKind = "github"
)

func (k IDPTypeKind) String() string { return string(k) }

func (k IDPTypeKind) IsValid() bool {
	switch k {
	case IDPTypeKindOIDC, IDPTypeKindGithub:
		return true
	}

	return false
}

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
	IssuerHint     string
	Kind           IDPTypeKind
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
	IssuerHint     string
	Kind           IDPTypeKind
	Scopes         []string
	ID             uuid.UUID
}
