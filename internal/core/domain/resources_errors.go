package domain

// ResourceNotFoundError represents an error when a resource cannot be found
type ResourceNotFoundError struct {
	ID      string
	Name    string // optional: search by name
	Message string // optional: additional context
}

func (e *ResourceNotFoundError) Error() string {
	base := &BaseNotFoundError{
		Entity:  "resource",
		Message: e.Message,
	}

	if e.ID != "" {
		base.Field = "ID"
		base.Value = e.ID
	}

	if e.Name != "" {
		base.Field = "name"
		base.Value = e.Name
	}

	return base.Error()
}

// ResourceAlreadyExistsError represents an error when a resource already exists
// Consolidates ResourceIDAlreadyExistsError, ResourceNameAlreadyExistsError
type ResourceAlreadyExistsError struct {
	ID      string
	Name    string // which field conflicts
	Message string // optional: additional context
}

func (e *ResourceAlreadyExistsError) Error() string {
	base := &BaseAlreadyExistsError{
		Entity:  "resource",
		Message: e.Message,
	}

	if e.Name != "" {
		base.Field = "name"
		base.Value = e.Name
	} else if e.ID != "" {
		base.Field = "ID"
		base.Value = e.ID
	}

	return base.Error()
}

// InvalidResourceIDError represents an invalid resource ID error
type InvalidResourceIDError struct {
	ID      string
	Reason  string // optional: why it's invalid
	Message string // optional: additional context
}

func (e *InvalidResourceIDError) Error() string {
	reason := e.Reason
	if reason == "" && e.Message != "" {
		reason = e.Message
	}

	if reason == "" && e.ID != "" {
		reason = e.ID
	}

	return (&BaseInvalidFieldError{
		Entity: "resource",
		Field:  "ID",
		Reason: reason,
	}).Error()
}

// InvalidActionError represents an invalid action error
type InvalidActionError struct {
	Action  string // the invalid action
	Reason  string // optional: why it's invalid
	Message string // optional: additional context
}

func (e *InvalidActionError) Error() string {
	reason := e.Reason
	if reason == "" {
		reason = e.Message
	}

	return (&BaseInvalidFieldError{
		Field:  "action",
		Value:  e.Action,
		Reason: reason,
	}).Error()
}

// InvalidResourceError represents an invalid resource error
type InvalidResourceError struct {
	Resource string // optional: the invalid resource
	Reason   string // optional: why it's invalid
	Message  string // optional: additional context
}

func (e *InvalidResourceError) Error() string {
	reason := e.Reason
	if reason == "" {
		reason = e.Message
	}

	return (&BaseInvalidFieldError{
		Field:  "resource",
		Value:  e.Resource,
		Reason: reason,
	}).Error()
}
