package domain

import (
	"uuid"
)

// UserNotFoundError represents an error when a user cannot be found
type UserNotFoundError struct {
	Email   string // optional: search by email
	Message string // optional: additional context
	ID      uuid.UUID
}

func (e *UserNotFoundError) Error() string {
	base := &BaseNotFoundError{
		Entity:  "user",
		ID:      e.ID,
		Message: e.Message,
	}

	if e.Email != "" {
		base.Field = "email"
		base.Value = e.Email
	}

	return base.Error()
}

// UserAlreadyExistsError represents an error when a user already exists
// Consolidates UserExistsError, UserAlreadyExistsError, EmailExistsError, UserEmailAlreadyExistsError
type UserAlreadyExistsError struct {
	Email   string // which field conflicts
	Message string // optional: additional context
	ID      uuid.UUID
}

func (e *UserAlreadyExistsError) Error() string {
	base := &BaseAlreadyExistsError{
		Entity:  "user",
		ID:      e.ID,
		Message: e.Message,
	}

	if e.Email != "" {
		base.Field = "email"
		base.Value = e.Email
	}

	return base.Error()
}

// InvalidUserIDError represents an invalid user ID error
type InvalidUserIDError struct {
	Message string
	ID      uuid.UUID
}

func (e *InvalidUserIDError) Error() string {
	return (&BaseInvalidFieldError{
		Entity: "user",
		Field:  "ID",
		Value:  e.ID.String(),
		Reason: e.Message,
	}).Error()
}

// UserDisabledError represents an error when trying to use a disabled user account
type UserDisabledError struct {
	Username string
	UserID   uuid.UUID
}

func (e *UserDisabledError) Error() string {
	base := &BaseInvalidFieldError{
		Entity: "user",
		Field:  "status",
		Reason: "user is disabled",
	}

	if e.Username != "" {
		base.Value = e.Username
	}

	if e.UserID != uuid.Nil() {
		base.Value = e.UserID.String()
	}

	return base.Error()
}

// InvalidEmailError represents an invalid email error
type InvalidEmailError struct {
	Email   string
	Reason  string // optional: why it's invalid
	Message string // optional: additional context
}

func (e *InvalidEmailError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "email",
		Value:  e.Email,
		Reason: e.Reason,
	}).Error()
}

// InvalidPasswordError represents an invalid password error
type InvalidPasswordError struct {
	Message   string
	MinLength int
	MaxLength int
}

func (e *InvalidPasswordError) Error() string {
	return (&BaseInvalidFieldError{
		Field:     "password",
		Reason:    e.Message,
		MinLength: e.MinLength,
		MaxLength: e.MaxLength,
	}).Error()
}

// InvalidUserPasswordError represents an invalid user password error (for updates/changes)
type InvalidUserPasswordError struct {
	Reason  string // optional: why password is invalid
	Message string // optional: additional context
}

func (e *InvalidUserPasswordError) Error() string {
	return (&BaseInvalidFieldError{
		Entity: "user",
		Field:  "password",
		Reason: e.Reason,
	}).Error()
}

// InvalidProjectLinkError represents an invalid project link error
type InvalidProjectLinkError struct {
	Message   string
	ProjectID uuid.UUID
	UserID    uuid.UUID
}

func (e *InvalidProjectLinkError) Error() string {
	base := &BaseInvalidFieldError{
		Field:  "projectLink",
		Reason: e.Message,
	}

	if e.ProjectID != uuid.Nil() {
		base.Value = e.ProjectID.String()
	}

	if e.UserID != uuid.Nil() && e.ProjectID == uuid.Nil() {
		base.Value = e.UserID.String()
	}

	return base.Error()
}
