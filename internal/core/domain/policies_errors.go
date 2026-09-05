package domain

import (
	"uuid"
)

// PolicyNotFoundError represents an error when a policy cannot be found
type PolicyNotFoundError struct {
	Name    string // optional: search by name
	Message string // optional: additional context
	ID      uuid.UUID
}

func (e *PolicyNotFoundError) Error() string {
	base := &BaseNotFoundError{
		Entity:  "policy",
		ID:      e.ID,
		Message: e.Message,
	}

	if e.Name != "" {
		base.Field = "name"
		base.Value = e.Name
	}

	return base.Error()
}

// PolicyAlreadyExistsError represents an error when a policy already exists
type PolicyAlreadyExistsError struct {
	Name    string // which field conflicts
	Message string // optional: additional context
	ID      uuid.UUID
}

func (e *PolicyAlreadyExistsError) Error() string {
	base := &BaseAlreadyExistsError{
		Entity:  "policy",
		ID:      e.ID,
		Message: e.Message,
	}

	if e.Name != "" {
		base.Field = "name"
		base.Value = e.Name
	}

	return base.Error()
}

// InvalidPolicyIDError represents an invalid policy ID error
type InvalidPolicyIDError struct {
	Reason  string    // optional: why it's invalid
	Message string    // optional: additional context
	ID      uuid.UUID // optional: the invalid ID
}

func (e *InvalidPolicyIDError) Error() string {
	reason := e.Reason
	if reason == "" && e.Message != "" {
		reason = e.Message
	}

	if reason == "" && e.ID != uuid.Nil() {
		reason = e.ID.String()
	}

	return (&BaseInvalidFieldError{
		Entity: "policy",
		Field:  "ID",
		Reason: reason,
	}).Error()
}

// InvalidPolicyNameError represents an invalid policy name error
type InvalidPolicyNameError struct {
	Name      string // optional: the invalid name
	MinLength int
	MaxLength int
}

func (e *InvalidPolicyNameError) Error() string {
	return (&BaseInvalidFieldError{
		Entity:    "policy",
		Field:     "name",
		Value:     e.Name,
		MinLength: e.MinLength,
		MaxLength: e.MaxLength,
	}).Error()
}

// InvalidPolicyDescriptionError represents an invalid policy description error
type InvalidPolicyDescriptionError struct {
	Description string // optional: the invalid description
	MinLength   int
	MaxLength   int
}

func (e *InvalidPolicyDescriptionError) Error() string {
	return (&BaseInvalidFieldError{
		Entity:    "policy",
		Field:     "description",
		Value:     e.Description,
		MinLength: e.MinLength,
		MaxLength: e.MaxLength,
	}).Error()
}

// InvalidPolicyAllowedActionError represents an invalid policy action error
type InvalidPolicyAllowedActionError struct {
	Action  string // optional: the invalid action
	Message string // optional: additional context
}

func (e *InvalidPolicyAllowedActionError) Error() string {
	return (&BaseInvalidFieldError{
		Entity: "policy",
		Field:  "action",
		Value:  e.Action,
		Reason: e.Message,
	}).Error()
}

// InvalidPolicyAllowedResourceError represents an invalid policy resource error
type InvalidPolicyAllowedResourceError struct {
	Resource string // optional: the invalid resource
	Message  string // optional: additional context
}

func (e *InvalidPolicyAllowedResourceError) Error() string {
	return (&BaseInvalidFieldError{
		Entity: "policy",
		Field:  "resource",
		Value:  e.Resource,
		Reason: e.Message,
	}).Error()
}

// SystemPolicyError represents an error when trying to modify a system policy
type SystemPolicyError struct {
	Name     string // optional: policy name
	Action   string // optional: what action was attempted
	PolicyID uuid.UUID
}

func (e *SystemPolicyError) Error() string {
	return (&BaseProtectedError{
		Entity: "policy",
		ID:     e.PolicyID,
		Name:   e.Name,
		Action: e.Action,
	}).Error()
}

// InvalidPolicyLinkRolesError represents an error when linking roles to a policy
type InvalidPolicyLinkRolesError struct {
	Message  string    // optional: additional context
	PolicyID uuid.UUID // optional: which policy
	RoleID   uuid.UUID // optional: which role
}

func (e *InvalidPolicyLinkRolesError) Error() string {
	reason := e.Message

	if reason == "" && e.PolicyID != uuid.Nil() && e.RoleID != uuid.Nil() {
		reason = "policyID=" + e.PolicyID.String() + ", roleID=" + e.RoleID.String()
	} else if reason == "" && e.PolicyID != uuid.Nil() {
		reason = "policyID=" + e.PolicyID.String()
	} else if reason == "" && e.RoleID != uuid.Nil() {
		reason = "roleID=" + e.RoleID.String()
	}

	return (&BaseInvalidFieldError{
		Entity: "policy",
		Field:  "link roles",
		Reason: reason,
	}).Error()
}

// InvalidPolicyCreateError represents an error during policy creation
type InvalidPolicyCreateError struct {
	Field   string // optional: which field
	Reason  string // optional: why creation failed
	Message string // optional: additional context
}

func (e *InvalidPolicyCreateError) Error() string {
	field := e.Field
	if field == "" {
		field = "create"
	}

	reason := e.Reason
	if reason == "" {
		reason = e.Message
	}

	return (&BaseInvalidFieldError{
		Entity: "policy",
		Field:  field,
		Reason: reason,
	}).Error()
}

// InvalidPolicyUpdateError represents an error during policy update
type InvalidPolicyUpdateError struct {
	Field   string // optional: which field
	Reason  string // optional: why update failed
	Message string // optional: additional context
}

func (e *InvalidPolicyUpdateError) Error() string {
	reason := e.Reason
	if reason == "" {
		reason = e.Message
	}

	if e.Field != "" {
		reason = "field '" + e.Field + "': " + reason
	}

	return (&BaseInvalidUpdateError{
		Entity: "policy",
		Reason: reason,
	}).Error()
}
