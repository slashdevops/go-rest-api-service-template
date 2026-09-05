package domain

import "uuid"

// ProjectNotFoundError represents an error when a project cannot be found
type ProjectNotFoundError struct {
	Name    string // optional: search by name
	Message string // optional: additional context
	ID      uuid.UUID
}

func (e *ProjectNotFoundError) Error() string {
	base := &BaseNotFoundError{
		Entity:  "project",
		ID:      e.ID,
		Message: e.Message,
	}

	if e.Name != "" {
		base.Field = "name"
		base.Value = e.Name
	}

	return base.Error()
}

// ProjectAlreadyExistsError represents an error when a project already exists
// Consolidates ProjectIDExistsError, ProjectNameExistsError, ProjectIDAlreadyExistsError, ProjectNameAlreadyExistsError
type ProjectAlreadyExistsError struct {
	Name    string // which field conflicts
	Message string // optional: additional context
	ID      uuid.UUID
}

func (e *ProjectAlreadyExistsError) Error() string {
	base := &BaseAlreadyExistsError{
		Entity:  "project",
		ID:      e.ID,
		Message: e.Message,
	}

	if e.Name != "" {
		base.Field = "name"
		base.Value = e.Name
	}

	return base.Error()
}

// SystemProjectError represents an error when trying to modify a system project
type SystemProjectError struct {
	Name    string
	Action  string // optional: what action was attempted
	Message string
	ID      uuid.UUID
}

func (e *SystemProjectError) Error() string {
	return (&BaseProtectedError{
		Entity:  "project",
		ID:      e.ID,
		Name:    e.Name,
		Action:  e.Action,
		Message: e.Message,
	}).Error()
}

// InvalidProjectIDError represents an invalid project ID error
type InvalidProjectIDError struct {
	Reason  string // optional: why it's invalid
	Message string // optional: additional context
	ID      uuid.UUID
}

func (e *InvalidProjectIDError) Error() string {
	reason := e.Reason
	if reason == "" && e.Message != "" {
		reason = e.Message
	}

	return (&BaseInvalidFieldError{
		Entity: "project",
		Field:  "ID",
		Value:  e.ID.String(),
		Reason: reason,
	}).Error()
}

// InvalidProjectNameError represents an invalid project name error
type InvalidProjectNameError struct {
	Name      string // optional: the invalid name
	MinLength int
	MaxLength int
}

func (e *InvalidProjectNameError) Error() string {
	return (&BaseInvalidFieldError{
		Entity:    "project",
		Field:     "name",
		Value:     e.Name,
		MinLength: e.MinLength,
		MaxLength: e.MaxLength,
	}).Error()
}

// InvalidProjectDescriptionError represents an invalid project description error
type InvalidProjectDescriptionError struct {
	Description string // optional: the invalid description
	MinLength   int
	MaxLength   int
}

func (e *InvalidProjectDescriptionError) Error() string {
	return (&BaseInvalidFieldError{
		Entity:    "project",
		Field:     "description",
		Value:     e.Description,
		MinLength: e.MinLength,
		MaxLength: e.MaxLength,
	}).Error()
}

// InvalidProjectUpdateError represents an invalid project update error
type InvalidProjectUpdateError struct {
	Field   string // optional: which field
	Reason  string // optional: why update is invalid
	Message string // optional: additional context
}

func (e *InvalidProjectUpdateError) Error() string {
	reason := e.Reason
	if reason == "" && e.Message != "" {
		reason = e.Message
	}

	return (&BaseInvalidUpdateError{
		Entity: "project",
		Reason: reason,
	}).Error()
}
