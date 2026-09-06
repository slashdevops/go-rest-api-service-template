package domain

import (
	"net/url"
	"time"

	"uuid"
)

const (
	ValidIDPNameMinLength         = 2
	ValidIDPNameMaxLength         = 100
	ValidIDPDescriptionMinLength  = 10
	ValidIDPDescriptionMaxLength  = 500
	ValidIDPLogoMinLength         = 10
	ValidIDPLogoMaxLength         = 1024
	ValidIDPClientIDMinLength     = 2
	ValidIDPClientIDMaxLength     = 255
	ValidIDPClientSecretMinLength = 2
	ValidIDPClientSecretMaxLength = 255

	IDPsIDPCreatedSuccessfully = "IDP created successfully"
	IDPsIDPUpdatedSuccessfully = "IDP updated successfully"
	IDPsIDPDeletedSuccessfully = "IDP deleted successfully"
	IDPsIDPFound               = "IDP found"
)

// IDPEventType represents the type of event for an identity provider.
type IDPEventType string

const (
	IDPEventTypeUnknown  IDPEventType = "unknown"
	IDPEventTypeLogin    IDPEventType = "login"
	IDPEventTypeRegister IDPEventType = "register"

	// IDPEventTypeLink is a signed-in user attaching a provider identity to
	// their own account. It is the only moment both sides of a link are
	// proven: the session proves the account, the provider proves the
	// identity. An IdP sign-in whose identity is unknown is refused, never
	// auto-linked by email.
	IDPEventTypeLink IDPEventType = "link"
)

func (e IDPEventType) String() string {
	return string(e)
}

var (
	IDPsFilterFields  = []string{FieldID, FieldName, FieldSystem, FieldCreatedAt, FieldUpdatedAt}
	IDPsSortFields    = []string{FieldID, FieldName, FieldSystem, FieldCreatedAt, FieldUpdatedAt}
	IDPsPartialFields = []string{
		FieldID, FieldName, FieldDescription, FieldCallbackURL, FieldIssuerURL, FieldLogo,
		FieldClientID, FieldEnabled, FieldAutoProvision, FieldCreatedAt, FieldUpdatedAt,
	}
)

// IDPAvailable is the partial-projection IDP entity used by the "available providers" listing.
type IDPAvailable struct {
	Name          string
	Description   string
	Logo          string
	IDPType       IDPTypes
	ID            uuid.UUID
	AutoProvision bool
}

// IDP is the identity provider entity.
type IDP struct {
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Name        string
	Description string

	// CallbackURL is the redirect_uri registered with the provider: the
	// FRONTEND's callback route, whose server hands state and code to this API.
	CallbackURL string

	// IssuerURL is, for the oidc kind, the issuer whose discovery document
	// describes the provider and the value the ID token's iss must equal. One
	// tenant per row: the issuer is what pins it.
	IssuerURL    string
	Logo         string
	ClientID     string
	ClientSecret string
	IDPType      IDPTypes
	SerialID     int64
	ID           uuid.UUID

	// Enabled: offered on the login page and accepted at the callback.
	Enabled bool

	// AutoProvision: a sign-in from an unknown identity with a provider-verified
	// email may create an account. Off means only linked identities sign in.
	AutoProvision bool
}

// IsOIDC reports whether the provider speaks OpenID Connect.
func (ref *IDP) IsOIDC() bool { return ref.IDPType.Kind == IDPTypeKindOIDC }

type InsertIDPInput struct {
	Name          string
	Description   string
	CallbackURL   string
	IssuerURL     string
	Logo          string
	ClientID      string
	ClientSecret  string
	ID            uuid.UUID
	IDPTypeID     uuid.UUID
	Enabled       bool
	AutoProvision bool
}

func (ref *InsertIDPInput) Validate() error {
	var errs ValidationErrors

	if err := ValidateUUID(ref.ID, 7, FieldID); err != nil {
		errs.Add(err)
	}
	if err := ValidateUUID(ref.IDPTypeID, 7, FieldIDPTypeID); err != nil {
		errs.Add(err)
	}

	for field, val := range map[string]string{
		FieldName:         ref.Name,
		FieldDescription:  ref.Description,
		FieldCallbackURL:  ref.CallbackURL,
		FieldClientID:     ref.ClientID,
		FieldClientSecret: ref.ClientSecret,
	} {
		if val == "" {
			errs.Add(&ValidationError{Field: field, Message: field + " is required", Code: "REQUIRED"})
		}
	}

	errs.Add(validateIDPURL(FieldCallbackURL, ref.CallbackURL, true))
	errs.Add(validateIDPURL(FieldIssuerURL, ref.IssuerURL, false))

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// validateIDPURL accepts an absolute http(s) URL. The callback is where the
// provider sends the browser and the issuer is where the adapter fetches
// discovery from, so a relative path, a bare host or another scheme is a
// provider that can never complete a sign-in -- refused here, where the
// operator sees it, rather than as a callback that never arrives.
//
// The issuer is validated only when set: the github kind has none, and which
// kind needs one is the use case's question because it has the type row.
func validateIDPURL(field, value string, required bool) error {
	if value == "" {
		if required {
			return &ValidationError{Field: field, Message: field + " is required", Code: "REQUIRED"}
		}

		return nil
	}

	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return &ValidationError{Field: field, Message: field + " must be an absolute http(s) URL", Code: "INVALID_URL"}
	}

	return nil
}

type CreateIDPInput = InsertIDPInput

type UpdateIDPInput struct {
	IDPTypeID     *uuid.UUID
	Name          *string
	Description   *string
	CallbackURL   *string
	IssuerURL     *string
	Logo          *string
	ClientID      *string
	ClientSecret  *string
	Enabled       *bool
	AutoProvision *bool
	ID            uuid.UUID
}

func (ref *UpdateIDPInput) Validate() error {
	var errs ValidationErrors

	if err := ValidateUUID(ref.ID, 7, FieldID); err != nil {
		errs.Add(err)
	}

	if ref.IDPTypeID != nil {
		if err := ValidateUUID(*ref.IDPTypeID, 7, FieldIDPTypeID); err != nil {
			errs.Add(err)
		}
	}

	checks := []struct {
		field string
		val   *string
	}{
		{FieldName, ref.Name},
		{FieldDescription, ref.Description},
		{FieldClientID, ref.ClientID},
		{FieldClientSecret, ref.ClientSecret},
	}
	for _, c := range checks {
		if c.val != nil && *c.val == "" {
			errs.Add(&ValidationError{Field: c.field, Message: c.field + " is required", Code: "REQUIRED"})
		}
	}

	if ref.CallbackURL != nil {
		errs.Add(validateIDPURL(FieldCallbackURL, *ref.CallbackURL, true))
	}

	// An empty issuer on update means "clear it", right for a row moving to
	// the github kind and wrong for an oidc row; the use case, which holds the
	// type, decides. Here only a non-empty value is parsed.
	if ref.IssuerURL != nil {
		errs.Add(validateIDPURL(FieldIssuerURL, *ref.IssuerURL, false))
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type DeleteIDPInput struct {
	ID uuid.UUID
}

func (ref *DeleteIDPInput) Validate() error {
	var errs ValidationErrors
	if err := ValidateUUID(ref.ID, 7, FieldID); err != nil {
		errs.Add(err)
	}
	if errs.HasErrors() {
		return &errs
	}
	return nil
}

// UserInfo represents the user information returned by an IDP.
// UserInfo is what a provider says about the person who just signed in.
//
// Subject is the identity; Email is a hint. EmailVerified is what the
// provider asserts, or what the kind implies (Entra ID, single tenant: the
// directory's own attribute), and it gates account creation.
type UserInfo struct {
	Subject       string
	Email         string
	FirstName     string
	LastName      string
	EmailVerified bool
}

// IDPCallbackOutput is what a provider callback produced.
//
// Login and Register end with a session, so Login carries the tokens the
// frontend stores. Link ends with a row and no new session -- the caller was
// already signed in -- so Login is nil and Linked says which account.
type IDPCallbackOutput struct {
	Login     *LoginUserOutput
	EventType IDPEventType
	Linked    uuid.UUID
}

type SelectIDPsInput struct {
	Sort      string
	Filter    string
	Fields    string
	Paginator Paginator
}

func (ref *SelectIDPsInput) Validate() error {
	var errs ValidationErrors

	if err := ref.Paginator.Validate(); err != nil {
		errs.Add(err)
	}
	if err := ValidateSortExpression(ref.Sort, FieldSort); err != nil {
		errs.Add(err)
	}
	if err := ValidateFilterExpression(ref.Filter, FieldFilter); err != nil {
		errs.Add(err)
	}
	if err := ValidateFieldsExpression(ref.Fields, FieldFields); err != nil {
		errs.Add(err)
	}

	if ref.Sort != "" {
		if _, err := IDPsSortParser.Parse(ref.Sort); err != nil {
			errs.Add(&ValidationError{Field: FieldSort, Message: err.Error(), Code: "INVALID_SORT_FIELD"})
		}
	}

	if ref.Filter != "" {
		if _, err := IDPsFilterParser.Parse(ref.Filter); err != nil {
			errs.Add(&ValidationError{Field: FieldFilter, Message: err.Error(), Code: "INVALID_FILTER_FIELD"})
		}
	}

	if ref.Fields != "" {
		if _, err := IDPsFieldsParser.Parse(ref.Fields); err != nil {
			errs.Add(&ValidationError{Field: FieldFields, Message: err.Error(), Code: "INVALID_FIELD"})
		}
	}

	if errs.HasErrors() {
		return &errs
	}
	return nil
}

type ListIDPsInput = SelectIDPsInput

type SelectIDPsOutput struct {
	Items     []IDP
	Paginator Paginator
}

type ListIDPsOutput = SelectIDPsOutput

type SelectIDPAvailableOutput struct {
	Items []IDPAvailable
}
