package domain

import (
	"time"

	"uuid"
)

// UserIdentity links one provider identity to one account.
//
// The identity is the provider's stable subject -- OIDC `sub`, GitHub's
// numeric user id -- never the email. An email is a mutable attribute that an
// Entra admin, a GitHub user or the account holder can change, and matching
// accounts on it let anyone who controlled an email at ONE provider sign in as
// the account behind it. The email is kept only as the value seen at link
// time, for the profile page.
type UserIdentity struct {
	LinkedAt time.Time
	Subject  string
	Email    string

	// Provider name and type, filled by the listing for the profile page.
	IDPName     string
	IDPTypeName string

	UserID uuid.UUID
	IDPID  uuid.UUID
}

// LinkUserIdentityInput is what a link writes.
type LinkUserIdentityInput struct {
	Subject string
	Email   string
	UserID  uuid.UUID
	IDPID   uuid.UUID
}

func (ref *LinkUserIdentityInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ValidateUUID(ref.UserID, 7, FieldUserID))
	errs.Add(ValidateUUID(ref.IDPID, 7, FieldIDPID))

	if ref.Subject == "" {
		errs.AddError(FieldSubject, "the provider's subject is required", "REQUIRED")
	}

	if len(ref.Subject) > ValidIDPClientIDMaxLength {
		errs.AddError(FieldSubject, "the provider's subject is too long", "TOO_LONG")
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// UnlinkUserIdentityInput removes one provider from one account.
type UnlinkUserIdentityInput struct {
	UserID uuid.UUID
	IDPID  uuid.UUID
}

func (ref *UnlinkUserIdentityInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ValidateUUID(ref.UserID, 7, FieldUserID))
	errs.Add(ValidateUUID(ref.IDPID, 7, FieldIDPID))

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// UserIdentityNotFoundError: the (idp, subject) or (user, idp) pair has no row.
// The ordinary case for a first sign-in; an error only because the caller has
// to branch on it.
type UserIdentityNotFoundError struct {
	Message string
}

func (e *UserIdentityNotFoundError) Error() string {
	if e.Message == "" {
		return "no account is linked to this provider identity"
	}

	return e.Message
}

// UserIdentityAlreadyLinkedError: the provider identity belongs to another
// account, or this account already has an identity at that provider.
type UserIdentityAlreadyLinkedError struct {
	Message string
}

func (e *UserIdentityAlreadyLinkedError) Error() string {
	if e.Message == "" {
		return "this provider identity is already linked to an account"
	}

	return e.Message
}

// IDPIdentityNotLinkedError is what an IdP sign-in answers when the identity
// is unknown and cannot be provisioned. One wording whatever the reason -- an
// existing account with that email, a provider that does not vouch for the
// email, auto-provisioning switched off -- because the differences would tell
// whoever controls the provider account which addresses have accounts here.
type IDPIdentityNotLinkedError struct{}

func (e *IDPIdentityNotLinkedError) Error() string {
	return "this identity is not linked to an account here; sign in with your password and link the provider from your profile, or ask an administrator"
}
