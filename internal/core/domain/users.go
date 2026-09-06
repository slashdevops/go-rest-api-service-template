package domain

import (
	"fmt"
	"reflect"
	"time"

	"uuid"
)

const (
	UsersUserCreatedSuccessfully          = "User created successfully"
	UsersUserUpdatedSuccessfully          = "User updated successfully"
	UsersUserDeletedSuccessfully          = "User deleted successfully"
	UsersRoleLinkedToUserSuccessfully     = "User role linked successfully"
	UsersRoleUnlinkedFromUserSuccessfully = "User role unlinked successfully"
	UsersUserFound                        = "User found"

	UsersProjectsLinkedToUserSuccessfully     = "Projects linked to user successfully"
	UsersProjectsUnlinkedFromUserSuccessfully = "Projects unlinked from user successfully"
)

var (
	UsersFilterFields  = []string{FieldID, FieldFirstName, FieldLastName, FieldEmail, FieldDisabled, FieldVerified, FieldLocalAccount, FieldCreatedAt, FieldUpdatedAt}
	UsersSortFields    = []string{FieldID, FieldFirstName, FieldLastName, FieldEmail, FieldDisabled, FieldVerified, FieldLocalAccount, FieldCreatedAt, FieldUpdatedAt}
	UsersPartialFields = []string{FieldID, FieldFirstName, FieldLastName, FieldEmail, FieldDisabled, FieldVerified, FieldLocalAccount, FieldCreatedAt, FieldUpdatedAt}
)

// User is the user-account entity.
//
// Disabled and Verified are two facts, not one. Disabled says whether the
// account may sign in and is what an administrator switches. Verified says
// whether the address has been proven by following the verification link (or
// by an identity provider, which proves its subject). A registration is
// unverified and disabled; verification flips both; nothing else sets
// Verified. They used to be one flag, and password recovery could not tell an
// account that had never followed its link from one that had been switched
// off, so it refused both.
type User struct {
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Disabled     *bool
	Verified     *bool
	Admin        *bool
	LocalAccount *bool
	FirstName    string
	LastName     string
	Email        string
	Password     string
	PasswordHash string
	SerialID     int64
	ID           uuid.UUID
}

type InsertUserInput struct {
	Disabled     *bool
	Verified     *bool
	LocalAccount *bool
	FirstName    string
	LastName     string
	Email        string
	Password     string
	PasswordHash string
	ID           uuid.UUID
}

func (ref *InsertUserInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ValidateUUID(ref.ID, 7, FieldID))

	if normalizedFirstName, err := ValidateString(ref.FirstName, StringValidationOptions{
		MinLength: ValidUserFirstNameMinLength, MaxLength: ValidUserFirstNameMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldFirstName,
	}); err != nil {
		errs.Add(err)
	} else {
		ref.FirstName = normalizedFirstName
	}

	if normalizedLastName, err := ValidateString(ref.LastName, StringValidationOptions{
		MinLength: ValidUserLastNameMinLength, MaxLength: ValidUserLastNameMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldLastName,
	}); err != nil {
		errs.Add(err)
	} else {
		ref.LastName = normalizedLastName
	}

	if normalizedEmail, err := ValidateEmail(ref.Email, FieldEmail); err != nil {
		errs.Add(err)
	} else {
		ref.Email = normalizedEmail
	}

	if ref.PasswordHash != "" {
		if _, err := ValidateString(ref.PasswordHash, StringValidationOptions{
			MinLength: ValidUserPasswordMinLength, MaxLength: ValidUserPasswordMaxLength,
			TrimWhitespace: false, AllowEmpty: false, NoControlChars: true, NoNullBytes: true,
			FieldName: FieldPasswordHash,
		}); err != nil {
			errs.Add(err)
		}
	}

	if ref.Password != "" {
		errs.Add(ValidatePassword(ref.Password, FieldPassword))
	}

	if ref.LocalAccount == nil {
		errs.AddError("local_account", "local_account cannot be nil", "REQUIRED")
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type CreateUserInput = InsertUserInput

type UpdateUserInput struct {
	FirstName    *string
	LastName     *string
	Email        *string
	Password     *string
	PasswordHash *string
	Disabled     *bool
	// Verified is set by the verification flow only; no request payload maps
	// to it.
	Verified     *bool
	LocalAccount *bool
	ID           uuid.UUID
}

func (ref *UpdateUserInput) Validate() error {
	if reflect.DeepEqual(ref, &UpdateUserInput{}) {
		return &InvalidUserUpdateError{Message: "at least one field must be updated"}
	}

	var errs ValidationErrors

	if err := ValidateUUID(ref.ID, 7, FieldID); err != nil {
		errs.Add(err)
	}

	if ref.FirstName != nil {
		if normalizedFirstName, err := ValidateString(*ref.FirstName, StringValidationOptions{
			MinLength: ValidUserFirstNameMinLength, MaxLength: ValidUserFirstNameMaxLength,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
			NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldFirstName,
		}); err != nil {
			errs.Add(err)
		} else {
			ref.FirstName = &normalizedFirstName
		}
	}

	if ref.LastName != nil {
		if normalizedLastName, err := ValidateString(*ref.LastName, StringValidationOptions{
			MinLength: ValidUserLastNameMinLength, MaxLength: ValidUserLastNameMaxLength,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
			NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldLastName,
		}); err != nil {
			errs.Add(err)
		} else {
			ref.LastName = &normalizedLastName
		}
	}

	if ref.Email != nil {
		if normalizedEmail, err := ValidateEmail(*ref.Email, FieldEmail); err != nil {
			errs.Add(err)
		} else {
			ref.Email = &normalizedEmail
		}
	}

	if ref.PasswordHash != nil && *ref.PasswordHash != "" {
		if _, err := ValidateString(*ref.PasswordHash, StringValidationOptions{
			MinLength: ValidUserPasswordMinLength, MaxLength: ValidUserPasswordMaxLength,
			TrimWhitespace: false, AllowEmpty: false, NoControlChars: true, NoNullBytes: true,
			FieldName: FieldPasswordHash,
		}); err != nil {
			errs.Add(err)
		}
	}

	if ref.Password != nil && *ref.Password != "" {
		errs.Add(ValidatePassword(*ref.Password, FieldPassword))
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type DeleteUserInput struct {
	ID uuid.UUID
}

func (ref *DeleteUserInput) Validate() error {
	var errs ValidationErrors
	errs.Add(ValidateUUID(ref.ID, 7, FieldID))
	if errs.HasErrors() {
		return &errs
	}
	return nil
}

type SelectUsersInput struct {
	Sort      string
	Filter    string
	Fields    string
	Paginator Paginator
}

func (ref *SelectUsersInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ref.Paginator.Validate())
	errs.Add(ValidateSortExpression(ref.Sort, FieldSort))
	errs.Add(ValidateFilterExpression(ref.Filter, FieldFilter))
	errs.Add(ValidateFieldsExpression(ref.Fields, FieldFields))

	if ref.Sort != "" {
		_, err := UsersSortParser.Parse(ref.Sort)
		if err != nil {
			errs.AddError("sort", err.Error(), "INVALID_SORT_FIELD")
		}
	}

	if ref.Filter != "" {
		_, err := UsersFilterParser.Parse(ref.Filter)
		if err != nil {
			errs.AddError("filter", err.Error(), "INVALID_FILTER_FIELD")
		}
	}

	if ref.Fields != "" {
		_, err := UsersFieldsParser.Parse(ref.Fields)
		if err != nil {
			errs.AddError("fields", err.Error(), "INVALID_FIELD")
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type ListUsersInput = SelectUsersInput

type SelectUsersOutput struct {
	Items     []User
	Paginator Paginator
}

type ListUsersOutput = SelectUsersOutput

type LinkRolesToUserInput struct {
	RoleIDs []uuid.UUID
	// CallerID is who is granting; the guard checks they hold what they hand
	// out, and refuses a zero value. Unlink paths share this input and leave it
	// zero: nothing is granted there.
	CallerID uuid.UUID
	UserID   uuid.UUID
}

func (ref *LinkRolesToUserInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ValidateUUID(ref.UserID, 7, FieldUserID))

	if len(ref.RoleIDs) < 1 {
		errs.AddError("role_ids", "must be at least one role ID", "REQUIRED")
	}

	for i, roleID := range ref.RoleIDs {
		if err := ValidateUUID(roleID, 7, fmt.Sprintf("role_ids[%d]", i)); err != nil {
			errs.Add(err)
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type UnlinkRolesFromUsersInput = LinkRolesToUserInput

type LinkProjectsToUserInput struct {
	ProjectIDs []uuid.UUID
	UserID     uuid.UUID
}

func (ref *LinkProjectsToUserInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ValidateUUID(ref.UserID, 7, FieldUserID))

	for i, projectID := range ref.ProjectIDs {
		if err := ValidateUUID(projectID, 7, fmt.Sprintf("project_ids[%d]", i)); err != nil {
			errs.Add(err)
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type UnlinkProjectsFromUserInput = LinkProjectsToUserInput

type SelectAuthzOutput struct {
	Permissions map[string]any `json:"permissions"`
	Roles       []string       `json:"roles"`
	Policies    []string       `json:"policies"`
}
