package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"sync"
	"time"
	"uuid"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/cache"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/notifier"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/throttle"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/token"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

type UserServiceConsumer interface {
	GetByEmail(ctx context.Context, email string) (*domain.User, error)

	// GetByEmailForAuth returns the user with the password hash intact. It is
	// uncached, and it is the only method here that returns credentials — see
	// UsersService.GetByEmailForAuth. Use GetByEmail for anything that does not
	// verify a password.
	GetByEmailForAuth(ctx context.Context, email string) (*domain.User, error)
	SelectAuthz(ctx context.Context, userID uuid.UUID) (map[string]any, error)
	UpdateByID(ctx context.Context, input *domain.UpdateUserInput) error
	Create(ctx context.Context, input *domain.CreateUserInput) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

// AuthnServiceConf represents the configuration for the auth service.
type AuthnServiceConf struct {
	UserService         UserServiceConsumer
	CacheService        cache.Cache
	LoginThrottle       throttle.Throttle
	RevokedTokens       repository.RevokedTokens
	RevokedAccessTokens RevokedAccessTokenSet
	Notifier            notifier.Notifier
	TokenSigner         token.Signer
	OT                  *o11y.OpenTelemetry
	// TokenLifetimes answers how long the tokens issued RIGHT NOW should live.
	// It is read at issuance, never captured, so a change made through
	// PUT /auth/token_lifetimes reaches the next login and the next refresh
	// without a restart. Required: there is no fallback lifetime in Go.
	TokenLifetimes TokenLifetimesProvider

	Issuer                    string
	MetricsPrefix             string
	RefreshRotationGrace      time.Duration
	UserVerificationTokenTTL  time.Duration
	UserResetPasswordTokenTTL time.Duration
	RefreshRotationEnabled    bool
}

type AuthnService struct {
	userService               UserServiceConsumer
	cacheService              cache.Cache
	loginThrottle             throttle.Throttle
	revokedTokens             repository.RevokedTokens
	revokedAccessTokens       RevokedAccessTokenSet
	notifier                  notifier.Notifier
	tokenSigner               token.Signer
	ot                        *o11y.OpenTelemetry
	metrics                   *o11y.LayerMetrics
	metricsMetadata           o11y.Metadata
	tokenLifetimes            TokenLifetimesProvider
	issuer                    string
	metricsPrefix             string
	refreshRotationGrace      time.Duration
	userVerificationTokenTTL  time.Duration
	userResetPasswordTokenTTL time.Duration
	refreshRotationEnabled    bool
}

// NewAuthnService creates a new AuthnService.
func NewAuthnService(conf AuthnServiceConf) (*AuthnService, error) {
	if conf.UserService == nil {
		return nil, &domain.InvalidUserServiceError{Message: "UserService is nil, but it is required for AuthnService"}
	}

	if conf.Notifier == nil {
		return nil, &domain.InvalidMailQueueServiceError{Message: "Notifier is nil, but it is required for AuthnService"}
	}

	if conf.TokenSigner == nil {
		return nil, &domain.InvalidTokenSignerError{Message: "TokenSigner is nil, but it is required for AuthnService"}
	}

	if len(conf.Issuer) <= domain.ValidAuthnIssuerMinLength || len(conf.Issuer) > domain.ValidAuthnIssuerMaxLength {
		return nil, &domain.InvalidIssuerError{Message: "Issuer is invalid, but it is required for AuthnService"}
	}

	if conf.TokenLifetimes == nil {
		return nil, &domain.InvalidInputError{Message: "TokenLifetimes is nil, but it is required for AuthnService; there is no fallback lifetime"}
	}

	if conf.UserVerificationTokenTTL < domain.ValidAuthnMinUserVerificationTokenTTL ||
		conf.UserVerificationTokenTTL > domain.ValidAuthnMaxUserVerificationTokenTTL {
		return nil, &domain.InvalidJWTError{Message: fmt.Sprintf("UserVerificationTokenTTL is invalid: %v", conf.UserVerificationTokenTTL)}
	}

	if conf.UserResetPasswordTokenTTL < domain.ValidAuthnMinUserResetPasswordTokenTTL ||
		conf.UserResetPasswordTokenTTL > domain.ValidAuthnMaxUserResetPasswordTokenTTL {
		return nil, &domain.InvalidJWTError{Message: fmt.Sprintf("UserResetPasswordTokenTTL is invalid: %v", conf.UserResetPasswordTokenTTL)}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for AuthnService"}
	}

	ref := &AuthnService{
		userService:               conf.UserService,
		cacheService:              conf.CacheService,
		loginThrottle:             conf.LoginThrottle,
		revokedTokens:             conf.RevokedTokens,
		revokedAccessTokens:       conf.RevokedAccessTokens,
		notifier:                  conf.Notifier,
		tokenSigner:               conf.TokenSigner,
		issuer:                    conf.Issuer,
		userVerificationTokenTTL:  conf.UserVerificationTokenTTL,
		userResetPasswordTokenTTL: conf.UserResetPasswordTokenTTL,
		tokenLifetimes:            conf.TokenLifetimes,
		refreshRotationGrace:      conf.RefreshRotationGrace,
		refreshRotationEnabled:    conf.RefreshRotationEnabled,
		ot:                        conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Authn",
			Action: "NewAuthnService",
		},
	}
	if conf.MetricsPrefix != "" {
		ref.metricsPrefix = strings.ReplaceAll(conf.MetricsPrefix, "-", "_")
		ref.metricsPrefix += "_"
	}

	callsCounter, err := ref.ot.Metrics.Meter.Int64Counter(
		fmt.Sprintf("%s%s", ref.metricsPrefix, MetricCallsCounterName),
		metric.WithDescription(fmt.Sprintf("Total number of %s calls", AppLayer)),
	)
	if err != nil {
		return nil, err
	}

	callsDuration, err := ref.ot.Metrics.Meter.Float64Histogram(
		fmt.Sprintf("%s%s", ref.metricsPrefix, MetricDurationHistogramName),
		metric.WithDescription(fmt.Sprintf("Duration of %s handler calls", AppLayer)),
		metric.WithUnit("s"), // Seconds
	)
	if err != nil {
		return nil, err
	}

	ref.metrics = &o11y.LayerMetrics{
		Counter:   callsCounter,
		Histogram: callsDuration,
	}

	return ref, nil
}

// LoginUser logs in a user.
func (ref *AuthnService) LoginUser(ctx context.Context, input *domain.LoginUserInput) (*domain.LoginUserOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "LoginUser")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.email", input.Email))

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Bound guessing against this one account, independently of where the
	// guesses come from. The per-IP limiter in the HTTP middleware does not
	// help here: spread the same guesses over enough addresses and every one of
	// them stays under its own limit.
	//
	// The key is derived from the address that was SUBMITTED, before anything is
	// looked up, so an attempt against an unknown address costs exactly what an
	// attempt against a real one costs. Throttling only real accounts would
	// answer "does this address have an account?" through the difference.
	loginKey := throttleKey(throttlePurposeLogin, input.Email)

	if ref.loginThrottle != nil {
		// This spends one unit whatever happens next; the success path below
		// hands the whole budget back, so only failures accumulate.
		if retryAfter, allowed := ref.loginThrottle.Attempt(loginKey); !allowed {
			errorValue := &domain.TooManyLoginAttemptsError{RetryAfter: retryAfter}
			return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
		}
	}

	// ForAuth because the compare below needs the password hash, and the
	// cached read deliberately does not carry one — see
	// UsersService.withoutCredentials.
	user, lookupErr := ref.userService.GetByEmailForAuth(ctx, input.Email)

	// Exactly one bcrypt compare happens on every login, whether or not an
	// account was found and whichever method was asked for. An address with no
	// account used to return before any hashing, so "no such address" answered
	// in a millisecond and "wrong password" in fifty — a timing oracle that
	// survives any amount of care taken over the response body.
	//
	// The method is caller-supplied, so the work cannot be skipped for a
	// non-password login either: that would just move the fast path somewhere an
	// attacker can still reach.
	comparisonHash := dummyPasswordHash()
	if lookupErr == nil {
		comparisonHash = user.PasswordHash
	}

	passwordMatches := ComparePasswords(comparisonHash, input.Password)

	// Every rejection below answers with the same error. The reason is recorded
	// for operators, not returned to the caller.
	switch {
	case lookupErr != nil:
		if _, notFound := errors.AsType[*domain.UserNotFoundError](lookupErr); !notFound {
			// A repository fault is not a credential problem, and reporting it
			// as one would hide an outage behind a login failure.
			return nil, o11y.RecordError(ctx, span, start, lookupErr, ref.metrics, attrs)
		}

		return nil, ref.rejectLogin(ctx, span, start, attrs, "no account for that address")

	case user.Disabled != nil && *user.Disabled:
		return nil, ref.rejectLogin(ctx, span, start, attrs, "account is disabled")

	case user.LocalAccount == nil:
		return nil, ref.rejectLogin(ctx, span, start, attrs, "local account status is unknown")

	case input.LoginMethod == domain.LoginMethodPassword && !*user.LocalAccount:
		return nil, ref.rejectLogin(ctx, span, start, attrs, "account authenticates through an identity provider")

	case input.LoginMethod == domain.LoginMethodPassword && !passwordMatches:
		return nil, ref.rejectLogin(ctx, span, start, attrs, "password does not match")
	}

	// The credentials were right: give the account its full budget back, so a
	// user who mistypes a few times and then succeeds starts clean.
	if ref.loginThrottle != nil {
		ref.loginThrottle.Succeed(loginKey)
	}

	result, err := ref.issueSession(ctx, user)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "login successful",
		attribute.String("user.id", user.ID.String()),
		attribute.String("login.method", input.LoginMethod.String()),
	)

	return result, nil
}

// LoginUserByID starts a session for an account that something else has
// already authenticated: an identity-provider callback whose (idp, subject)
// resolved to this account.
//
// It is the ONLY way an IdP sign-in reaches a session, and it takes an id, not
// an email, on purpose. LoginUser used to be called with the provider's email
// and no password, which looked the account up by that email and -- if it was
// a local one -- switched its password off. Nothing proved the email belonged
// to the account; the provider's word was enough. The resolution by subject
// happens in the IdP use case, before this is called.
func (ref *AuthnService) LoginUserByID(ctx context.Context, userID uuid.UUID, method domain.LoginMethod) (*domain.LoginUserOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "LoginUserByID")
	defer span.End()

	if method == domain.LoginMethodPassword {
		// A password login goes through LoginUser, which is where the password
		// is compared. Arriving here with that method is a caller bug.
		return nil, o11y.RecordError(ctx, span, start, &domain.InvalidInputError{Message: "LoginUserByID does not verify passwords"}, ref.metrics, attrs)
	}

	user, err := ref.userService.GetByID(ctx, userID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if user.Disabled != nil && *user.Disabled {
		return nil, ref.rejectLogin(ctx, span, start, attrs, "account is disabled")
	}

	result, err := ref.issueSession(ctx, user)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "login successful",
		attribute.String("user.id", user.ID.String()),
		attribute.String("login.method", method.String()),
	)

	return result, nil
}

// issueSession signs the access and refresh tokens for an authenticated user
// and gathers their permissions. Every login path ends here, and none of them
// changes the account on the way through: the local_account flip that an
// IdP login used to make -- silently disabling the password -- is gone, and
// a password stays until the user removes it.
func (ref *AuthnService) issueSession(ctx context.Context, user *domain.User) (*domain.LoginUserOutput, error) {
	// One read for both tokens, so a change landing between the two cannot
	// issue a pair from different settings.
	lifetimes := ref.tokenLifetimes.Current()

	accessToken, err := ref.tokenSigner.Sign(ctx, domain.JWTClaims{
		Email:         user.Email,
		Subject:       user.ID.String(),
		Issuer:        ref.issuer,
		TokenType:     domain.TokenTypeAccess,
		TokenDuration: lifetimes.AccessTokenDuration,
	})
	if err != nil {
		return nil, err
	}

	refreshToken, err := ref.tokenSigner.Sign(ctx, domain.JWTClaims{
		Email:         user.Email,
		Subject:       user.ID.String(),
		Issuer:        ref.issuer,
		TokenType:     domain.TokenTypeRefresh,
		TokenDuration: lifetimes.RefreshTokenDuration,
	})
	if err != nil {
		return nil, err
	}

	permissions, err := ref.userService.SelectAuthz(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	if permissions == nil || permissions["permissions"] == nil {
		slog.Warn("service.Authn.issueSession: user does not have any permissions", "user.id", user.ID.String())

		permissions = map[string]any{"permissions": map[string]any{}}
	}

	// remove the first level of the permissions map which is the key "permissions"
	permissionsL1, ok := permissions["permissions"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("failed to cast permissions to map")
	}

	return &domain.LoginUserOutput{
		UserID:       user.ID,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    domain.TokenTypeBearer,
		Resources:    permissionsL1,
	}, nil
}

// RegisterUser creates a new user.
func (ref *AuthnService) RegisterUser(ctx context.Context, input *domain.RegisterUserInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "RegisterUser")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	var err error
	input.ID, err = domain.EnsureUUIDV7(input.ID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	user := &domain.CreateUserInput{
		ID:        input.ID,
		FirstName: input.FirstName,
		LastName:  input.LastName,
		Email:     input.Email,
		Password:  input.Password,
	}

	if input.Disabled != nil {
		user.Disabled = input.Disabled
	}

	if input.RegisterMethod == domain.RegisterMethodPassword {
		user.LocalAccount = new(true)
	} else {
		user.LocalAccount = new(false)
	}

	if err := ref.userService.Create(ctx, user); err != nil {
		// An address that already has an account answers exactly like one that
		// does not.
		//
		// It used to answer 409 "user: already exists: email=<address>", which
		// made registration the account oracle login and password recovery were
		// both closed for. Unlike those two this one has a real cost -- somebody
		// who has simply forgotten they have an account is now told
		// "registered" and gets no verification email -- so the owner is told by
		// mail instead. That mail is the whole reason this is acceptable: the
		// person who needs to know still finds out, and the person probing
		// learns nothing.
		if _, exists := errors.AsType[*domain.UserAlreadyExistsError](err); exists {
			return ref.registrationTaken(ctx, span, start, attrs, input)
		}

		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// send a verification email only when the user is created via local account
	if input.RegisterMethod == domain.RegisterMethodPassword {

		jwtClaims := domain.JWTClaims{
			Email:         input.Email,
			Subject:       input.ID.String(),
			Issuer:        ref.issuer,
			TokenType:     domain.TokenTypeEmailVerification,
			TokenDuration: ref.userVerificationTokenTTL,
		}

		emailToken, err := ref.tokenSigner.Sign(ctx, jwtClaims)
		if err != nil {
			return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		recipient := notifier.Recipient{
			Name:  fmt.Sprintf("%s %s", input.FirstName, input.LastName),
			Email: input.Email,
		}
		if err := ref.notifier.SendAccountVerification(ctx, recipient, emailToken, ref.userVerificationTokenTTL.String()); err != nil {
			return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user created successfully")

	return nil
}

// registrationTaken answers a registration for an address that already has an
// account, and tells the owner it happened.
//
// The caller gets the same success a real registration gets. The owner gets an
// email that carries no token and no action -- anyone can cause it to be sent
// to any address, so it must not be something its recipient can be walked
// through.
//
// A failure to send is logged, not returned: the answer to the caller must not
// depend on it, or the difference becomes the oracle again.
func (ref *AuthnService) registrationTaken(
	ctx context.Context, span trace.Span, start time.Time, attrs []attribute.KeyValue, input *domain.RegisterUserInput,
) error {
	span.SetAttributes(attribute.Bool("authn.register.address_taken", true))

	if ref.notifier != nil {
		recipient := notifier.Recipient{
			Name:  fmt.Sprintf("%s %s", input.FirstName, input.LastName),
			Email: input.Email,
		}

		if err := ref.notifier.SendAccountExists(ctx, recipient); err != nil {
			slog.Error("service.Authn.RegisterUser: could not tell an existing account that its address was used to register", "error", err)
		}
	}

	slog.Debug("service.Authn.RegisterUser: the address already has an account; answered as a successful registration")
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "registration accepted")

	return nil
}

// VerifyUser verifies a user.
func (ref *AuthnService) VerifyUser(ctx context.Context, jwtToken string) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "VerifyUser")
	defer span.End()

	if jwtToken == "" {
		errorType := &domain.InvalidJWTError{Message: "JWT token is empty"}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	claims, err := ref.tokenSigner.Verify(ctx, jwtToken)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	tokenType, ok := claims["token_type"].(string)
	if !ok {
		errorValue := &domain.InvalidJWTError{Message: "token_type claim is missing"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if tokenType != domain.TokenTypeEmailVerification.String() {
		errorValue := &domain.InvalidJWTError{Message: "token_type claim is invalid"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	userID, err := uuid.Parse(claims["sub"].(string))
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	email, ok := claims["email"].(string)
	if !ok {
		errorValue := &domain.InvalidJWTError{Message: "email claim is missing"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// check the expiration time
	exp, ok := claims["exp"].(float64)
	if !ok {
		errorValue := &domain.InvalidJWTError{Message: "exp claim is missing"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if time.Now().Unix() > int64(exp) {
		errorValue := &domain.InvalidJWTError{Value: jwtToken, Message: "token expired"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// Single-use. The link is spent on its first click; a second one is
	// refused as an invalid token, not reported as "already verified", and a
	// store fault refuses rather than admits. It used to work until expiry.
	if err := ref.spendSingleUseToken(ctx, claims, userID, domain.TokenTypeEmailVerification, time.Unix(int64(exp), 0)); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	user, err := ref.userService.GetByID(ctx, userID)
	if err != nil {
		// grateful answer when user not found, because security reason
		if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
			return nil
		}

		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if user == nil {
		ErrorType := &domain.UserNotFoundError{ID: userID, Message: "User not found"}
		return o11y.RecordError(ctx, span, start, ErrorType, ref.metrics, attrs)
	}

	if email != user.Email {
		errorValue := &domain.InvalidJWTError{Message: "email claim is invalid"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if user.Disabled == nil || !*user.Disabled {
		errorType := &domain.UserAlreadyVerifiedError{Email: email}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	isDisabled := new(bool)
	*isDisabled = false

	updateInput := &domain.UpdateUserInput{
		ID:       user.ID,
		Disabled: isDisabled,
	}

	if err := ref.userService.UpdateByID(ctx, updateInput); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user verified successfully")

	return nil
}

// ReVerifyUser re-verifies a user.
func (ref *AuthnService) ReVerifyUser(ctx context.Context, email string) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ReVerifyUser")
	defer span.End()

	if email == "" {
		errorType := &domain.InvalidEmailError{Email: email, Message: "The email is empty. Please provide a valid email address."}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	if len(email) < domain.ValidUserEmailMinLength || len(email) > domain.ValidUserEmailMaxLength {
		errorType := &domain.InvalidEmailError{Email: email, Message: fmt.Sprintf("The email must be between %d and %d characters long.", domain.ValidUserEmailMinLength, domain.ValidUserEmailMaxLength)}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	_, err := mail.ParseAddress(email)
	if err != nil {
		errorType := &domain.InvalidEmailError{Email: email, Message: fmt.Sprintf("The email '%s' is not valid.", email)}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	user, err := ref.userService.GetByEmail(ctx, email)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// grateful answer when user not found, because security reason
	if user == nil {
		slog.Warn("service.Authn.ReVerifyUser: user not found", "email", email)
		return nil
	}

	// grateful answer when user is already verified, because security reason
	if user.Disabled != nil && !*user.Disabled {
		slog.Warn("service.Authn.ReVerifyUser: user already verified", "email", email)
		return nil
	}

	// if user is not a local account, do not send a verification email
	if user.LocalAccount == nil || !*user.LocalAccount {
		slog.Warn("service.Authn.ReVerifyUser: user is not a local account, cannot re-verify", "email", email)
		return nil
	}

	jwtClaims := domain.JWTClaims{
		Email:         user.Email,
		Subject:       user.ID.String(),
		Issuer:        ref.issuer,
		TokenType:     domain.TokenTypeEmailVerification,
		TokenDuration: ref.userVerificationTokenTTL,
	}

	emailToken, err := ref.tokenSigner.Sign(ctx, jwtClaims)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	recipient := notifier.Recipient{
		Name:  fmt.Sprintf("%s %s", user.FirstName, user.LastName),
		Email: user.Email,
	}
	slog.Debug("service.Authn.ReVerifyUser: enqueuing verification email", "to", user.Email)
	if err := ref.notifier.SendAccountVerification(ctx, recipient, emailToken, ref.userVerificationTokenTTL.String()); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user re-verification email sent successfully")

	return nil
}

// RefreshAccessToken refreshes an access token.
func (ref *AuthnService) RefreshAccessToken(ctx context.Context, input *domain.RefreshAccessTokenInput) (*domain.RefreshAccessTokenOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "RefreshAccessToken")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidRefreshTokenError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	rtClaims, err := ref.tokenSigner.Verify(ctx, input.RefreshToken)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// The jti claim is required for a refresh token only and difference it from an access token
	if rtClaims["jti"] == nil || rtClaims["jti"] == "" {
		errorValue := &domain.InvalidRefreshTokenError{Message: "jti claim is missing"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	tokenID, err := uuid.Parse(rtClaims["jti"].(string))
	if err != nil {
		errorValue := &domain.InvalidRefreshTokenError{Message: "jti claim is not a uuid"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// The denylist. A signature that verifies only says the token was issued by
	// this service, not that it is still meant to work — a logout puts a jti
	// here, and so does spending one on a refresh.
	//
	// A failure to reach the store is fatal, never "not revoked". Treating an
	// unreachable denylist as an empty one would mean a database blip silently
	// re-validates every token anyone has logged out of, which is the one
	// question in the service that must fail closed.
	//
	// reissueJTI carries the one non-fatal answer out of that check: a client
	// that never received the successor this token already issued, and is
	// asking again. It gets that successor rather than a new link in the chain.
	reissueJTI := uuid.Nil()

	if ref.revokedTokens != nil {
		record, err := ref.revokedTokens.Get(ctx, tokenID)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		if record != nil {
			reissueJTI, err = ref.resolveSpentRefreshToken(ctx, record)
			if err != nil {
				return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			}
		}
	}

	userID, err := uuid.Parse(rtClaims["sub"].(string))
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	slog.Debug("service.Authn.RefreshAccessToken", "userID", userID)

	// Presence only. A refresh token carries an email claim and an access token
	// does not, so this is part of telling the two apart — but the value is not
	// used to build the new token; the account record below is the authority.
	claimEmail, ok := rtClaims["email"].(string)
	if !ok {
		errorValue := &domain.InvalidRefreshTokenError{Message: "email claim is missing"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}
	slog.Debug("service.Authn.RefreshAccessToken", "claimEmail", claimEmail)

	tokenType, ok := rtClaims["token_type"].(string)
	if !ok {
		errorValue := &domain.InvalidRefreshTokenError{Message: "token_type claim is missing"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if tokenType != domain.TokenTypeRefresh.String() {
		errorValue := &domain.InvalidRefreshTokenError{Message: "token_type is not a refresh token"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// Re-read the user rather than trusting the claims.
	//
	// A refresh token is a bearer credential minted at login and valid for far
	// longer than the access tokens it produces. Rebuilding an identity from its
	// claims alone meant nothing that happened to the account in between could
	// take effect: disabling a user stopped them logging in but not refreshing,
	// so the holder kept minting fresh access tokens for the whole refresh
	// lifetime. Deleting one behaved the same way until their roles were gone.
	//
	// This is a cached read on the same entry CheckAuthz already warms, so the
	// hot path is a cache hit, not a query.
	user, err := ref.userService.GetByID(ctx, userID)
	if err != nil {
		if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
			errorValue := &domain.InvalidRefreshTokenError{Message: "the account this token was issued for no longer exists"}
			return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
		}

		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if user.Disabled != nil && *user.Disabled {
		errorValue := &domain.InvalidRefreshTokenError{Message: "the account this token was issued for is disabled"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// The email is taken from the record, not the token, so a change made since
	// the refresh token was issued reaches the new access token.
	atJWTClaims := domain.JWTClaims{
		Email:         user.Email,
		Subject:       user.ID.String(),
		Issuer:        ref.issuer,
		TokenType:     domain.TokenTypeAccess,
		TokenDuration: ref.tokenLifetimes.Current().AccessTokenDuration,
	}

	accessTokenSigned, err := ref.tokenSigner.Sign(ctx, atJWTClaims)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	refreshTokenSigned, err := ref.rotateRefreshToken(ctx, user, rtClaims, input.RefreshToken, tokenID, reissueJTI)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "access token refreshed successfully")

	return &domain.RefreshAccessTokenOutput{
		AccessToken:  accessTokenSigned,
		RefreshToken: refreshTokenSigned,
		TokenType:    domain.TokenTypeBearer,
	}, nil
}

// resolveSpentRefreshToken decides what it means that a refresh token already
// has a denylist record, and returns the jti to re-issue when the answer is
// "this is a retry, not a replay".
//
// Note that every rejection here says the same thing the ordinary revoked case
// says. A caller learns that their token no longer works, never that the
// service noticed it was replayed — which would tell whoever stole it exactly
// how much the service knows.
func (ref *AuthnService) resolveSpentRefreshToken(ctx context.Context, record *domain.TokenRevocation) (uuid.UUID, error) {
	// Revoked outright, by a logout. Nothing replaced it, so there is no chain
	// and nothing to detect: this is a client holding a credential it was told
	// to stop using.
	if !record.Rotated() {
		return uuid.Nil(), &domain.InvalidRefreshTokenError{Message: "this token has been revoked"}
	}

	// Rotated — and the answer carrying its successor may simply never have
	// arrived. A dropped response, a client that died before storing the new
	// token, two requests refreshing at once: all of them present a token that
	// was already spent, and none of them is a theft. Inside the grace window
	// the retry is far more likely than the attack, and treating it as the
	// attack would end sessions over a lost packet — which would also make the
	// alarm below worthless, because it would fire all day.
	if age := time.Since(record.RevokedAt); age <= ref.refreshRotationGrace {
		// Only when the successor is still live. It may not be: the chain can
		// have moved on already, or a logout can have ended it in the seconds
		// since. Handing back a token that is itself revoked would report a
		// success that delivers a dead credential, which is the bug logout was
		// fixed for in the same breath as this one.
		successor, err := ref.revokedTokens.Get(ctx, record.ReplacedBy)
		if err != nil {
			return uuid.Nil(), err
		}

		if successor == nil {
			slog.Info("service.Authn.RefreshAccessToken: re-issuing the successor of an already-rotated refresh token",
				"userID", record.UserID, "rotatedAgo", age)

			return record.ReplacedBy, nil
		}

		// Deliberately not treated as a replay. A client refreshing twice in
		// quick succession lands here, and ending its session over that would
		// be the false alarm the grace window exists to avoid.
		slog.Info("service.Authn.RefreshAccessToken: refusing an already-rotated refresh token whose successor is no longer live",
			"userID", record.UserID, "rotatedAgo", age)

		return uuid.Nil(), &domain.InvalidRefreshTokenError{Message: "this token has been revoked"}
	}

	// Outside the window, two parties hold a token only one of them should
	// have: the legitimate client moved on to the successor long ago, so this
	// presentation is a copy. Nothing in the request says which party is
	// which, so the chain ends for both — the alternative is leaving the thief
	// with a working session.
	tip, err := ref.revokedTokens.RevokeChain(ctx, record.JTI, record.UserID, record.ExpiresAt)
	if err != nil {
		return uuid.Nil(), err
	}

	slog.Warn("service.Authn.RefreshAccessToken: a refresh token was replayed after it had been rotated; the session has been ended",
		"userID", record.UserID, "jti", record.JTI, "chainTip", tip, "rotatedAgo", time.Since(record.RevokedAt))

	return uuid.Nil(), &domain.InvalidRefreshTokenError{Message: "this token has been revoked"}
}

// rotateRefreshToken issues the refresh token that replaces the one just spent,
// and records the link between them.
//
// # Why the expiry is carried over rather than renewed
//
// Every token in a chain expires when the first one would have. Renewing the
// expiry on each refresh would make any session immortal as long as it stayed
// active, which is a product decision about how long people stay logged in —
// not something rotation should change on its way past.
//
// # Why it signs before it records
//
// A failure between the two has to leave the old token working. Recording the
// rotation first and then failing to sign would revoke the credential the
// caller still holds and hand back nothing in its place: a locked-out client
// with no way to recover but to log in again. This way a failure means an
// error and a retry that works.
func (ref *AuthnService) rotateRefreshToken(
	ctx context.Context, user *domain.User, rtClaims map[string]any, presented string, spentJTI, reissueJTI uuid.UUID,
) (string, error) {
	// Rotation without revocation is strictly worse than no rotation: it hands
	// out a second usable credential and retires nothing. Without the store
	// there is nowhere to record that the old token is spent, so the old token
	// is what the caller keeps.
	if !ref.refreshRotationEnabled || ref.revokedTokens == nil {
		return presented, nil
	}

	expiresAt, ok := claimExpiry(rtClaims)
	if !ok {
		return "", &domain.InvalidRefreshTokenError{Message: "exp claim is missing"}
	}

	newJTI := reissueJTI
	if newJTI == uuid.Nil() {
		newJTI = uuid.NewV7()
	}

	signed, err := ref.tokenSigner.Sign(ctx, domain.JWTClaims{
		Email:         user.Email,
		Subject:       user.ID.String(),
		Issuer:        ref.issuer,
		TokenType:     domain.TokenTypeRefresh,
		TokenDuration: time.Until(expiresAt),
		TokenID:       newJTI,
	})
	if err != nil {
		return "", err
	}

	// A re-issue is not a rotation: the link it names was written by the call
	// whose answer went missing, and writing it again would either be a no-op
	// or, if it were an upsert, lose the successor that call handed out.
	if reissueJTI == uuid.Nil() {
		if err := ref.revokedTokens.Rotate(ctx, spentJTI, newJTI, user.ID, expiresAt); err != nil {
			return "", err
		}
	}

	return signed, nil
}

// claimExpiry reads the exp claim as a time. It reports false when the claim is
// absent or not a number, so the caller can refuse rather than invent a deadline.
func claimExpiry(claims map[string]any) (time.Time, bool) {
	exp, ok := claims["exp"].(float64)
	if !ok {
		return time.Time{}, false
	}

	return time.Unix(int64(exp), 0), true
}

func (ref *AuthnService) RecoverPassword(ctx context.Context, input *domain.RecoverPasswordInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "RecoverPassword")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Bound how often one address can be asked about, before anything is looked
	// up.
	//
	// Recovery was reachable at the per-IP limiter's rate and no slower, which
	// left two things open: enumeration at speed, and a way to send a great deal
	// of mail to somebody else's address. The budget is keyed on the SUBMITTED
	// address, so an address with no account is throttled exactly like a real
	// one -- a throttle that only bit real accounts would be the oracle it
	// exists to make expensive.
	//
	// Every request spends and nothing refunds. Unlike a login there is no
	// "success" that proves the caller had any business asking, so the budget is
	// simply how many recovery emails one address can provoke in a window.
	if ref.loginThrottle != nil {
		if retryAfter, allowed := ref.loginThrottle.Attempt(throttleKey(throttlePurposeRecovery, input.Email)); !allowed {
			errorValue := &domain.TooManyRecoveryRequestsError{RetryAfter: retryAfter}
			return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
		}
	}

	// Password recovery answers the same way whatever it finds.
	//
	// It used to answer three different ways, which made it a better account
	// oracle than login ever was -- no password to guess, and no throttle.
	// Measured against the running API:
	//
	//	no such address      -> 500, echoing the probed address back
	//	a local account      -> 200 "Password recovery email sent"
	//	an IdP-backed account-> 500 "user is not a local account"
	//
	// So one unauthenticated request told a caller whether an address had an
	// account AND whether that account signs in through an identity provider.
	// This is the same rule login already follows -- see rejectLogin -- applied
	// to the endpoint it was missing from.
	//
	// The reason still reaches the span and the log, so an operator can tell a
	// typo from an SSO account; the caller cannot.
	user, err := ref.userService.GetByEmail(ctx, input.Email)
	if err != nil {
		// Not-found is an answer, not a fault. It used to escape as a 500 with
		// the address in the message.
		if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
			return ref.silentRecovery(ctx, span, start, attrs, "no account for this address")
		}

		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if user == nil {
		return ref.silentRecovery(ctx, span, start, attrs, "no account for this address")
	}

	if user.Disabled != nil && *user.Disabled {
		return ref.silentRecovery(ctx, span, start, attrs, "the account is disabled")
	}

	// Only a local account has a password to recover. Saying so out loud
	// identifies which addresses use SSO, which is exactly what an attacker
	// choosing a target wants to know.
	if user.LocalAccount == nil || !*user.LocalAccount {
		return ref.silentRecovery(ctx, span, start, attrs, "the account authenticates through an identity provider")
	}

	jwtClaims := domain.JWTClaims{
		Email:         user.Email,
		Subject:       user.ID.String(),
		Issuer:        ref.issuer,
		TokenType:     domain.TokenTypePasswordReset,
		TokenDuration: ref.userResetPasswordTokenTTL,
	}

	emailToken, err := ref.tokenSigner.Sign(ctx, jwtClaims)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	recipient := notifier.Recipient{
		Name:  fmt.Sprintf("%s %s", user.FirstName, user.LastName),
		Email: user.Email,
	}
	if err := ref.notifier.SendPasswordReset(ctx, recipient, emailToken, ref.userResetPasswordTokenTTL.String()); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "password recovery email sent")

	return nil
}

func (ref *AuthnService) ResetPassword(ctx context.Context, input *domain.ResetPasswordInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ResetPassword")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Single-use, and spent BEFORE the password changes: a token that was
	// consumed and then failed to apply costs one more link; a password that
	// changed and then failed to record the token is a link that still works.
	if err := ref.consumeSingleUseToken(ctx, input.TokenID, input.UserID, domain.TokenTypePasswordReset, input.TokenExpiresAt); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// update user password
	uuInput := &domain.UpdateUserInput{
		ID: input.UserID,
	}

	if len(input.Password) < domain.ValidUserPasswordMinLength || len(input.Password) > domain.ValidUserPasswordMaxLength {
		errorType := &domain.InvalidPasswordError{Message: fmt.Sprintf("The password must be between %d and %d characters long.", domain.ValidUserPasswordMinLength, domain.ValidUserPasswordMaxLength)}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	hashPwd, err := HashAndSaltPassword(input.Password)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	uuInput.PasswordHash = &hashPwd

	if err := ref.userService.UpdateByID(ctx, uuInput); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "password reset successfully")

	return nil
}

func (ref *AuthnService) LogoutUser(ctx context.Context, input *domain.LogoutUserInput) (*domain.LogoutUserOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "LogoutUser")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Revoke the refresh token. This is what makes logout end a session: the
	// access token expires on its own within its (short) lifetime, but the
	// refresh token is the one that can mint new ones for days.
	//
	// Verified rather than trusted. The caller presents it over an
	// access-token-authenticated request, so it is not attacker-supplied in the
	// usual sense, but revoking whatever arrives would let a caller revoke a
	// token belonging to somebody else — a denial of service against another
	// account, from an ordinary logged-in user.
	if ref.revokedTokens != nil && input.RefreshToken != "" {
		if err := ref.revokeRefreshToken(ctx, input.UserID, input.RefreshToken); err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	// Revoke the access token this request was authorised with.
	//
	// Without this, logout ends the ability to EXTEND a session but not the
	// session itself: the refresh token is dead, and the access token in the
	// caller's hand keeps authenticating until it expires on its own. Measured
	// against the running API before this existed, a logged-out access token
	// answered GET /me with the full profile.
	//
	// The store first, then the local set. The store is what the other replicas
	// will see on their next reload and what survives a restart here; the local
	// add is what makes it effective on this replica before this response is
	// written. Adding locally first and then failing to record would leave a
	// revocation that exists on exactly one replica until it restarts.
	if ref.revokedTokens != nil && input.AccessTokenJTI != uuid.Nil() {
		if err := ref.revokedTokens.Revoke(ctx, input.AccessTokenJTI, input.UserID, domain.TokenTypeAccess, input.AccessTokenExpiresAt); err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		if ref.revokedAccessTokens != nil {
			ref.revokedAccessTokens.Add(input.AccessTokenJTI, input.AccessTokenExpiresAt)
		}
	}

	if ref.revokedTokens != nil && input.RefreshToken == "" {
		// Not an error: the endpoint has always accepted a bare access token,
		// and refusing now would break callers. But the session does not end,
		// so it should not pass silently either.
		slog.Warn(
			"service.Authn.LogoutUser: no refresh token supplied, so the session was not ended",
			"userID", input.UserID,
			"effect", "the access token presented has been revoked, but the refresh token stays valid until it expires and can still mint new ones",
		)
	}

	if ref.cacheService != nil {
		slog.Debug("service.Authn.LogoutUser: invalidating refresh token in cache", "userID", input.UserID)

		cacheKeys := []cache.Identifier{
			{
				Type: "user",
				ID:   input.UserID.String(),
			},
			{
				Type: "authz",
				ID:   input.UserID.String(),
			},
		}

		for _, cacheKey := range cacheKeys {
			slog.Debug("service.Authn.LogoutUser: invalidating cache", "type", cacheKey.Type, "id", cacheKey.ID)

			if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
				slog.Warn("service.Authn.LogoutUser: failed to invalidate cache", "type", cacheKey.Type, "id", cacheKey.ID, "error", err)
			}
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.AuthnUserLoggedOutSuccessfully)

	return &domain.LogoutUserOutput{
		Message: domain.AuthnUserLoggedOutSuccessfully,
	}, nil
}

// Throttle purposes. They keep login and password recovery in separate buckets
// of the same throttle; see [throttleKey].
const (
	throttlePurposeLogin    = "login"
	throttlePurposeRecovery = "recovery"
)

// throttleKey derives a per-account throttle key from a submitted email address.
//
// Normalised so that "User@Example.com " and "user@example.com" share one
// budget rather than two — otherwise changing the case of a letter would buy a
// fresh set of attempts.
//
// Hashed for two reasons. It bounds the key to 32 bytes whatever was submitted,
// and it keeps addresses out of a structure that is not meant to hold them, so
// nothing downstream — a log line, a metric label, a panic dump — can leak one
// by accident.
//
// purpose separates the budgets. Login and password recovery share one throttle
// instance and one set of tuning, but must not share a bucket: spending a
// recovery budget would otherwise lock the same address out of signing in,
// which turns a mild abuse control into a denial of service anyone can trigger
// against any address they know.
func throttleKey(purpose, email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	sum := sha256.Sum256([]byte(purpose + ":" + normalized))

	return hex.EncodeToString(sum[:])
}

// rejectLogin answers a failed login with the one opaque error, after recording
// what actually happened where an operator can see it.
//
// The reason goes on the span and into the log; it never reaches the caller.
// That split is the whole point: an operator debugging a support ticket needs to
// know a typo from a disabled account, and an attacker enumerating addresses
// must not be able to tell those apart.
func (ref *AuthnService) rejectLogin(
	ctx context.Context,
	span trace.Span,
	start time.Time,
	attrs []attribute.KeyValue,
	reason string,
) error {
	span.SetAttributes(attribute.String("authn.login.failure_reason", reason))
	slog.Info("service.Authn.LoginUser: login rejected", "reason", reason)

	return o11y.RecordError(ctx, span, start, &domain.InvalidCredentialsError{}, ref.metrics, attrs)
}

// dummyPasswordHash is a valid bcrypt hash of a value nobody knows, used as the
// comparison input when no account was found so that the compare costs what a
// real one costs.
//
// Generated once at first use rather than written into the source, so there is
// no constant in the repository that reads like a leaked credential. It is
// derived from a fresh UUID, so it matches no password anyone could supply.
var dummyPasswordHash = sync.OnceValue(func() string {
	hash, err := HashAndSaltPassword(uuid.NewV7().String())
	if err != nil {
		// Cannot happen with the default cost. If it somehow does, an empty
		// hash makes the compare fail immediately, which costs the timing
		// equalisation but never grants a login.
		slog.Error("service.Authn: could not build the dummy password hash; login timing will differ for unknown addresses", "error", err)

		return ""
	}

	return hash
})

// revokeRefreshToken verifies a token really is this user's refresh token and
// records its jti on the denylist until it would have expired anyway.
//
// Every check here exists to stop one caller revoking another caller's token:
// the signature proves we issued it, token_type proves it is a refresh token and
// not something else with a jti, and sub proves it is theirs.
func (ref *AuthnService) revokeRefreshToken(ctx context.Context, userID uuid.UUID, refreshToken string) error {
	claims, err := ref.tokenSigner.Verify(ctx, refreshToken)
	if err != nil {
		// An EXPIRED token is genuinely nothing to revoke: the session it named
		// is already over, so reporting success is accurate.
		if invalid, ok := errors.AsType[*domain.InvalidJWTError](err); ok && invalid.Expired {
			slog.Info("service.Authn.LogoutUser: the supplied refresh token had already expired, nothing to revoke")

			return nil
		}

		// Anything else means we could not read what we were asked to revoke —
		// malformed, truncated, signed by something else. The caller's real
		// session is still live, and answering 200 here would repeat the exact
		// bug this endpoint was fixed for: reporting that a session ended when
		// it did not.
		slog.Warn("service.Authn.LogoutUser: the supplied refresh token could not be verified, so nothing was revoked", "error", err)

		return &domain.InvalidRefreshTokenError{Message: "the token supplied to logout could not be verified, so the session was not ended"}
	}

	tokenType, _ := claims["token_type"].(string)
	if tokenType != domain.TokenTypeRefresh.String() {
		return &domain.InvalidRefreshTokenError{Message: "the token supplied to logout is not a refresh token"}
	}

	subject, _ := claims["sub"].(string)
	if subject != userID.String() {
		return &domain.InvalidRefreshTokenError{Message: "the token supplied to logout belongs to a different account"}
	}

	jti, err := uuid.Parse(claimString(claims, "jti"))
	if err != nil {
		return &domain.InvalidRefreshTokenError{Message: "jti claim is not a uuid"}
	}

	// exp bounds how long the row is worth keeping: past it the token is
	// refused for being expired, so the revocation is no longer load-bearing.
	// The verifier requires exp, so the fallback below is unreachable; it is
	// the current refresh lifetime only so that a row is never kept for less
	// than the token could have lived.
	expiresAt := time.Now().Add(ref.tokenLifetimes.Current().RefreshTokenDuration)
	if exp, ok := claimExpiry(claims); ok {
		expiresAt = exp
	}

	if err := ref.revokedTokens.Revoke(ctx, jti, userID, domain.TokenTypeRefresh, expiresAt); err != nil {
		return err
	}

	// Rotation means the token a client hands to logout is not necessarily the
	// live one. A client that refreshed and then logged out with the token it
	// started the session with would otherwise revoke a link that was already
	// spent — a no-op — and answer 200 while the session carried on. Following
	// the chain ends the session the caller actually asked to end.
	//
	// For a token that has not been rotated there is no successor and this
	// finds nothing, which is the ordinary case and costs one indexed read.
	tip, err := ref.revokedTokens.RevokeChain(ctx, jti, userID, expiresAt)
	if err != nil {
		return err
	}

	if tip != uuid.Nil() {
		slog.Info("service.Authn.LogoutUser: the token supplied to logout had already been rotated; ended the session at the live end of its chain",
			"userID", userID, "chainTip", tip)
	}

	return nil
}

// silentRecovery ends a password-recovery request that will send no email,
// reporting success to the caller and the real reason to the operator.
//
// The caller gets the same answer every recovery request gets, because the
// difference between "no such address", "disabled", and "signs in through an
// identity provider" is precisely what an attacker is asking for. The reason
// goes on the span and into a DEBUG log, so support can still tell a typo from
// an SSO account.
//
// The probed address is deliberately NOT logged at WARN. It used to be, on
// every miss, which turns an unauthenticated endpoint into a way to write
// attacker-chosen text into the log at will.
func (ref *AuthnService) silentRecovery(
	ctx context.Context, span trace.Span, start time.Time, attrs []attribute.KeyValue, reason string,
) error {
	span.SetAttributes(attribute.String("authn.recovery.no_email_reason", reason))
	slog.Debug("service.Authn.RecoverPassword: no recovery email sent", "reason", reason)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "password recovery request accepted")

	return nil
}

// claimString reads a string claim, returning "" when it is absent or of
// another type, so callers can validate the value rather than the shape.
func claimString(claims map[string]any, name string) string {
	value, _ := claims[name].(string)

	return value
}

// spendSingleUseToken reads the jti out of verified claims and consumes it.
func (ref *AuthnService) spendSingleUseToken(ctx context.Context, claims map[string]any, userID uuid.UUID, tokenType domain.TokenType, expiresAt time.Time) error {
	jtiStr, _ := claims["jti"].(string)

	jti, err := uuid.Parse(jtiStr)
	if err != nil {
		return &domain.InvalidJWTError{Message: "jti claim is missing"}
	}

	return ref.consumeSingleUseToken(ctx, jti, userID, tokenType, expiresAt)
}

// consumeSingleUseToken records a verification or reset token as spent and
// refuses it when it already was. The revoked-token store is the same one
// logout and rotation use, keyed by token type; every lookup there carries
// expires_at > NOW(), so the row is dead weight once the token would have
// expired anyway and the sweeper removes it.
//
// Fail closed: with no store, or a store that cannot answer, the token is
// refused. A single-use token that cannot be recorded as used is one that
// would work twice.
func (ref *AuthnService) consumeSingleUseToken(ctx context.Context, jti, userID uuid.UUID, tokenType domain.TokenType, expiresAt time.Time) error {
	if ref.revokedTokens == nil {
		return &domain.InvalidAuthnServiceError{Message: "single-use tokens need a revoked-token store"}
	}

	if jti == uuid.Nil() {
		return &domain.InvalidJWTError{Message: "jti claim is missing"}
	}

	firstUse, err := ref.revokedTokens.Consume(ctx, jti, userID, tokenType, expiresAt)
	if err != nil {
		return err
	}

	if !firstUse {
		slog.Warn("service.Authn: single-use token presented again",
			"token_type", tokenType.String(), "jti", jti.String(), "user.id", userID.String())

		return &domain.InvalidJWTError{Message: "token already used"}
	}

	return nil
}
