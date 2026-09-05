package domain

import "uuid"

// ProductNotFoundError represents an error when a product cannot be found
type ProductNotFoundError struct {
	Name    string // optional: search by name
	Message string // optional: additional context
	ID      uuid.UUID
}

func (e *ProductNotFoundError) Error() string {
	base := &BaseNotFoundError{
		Entity:  "product",
		ID:      e.ID,
		Message: e.Message,
	}

	if e.Name != "" {
		base.Field = "name"
		base.Value = e.Name
	}

	return base.Error()
}

// ProductAlreadyExistsError represents an error when a product already exists.
//
// The uniqueness the database enforces is (projects_id, name), not name alone,
// so the same product name in two different projects is not a conflict. The
// message says "in this project" for that reason: a bare "product already
// exists" sent a caller looking for a name that, from their side of the API,
// was free.
type ProductAlreadyExistsError struct {
	Name    string // which field conflicts
	Message string // optional: additional context
	ID      uuid.UUID
}

func (e *ProductAlreadyExistsError) Error() string {
	base := &BaseAlreadyExistsError{
		Entity:  "product in this project",
		ID:      e.ID,
		Message: e.Message,
	}

	if e.Name != "" {
		base.Field = "name"
		base.Value = e.Name
	}

	return base.Error()
}

// InvalidProductError represents an error when a product is invalid
type InvalidProductError struct {
	Message string
}

func (e *InvalidProductError) Error() string {
	if e.Message == "" {
		return "invalid product"
	}

	return "invalid product: " + e.Message
}
