package domain

import (
	"uuid"
)

// RoleNotFoundError represents an error when a role cannot be found
type RoleNotFoundError struct {
	RoleID  string // optional: string ID for backward compatibility
	Name    string // optional: search by name
	Message string // optional: additional context
	ID      uuid.UUID
}

func (e *RoleNotFoundError) Error() string {
	base := &BaseNotFoundError{
		Entity:  "role",
		ID:      e.ID,
		Message: e.Message,
	}

	if e.Name != "" {
		base.Field = "name"
		base.Value = e.Name
	} else if e.RoleID != "" {
		base.Field = "ID"
		base.Value = e.RoleID
	}

	return base.Error()
}

// RoleAlreadyExistsError represents an error when a role already exists
// Consolidates RoleNameExistsError, RoleIDExistsError, RoleNameAlreadyExistsError, RoleIDAlreadyExistsError
type RoleAlreadyExistsError struct {
	RoleID  string // optional: string ID for backward compatibility
	Name    string // which field conflicts
	Message string // optional: additional context
	ID      uuid.UUID
}

func (e *RoleAlreadyExistsError) Error() string {
	base := &BaseAlreadyExistsError{
		Entity:  "role",
		ID:      e.ID,
		Message: e.Message,
	}

	if e.Name != "" {
		base.Field = "name"
		base.Value = e.Name
	} else if e.RoleID != "" {
		base.Field = "ID"
		base.Value = e.RoleID
	}

	return base.Error()
}

// InvalidRoleIDError represents an invalid role ID error
type InvalidRoleIDError struct {
	RoleID  string // optional: string ID for backward compatibility
	Reason  string // optional: why it's invalid
	Message string // optional: additional context
	ID      uuid.UUID
}

func (e *InvalidRoleIDError) Error() string {
	base := &BaseInvalidFieldError{
		Entity: "role",
		Field:  "ID",
		Reason: e.Reason,
	}

	if e.ID != uuid.Nil() {
		base.Value = e.ID.String()
	} else if e.RoleID != "" {
		base.Value = e.RoleID
	}

	if e.Message != "" && e.Reason == "" {
		base.Reason = e.Message
	}

	return base.Error()
}

// SystemRoleError represents an error when trying to modify a system role
type SystemRoleError struct {
	Name   string // optional: role name
	Action string // optional: what action was attempted
	RoleID uuid.UUID
}

func (e *SystemRoleError) Error() string {
	return (&BaseProtectedError{
		Entity: "role",
		ID:     e.RoleID,
		Name:   e.Name,
		Action: e.Action,
	}).Error()
}

// InvalidRoleLinkError represents an error when linking roles
type InvalidRoleLinkError struct {
	Message string    // optional: additional context
	RoleID  uuid.UUID // optional: which role
	UserID  uuid.UUID // optional: which user
}

func (e *InvalidRoleLinkError) Error() string {
	base := &BaseInvalidFieldError{
		Field:  "roleLink",
		Reason: e.Message,
	}

	if e.RoleID != uuid.Nil() {
		base.Value = e.RoleID.String()
	}

	if e.UserID != uuid.Nil() && e.RoleID == uuid.Nil() {
		base.Value = e.UserID.String()
	}

	return base.Error()
}
