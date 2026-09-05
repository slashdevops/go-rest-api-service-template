package domain

import "uuid"

// IDPTypesNotFoundError represents an error when an identity provider type cannot be found
type IDPTypesNotFoundError struct {
	Name    string // optional: search by name
	Message string // optional: additional context
	ID      uuid.UUID
}

func (e *IDPTypesNotFoundError) Error() string {
	base := &BaseNotFoundError{
		Entity:  "identity provider type",
		ID:      e.ID,
		Message: e.Message,
	}

	if e.Name != "" {
		base.Field = "name"
		base.Value = e.Name
	}

	return base.Error()
}
