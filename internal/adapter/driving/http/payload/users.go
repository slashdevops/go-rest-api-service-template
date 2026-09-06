package payload

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// UserResponse represents a user entity used to model the data stored in the database.
//
//	@Description	UserResponse represents a user entity.
type UserResponse struct {
	CreatedAt    time.Time `json:"created_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`              // Timestamp when the user account was created
	UpdatedAt    time.Time `json:"updated_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`              // Timestamp when the user account was last updated
	Disabled     *bool     `json:"disabled,omitempty" example:"false"`                                                 // Indicates if the user account is currently disabled
	Admin        *bool     `json:"admin,omitempty" example:"false"`                                                    // Indicates if the user has administrative privileges
	LocalAccount *bool     `json:"local_account,omitempty" example:"true"`                                             // Indicates if this is a locally managed account (vs SSO/federated)
	FirstName    string    `json:"first_name,omitempty" example:"John" format:"string"`                                // User's first name
	LastName     string    `json:"last_name,omitempty" example:"Doe" format:"string"`                                  // User's last name
	Email        string    `json:"email,omitempty" example:"john.doe@example.com" format:"email"`                      // User's email address
	ID           uuid.UUID `json:"id,omitempty,omitzero" example:"019b4b0d-a682-7e82-a25a-b0671dc354c2" format:"uuid"` // Unique identifier for the user
}

// ListUsersResponse represents a list of users.
//
//	@Description	Paginated list of user accounts in the system
type ListUsersResponse struct {
	Items     []UserResponse   `json:"items"`     // Array of user account details
	Paginator domain.Paginator `json:"paginator"` // Pagination information including total count and page details
}

// CreateUserRequest represents the input for the CreateUser method.
//
//	@Description	Request payload for creating a new user account
type CreateUserRequest struct {
	FirstName string    `json:"first_name" example:"John" format:"string" validate:"required" minLength:"2" maxLength:"50"`              // User's first name
	LastName  string    `json:"last_name" example:"Doe" format:"string" validate:"required" minLength:"2" maxLength:"50"`                // User's last name
	Email     string    `json:"email" example:"john.doe@example.com" format:"email" validate:"required"`                                 // User's email address (must be unique)
	Password  string    `json:"password" example:"SecureP@ssw0rd123" format:"password" validate:"required" minLength:"8" maxLength:"72"` // User's password (minimum 8 characters)
	ID        uuid.UUID `json:"id,omitempty" example:"019b4b0d-a682-7013-91b4-d452c93dfa47" format:"uuid" validate:"optional"`           // Optional custom user ID
}

func (req *CreateUserRequest) Validate() error {
	var errs domain.ValidationErrors

	if err := domain.ValidateUUID(req.ID, 7, domain.FieldID); err != nil {
		errs.Add(err)
	}

	if normalizedFirstName, err := domain.ValidateString(req.FirstName, domain.StringValidationOptions{
		MinLength: domain.ValidUserFirstNameMinLength, MaxLength: domain.ValidUserFirstNameMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoNullBytes: true, NormalizeUnicode: true, FieldName: domain.FieldFirstName,
	}); err != nil {
		errs.Add(err)
	} else {
		req.FirstName = normalizedFirstName
	}

	if normalizedLastName, err := domain.ValidateString(req.LastName, domain.StringValidationOptions{
		MinLength: domain.ValidUserLastNameMinLength, MaxLength: domain.ValidUserLastNameMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoNullBytes: true, NormalizeUnicode: true, FieldName: domain.FieldLastName,
	}); err != nil {
		errs.Add(err)
	} else {
		req.LastName = normalizedLastName
	}

	if normalizedEmail, err := domain.ValidateEmail(req.Email, domain.FieldEmail); err != nil {
		errs.Add(err)
	} else {
		req.Email = normalizedEmail
	}

	if err := domain.ValidatePassword(req.Password, domain.FieldPassword); err != nil {
		errs.Add(err)
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// UpdateUserRequest represents the input for the UpdateUser method.
//
//	@Description	Request payload for updating an existing user account (all fields optional). A password is never set this way: POST /users/{user_id}/password/reset emails the account holder a reset link, so taking over an account needs its mailbox, not a grant on PUT /users.
type UpdateUserRequest struct {
	FirstName    *string `json:"first_name,omitempty" example:"John" format:"string" validate:"optional" minLength:"2" maxLength:"50"` // Updated first name
	LastName     *string `json:"last_name,omitempty" example:"Doe" format:"string" validate:"optional" minLength:"2" maxLength:"50"`   // Updated last name
	Email        *string `json:"email,omitempty" example:"john.doe@example.com" format:"email" validate:"optional"`                    // Updated email address
	Disabled     *bool   `json:"disabled,omitempty" example:"false" validate:"optional"`                                               // Set to true to disable the account, false to enable
	LocalAccount *bool   `json:"local_account,omitempty" example:"true" validate:"optional"`                                           // Set account type (local vs federated)
}

func (req *UpdateUserRequest) Validate() error {
	if reflect.DeepEqual(req, &UpdateUserRequest{}) {
		return &domain.InvalidUserUpdateError{Message: "at least one field must be updated"}
	}

	var errs domain.ValidationErrors

	if req.FirstName != nil {
		if normalizedFirstName, err := domain.ValidateString(*req.FirstName, domain.StringValidationOptions{
			MinLength: domain.ValidUserFirstNameMinLength, MaxLength: domain.ValidUserFirstNameMaxLength,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
			NoNullBytes: true, NormalizeUnicode: true, FieldName: domain.FieldFirstName,
		}); err != nil {
			errs.Add(err)
		} else {
			req.FirstName = &normalizedFirstName
		}
	}

	if req.LastName != nil {
		if normalizedLastName, err := domain.ValidateString(*req.LastName, domain.StringValidationOptions{
			MinLength: domain.ValidUserLastNameMinLength, MaxLength: domain.ValidUserLastNameMaxLength,
			TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
			NoNullBytes: true, NormalizeUnicode: true, FieldName: domain.FieldLastName,
		}); err != nil {
			errs.Add(err)
		} else {
			req.LastName = &normalizedLastName
		}
	}

	if req.Email != nil {
		if normalizedEmail, err := domain.ValidateEmail(*req.Email, domain.FieldEmail); err != nil {
			errs.Add(err)
		} else {
			req.Email = &normalizedEmail
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// LinkRolesToUserRequest represents the input for the LinkRoles method.
//
//	@Description	Request payload for assigning multiple roles to a user
type LinkRolesToUserRequest struct {
	RoleIDs []uuid.UUID `json:"role_ids" format:"array" validate:"required" example:"019b4b0d-a682-7e34-a20c-c71a7147d7e7,019b4b0d-a682-7e38-b235-3dfcb59f4d9e"` // Array of role IDs to assign to the user
}

func (req *LinkRolesToUserRequest) Validate() error {
	if reflect.DeepEqual(req, &LinkRolesToUserRequest{}) {
		return &domain.ValidationError{
			Field:   "role_ids",
			Message: "at least one role ID must be provided",
			Code:    "REQUIRED",
		}
	}

	var errs domain.ValidationErrors

	if len(req.RoleIDs) < 1 {
		errs.Add(&domain.ValidationError{Field: "role_ids", Message: "must be at least one role ID", Code: "REQUIRED"})
		return &errs
	}

	for i, roleID := range req.RoleIDs {
		if err := domain.ValidateUUID(roleID, 7, fmt.Sprintf("role_ids[%d]", i)); err != nil {
			errs.Add(err)
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// LinkProjectsToUserRequest represents the input for linking projects to a user.
//
//	@Description	Request payload for associating multiple projects with a user
type LinkProjectsToUserRequest struct {
	ProjectIDs []uuid.UUID `json:"project_ids" format:"array" validate:"required" example:"019b4b0d-a682-7e34-a20c-c71a7147d7e7,019b4b0d-a682-7e38-b235-3dfcb59f4d9e"` // Array of project IDs to associate with the user
}

func (req *LinkProjectsToUserRequest) Validate() error {
	if reflect.DeepEqual(req, &LinkProjectsToUserRequest{}) {
		return &domain.InvalidProjectLinkError{Message: "at least one project ID must be provided"}
	}

	var errs domain.ValidationErrors

	if len(req.ProjectIDs) < 1 {
		errs.Add(&domain.ValidationError{Field: "project_ids", Message: "must be at least one project ID", Code: "REQUIRED"})
	}

	for i, projectID := range req.ProjectIDs {
		if err := domain.ValidateUUID(projectID, 7, fmt.Sprintf("project_ids[%d]", i)); err != nil {
			errs.Add(err)
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// UnlinkProjectsFromUserRequest represents the input for the UnlinkProjects method.
//
//	@Description	Request payload for removing multiple projects from a user
type UnlinkProjectsFromUserRequest = LinkProjectsToUserRequest

// UnlinkRolesFromUserRequest represents the input for the UnlinkRoles method.
//
//	@Description	Request payload for removing multiple roles from a user
type UnlinkRolesFromUserRequest = LinkRolesToUserRequest

// AuthzMethods are the HTTP methods granted on one path. "*" means every
// method; a path key of "*" means every path.
//
//	@Description	HTTP methods granted on a path. "*" means every method
type AuthzMethods = []string

// AuthzPaths maps an API path to the methods granted on it.
//
// A key of "*" is the global grant for the subject. Note that elsewhere in a
// path a "*" is NOT "any segment": the Rego bundle rewrites it to a UUID
// pattern before matching (see policyopa/rego/bundle/authorization/policy.rego),
// so it means "an id here". A client that matches it more loosely will offer
// controls the API then refuses.
//
//	@Description	API path to the methods granted on it. A key of "*" grants every path
type AuthzPaths = map[string]AuthzMethods

// AuthzSubjects maps a subject id -- a user id today -- to its paths.
//
//	@Description	Subject id to the paths granted to it
type AuthzSubjects = map[string]AuthzPaths

// AuthzPermissions is the permission set, by category.
//
// The only category today is "users", so the shape is
//
//	users -> <user id> -> <path> -> [methods]
//
// It is a map at every level because all three key sets are data: categories
// may grow, subjects are ids, and paths are whatever the policies name. That is
// why this is not a struct -- but it is also why leaving it as `any` published
// nothing at all to a generated client.
//
//	@Description	Permission set by category; today the only category is "users"
type AuthzPermissions = map[string]AuthzSubjects

// GetAuthenticatedUserResponse represents the response for the GetAuthenticatedUser method.
//
//	@Description	Response containing authenticated user details and their complete permission set
type GetAuthenticatedUserResponse struct {
	Permissions AuthzPermissions `json:"permissions"` // Complete set of permissions granted to the user through roles and policies
	Account     UserResponse     `json:"account"`     // Authenticated user's account information
}

// UserAuthzResponse is the body of GET /users/{user_id}/authz.
//
// # Why this exists
//
// The endpoint published `{"type": "object", "additionalProperties": true}` --
// the only operation in the API that did -- so every generated client received
// `any` for an authorization payload. The shape was never in doubt; it was
// simply never written down.
//
// # The extra nesting level, which is NOT a mistake here
//
// This body wraps the permissions one level deeper than /me/authz does:
//
//	GET /users/{id}/authz  ->  {"permissions": {"users": {...}}}
//	GET /me/authz          ->  {"account": {...}, "permissions": {"users": {...}}}
//
// Both carry the same inner structure. The repository returns a map already
// keyed by "permissions" and usecase.Authn strips that level for /me/authz
// while this endpoint passes it through. Unifying them would change bytes a
// client already depends on, so this type documents what ships rather than
// what would be tidier.
//
//	@Description	A user's effective permissions, by category
type UserAuthzResponse struct {
	Permissions AuthzPermissions `json:"permissions"` // Effective permissions, keyed by category
}

// NewUserAuthzResponse types the permission map the use-case returns.
//
// The map stays `map[string]any` all the way through the core because it is fed
// to OPA as Rego input, which wants generic maps. Only the wire body is typed,
// and only here.
//
// # Why this goes through JSON rather than walking the map
//
// The value came from a JSON column and is on its way back out as JSON, so a
// round trip is the operation that already defines its shape. A hand-written
// walker would have to decide what to do with a level that does not match, and
// every answer to that is wrong: skipping loses permissions silently, and
// permissions are the last thing to lose quietly. Unmarshal either matches the
// whole shape or reports that it does not.
//
// The caller decides what a mismatch means. It should not be fatal: an
// authorization read failing because the response could not be typed would
// trade a real answer for a presentation concern.
func NewUserAuthzResponse(permissions map[string]any) (UserAuthzResponse, error) {
	return decodeAuthz[UserAuthzResponse](permissions)
}

// NewAuthzPermissions types a permission set that is NOT wrapped in a
// "permissions" key -- what /me/authz and /auth/login carry, where the wrapper
// has already been stripped by usecase.Authn.
//
// Failing here is safe in a way it would not be on an enforcement path: these
// sets tell a CLIENT which controls to offer, and nothing else. Authorization
// is decided by CheckAuthz on every request, from the same data, server-side.
// So an empty result hides controls a caller may use; it can never reveal one
// they may not. The caller is still expected to log it loudly -- silently
// showing a user an empty application is its own kind of wrong.
func NewAuthzPermissions(permissions map[string]any) (AuthzPermissions, error) {
	return decodeAuthz[AuthzPermissions](permissions)
}

// decodeAuthz round-trips a permission map through JSON into T.
//
// See [NewUserAuthzResponse] for why this is a round trip rather than a walk
// over the map: the value came from a JSON column and is on its way back out as
// JSON, and unmarshal either matches the whole shape or says it does not, where
// a walker would have to silently drop what it did not recognise.
func decodeAuthz[T any](permissions map[string]any) (T, error) {
	var out T

	encoded, err := json.Marshal(permissions)
	if err != nil {
		var zero T

		return zero, fmt.Errorf("encoding the permission map: %w", err)
	}

	if err := json.Unmarshal(encoded, &out); err != nil {
		// The zero value, NOT what unmarshal managed to fill in. It populates
		// the destination as it goes and stops at the first entry it cannot
		// decode, so `out` here holds whatever happened to parse first -- and
		// the callers send what they are given even when this returned an
		// error. That would put a SILENTLY PARTIAL permission set on the wire:
		// a client offering exactly the controls that survived, with nothing to
		// say the rest were dropped. Empty is a state a caller can recognise;
		// arbitrarily truncated is not.
		var zero T

		return zero, fmt.Errorf("the permission map does not match the documented shape: %w", err)
	}

	return out, nil
}
