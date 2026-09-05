package domain

import "time"

// InvalidJWTError represents an invalid JWT token error
type InvalidJWTError struct {
	Value   string // optional: the JWT value
	Reason  string // optional: why it's invalid
	Message string // optional: additional context

	// Expired distinguishes a token that was ours and has simply run out from
	// one that is malformed, unsigned, or signed by something else.
	//
	// It is a field rather than something a caller derives from Message,
	// because the two cases mean opposite things to a caller that is trying to
	// end a session: an expired token needs no revoking, while an unreadable
	// one means the session was NOT ended and must not be reported as if it
	// had been. Matching on the text would put that distinction one wording
	// change away from silently inverting.
	Expired bool
}

func (e *InvalidJWTError) Error() string {
	reason := e.Reason
	if reason == "" {
		reason = e.Message
	}
	return (&BaseInvalidFieldError{
		Field:  "JWT",
		Value:  e.Value,
		Reason: reason,
	}).Error()
}

// InvalidSenderError represents an invalid email sender error
type InvalidSenderError struct {
	Sender  string // optional: the invalid sender
	Message string
}

func (e *InvalidSenderError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "sender",
		Value:  e.Sender,
		Reason: e.Message,
	}).Error()
}

// InvalidRefreshTokenError represents an invalid refresh token error
type InvalidRefreshTokenError struct {
	Token   string // optional: the invalid token
	Message string
}

func (e *InvalidRefreshTokenError) Error() string {
	return (&BaseInvalidFieldError{
		Field:  "refresh token",
		Value:  e.Token,
		Reason: e.Message,
	}).Error()
}

// UserAlreadyVerifiedError represents an error when a user is already verified
type UserAlreadyVerifiedError struct {
	Email   string // optional: user email
	UserID  string // optional: user ID
	Message string // optional: additional context
}

func (e *UserAlreadyVerifiedError) Error() string {
	base := &BaseInvalidFieldError{
		Field:  "user",
		Reason: "user is already verified",
	}
	if e.Email != "" {
		base.Value = e.Email
	}
	if e.UserID != "" {
		base.Field = "userID"
		base.Value = e.UserID
	}
	if e.Message != "" {
		base.Reason = e.Message
	}
	return base.Error()
}

// InvalidVerificationEndpointError represents an invalid verification endpoint error
type InvalidVerificationEndpointError struct {
	Endpoint string // optional: the invalid endpoint
	Message  string
}

func (e *InvalidVerificationEndpointError) Error() string {
	base := &BaseInvalidFieldError{
		Field:  "verificationEndpoint",
		Reason: "invalid verification endpoint",
	}
	if e.Endpoint != "" {
		base.Value = e.Endpoint
	}
	if e.Message != "" {
		base.Reason = e.Message
	}
	return base.Error()
}

// InvalidRecoveryEndpointError represents an invalid password recovery endpoint error
type InvalidRecoveryEndpointError struct {
	Endpoint string // optional: the invalid endpoint
	Message  string
}

func (e *InvalidRecoveryEndpointError) Error() string {
	base := &BaseInvalidFieldError{
		Field:  "recoveryEndpoint",
		Reason: "invalid recovery endpoint",
	}
	if e.Endpoint != "" {
		base.Value = e.Endpoint
	}
	if e.Message != "" {
		base.Reason = e.Message
	}
	return base.Error()
}

// InvalidLoginError represents an invalid login attempt error
type InvalidLoginError struct {
	Username string // optional: username/email attempted
	Reason   string // optional: why login failed
	Message  string // optional: additional context
}

func (e *InvalidLoginError) Error() string {
	base := &BaseInvalidFieldError{
		Field:  "login",
		Reason: "invalid login",
	}
	if e.Username != "" {
		base.Field = "username"
		base.Value = e.Username
	}
	if e.Reason != "" {
		base.Reason = e.Reason
	}
	if e.Message != "" {
		base.Reason = e.Message
	}
	return base.Error()
}

// InvalidRecoverPasswordError represents an invalid password recovery attempt error
type InvalidRecoverPasswordError struct {
	Email   string // optional: email attempted
	Reason  string // optional: why recovery failed
	Message string // optional: additional context
}

func (e *InvalidRecoverPasswordError) Error() string {
	base := &BaseInvalidFieldError{
		Field:  "recoverPassword",
		Reason: "invalid recover password",
	}
	if e.Email != "" {
		base.Field = "email"
		base.Value = e.Email
	}
	if e.Reason != "" {
		base.Reason = e.Reason
	}
	if e.Message != "" {
		base.Reason = e.Message
	}
	return base.Error()
}

// TooManyLoginAttemptsError is returned when an account's login budget is
// spent. It carries how long until an attempt is possible again so the
// transport can set Retry-After.
//
// The message deliberately says nothing about the account. Whether the address
// exists, is disabled, or has simply been guessed at by someone else must not
// be inferable from a throttle response — that would hand back the enumeration
// signal the throttle is meant to make expensive.
type TooManyLoginAttemptsError struct {
	RetryAfter time.Duration
}

func (e *TooManyLoginAttemptsError) Error() string {
	return "too many failed login attempts, please try again later"
}

// TooManyRecoveryRequestsError is returned when an address's password-recovery
// budget is spent.
//
// Separate from [TooManyLoginAttemptsError] because the wording has to be true:
// nothing about a recovery request is a "failed login attempt". Same shape, and
// the same silence about the account — the budget is keyed on the address that
// was submitted, before anything is looked up, so an address with no account is
// throttled exactly like a real one and the response says nothing either way.
type TooManyRecoveryRequestsError struct {
	RetryAfter time.Duration
}

func (e *TooManyRecoveryRequestsError) Error() string {
	return "too many password recovery requests, please try again later"
}

// AuthnInvalidCredentials is the single message every failed login answers
// with, whatever actually went wrong.
const AuthnInvalidCredentials = "invalid email or password"

// Machine-readable reasons for a 401, carried in the "code" field of the
// response so a client can branch without matching on prose.
//
// There are exactly two, and the distinction is the whole reason the field
// exists: an EXPIRED access token should be refreshed and the request retried,
// a REVOKED one must not be. On a revocation the refresh token was revoked in
// the same breath, so the retry cannot succeed — it burns two more requests and
// then bounces the user to a login screen that says the wrong thing.
//
// These strings are a published contract. The prose next to them is not.
const (
	// CodeTokenRevoked means the session is over: signed out here or
	// elsewhere, or ended by an administrator. Clear the credentials and ask
	// for a sign-in. Do not refresh.
	CodeTokenRevoked = "token_revoked"
)

// AuthnTokenRevoked is the message that accompanies [CodeTokenRevoked].
//
// It says the session ended and not how that was discovered. A caller learning
// that a specific token was found on a denylist learns something about the
// service's internals for no benefit; "you were signed out" is the whole of
// what they can act on.
const AuthnTokenRevoked = "session ended, please sign in again"

// InvalidCredentialsError is returned by a login that did not succeed, for
// every reason a login does not succeed: no such address, the wrong password, a
// disabled account, an account that authenticates through an identity provider.
//
// One error type and one message on purpose. Distinct messages let a caller ask
// "does this address have an account?" and get an answer, which is the first
// step of the attack the per-account throttle is there to slow down — a throttle
// makes enumeration expensive, and this makes it uninformative.
//
// The reason is not lost: LoginUser records it on the span and in the log, so an
// operator can still tell a typo from a disabled account.
type InvalidCredentialsError struct{}

func (e *InvalidCredentialsError) Error() string {
	return AuthnInvalidCredentials
}
