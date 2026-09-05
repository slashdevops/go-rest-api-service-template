package payload

import (
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// LoginUserRequest is the request struct for the LoginUser handler.
//
//	@Description	Request payload for user authentication via email and password
type LoginUserRequest struct {
	Email    string `json:"email" example:"admin@goapitemplate.local" format:"email" validate:"required"` // User's email address
	Password string `json:"password" example:"ThisIsApassw0rd.," format:"password" validate:"required"`   // User's password
}

func (req *LoginUserRequest) Validate() error {
	var errs domain.ValidationErrors

	if normalizedEmail, err := domain.ValidateEmail(req.Email, domain.FieldEmail); err != nil {
		errs.Add(err)
	} else {
		req.Email = normalizedEmail
	}

	errs.Add(domain.ValidatePassword(req.Password, domain.FieldPassword))

	if errs.HasErrors() {
		return &errs
	}
	return nil
}

// LoginUserResponse is the response when a user logs in.
//
//	@Description	Response containing authentication tokens and user information after successful login
type LoginUserResponse struct {
	Resources    AuthzPermissions `json:"permissions" format:"object"`                                                     // User's permissions and accessible resources
	AccessToken  string           `json:"access_token" format:"string" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`  // JWT access token
	RefreshToken string           `json:"refresh_token" format:"string" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."` // JWT refresh token
	TokenType    domain.TokenType `json:"token_type" example:"Bearer" format:"string"`                                     // Token type for Authorization header
	UserID       uuid.UUID        `json:"user_id" example:"019b4b0d-a682-7e40-a1c3-d5e8f9a2b4c6" format:"uuid"`            // Unique identifier of the authenticated user
}

// RefreshTokenRequest is the request struct for the RefreshToken handler.
//
//	@Description	Request payload for obtaining a new access token using a refresh token
type RefreshTokenRequest struct {
	// Optional. The token actually spent is the one in the Authorization
	// header, which is what the middleware verified. This field is kept because
	// every existing client sends the token in both places, but it may not
	// disagree with the header: a request authorised with one token and asking
	// to spend another is refused rather than resolved by picking one.
	RefreshToken string `json:"refresh_token" format:"string" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

// RefreshTokenResponse is the response when a user refreshes their token.
//
//	@Description	Response containing new authentication tokens after successful refresh
type RefreshTokenResponse struct {
	AccessToken  string           `json:"access_token" format:"string" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`  // New JWT access token
	RefreshToken string           `json:"refresh_token" format:"string" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."` // New JWT refresh token
	TokenType    domain.TokenType `json:"token_type" example:"Bearer" format:"string"`                                     // Token type (always "Bearer")
}

// RegisterUserRequest is the request struct for the RegisterUser handler.
//
//	@Description	Request payload for creating a new user account with email verification
type RegisterUserRequest struct {
	FirstName string    `json:"first_name" example:"John" format:"string" validate:"required" minLength:"2" maxLength:"50"`              // User's first name
	LastName  string    `json:"last_name" example:"Doe" format:"string" validate:"required" minLength:"2" maxLength:"50"`                // User's last name
	Email     string    `json:"email" example:"john.doe@example.com" format:"email" validate:"required"`                                 // User's email address
	Password  string    `json:"password" example:"SecureP@ssw0rd123" format:"password" validate:"required" minLength:"8" maxLength:"72"` // User's password (minimum 8 characters)
	ID        uuid.UUID `json:"id" example:"019b4b0d-a682-7e41-8c2d-f3a4b5c6d7e8" format:"uuid" validate:"optional"`                     // Optional custom user ID
}

func (req *RegisterUserRequest) Validate() error {
	var errs domain.ValidationErrors

	errs.Add(domain.ValidateUUID(req.ID, 7, domain.FieldID))

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

	errs.Add(domain.ValidatePassword(req.Password, domain.FieldPassword))

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// ReVerifyUserRequest is the request struct for the ReVerifyUser handler.
//
//	@Description	Request payload for resending email verification to an unverified user
type ReVerifyUserRequest struct {
	Email string `json:"email" format:"email" example:"user@example.com" validate:"required"` // Email address of the user to re-verify
}

func (req *ReVerifyUserRequest) Validate() error {
	if normalizedEmail, err := domain.ValidateEmail(req.Email, domain.FieldEmail); err != nil {
		return err
	} else {
		req.Email = normalizedEmail
	}
	return nil
}

// RecoverPasswordRequest is the request struct for the RecoverPassword handler.
//
//	@Description	Request payload for initiating password recovery via email
type RecoverPasswordRequest struct {
	Email string `json:"email" format:"email" example:"user@example.com" validate:"required"` // Email address of the account to recover
}

func (req *RecoverPasswordRequest) Validate() error {
	if normalizedEmail, err := domain.ValidateEmail(req.Email, domain.FieldEmail); err != nil {
		return err
	} else {
		req.Email = normalizedEmail
	}
	return nil
}

// ResetPasswordRequest is the request struct for the ResetPassword handler.
//
//	@Description	Request payload for setting a new password using a reset token
type ResetPasswordRequest struct {
	Password string `json:"password" format:"password" example:"NewSecureP@ssw0rd" validate:"required" minLength:"8" maxLength:"72"` // New password to set (minimum 8 characters)
}

func (req *ResetPasswordRequest) Validate() error {
	var errs domain.ValidationErrors

	errs.Add(domain.ValidatePassword(req.Password, domain.FieldPassword))

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// LogoutUserRequest is the optional body of a logout request.
//
// The refresh token is what logout revokes. It is optional so that existing
// callers, which send only an access token, keep working — but without it the
// session cannot be ended: see domain.LogoutUserInput.
type LogoutUserRequest struct {
	RefreshToken string `json:"refresh_token,omitempty" example:"eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9..."`
}
