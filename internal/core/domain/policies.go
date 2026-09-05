package domain

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"uuid"
)

const (
	PolicyNameMinLength        = 2
	PolicyNameMaxLength        = 255
	PolicyDescriptionMinLength = 2
	PolicyDescriptionMaxLength = 1024

	PoliciesPolicyCreatedSuccessfully = "Policy created successfully"
	PoliciesPolicyUpdatedSuccessfully = "Policy updated successfully"
	PoliciesPolicyDeletedSuccessfully = "Policy deleted successfully"
	PoliciesRolesLinkedSuccessfully   = "Roles linked successfully"
	PoliciesRolesUnlinkedSuccessfully = "Roles unlinked successfully"

	// https://regex101.com/r/xIOyX2/2
	ValidResourceRegex = `^(\/[a-z_]{1,50}|\*{1})((\/[a-z_]{1,50})|(\/\*{1})|(\/[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})){0,7}$`

	ValidActionsRegex    = `^(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD|\*)$`
	ValidUUIDOrStarRegex = `[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}|\*{1}`
)

var (
	// PoliciesFilterFields is a list of valid fields for filtering models.
	PoliciesFilterFields = []string{FieldID, FieldName, FieldAllowedAction, FieldAllowedResource, FieldSystem, FieldCreatedAt, FieldUpdatedAt}

	// PoliciesSortFields is a list of valid fields for sorting models.
	PoliciesSortFields = []string{FieldID, FieldName, FieldAllowedAction, FieldAllowedResource, FieldSystem, FieldCreatedAt, FieldUpdatedAt}

	// PoliciesPartialFields is a list of valid fields for partial responses.
	PoliciesPartialFields = []string{
		FieldID,
		FieldName,
		FieldDescription,
		FieldAllowedAction,
		FieldAllowedResource,
		FieldSystem,
		FieldCreatedAt,
		FieldUpdatedAt,
	}
)

func GetValidActions() string {
	validStr := strings.Trim(ValidActionsRegex, "^()$")
	validStr = strings.ReplaceAll(validStr, "\\", "")
	validStr = strings.ReplaceAll(validStr, "|", ", ")

	return validStr
}

// IsValidAction validates the action string.
func IsValidAction(action string) error {
	if action == "" {
		return fmt.Errorf("action cannot be empty")
	}

	re := regexp.MustCompile(ValidActionsRegex)
	if !re.MatchString(action) {
		return fmt.Errorf("invalid action: %s, must be one of %s in Uppercase", action, GetValidActions())
	}

	return nil
}

// IsValidResource validates the resource string.
func IsValidResource(resource string) error {
	if resource == "" {
		return fmt.Errorf("resource cannot be empty")
	}

	re := regexp.MustCompile(ValidResourceRegex)
	if !re.MatchString(resource) {
		return fmt.Errorf("invalid resource: %s, do not match the required format", resource)
	}

	return nil
}

// Policy is the authorization policy entity.
type Policy struct {
	CreatedAt       time.Time
	UpdatedAt       time.Time
	System          *bool
	Name            string
	Description     string
	AllowedAction   string
	AllowedResource string
	Resource        Resource
	SerialID        int64
	ID              uuid.UUID
}

type LinkRolesToPolicyInput struct {
	RoleIDs  []uuid.UUID
	PolicyID uuid.UUID
}

func (ref *LinkRolesToPolicyInput) Validate() error {
	var errs ValidationErrors

	if reflect.DeepEqual(ref, &LinkRolesToPolicyInput{}) {
		errs.AddError(FieldRequest, "at least one field must be updated", "REQUIRED")
		return &errs
	}

	errs.Add(ValidateUUID(ref.PolicyID, 7, FieldPolicyID))

	if len(ref.RoleIDs) == 0 {
		errs.AddError(FieldRoleIDs, "must be at least one role ID", "REQUIRED")
	}

	if ref.RoleIDs != nil {
		for i, roleID := range ref.RoleIDs {
			if err := ValidateUUID(roleID, 7, fmt.Sprintf("%s[%d]", FieldRoleIDs, i)); err != nil {
				errs.Add(err)
			}
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type UnlinkRolesFromPolicyInput = LinkRolesToPolicyInput

type SelectPoliciesInput struct {
	Sort      string
	Filter    string
	Fields    string
	Paginator Paginator
}

func (ref *SelectPoliciesInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ref.Paginator.Validate())

	if ref.Sort != "" {
		_, err := PoliciesSortParser.Parse(ref.Sort)
		if err != nil {
			errs.Add(&ValidationError{Field: FieldSort, Message: err.Error(), Code: "INVALID_SORT_FIELD"})
		}
	}

	if ref.Filter != "" {
		_, err := PoliciesFilterParser.Parse(ref.Filter)
		if err != nil {
			errs.Add(&ValidationError{Field: FieldFilter, Message: err.Error(), Code: "INVALID_FILTER_FIELD"})
		}
	}

	if ref.Fields != "" {
		_, err := PoliciesFieldsParser.Parse(ref.Fields)
		if err != nil {
			errs.Add(&ValidationError{Field: FieldFields, Message: err.Error(), Code: "INVALID_FIELD"})
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type ListPoliciesInput = SelectPoliciesInput

type SelectPoliciesOutput struct {
	Items     []Policy
	Paginator Paginator
}

type ListPoliciesOutput = SelectPoliciesOutput

type CreatePolicyInput struct {
	Name            string
	Description     string
	AllowedAction   string
	AllowedResource string
	ID              uuid.UUID
	ResourceID      uuid.UUID
}

func (ref *CreatePolicyInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ValidateUUID(ref.ID, 7, FieldID))

	nameOptions := StringValidationOptions{
		MinLength: PolicyNameMinLength, MaxLength: PolicyNameMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldName,
	}
	if _, err := ValidateString(ref.Name, nameOptions); err != nil {
		errs.Add(err)
	}

	descriptionOptions := StringValidationOptions{
		MinLength: PolicyDescriptionMinLength, MaxLength: PolicyDescriptionMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldDescription,
	}
	if _, err := ValidateString(ref.Description, descriptionOptions); err != nil {
		errs.Add(err)
	}

	if ref.AllowedAction == "" {
		errs.AddError(FieldAllowedAction, "cannot be empty", "REQUIRED")
	} else if !regexp.MustCompile(ValidActionsRegex).MatchString(ref.AllowedAction) {
		errs.AddError(FieldAllowedAction, "invalid action format, must be one of "+GetValidActions()+" in Uppercase", "INVALID_FORMAT")
	}

	if ref.AllowedResource == "" {
		errs.AddError(FieldAllowedResource, "cannot be empty", "REQUIRED")
	} else if !regexp.MustCompile(ValidResourceRegex).MatchString(ref.AllowedResource) {
		errs.AddError(FieldAllowedResource, "invalid resource format", "INVALID_FORMAT")
	}

	errs.Add(ValidateUUID(ref.ResourceID, 7, FieldResourceID))

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type UpdatePolicyInput struct {
	Name            *string
	Description     *string
	AllowedAction   *string
	AllowedResource *string
	ID              uuid.UUID
}

func (ref *UpdatePolicyInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ValidateUUID(ref.ID, 7, FieldID))

	if ref.Name != nil {
		nameOptions := StringValidationOptions{
			MinLength: PolicyNameMinLength, MaxLength: PolicyNameMaxLength,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
			NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldName,
		}
		if _, err := ValidateString(*ref.Name, nameOptions); err != nil {
			errs.Add(err)
		}
	}

	if ref.Description != nil {
		descriptionOptions := StringValidationOptions{
			MinLength: PolicyDescriptionMinLength, MaxLength: PolicyDescriptionMaxLength,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
			NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldDescription,
		}
		if _, err := ValidateString(*ref.Description, descriptionOptions); err != nil {
			errs.Add(err)
		}
	}

	if ref.AllowedAction != nil {
		if *ref.AllowedAction == "" {
			errs.AddError(FieldAllowedAction, "cannot be empty", "REQUIRED")
		} else if !regexp.MustCompile(ValidActionsRegex).MatchString(*ref.AllowedAction) {
			errs.AddError(FieldAllowedAction, "invalid action format, must be one of "+GetValidActions()+" in Uppercase", "INVALID_FORMAT")
		}
	}

	if ref.AllowedResource != nil {
		if *ref.AllowedResource == "" {
			errs.AddError(FieldAllowedResource, "cannot be empty", "REQUIRED")
		} else if !regexp.MustCompile(ValidResourceRegex).MatchString(*ref.AllowedResource) {
			errs.AddError(FieldAllowedResource, "invalid resource format", "INVALID_FORMAT")
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type DeletePolicyInput struct {
	ID uuid.UUID
}

func (ref *DeletePolicyInput) Validate() error {
	var errs ValidationErrors
	errs.Add(ValidateUUID(ref.ID, 7, FieldID))
	if errs.HasErrors() {
		return &errs
	}
	return nil
}
