package domain

import (
	"time"

	"uuid"
)

const (
	// Bounds on the PATH of a key file, not on the key inside it. All three
	// key settings name a file, and a path is a path.
	//
	// There used to be three identical pairs, one per setting, and the
	// symmetric one was also used by the AES adapter as if it bounded the KEY.
	// It does not: AES takes exactly 16, 24 or 32 bytes, so a 3-to-255-byte
	// check accepted keys AES would refuse -- and refused nothing it would
	// accept. See ValidAESKeySize.
	ValidAuthnKeyFilePathMinLength         = 3
	ValidAuthnKeyFilePathMaxLength         = 255
	ValidAuthnIssuerMinLength              = 3
	ValidAuthnIssuerMaxLength              = 100
	ValidAuthnMaxEntitiesCacheTTL          = 72 * time.Hour
	ValidAuthnMinEntitiesCacheTTL          = 1 * time.Hour
	ValidAuthnMaxUserVerificationTokenTTL  = 3 * 24 * time.Hour
	ValidAuthnMinUserVerificationTokenTTL  = 1 * time.Hour
	ValidAuthnMinUserResetPasswordTokenTTL = 1 * time.Minute
	ValidAuthnMaxUserResetPasswordTokenTTL = 72 * time.Hour

	AuthnUserRegisteredSuccessfully       = "User registered successfully"
	AuthnUserVerifiedSuccessfully         = "User verified successfully"
	AuthnUserVerificationEmailSent        = "User verification email sent"
	AuthnUserLoggedOutSuccessfully        = "User logged out successfully"
	AuthnPasswordRecoveryEmailSent        = "Password recovery email sent"
	AuthnPasswordResetSuccessfully        = "Password reset successfully"
	AuthnAccessTokenRefreshedSuccessfully = "access token refreshed"

	AuthnFailedToGetUserIDFromContext   = "failed to get user id from context"
	AuthnFailedToParseUserIDFromContext = "failed to parse user ID from context"

	ValidJWTMinLength = 20
	ValidJWTMaxLength = 2048
)

// JWTClaims represents the claims in a JWT token.
type JWTClaims struct {
	IDP           string        `json:"idp,omitempty"`
	Email         string        `json:"email,omitempty"`
	Subject       string        `json:"sub"`
	TokenType     TokenType     `json:"token_type"`
	Issuer        string        `json:"iss"`
	TokenDuration time.Duration `json:"token_duration,omitempty"`

	// TokenID becomes the jti claim. Leave it zero and the signer generates
	// one, which is what every short-lived token wants.
	//
	// A personal access token sets it to the id of its own pa_tokens row, so
	// that presenting the token is enough to find the record that governs it.
	// Without that link the row and the credential are unrelated: deleting the
	// row leaves the token working, which is precisely what it used to do.
	TokenID uuid.UUID `json:"-"`
}

// LoginMethod represents the method used to login.
type LoginMethod string

const (
	LoginMethodPassword LoginMethod = "password"
	LoginMethodOAuth    LoginMethod = "oauth"
	LoginMethodSAML     LoginMethod = "saml"
	LoginMethodOIDC     LoginMethod = "oidc"
)

func (lm LoginMethod) IsValid() bool {
	switch lm {
	case LoginMethodPassword, LoginMethodOAuth, LoginMethodSAML, LoginMethodOIDC:
		return true
	}
	return false
}

func (lm LoginMethod) String() string {
	return string(lm)
}

// RegisterMethod represents the method used to register.
type RegisterMethod string

const (
	RegisterMethodPassword RegisterMethod = "password"
	RegisterMethodOAuth    RegisterMethod = "oauth"
	RegisterMethodSAML     RegisterMethod = "saml"
	RegisterMethodOIDC     RegisterMethod = "oidc"
)

func (rm RegisterMethod) IsValid() bool {
	switch rm {
	case RegisterMethodPassword, RegisterMethodOAuth, RegisterMethodSAML, RegisterMethodOIDC:
		return true
	}
	return false
}

func (rm RegisterMethod) String() string {
	return string(rm)
}

// LoginUserInput is the input for the LoginUser use-case.
type LoginUserInput struct {
	Email       string
	Password    string
	LoginMethod LoginMethod
}

func (req *LoginUserInput) Validate() error {
	var errs ValidationErrors

	if normalizedEmail, err := ValidateEmail(req.Email, FieldEmail); err != nil {
		errs.Add(err)
	} else {
		req.Email = normalizedEmail
	}

	if req.LoginMethod == LoginMethodPassword {
		errs.Add(ValidatePassword(req.Password, FieldPassword))
	}

	if errs.HasErrors() {
		return &errs
	}
	return nil
}

// LoginUserOutput is the output for the LoginUser use-case.
type LoginUserOutput struct {
	Resources    map[string]any `json:"resources"`
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	TokenType    TokenType      `json:"token_type"`
	UserID       uuid.UUID      `json:"user_id"`
}

// RefreshAccessTokenInput is the input for the RefreshAccessToken use-case.
type RefreshAccessTokenInput struct {
	RefreshToken string
}

func (req *RefreshAccessTokenInput) Validate() error {
	if _, err := ValidateString(req.RefreshToken, StringValidationOptions{
		MinLength: 10, MaxLength: 2048,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoNullBytes: true,
		FieldName: FieldRefreshToken,
	}); err != nil {
		return err
	}
	return nil
}

type RefreshAccessTokenOutput struct {
	AccessToken  string
	RefreshToken string
	TokenType    TokenType
}

// TokenRevocation is what the denylist knows about one token.
//
// The presence of a record means the token is refused. ReplacedBy is what says
// WHY, and the two reasons call for opposite responses:
//
//   - zero — the token was revoked outright, by a logout. Presenting it is an
//     ordinary logged-out client with a stale credential. Refuse it and stop.
//   - set — the token was rotated: spent on a refresh that issued the named
//     successor. The legitimate client moved on to that successor, so a second
//     presentation of this one means somebody else has a copy. Refuse it, and
//     end the chain it belongs to.
//
// RevokedAt exists for the third case, which is neither: a client that never
// received the answer to its refresh and retried. That is indistinguishable
// from a replay except by how soon it arrives, which is why the rotation grace
// window is a duration and not a flag.
type TokenRevocation struct {
	RevokedAt  time.Time
	ExpiresAt  time.Time
	JTI        uuid.UUID
	UserID     uuid.UUID
	ReplacedBy uuid.UUID
}

// Rotated reports whether the token was spent on a refresh rather than revoked
// outright — the distinction between a replay and a logged-out client.
func (ref *TokenRevocation) Rotated() bool {
	return ref.ReplacedBy != uuid.Nil()
}

type RegisterUserInput struct {
	Disabled       *bool
	FirstName      string
	LastName       string
	Email          string
	Password       string
	RegisterMethod RegisterMethod
	ID             uuid.UUID
}

func (ref *RegisterUserInput) Validate() error {
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

	if ref.RegisterMethod == RegisterMethodPassword {
		errs.Add(ValidatePassword(ref.Password, FieldPassword))
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type RecoverPasswordInput struct {
	Email string
}

func (ref *RecoverPasswordInput) Validate() error {
	if normalizedEmail, err := ValidateEmail(ref.Email, FieldEmail); err != nil {
		return err
	} else {
		ref.Email = normalizedEmail
	}
	return nil
}

type ResetPasswordInput struct {
	Password string    `json:"password" format:"string" example:"ThisIs4Passw0rd" validate:"required"`
	UserID   uuid.UUID `json:"user_id" format:"uuid" example:"01982303-f0f9-7dd3-9b51-779cddf01211" validate:"required"`
}

func (ref *ResetPasswordInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ValidateUUID(ref.UserID, 7, FieldUserID))
	errs.Add(ValidatePassword(ref.Password, FieldPassword))

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type LogoutUserInput struct {

	// RefreshToken is the token to revoke, and it is what makes logout end a
	// session rather than merely say it did.
	//
	// Optional, because the endpoint has always been callable with only an
	// access token and making it mandatory would break every existing caller.
	// When it is absent the session cannot be ended: the access token expires
	// on its own, but the refresh token stays valid and can mint new ones for
	// the rest of its life. LogoutUser says so in the log.
	RefreshToken string
	UserID       uuid.UUID

	// AccessTokenJTI and AccessTokenExpiresAt name the access token the request
	// was AUTHORISED with — the one the middleware verified and put on the
	// context — so that logging out ends it too.
	//
	// They are read from the verified claims and never from the request body.
	// The refresh token above is supplied by the caller and therefore has to be
	// verified before it is acted on; this one has already been verified by the
	// time the handler runs, and re-reading a credential the middleware has
	// already checked is the bug /auth/refresh was fixed for.
	//
	// Zero when the caller authenticated with a personal access token, which is
	// revoked by deleting its row rather than by a denylist entry.
	AccessTokenJTI       uuid.UUID
	AccessTokenExpiresAt time.Time
}

func (ref *LogoutUserInput) Validate() error {
	var errs ValidationErrors
	errs.Add(ValidateUUID(ref.UserID, 7, FieldUserID))
	if errs.HasErrors() {
		return &errs
	}
	return nil
}

type LogoutUserOutput struct {
	Message string
}

// AES key sizes, in bytes, as fixed by FIPS 197: AES-128, AES-192 and AES-256.
// There is no other valid length, and no way to configure one.
const (
	AESKeySize128 = 16
	AESKeySize192 = 24
	AESKeySize256 = 32
)

// ValidAESKeySize reports whether n is a length crypto/aes will accept.
//
// This exists because the check that used to guard the symmetric key was a
// bound on a FILE PATH -- between 3 and 255 -- borrowed by the AES adapter as
// though it bounded the key. A 4-byte key passed it, the service started
// cleanly, and the failure arrived later as
//
//	crypto/aes: invalid key size 4
//
// on the first request that needed the key. After #371 that is a query reaching
// a third-party provider, so a truncated key presented as a broken integration
// rather than as a broken configuration.
func ValidAESKeySize(n int) bool {
	switch n {
	case AESKeySize128, AESKeySize192, AESKeySize256:
		return true
	default:
		return false
	}
}
