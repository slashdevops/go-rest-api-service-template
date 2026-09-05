package domain

import "fmt"

// InvalidScopeTypeError represents an invalid resource limit scope type error
type InvalidScopeTypeError struct {
	ScopeType string // optional: the invalid scope type
	Message   string
}

func (e *InvalidScopeTypeError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "scopeType",
		Value:  e.ScopeType,
		Reason: e.Message,
	}).Error()
}

// InvalidResourceTypeError represents an invalid resource type error
type InvalidResourceTypeError struct {
	ResourceType string // optional: the invalid resource type
	Message      string
}

func (e *InvalidResourceTypeError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "resourceType",
		Value:  e.ResourceType,
		Reason: e.Message,
	}).Error()
}

// ResourcesLimitsInvalidSignatureError represents an invalid signature error for resource limits
type ResourcesLimitsInvalidSignatureError struct {
	Message string
}

func (e *ResourcesLimitsInvalidSignatureError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "signature",
		Reason: e.Message,
	}).Error()
}

// ResourcesLimitsHardLimitReachedError represents an error when a hard limit is reached
type ResourcesLimitsHardLimitReachedError struct {
	Resource string // optional: which resource
	Message  string
	Limit    int64 // optional: the limit value
	Current  int64 // optional: current usage
}

func (e *ResourcesLimitsHardLimitReachedError) Error() string {
	base := &BaseInvalidFieldError{
		Field:  "resourceLimit",
		Reason: "hard limit reached",
	}

	if e.Resource != "" {
		base.Value = e.Resource
	}

	if e.Message != "" {
		base.Reason = e.Message
	}

	return base.Error()
}

// InvalidPrivateKeyPEMError represents an invalid private key PEM format error
type InvalidPrivateKeyPEMError struct {
	Message string
}

func (e *InvalidPrivateKeyPEMError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "privateKeyPEM",
		Reason: e.Message,
	}).Error()
}

// InvalidPrivateKeyParseError represents an error parsing a private key
type InvalidPrivateKeyParseError struct {
	Err     error
	Message string
}

func (e *InvalidPrivateKeyParseError) Error() string {
	if e.Err != nil {
		return (&BaseWrappedError{
			Message: fmt.Sprintf("invalid private key parse: %s", e.Message),
			Err:     e.Err,
		}).Error()
	}

	return (&BaseInvalidFieldError{
		Field:  "privateKey",
		Reason: e.Message,
	}).Error()
}

func (e *InvalidPrivateKeyParseError) Unwrap() error {
	return e.Err
}

// InvalidPublicKeyPEMError represents an invalid public key PEM format error
type InvalidPublicKeyPEMError struct {
	Message string
}

func (e *InvalidPublicKeyPEMError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "publicKeyPEM",
		Reason: e.Message,
	}).Error()
}

// InvalidPublicKeyParseError represents an error parsing a public key
type InvalidPublicKeyParseError struct {
	Err     error
	Message string
}

func (e *InvalidPublicKeyParseError) Error() string {
	if e.Err != nil {
		return (&BaseWrappedError{
			Message: fmt.Sprintf("invalid public key parse: %s", e.Message),
			Err:     e.Err,
		}).Error()
	}

	return (&BaseInvalidFieldError{
		Field:  "publicKey",
		Reason: e.Message,
	}).Error()
}

func (e *InvalidPublicKeyParseError) Unwrap() error {
	return e.Err
}

// InvalidPublicKeyTypeError represents an invalid public key type error
type InvalidPublicKeyTypeError struct {
	Expected string // optional: expected type
	Got      string // optional: actual type
	Message  string
}

func (e *InvalidPublicKeyTypeError) Error() string {
	base := &BaseInvalidFieldError{
		Field:  "publicKeyType",
		Reason: e.Message,
	}

	if e.Got != "" {
		base.Value = e.Got
	}

	return base.Error()
}

// ResourcesLimitsAlreadyExistsError represents an error when resource limits already exist
type ResourcesLimitsAlreadyExistsError struct {
	ScopeType string // optional: which scope
	ScopeID   string // optional: scope identifier
	Message   string
}

func (e *ResourcesLimitsAlreadyExistsError) Error() string {
	base := &BaseAlreadyExistsError{
		Entity:  "resourcesLimits",
		Message: e.Message,
	}

	if e.ScopeType != "" {
		base.Field = "scopeType"
		base.Value = e.ScopeType
	}

	if e.ScopeID != "" {
		base.Field = "scopeID"
		base.Value = e.ScopeID
	}

	return base.Error()
}

// ResourcesLimitsForeignKeyError represents a foreign key constraint violation
type ResourcesLimitsForeignKeyError struct {
	Table      string // optional: which table
	Constraint string // optional: constraint name
	Message    string
}

func (e *ResourcesLimitsForeignKeyError) Error() string {
	base := &BaseInvalidFieldError{
		Field:  "foreignKey",
		Reason: "foreign key constraint violation",
	}

	if e.Table != "" {
		base.Value = e.Table
	}

	if e.Message != "" {
		base.Reason = e.Message
	}

	return base.Error()
}
