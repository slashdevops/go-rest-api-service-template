package domain

// User string-length constraints. Used by ValidateEmail / ValidatePassword
// in validation.go and by the User entity's own Validate() implementations.
const (
	ValidUserFirstNameMinLength = 2
	ValidUserFirstNameMaxLength = 25
	ValidUserLastNameMinLength  = 2
	ValidUserLastNameMaxLength  = 25
	ValidUserEmailMinLength     = 6
	ValidUserEmailMaxLength     = 50
	ValidUserPasswordMinLength  = 6
	ValidUserPasswordMaxLength  = 255
)

// InvalidUserUpdateError represents an invalid user update error.
type InvalidUserUpdateError struct {
	Field   string // optional: which field
	Reason  string // optional: why update is invalid
	Message string // optional: additional context
}

func (e *InvalidUserUpdateError) Error() string {
	reason := e.Reason
	if e.Field != "" && e.Reason != "" {
		reason = "field '" + e.Field + "': " + e.Reason
	} else if e.Field != "" {
		reason = "field '" + e.Field + "'"
	}

	if e.Message != "" && reason == "" {
		reason = e.Message
	}

	return (&BaseInvalidUpdateError{
		Entity: "user",
		Reason: reason,
	}).Error()
}
