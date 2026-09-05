package domain

import (
	"errors"
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
)

func (e IDPEventType) String() string {
	return string(e)
}

var (
	IDPsFilterFields  = []string{FieldID, FieldName, FieldSystem, FieldCreatedAt, FieldUpdatedAt}
	IDPsSortFields    = []string{FieldID, FieldName, FieldSystem, FieldCreatedAt, FieldUpdatedAt}
	IDPsPartialFields = []string{
		FieldID, FieldName, FieldDescription, FieldSystem,
		FieldProjects, FieldLLMEngineTypes, FieldCreatedAt, FieldUpdatedAt,
	}
)

// IDPAvailable is the partial-projection IDP entity used by the "available providers" listing.
type IDPAvailable struct {
	Name        string
	Description string
	Logo        string
	IDPType     IDPTypes
	ID          uuid.UUID
}

// IDP is the identity provider entity.
type IDP struct {
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Name                string
	Description         string
	CallbackURL         string
	LoginRedirectURL    string
	RegisterRedirectURL string
	Logo                string
	ClientID            string
	ClientSecret        string
	IDPType             IDPTypes
	SerialID            int64
	ID                  uuid.UUID
}

type InsertIDPInput struct {
	Name                string
	Description         string
	CallbackURL         string
	LoginRedirectURL    string
	RegisterRedirectURL string
	Logo                string
	ClientID            string
	ClientSecret        string
	ID                  uuid.UUID
	IDPTypeID           uuid.UUID
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
		FieldName:                ref.Name,
		FieldDescription:         ref.Description,
		FieldCallbackURL:         ref.CallbackURL,
		FieldLoginRedirectURL:    ref.LoginRedirectURL,
		FieldRegisterRedirectURL: ref.RegisterRedirectURL,
		FieldClientID:            ref.ClientID,
		FieldClientSecret:        ref.ClientSecret,
	} {
		if val == "" {
			errs.Add(&ValidationError{Field: field, Message: field + " is required", Code: "REQUIRED"})
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type CreateIDPInput = InsertIDPInput

type UpdateIDPInput struct {
	IDPTypeID           *uuid.UUID
	Name                *string
	Description         *string
	CallbackURL         *string
	LoginRedirectURL    *string
	RegisterRedirectURL *string
	Logo                *string
	ClientID            *string
	ClientSecret        *string
	ID                  uuid.UUID
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
		{FieldRegisterRedirectURL, ref.RegisterRedirectURL},
		{FieldLogo, ref.Logo},
		{FieldClientID, ref.ClientID},
		{FieldClientSecret, ref.ClientSecret},
	}
	for _, c := range checks {
		if c.val != nil && *c.val == "" {
			errs.Add(&ValidationError{Field: c.field, Message: c.field + " is required", Code: "REQUIRED"})
		}
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
type UserInfo struct {
	Email     string
	FirstName string
	LastName  string
}

// IDPCallbackResult represents the result of an IDP callback.
type IDPCallbackResult interface {
	GetEventType() IDPEventType
	GetLoginResponse() (*LoginUserOutput, error)
	GetRegisterResponse() (*LoginUserOutput, error)
	GetUnknownResponse() error
}

type LoginCallbackResult struct {
	Result *LoginUserOutput
	Err    error
}

func (r *LoginCallbackResult) GetEventType() IDPEventType {
	return IDPEventTypeLogin
}

func (r *LoginCallbackResult) GetLoginResponse() (*LoginUserOutput, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	return r.Result, nil
}

func (r *LoginCallbackResult) GetRegisterResponse() (*LoginUserOutput, error) {
	return nil, errors.New("not applicable for login event")
}

func (r *LoginCallbackResult) GetUnknownResponse() error {
	return nil
}

type RegisterCallbackResult struct {
	Result *LoginUserOutput
	Err    error
}

func (r *RegisterCallbackResult) GetEventType() IDPEventType {
	return IDPEventTypeRegister
}

func (r *RegisterCallbackResult) GetLoginResponse() (*LoginUserOutput, error) {
	return nil, errors.New("not applicable for register event")
}

func (r *RegisterCallbackResult) GetRegisterResponse() (*LoginUserOutput, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	return r.Result, nil
}

func (r *RegisterCallbackResult) GetUnknownResponse() error {
	return r.Err
}

type UnknownCallbackResult struct {
	Err error
}

func (r *UnknownCallbackResult) GetEventType() IDPEventType {
	return IDPEventTypeUnknown
}

func (r *UnknownCallbackResult) GetLoginResponse() (*LoginUserOutput, error) {
	return nil, errors.New("not applicable for unknown event")
}

func (r *UnknownCallbackResult) GetRegisterResponse() (*LoginUserOutput, error) {
	return nil, errors.New("not applicable for unknown event")
}

func (r *UnknownCallbackResult) GetUnknownResponse() error {
	return r.Err
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
