package domain

import (
	"fmt"
	"strings"

	"uuid"
)

// BaseNotFoundError provides a standard structure for "not found" errors
type BaseNotFoundError struct {
	Entity  string    // e.g., "user", "project", "model"
	Field   string    // optional: field name for non-ID lookups (e.g., "email", "name")
	Value   string    // optional: field value for non-ID lookups
	Message string    // optional: additional context
	ID      uuid.UUID // optional: entity ID
}

func (e *BaseNotFoundError) Error() string {
	parts := []string{e.Entity, "not found"}

	if e.ID != uuid.Nil() {
		parts = append(parts, fmt.Sprintf("ID=%s", e.ID))
	} else if e.Field != "" && e.Value != "" {
		parts = append(parts, fmt.Sprintf("%s=%s", e.Field, e.Value))
	} else if e.Field != "" {
		parts = append(parts, e.Field)
	}

	if e.Message != "" {
		parts = append(parts, e.Message)
	}

	return strings.Join(parts, ": ")
}

// BaseAlreadyExistsError provides a standard structure for "already exists" errors
type BaseAlreadyExistsError struct {
	Entity  string    // e.g., "user", "project", "model"
	Field   string    // which field conflicts (e.g., "email", "name")
	Value   string    // conflicting value
	Message string    // optional: additional context
	ID      uuid.UUID // optional: entity ID
}

func (e *BaseAlreadyExistsError) Error() string {
	parts := []string{e.Entity, "already exists"}

	if e.Field != "" && e.Value != "" {
		parts = append(parts, fmt.Sprintf("%s=%s", e.Field, e.Value))
	} else if e.ID != uuid.Nil() {
		parts = append(parts, fmt.Sprintf("ID=%s", e.ID))
	} else if e.Value != "" {
		parts = append(parts, e.Value)
	}

	if e.Message != "" {
		parts = append(parts, e.Message)
	}

	return strings.Join(parts, ": ")
}

// BaseInvalidFieldError provides a standard structure for field validation errors
type BaseInvalidFieldError struct {
	Entity    string // e.g., "user", "project", "model"
	Field     string // field name (e.g., "email", "name", "description")
	Value     string // optional: the invalid value
	Reason    string // what's wrong
	MinLength int    // optional: for length constraints
	MaxLength int    // optional: for length constraints
}

func (e *BaseInvalidFieldError) Error() string {
	prefix := "invalid"
	if e.Entity != "" && e.Field != "" {
		prefix = fmt.Sprintf("invalid %s %s", e.Entity, e.Field)
	} else if e.Field != "" {
		prefix = fmt.Sprintf("invalid %s", e.Field)
	}

	if e.MinLength > 0 && e.MaxLength > 0 {
		return fmt.Sprintf("%s: must be between %d and %d characters", prefix, e.MinLength, e.MaxLength)
	}

	if e.Reason != "" {
		return fmt.Sprintf("%s: %s", prefix, e.Reason)
	}

	if e.Value != "" {
		return fmt.Sprintf("%s: '%s'", prefix, e.Value)
	}

	return prefix
}

// BaseProtectedError provides a standard structure for protected/system resource errors
type BaseProtectedError struct {
	Entity  string    // e.g., "user", "project", "policy"
	Name    string    // optional: entity name
	Action  string    // what action was attempted (e.g., "modified", "deleted")
	Message string    // optional: additional context
	ID      uuid.UUID // optional: entity ID
}

func (e *BaseProtectedError) Error() string {
	action := "modified"
	if e.Action != "" {
		action = e.Action
	}

	prefix := fmt.Sprintf("%s cannot be %s: system/protected resource", e.Entity, action)

	if e.Name != "" {
		return fmt.Sprintf("%s (name=%s)", prefix, e.Name)
	}

	if e.ID != uuid.Nil() {
		return fmt.Sprintf("%s (ID=%s)", prefix, e.ID)
	}

	if e.Message != "" {
		return fmt.Sprintf("%s: %s", prefix, e.Message)
	}

	return prefix
}

// BaseInvalidUpdateError provides a standard structure for update validation errors
type BaseInvalidUpdateError struct {
	Entity  string // e.g., "user", "project", "model"
	Reason  string // what's wrong with the update
	Message string // optional: additional context
}

func (e *BaseInvalidUpdateError) Error() string {
	prefix := fmt.Sprintf("invalid %s update", e.Entity)

	if e.Reason != "" {
		return fmt.Sprintf("%s: %s", prefix, e.Reason)
	}

	if e.Message != "" {
		return fmt.Sprintf("%s: %s", prefix, e.Message)
	}

	return prefix
}

// BaseWrappedError provides error wrapping support
type BaseWrappedError struct {
	Err     error
	Message string
}

func (e *BaseWrappedError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s", e.Message, e.Err.Error())
	}
	return e.Message
}

func (e *BaseWrappedError) Unwrap() error {
	return e.Err
}
