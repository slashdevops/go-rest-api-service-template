package domain

// InvalidInputError represents a validation error for input data
type InvalidInputError struct {
	Field   string // optional: specific field that's invalid
	Value   string // optional: the invalid value
	Message string // detailed error message
}

func (e *InvalidInputError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  e.Field,
		Value:  e.Value,
		Reason: e.Message,
	}).Error()
}

// InvalidDBConfigurationError represents a database configuration error
type InvalidDBConfigurationError struct {
	Message string
}

func (e *InvalidDBConfigurationError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "databaseConfiguration",
		Reason: e.Message,
	}).Error()
}

// InvalidDBMaxPingTimeoutError represents an invalid database ping timeout
type InvalidDBMaxPingTimeoutError struct {
	Message string
}

func (e *InvalidDBMaxPingTimeoutError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "dbMaxPingTimeout",
		Reason: e.Message,
	}).Error()
}

// InvalidDBMaxQueryTimeoutError represents an invalid database query timeout
type InvalidDBMaxQueryTimeoutError struct {
	Message string
}

func (e *InvalidDBMaxQueryTimeoutError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "dbMaxQueryTimeout",
		Reason: e.Message,
	}).Error()
}

// InvalidOTConfigurationError represents an invalid observability/telemetry configuration
type InvalidOTConfigurationError struct {
	Message string
}

func (e *InvalidOTConfigurationError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "observabilityConfiguration",
		Reason: e.Message,
	}).Error()
}

// InvalidByteSequenceError represents an invalid byte sequence error
type InvalidByteSequenceError struct {
	Message string
}

func (e *InvalidByteSequenceError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "byteSequence",
		Reason: e.Message,
	}).Error()
}

// InvalidMessageFormatError represents an invalid message format error
type InvalidMessageFormatError struct {
	Message string
}

func (e *InvalidMessageFormatError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "messageFormat",
		Reason: e.Message,
	}).Error()
}

// InvalidRepositoryError represents an error when a repository is invalid or nil
type InvalidRepositoryError struct {
	Repository string // optional: which repository
	Message    string
}

func (e *InvalidRepositoryError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "repository",
		Value:  e.Repository,
		Reason: e.Message,
	}).Error()
}

// InvalidRegoQueryError represents an invalid OPA Rego query
type InvalidRegoQueryError struct {
	Query   string // optional: the invalid query
	Message string
}

func (e *InvalidRegoQueryError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "regoQuery",
		Value:  e.Query,
		Reason: e.Message,
	}).Error()
}

// InvalidRegoPolicyError represents an invalid OPA Rego policy
type InvalidRegoPolicyError struct {
	Policy  string // optional: the invalid policy
	Message string
}

func (e *InvalidRegoPolicyError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "regoPolicy",
		Value:  e.Policy,
		Reason: e.Message,
	}).Error()
}

// InvalidCacheServiceError represents an invalid cache service error
type InvalidCacheServiceError struct {
	Service string // optional: which cache service
	Message string
}

func (e *InvalidCacheServiceError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "cacheService",
		Value:  e.Service,
		Reason: e.Message,
	}).Error()
}

// InvalidMailQueueServiceError represents an invalid mail queue service error
type InvalidMailQueueServiceError struct {
	Message string
}

func (e *InvalidMailQueueServiceError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "mailQueueService",
		Reason: e.Message,
	}).Error()
}

// InvalidPrivateKeyError represents an invalid private key error
type InvalidPrivateKeyError struct {
	Message string
}

func (e *InvalidPrivateKeyError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "privateKey",
		Reason: e.Message,
	}).Error()
}

// InvalidPublicKeyError represents an invalid public key error
type InvalidPublicKeyError struct {
	Message string
}

func (e *InvalidPublicKeyError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "publicKey",
		Reason: e.Message,
	}).Error()
}

// InvalidIssuerError represents an invalid JWT issuer error
type InvalidIssuerError struct {
	Issuer  string // optional: the invalid issuer
	Message string
}

func (e *InvalidIssuerError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "issuer",
		Value:  e.Issuer,
		Reason: e.Message,
	}).Error()
}

// InvalidSymmetricKeyError represents an invalid symmetric key error
type InvalidSymmetricKeyError struct {
	Message string
}

func (e *InvalidSymmetricKeyError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "symmetricKey",
		Reason: e.Message,
	}).Error()
}

// InvalidServiceError represents a generic invalid service error
type InvalidServiceError struct {
	Service string // optional: which service
	Message string
}

func (e *InvalidServiceError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "service",
		Value:  e.Service,
		Reason: e.Message,
	}).Error()
}

// InvalidRequestError represents a generic invalid request error
type InvalidRequestError struct {
	Field   string // optional: which field is invalid
	Message string
}

func (e *InvalidRequestError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  e.Field,
		Reason: e.Message,
	}).Error()
}

// InternalServerError represents an internal server error
type InternalServerError struct {
	Err     error // optional: wrapped error
	Message string
}

func (e *InternalServerError) Error() string {
	if e.Err != nil {
		return (&BaseWrappedError{
			Message: "internal server error: " + e.Message,
			Err:     e.Err,
		}).Error()
	}

	return (&BaseInvalidFieldError{
		Field:  "server",
		Reason: e.Message,
	}).Error()
}

func (e *InternalServerError) Unwrap() error {
	return e.Err
}

// InvalidUUIDError represents an invalid UUID error
type InvalidUUIDError struct {
	UUID    string
	Message string
}

func (e *InvalidUUIDError) Error() string {
	base := &BaseInvalidFieldError{
		Field:  "UUID",
		Value:  e.UUID,
		Reason: e.Message,
	}
	if e.UUID == "" {
		base.Reason = "empty string"
	}
	return base.Error()
}

// InvalidAuthnServiceError represents an invalid authentication service error
type InvalidAuthnServiceError struct {
	Message string
}

func (e *InvalidAuthnServiceError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "authenticationService",
		Reason: e.Message,
	}).Error()
}

// InvalidIDPsServiceError represents an invalid identity providers service error
type InvalidIDPsServiceError struct {
	Message string
}

func (e *InvalidIDPsServiceError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "identityProvidersService",
		Reason: e.Message,
	}).Error()
}

// InvalidNameError represents an invalid name error
type InvalidNameError struct {
	Name    string // optional: the invalid name
	Message string
}

func (e *InvalidNameError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "name",
		Value:  e.Name,
		Reason: e.Message,
	}).Error()
}

// InvalidResourcesLimitsError represents an invalid resources limits error
type InvalidResourcesLimitsError struct {
	Message string
}

func (e *InvalidResourcesLimitsError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "resourcesLimits",
		Reason: e.Message,
	}).Error()
}

// InvalidUserAgentError represents an invalid user agent error
type InvalidUserAgentError struct {
	UserAgent string // optional: the invalid user agent
	Message   string
}

func (e *InvalidUserAgentError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "userAgent",
		Value:  e.UserAgent,
		Reason: e.Message,
	}).Error()
}

// InvalidCipherError represents a missing cipher dependency.
type InvalidCipherError struct {
	Message string
}

func (e *InvalidCipherError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "cipher",
		Reason: e.Message,
	}).Error()
}

// InvalidPolicyEngineError represents a missing policy engine dependency.
type InvalidPolicyEngineError struct {
	Message string
}

func (e *InvalidPolicyEngineError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "policyEngine",
		Reason: e.Message,
	}).Error()
}

// InvalidTokenSignerError represents a missing token signer dependency.
type InvalidTokenSignerError struct {
	Message string
}

func (e *InvalidTokenSignerError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "tokenSigner",
		Reason: e.Message,
	}).Error()
}

// InvalidUserServiceError represents an invalid user service error
type InvalidUserServiceError struct {
	Message string
}

func (e *InvalidUserServiceError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "userService",
		Reason: e.Message,
	}).Error()
}

// InvalidGenerateCacheServiceError represents an invalid generate cache service error
type InvalidGenerateCacheServiceError struct {
	Message string
}

func (e *InvalidGenerateCacheServiceError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "generateCacheService",
		Reason: e.Message,
	}).Error()
}

// UndefinedColumnError represents an undefined database column error
type UndefinedColumnError struct {
	Column  string // optional: column name
	Message string
}

func (e *UndefinedColumnError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "column",
		Value:  e.Column,
		Reason: e.Message,
	}).Error()
}

// DatatypeMismatchError represents a datatype mismatch error
type DatatypeMismatchError struct {
	Expected string // optional: expected type
	Actual   string // optional: actual type
	Message  string
}

func (e *DatatypeMismatchError) Error() string {
	base := &BaseInvalidFieldError{
		Field:  "datatype",
		Reason: e.Message,
	}

	if e.Expected != "" && e.Actual != "" {
		base.Reason = "expected " + e.Expected + ", got " + e.Actual
		if e.Message != "" {
			base.Reason += ": " + e.Message
		}
	}

	return base.Error()
}
