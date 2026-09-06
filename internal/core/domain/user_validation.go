package domain

// User string-length constraints. Used by ValidateEmail / ValidatePassword
// in validation.go and by the User entity's own Validate() implementations.
const (
	ValidUserFirstNameMinLength = 2
	ValidUserFirstNameMaxLength = 25
	ValidUserLastNameMinLength  = 2
	ValidUserLastNameMaxLength  = 25
	ValidUserEmailMinLength     = 6
	// RFC 5321's cap, and users.email is a VARCHAR(254) to match. It was 50
	// here and in the column while ValidateEmail allowed 254, so an address of
	// 51 to 254 characters passed validation and died in Postgres as a 500.
	ValidUserEmailMaxLength = MaxEmailLength
	// Eight is the floor NIST SP 800-63B sets for a user-chosen password; six
	// was below it. Seventy-two is bcrypt's input limit: x/crypto refuses a
	// longer password with ErrPasswordTooLong, so the old cap of 255 accepted
	// a password that could not be hashed and answered a 500 for it.
	ValidUserPasswordMinLength = 8
	ValidUserPasswordMaxLength = 72
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
