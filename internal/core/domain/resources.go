package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"uuid"
)

var (
	// ResourcesFilterFields is a list of valid fields for filtering permissions.
	ResourcesFilterFields = []string{FieldID, FieldName, FieldAction, FieldResource, FieldSystem, FieldCreatedAt, FieldUpdatedAt}

	// ResourcesSortFields is a list of valid fields for sorting permissions.
	ResourcesSortFields = []string{FieldID, FieldName, FieldAction, FieldResource, FieldSystem, FieldCreatedAt, FieldUpdatedAt}

	// ResourcesPartialFields is a list of valid fields for partial responses.
	ResourcesPartialFields = []string{FieldID, FieldName, FieldDescription, FieldAction, FieldResource, FieldSystem, FieldCreatedAt, FieldUpdatedAt}
)

type Resource struct {
	CreatedAt   time.Time
	UpdatedAt   time.Time
	System      *bool
	Name        string
	Description string
	Action      string
	Resource    string
	SerialID    int64
	ID          uuid.UUID
}

type SelectResourcesInput struct {
	Sort      string
	Filter    string
	Fields    string
	Paginator Paginator
}

func (ref *SelectResourcesInput) Validate() error {
	var errs ValidationErrors

	// Validate paginator
	if err := ref.Paginator.Validate(); err != nil {
		errs.Add(err)
	}

	parseSortExpression(&errs, ResourcesSortParser, ref.Sort, "INVALID_SORT_FIELD")
	parseFilterExpression(&errs, ResourcesFilterParser, ref.Filter, "INVALID_FILTER_FIELD")
	parseFieldsExpression(&errs, ResourcesFieldsParser, ref.Fields, "INVALID_FIELD")

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// UniqueID generates a unique ID based on the field values via SHA-256
// hashing of a deterministic string representation. Used as a cache key
// for the SelectResources query.
func (ref *SelectResourcesInput) UniqueID() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s",
		ref.Sort,
		ref.Filter,
		ref.Fields,
		ref.Paginator.UniqueID(),
	)
	return hex.EncodeToString(h.Sum(nil))
}

type ListResourcesInput = SelectResourcesInput

type SelectResourcesOutput struct {
	Items     []Resource
	Paginator Paginator
}

type ListResourcesOutput = SelectResourcesOutput
