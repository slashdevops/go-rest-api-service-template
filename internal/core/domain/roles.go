package domain

import (
	"fmt"
	"time"

	"uuid"
)

const (
	RoleNameMinLength        = 2
	RoleNameMaxLength        = 255
	RoleDescriptionMinLength = 2
	RoleDescriptionMaxLength = 1000

	RolesRoleCreatedSuccessfully      = "Role created successfully"
	RolesRoleUpdatedSuccessfully      = "Role updated successfully"
	RolesRoleDeletedSuccessfully      = "Role deleted successfully"
	RolesPoliciesLinkedSuccessfully   = "Policies linked successfully"
	RolesPoliciesUnlinkedSuccessfully = "Policies unlinked successfully"
	RolesUsersLinkedSuccessfully      = "Users linked successfully"
	RolesUsersUnlinkedSuccessfully    = "Users unlinked successfully"
)

var (
	RolesFilterFields  = []string{FieldID, FieldName, FieldSystem, FieldAutoAssign, FieldCreatedAt, FieldUpdatedAt}
	RolesSortFields    = []string{FieldID, FieldName, FieldSystem, FieldAutoAssign, FieldCreatedAt, FieldUpdatedAt}
	RolesPartialFields = []string{FieldID, FieldPolicy, FieldName, FieldDescription, FieldSystem, FieldAutoAssign, FieldCreatedAt, FieldUpdatedAt}
)

// Role is the role entity.
type Role struct {
	CreatedAt   time.Time
	UpdatedAt   time.Time
	System      *bool
	AutoAssign  *bool
	Name        string
	Description string
	SerialID    int64
	ID          uuid.UUID
}

type InsertRoleInput struct {
	Name        string
	Description string
	ID          uuid.UUID
}

func (ref *InsertRoleInput) Validate() error {
	var errs ValidationErrors

	if err := ValidateUUID(ref.ID, 7, FieldID); err != nil {
		errs.Add(err)
	}

	if normalizedName, err := ValidateString(ref.Name, StringValidationOptions{
		MinLength: RoleNameMinLength, MaxLength: RoleNameMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldName,
	}); err != nil {
		errs.Add(err)
	} else {
		ref.Name = normalizedName
	}

	if normalizedDescription, err := ValidateString(ref.Description, StringValidationOptions{
		MinLength: RoleDescriptionMinLength, MaxLength: RoleDescriptionMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldDescription,
	}); err != nil {
		errs.Add(err)
	} else {
		ref.Description = normalizedDescription
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type CreateRoleInput = InsertRoleInput

type UpdateRoleInput struct {
	Name        *string
	Description *string
	ID          uuid.UUID
}

func (ref *UpdateRoleInput) Validate() error {
	var errs ValidationErrors

	if err := ValidateUUID(ref.ID, 7, FieldID); err != nil {
		errs.Add(err)
	}

	if ref.Name != nil {
		if normalizedName, err := ValidateString(*ref.Name, StringValidationOptions{
			MinLength: RoleNameMinLength, MaxLength: RoleNameMaxLength,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
			NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldName,
		}); err != nil {
			errs.Add(err)
		} else {
			ref.Name = &normalizedName
		}
	}

	if ref.Description != nil {
		if normalizedDescription, err := ValidateString(*ref.Description, StringValidationOptions{
			MinLength: RoleDescriptionMinLength, MaxLength: RoleDescriptionMaxLength,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
			NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldDescription,
		}); err != nil {
			errs.Add(err)
		} else {
			ref.Description = &normalizedDescription
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type DeleteRoleInput struct {
	ID uuid.UUID
}

func (ref *DeleteRoleInput) Validate() error {
	if err := ValidateUUID(ref.ID, 7, FieldID); err != nil {
		if valErr, ok := err.(*ValidationError); ok {
			return &InvalidRoleIDError{ID: ref.ID, Message: valErr.Message}
		}
		return &InvalidRoleIDError{ID: ref.ID}
	}
	return nil
}

type SelectRolesInput struct {
	Sort      string
	Filter    string
	Fields    string
	Paginator Paginator
}

func (ref *SelectRolesInput) Validate() error {
	var errs ValidationErrors

	if err := ref.Paginator.Validate(); err != nil {
		errs.Add(err)
	}

	if err := ValidateSortExpression(ref.Sort, FieldSort); err != nil {
		errs.Add(err)
	}
	if err := ValidateFilterExpression(ref.Filter, FieldFilter); err != nil {
		errs.Add(err)
	}
	if err := ValidateFieldsExpression(ref.Fields, FieldFields); err != nil {
		errs.Add(err)
	}

	if ref.Sort != "" {
		if _, err := RolesSortParser.Parse(ref.Sort); err != nil {
			errs.Add(&ValidationError{Field: FieldSort, Message: err.Error(), Code: "INVALID_SORT_FIELD"})
		}
	}

	if ref.Filter != "" {
		if _, err := RolesFilterParser.Parse(ref.Filter); err != nil {
			errs.Add(&ValidationError{Field: FieldFilter, Message: err.Error(), Code: "INVALID_FILTER_FIELD"})
		}
	}

	if ref.Fields != "" {
		if _, err := RolesFieldsParser.Parse(ref.Fields); err != nil {
			errs.Add(&ValidationError{Field: FieldFields, Message: err.Error(), Code: "INVALID_FIELD"})
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type ListRolesInput = SelectRolesInput

type SelectRolesOutput struct {
	Items     []Role
	Paginator Paginator
}

type ListRolesOutput = SelectRolesOutput

type LinkUsersToRoleInput struct {
	UserIDs []uuid.UUID
	// CallerID is who is granting; the guard checks they hold what they hand
	// out, and refuses a zero value. Unlink paths share this input and leave it
	// zero: nothing is granted there.
	CallerID uuid.UUID
	RoleID   uuid.UUID
}

func (ref *LinkUsersToRoleInput) Validate() error {
	var errs ValidationErrors

	if err := ValidateUUID(ref.RoleID, 7, FieldRoleID); err != nil {
		errs.Add(err)
	}

	if len(ref.UserIDs) < 1 {
		errs.Add(&ValidationError{Field: FieldUserIDs, Message: "must be at least one user ID", Code: "REQUIRED"})
	}

	for i, userID := range ref.UserIDs {
		if err := ValidateUUID(userID, 7, fmt.Sprintf("%s[%d]", FieldUserIDs, i)); err != nil {
			errs.Add(err)
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type UnlinkUsersFromRoleInput = LinkUsersToRoleInput

type LinkPoliciesToRoleInput struct {
	PolicyIDs []uuid.UUID
	// CallerID is who is granting; the guard checks they hold what they hand out.
	CallerID uuid.UUID
	RoleID   uuid.UUID
}

func (ref *LinkPoliciesToRoleInput) Validate() error {
	var errs ValidationErrors

	if err := ValidateUUID(ref.RoleID, 7, FieldRoleID); err != nil {
		errs.Add(err)
	}

	if len(ref.PolicyIDs) < 1 {
		errs.Add(&ValidationError{Field: FieldPolicyIDs, Message: "must be at least one policy ID", Code: "REQUIRED"})
	}

	for i, policyID := range ref.PolicyIDs {
		if err := ValidateUUID(policyID, 7, fmt.Sprintf("%s[%d]", FieldPolicyIDs, i)); err != nil {
			if valErr, ok := err.(*ValidationError); ok {
				errs.Add(&ValidationError{Field: fmt.Sprintf("%s[%d]", FieldPolicyIDs, i), Message: valErr.Message, Code: valErr.Code})
			}
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type UnlinkPoliciesFromRoleInput = LinkPoliciesToRoleInput
