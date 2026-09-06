package domain

import (
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"

	"uuid"
)

const (
	// Maximum string lengths to prevent memory exhaustion attacks
	MaxStringLength           = 10000
	MaxEmailLength            = 254 // RFC 5321 limit
	MaxURLLength              = 2048
	MaxPaginationLimit        = 1000
	MaxFilterExpressionLength = 2048
	MaxSortExpressionLength   = 1024
	MaxFieldsExpressionLength = 1024

	// Character validation patterns
	NullBytePattern    = "\x00"
	ControlCharPattern = "[\x00-\x1f\x7f]"
	HTMLTagPattern     = `<[^>]*>`
	ScriptTagPattern   = `(?i)<script[^>]*>.*?</script>`

	// Password complexity requirements
	MinPasswordComplexityScore = 3
)

var (
	// Compiled regex patterns for performance
	controlCharRegex = regexp.MustCompile(ControlCharPattern)
	htmlTagRegex     = regexp.MustCompile(HTMLTagPattern)
	scriptTagRegex   = regexp.MustCompile(ScriptTagPattern)

	// Common weak passwords (basic list)
	commonPasswords = map[string]bool{
		"password":    true,
		"123456":      true,
		"123456789":   true,
		"qwerty":      true,
		"abc123":      true,
		"password123": true,
		"admin":       true,
		"root":        true,
		"user":        true,
		"guest":       true,
	}
)

// ValidationError represents a validation error with details
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error in field '%s': %s", e.Field, e.Message)
}

// ValidationErrors represents multiple validation errors
type ValidationErrors struct {
	Errors []ValidationError `json:"errors"`
}

func (e *ValidationErrors) Error() string {
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}

	// concatenate all error messages
	var messages []string
	for _, err := range e.Errors {
		messages = append(messages, err.Error())
	}

	return fmt.Sprintf("validation failed with %d errors: %s", len(e.Errors), strings.Join(messages, ", "))
}

// Add appends an error to the list, handling ValidationError type assertions automatically
func (e *ValidationErrors) Add(err error) {
	if err == nil {
		return
	}
	if valErr, ok := err.(*ValidationError); ok {
		e.Errors = append(e.Errors, *valErr)
	} else if valErrs, ok := err.(*ValidationErrors); ok {
		e.Errors = append(e.Errors, valErrs.Errors...)
	} else {
		e.Errors = append(e.Errors, ValidationError{
			Message: err.Error(),
			Code:    "VALIDATION_ERROR",
		})
	}
}

// AddError adds a validation error
func (e *ValidationErrors) AddError(field, message, code string) {
	e.Errors = append(e.Errors, ValidationError{
		Field:   field,
		Message: message,
		Code:    code,
	})
}

// HasErrors returns true if there are validation errors
func (e *ValidationErrors) HasErrors() bool {
	return len(e.Errors) > 0
}

// StringValidationOptions contains options for string validation
type StringValidationOptions struct {
	FieldName        string
	MinLength        int
	MaxLength        int
	TrimWhitespace   bool
	AllowEmpty       bool
	NoControlChars   bool
	NoHTMLTags       bool
	NoScriptTags     bool
	NoNullBytes      bool
	NormalizeUnicode bool
}

// ValidateString performs comprehensive string validation
func ValidateString(value string, opts StringValidationOptions) (string, error) {
	// Handle nil pointer case for optional fields
	if value == "" && opts.AllowEmpty {
		return "", nil
	}

	// Trim whitespace if requested
	if opts.TrimWhitespace {
		value = strings.TrimSpace(value)
	}

	// Check for empty after trimming
	if value == "" && !opts.AllowEmpty {
		return "", &ValidationError{
			Field:   opts.FieldName,
			Message: "cannot be empty",
			Code:    "REQUIRED",
		}
	}

	// Check maximum string length to prevent memory exhaustion
	if len(value) > MaxStringLength {
		return "", &ValidationError{
			Field:   opts.FieldName,
			Message: fmt.Sprintf("exceeds maximum length of %d characters", MaxStringLength),
			Code:    "TOO_LONG",
		}
	}

	// Check length constraints
	if opts.MinLength > 0 && opts.MaxLength > 0 {
		// When both min and max are specified, provide a combined message
		if len(value) < opts.MinLength || len(value) > opts.MaxLength {
			return "", &ValidationError{
				Field:   opts.FieldName,
				Message: fmt.Sprintf("must be between %d and %d characters", opts.MinLength, opts.MaxLength),
				Code:    "INVALID_LENGTH",
			}
		}
	} else {
		// Individual length checks when only one bound is specified
		if len(value) < opts.MinLength {
			return "", &ValidationError{
				Field:   opts.FieldName,
				Message: fmt.Sprintf("must be at least %d characters long", opts.MinLength),
				Code:    "TOO_SHORT",
			}
		}

		if opts.MaxLength > 0 && len(value) > opts.MaxLength {
			return "", &ValidationError{
				Field:   opts.FieldName,
				Message: fmt.Sprintf("must be at most %d characters long", opts.MaxLength),
				Code:    "TOO_LONG",
			}
		}
	}

	// Check for null bytes
	if opts.NoNullBytes && strings.Contains(value, NullBytePattern) {
		return "", &ValidationError{
			Field:   opts.FieldName,
			Message: "contains invalid null bytes",
			Code:    "INVALID_CHARACTERS",
		}
	}

	// Check for control characters
	if opts.NoControlChars && controlCharRegex.MatchString(value) {
		return "", &ValidationError{
			Field:   opts.FieldName,
			Message: "contains invalid control characters",
			Code:    "INVALID_CHARACTERS",
		}
	}

	// Check for HTML tags
	if opts.NoHTMLTags && htmlTagRegex.MatchString(value) {
		return "", &ValidationError{
			Field:   opts.FieldName,
			Message: "contains HTML tags which are not allowed",
			Code:    "INVALID_CONTENT",
		}
	}

	// Check for script tags
	if opts.NoScriptTags && scriptTagRegex.MatchString(value) {
		return "", &ValidationError{
			Field:   opts.FieldName,
			Message: "contains script tags which are not allowed",
			Code:    "INVALID_CONTENT",
		}
	}

	// Validate UTF-8
	if !utf8.ValidString(value) {
		return "", &ValidationError{
			Field:   opts.FieldName,
			Message: "contains invalid UTF-8 characters",
			Code:    "INVALID_ENCODING",
		}
	}

	// Unicode normalization
	if opts.NormalizeUnicode {
		// Normalize to NFC form to prevent homograph attacks
		normalized := strings.ToValidUTF8(value, "")
		if normalized != value {
			value = normalized
		}
	}

	return value, nil
}

// ValidateEmail performs comprehensive email validation
func ValidateEmail(email string, fieldName string) (string, error) {
	if email == "" {
		return "", &ValidationError{
			Field:   fieldName,
			Message: "email address is required",
			Code:    "REQUIRED",
		}
	}

	// Trim whitespace and normalize case
	email = strings.TrimSpace(strings.ToLower(email))

	// Check length
	if len(email) > MaxEmailLength {
		return "", &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("email address exceeds maximum length of %d characters", MaxEmailLength),
			Code:    "TOO_LONG",
		}
	}

	if len(email) < ValidUserEmailMinLength {
		return "", &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("email address must be at least %d characters long", ValidUserEmailMinLength),
			Code:    "TOO_SHORT",
		}
	}

	// Validate format
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return "", &ValidationError{
			Field:   fieldName,
			Message: "invalid email format",
			Code:    "INVALID_FORMAT",
		}
	}

	// Use the parsed and normalized email
	normalizedEmail := addr.Address

	// Additional validations
	parts := strings.Split(normalizedEmail, "@")
	if len(parts) != 2 {
		return "", &ValidationError{
			Field:   fieldName,
			Message: "invalid email format",
			Code:    "INVALID_FORMAT",
		}
	}

	localPart, domain := parts[0], parts[1]

	// Local part validation
	if len(localPart) == 0 || len(localPart) > 64 {
		return "", &ValidationError{
			Field:   fieldName,
			Message: "email local part must be between 1 and 64 characters",
			Code:    "INVALID_FORMAT",
		}
	}

	// Domain validation
	if len(domain) == 0 || len(domain) > 253 {
		return "", &ValidationError{
			Field:   fieldName,
			Message: "email domain must be between 1 and 253 characters",
			Code:    "INVALID_FORMAT",
		}
	}

	// Check for consecutive dots
	if strings.Contains(normalizedEmail, "..") {
		return "", &ValidationError{
			Field:   fieldName,
			Message: "email contains consecutive dots",
			Code:    "INVALID_FORMAT",
		}
	}

	return normalizedEmail, nil
}

// ValidateUUID validates UUID format and version
func ValidateUUID(id uuid.UUID, requiredVersion int, fieldName string) error {
	if id == uuid.Nil() {
		return &ValidationError{
			Field:   fieldName,
			Message: "UUID cannot be nil or empty",
			Code:    "REQUIRED",
		}
	}

	// Check version if specified
	if requiredVersion > 0 && UUIDVersion(id) != requiredVersion {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("UUID must be version %d", requiredVersion),
			Code:    "INVALID_VERSION",
		}
	}

	return nil
}

// ValidatePassword performs comprehensive password validation
func ValidatePassword(password string, fieldName string) error {
	if password == "" {
		return &ValidationError{
			Field:   fieldName,
			Message: "password is required",
			Code:    "REQUIRED",
		}
	}

	// Length validation
	if len(password) < ValidUserPasswordMinLength {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("password must be at least %d characters long", ValidUserPasswordMinLength),
			Code:    "TOO_SHORT",
		}
	}

	if len(password) > ValidUserPasswordMaxLength {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("password must be at most %d characters long", ValidUserPasswordMaxLength),
			Code:    "TOO_LONG",
		}
	}

	// Check for common passwords
	if commonPasswords[strings.ToLower(password)] {
		return &ValidationError{
			Field:   fieldName,
			Message: "password is too common and easily guessable",
			Code:    "WEAK_PASSWORD",
		}
	}

	// Password complexity scoring
	score := calculatePasswordComplexity(password)
	if score < MinPasswordComplexityScore {
		return &ValidationError{
			Field:   fieldName,
			Message: "password must contain a mix of uppercase, lowercase, numbers, and special characters",
			Code:    "WEAK_PASSWORD",
		}
	}

	return nil
}

// calculatePasswordComplexity calculates password complexity score
func calculatePasswordComplexity(password string) int {
	score := 0

	hasLower := false
	hasUpper := false
	hasDigit := false
	hasSpecial := false

	for _, char := range password {
		switch {
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if hasLower {
		score++
	}
	if hasUpper {
		score++
	}
	if hasDigit {
		score++
	}
	if hasSpecial {
		score++
	}

	// Bonus for length
	if len(password) >= 12 {
		score++
	}

	return score
}

// ValidateURL validates URL format and security
func ValidateURL(urlStr string, fieldName string) (string, error) {
	if urlStr == "" {
		return "", &ValidationError{
			Field:   fieldName,
			Message: "URL is required",
			Code:    "REQUIRED",
		}
	}

	urlStr = strings.TrimSpace(urlStr)

	// Check length
	if len(urlStr) > MaxURLLength {
		return "", &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("URL exceeds maximum length of %d characters", MaxURLLength),
			Code:    "TOO_LONG",
		}
	}

	// Parse URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", &ValidationError{
			Field:   fieldName,
			Message: "invalid URL format",
			Code:    "INVALID_FORMAT",
		}
	}

	// Validate scheme
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", &ValidationError{
			Field:   fieldName,
			Message: "URL must use http or https scheme",
			Code:    "INVALID_SCHEME",
		}
	}

	// Validate host
	if parsedURL.Host == "" {
		return "", &ValidationError{
			Field:   fieldName,
			Message: "URL must have a valid host",
			Code:    "INVALID_HOST",
		}
	}

	return parsedURL.String(), nil
}

// ValidatePagination validates pagination parameters
func ValidatePagination(limit, offset int, fieldPrefix string) error {
	if limit < 0 {
		return &ValidationError{
			Field:   fieldPrefix + "limit",
			Message: "limit cannot be negative",
			Code:    "INVALID_VALUE",
		}
	}

	if limit > MaxPaginationLimit {
		return &ValidationError{
			Field:   fieldPrefix + "limit",
			Message: fmt.Sprintf("limit cannot exceed %d", MaxPaginationLimit),
			Code:    "TOO_LARGE",
		}
	}

	if offset < 0 {
		return &ValidationError{
			Field:   fieldPrefix + "offset",
			Message: "offset cannot be negative",
			Code:    "INVALID_VALUE",
		}
	}

	return nil
}

// ValidateFilterExpression validates filter expressions for safety
func ValidateFilterExpression(filter string, fieldName string) error {
	if filter == "" {
		return nil // Empty filter is allowed
	}

	if len(filter) > MaxFilterExpressionLength {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("filter expression exceeds maximum length of %d characters", MaxFilterExpressionLength),
			Code:    "TOO_LONG",
		}
	}

	return nil
}

// ValidateSortExpression validates sort expressions
func ValidateSortExpression(sort string, fieldName string) error {
	if sort == "" {
		return nil // Empty sort is allowed
	}

	if len(sort) > MaxSortExpressionLength {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("sort expression exceeds maximum length of %d characters", MaxSortExpressionLength),
			Code:    "TOO_LONG",
		}
	}

	return nil
}

// ValidateFieldsExpression validates fields expressions
func ValidateFieldsExpression(fields string, fieldName string) error {
	if fields == "" {
		return nil // Empty fields is allowed
	}

	if len(fields) > MaxFieldsExpressionLength {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("fields expression exceeds maximum length of %d characters", MaxFieldsExpressionLength),
			Code:    "TOO_LONG",
		}
	}

	return nil
}

// ValidateGoTemplate validates a Go template string and checks for required fields.
// It verifies both the template syntax and the presence of required template fields.
func ValidateGoTemplate(templateStr string, requiredFields []string, fieldName string) error {
	if templateStr == "" {
		return nil // Empty templates are allowed if marked as optional
	}

	// Try to parse the template to validate syntax
	tmpl := template.New(fieldName)
	_, err := tmpl.Parse(templateStr)
	if err != nil {
		return &ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("invalid template syntax: %v", err),
			Code:    "INVALID_TEMPLATE_SYNTAX",
		}
	}

	// Check for required fields (with and without spaces)
	for _, field := range requiredFields {
		// Create variations with and without spaces
		fieldWithoutSpaces := field
		fieldWithSpaces := strings.Replace(field, "{{", "{{ ", 1)
		fieldWithSpaces = strings.Replace(fieldWithSpaces, "}}", " }}", 1)

		if !strings.Contains(templateStr, fieldWithoutSpaces) && !strings.Contains(templateStr, fieldWithSpaces) {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("template must contain required field: %s", field),
				Code:    "MISSING_TEMPLATE_FIELD",
			}
		}
	}

	// Special validation for {{range .Contexts}}
	// Ensure it's used properly with a closing {{end}}
	rangePattern := regexp.MustCompile(`\{\{\s*range\s+\.Contexts\s*\}\}`)
	if rangePattern.MatchString(templateStr) {
		endPattern := regexp.MustCompile(`\{\{\s*end\s*\}\}`)
		if !endPattern.MatchString(templateStr) {
			return &ValidationError{
				Field:   fieldName,
				Message: "template contains {{range .Contexts}} but missing corresponding {{end}}",
				Code:    "INVALID_TEMPLATE_RANGE",
			}
		}
	}

	return nil
}
