package handler

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"uuid"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driving"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// AuthnHandlerConf is the configuration struct for the AuthnHandler.
type AuthnHandlerConf struct {
	Service       driving.Authn
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// AuthnHandler is the handler that will handle the authentication of users.
type AuthnHandler struct {
	service         driving.Authn
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewAuthnHandler creates a new AuthnHandler.
func NewAuthnHandler(conf AuthnHandlerConf) (*AuthnHandler, error) {
	if conf.Service == nil {
		return nil, &domain.InvalidServiceError{Message: "AuthnService is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &AuthnHandler{
		service:       conf.Service,
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Authn",
			Action: "NewAuthnHandler", // each method names its own action when it calls o11y.SetupTrace*
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
		metric.WithDescription(fmt.Sprintf("Duration of %s calls", AppLayer)),
		metric.WithUnit("s"), // Seconds
		// OTel usually has default buckets, but you can define custom explicit buckets here if needed
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

// RegisterRoutes registers the routes for the AuthnHandler.
func (ref *AuthnHandler) RegisterRoutes(mux *http.ServeMux, accessTokenMiddleware, logoutMiddleware, refreshTokenMiddleware, passwordResetTokenMiddleware, verificationTokenMiddleware middleware.Middleware) {
	// logoutMiddleware, not accessTokenMiddleware: the endpoint whose job is to
	// revoke a token must not itself be gated on that token not being revoked.
	//
	// Otherwise a second logout with the same access token is refused 401 — and
	// two tabs logging out at once is ordinary, not an error. Everything else
	// about the chain is identical; only the revocation check is dropped, and
	// letting an already-revoked token reach here costs nothing because logout
	// is idempotent and can only act on the caller's own tokens.
	mux.Handle("DELETE /auth/logout", logoutMiddleware.ThenFunc(ref.logout))
	mux.Handle("POST /auth/refresh", refreshTokenMiddleware.ThenFunc(ref.refreshAccessToken))

	mux.HandleFunc("POST /auth/login", ref.loginUser)
	mux.HandleFunc("POST /auth/register", ref.registerUser)
	mux.Handle("POST /auth/verify/confirm", verificationTokenMiddleware.ThenFunc(ref.confirmVerification))
	mux.HandleFunc("POST /auth/verify", ref.reVerifyUser)

	mux.HandleFunc("POST /auth/password/recover", ref.recoverPassword)
	mux.Handle("POST /auth/password/reset", passwordResetTokenMiddleware.ThenFunc(ref.resetPassword))
}

// loginUser handles login requests for users locally.
//
//	@Id				019822af-b448-755b-92ff-d167d37719c2
//	@Summary		Authenticate user
//	@Description	Authenticate with email and password to obtain access and refresh tokens.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			body	body		payload.LoginUserRequest	true	"Email and password credentials"
//	@Success		200		{object}	payload.LoginUserResponse	"Authentication successful - returns access and refresh tokens"
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid request body or missing required fields"
//	@Failure		401		{object}	payload.HTTPMessage			"Invalid email or password. The same answer is given for an unknown address, a wrong password, and a disabled account"
//	@Failure		413		{object}	payload.HTTPMessage			"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage			"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage			"Too many failed login attempts for this account; see Retry-After"
//	@Header			429		{integer}	Retry-After					"Seconds until an attempt is possible again"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error during authentication"
//	@Router			/auth/login [post]
func (ref *AuthnHandler) loginUser(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "loginUser")
	defer span.End()

	var req payload.LoginUserRequest
	if err := decodeJSONBody(r, &req); err != nil {
		errorType := &domain.InvalidRequestError{Message: "failed to decode request"}
		e := o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
		respond.WriteDecodeError(w, r, e)
		return
	}

	if err := req.Validate(); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	input := &domain.LoginUserInput{
		Email:       req.Email,
		Password:    req.Password,
		LoginMethod: domain.LoginMethodPassword,
	}

	out, err := ref.service.LoginUser(ctx, input)
	if err != nil {
		// The account's login budget is spent. Retry-After carries when it will
		// have refilled; the body says nothing about the account, because a
		// throttle that behaves differently for a real address than an invented
		// one is the enumeration oracle it exists to make expensive.
		if throttled, ok := errors.AsType[*domain.TooManyLoginAttemptsError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(throttled.RetryAfter)))
			respond.WriteJSONMessage(w, r, http.StatusTooManyRequests, e.Error())
			return
		}

		_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
		_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
		_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
		_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
		if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		// Every credential failure answers alike, and the service has already
		// put the real reason on the span and in the log. Matching the type
		// explicitly rather than letting it fall through matters: the fall-
		// through used to answer 401 for ANY error, so a repository fault or a
		// validation problem was reported to the caller as bad credentials —
		// which hides an outage behind a login failure and tells a user their
		// password is wrong when it is not.
		if _, ok := errors.AsType[*domain.InvalidCredentialsError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusUnauthorized, e.Error())
			return
		}

		_, isInvalidInput := errors.AsType[*domain.InvalidInputError](err)
		_, isValidation := errors.AsType[*domain.ValidationErrors](err)
		if isInvalidInput || isValidation {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	// See handler.Me.authz for why an unrecognised shape is logged rather than
	// failed. It matters more here: refusing a login over a presentation
	// concern would lock everyone out, while an empty set only means the client
	// offers no controls until the next read succeeds.
	typedResources, err := payload.NewAuthzPermissions(out.Resources)
	if err != nil {
		slog.Error("handler.Authn.login: unrecognised permission shape, sending an empty set",
			"user.id", out.UserID.String(), "error", err)
	}

	outResponse := payload.LoginUserResponse{
		UserID:       out.UserID,
		AccessToken:  out.AccessToken,
		RefreshToken: out.RefreshToken,
		TokenType:    out.TokenType,
		Resources:    typedResources,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user logged in",
		attribute.String("user.id", outResponse.UserID.String()),
	)
}

// registerUser Register a new user and send a confirmation email.
//
//	@Id				019822af-b448-7572-a268-4c7b20a70229
//	@Summary		Register new user
//	@Description	Create a new user account and send verification email.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			body	body		payload.RegisterUserRequest	true	"User registration details including email, password, and profile"
//	@Success		201		{object}	payload.HTTPMessage			"Registration accepted. Answered the same way whether or not the address already has an account — deliberately, so this endpoint cannot be used to discover which addresses are registered. If the address was already taken, its owner is told by email instead and no second account is created"
//	@Header			201		{string}	Location					"/users/{id}"	"URI of the created user resource"
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid request body or validation error"
//	@Failure		413		{object}	payload.HTTPMessage			"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage			"Body not declared as application/json"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error during registration"
//	@Router			/auth/register [post]
func (ref *AuthnHandler) registerUser(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "registerUser")
	defer span.End()

	var req payload.RegisterUserRequest
	if err := decodeJSONBody(r, &req); err != nil {
		errorType := &domain.InvalidRequestError{Message: "failed to decode request"}
		e := o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
		respond.WriteDecodeError(w, r, e)
		return
	}

	if req.ID == uuid.Nil() {
		req.ID = uuid.NewV7()
	}

	if err := req.Validate(); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	input := &domain.RegisterUserInput{
		ID:             req.ID,
		FirstName:      req.FirstName,
		LastName:       req.LastName,
		Email:          req.Email,
		Password:       req.Password,
		Disabled:       new(true), // users are disabled by default until they verify their email
		RegisterMethod: domain.RegisterMethodPassword,
	}

	if err := ref.service.RegisterUser(ctx, input); err != nil {
		_, isInvalidMail := errors.AsType[*domain.InvalidEmailError](err)
		_, isInvalidPassword := errors.AsType[*domain.InvalidPasswordError](err)
		if isInvalidMail || isInvalidPassword {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
		_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
		_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
		_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
		if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user registered",
		attribute.String("user.id", input.ID.String()),
	)

	respond.WriteJSONMessage(w, r, http.StatusCreated, domain.AuthnUserRegisteredSuccessfully)
}

// confirmVerification Verify a local user account.
//
//	@Id				01a02dbb-bc41-7287-9cfd-7ac08bf882ae
//	@Summary		Confirm email verification
//	@Description	Activate a user account with the verification token from the email. The token travels in the Authorization header, never in the URL.
//	@Tags			Authentication
//	@Produce		json
//	@Success		200	{object}	payload.HTTPMessage	"Email verified - account activated"
//	@Failure		400	{object}	payload.HTTPMessage	"Invalid or malformed token format"
//	@Failure		401	{object}	payload.HTTPMessage	"Token missing, expired, or not an email verification token"
//	@Failure		404	{object}	payload.HTTPMessage	"User not found"
//	@Failure		409	{object}	payload.HTTPMessage	"Account already verified"
//	@Failure		413	{object}	payload.HTTPMessage	"Request body larger than http.server.max.body.bytes"
//	@Failure		415	{object}	payload.HTTPMessage	"Body not declared as application/json"
//	@Failure		429	{object}	payload.HTTPMessage	"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500	{object}	payload.HTTPMessage	"Internal server error during verification"
//	@Router			/auth/verify/confirm [post]
//	@Security		VerificationToken
//
// The token arrives in the Authorization header and is verified by
// CheckVerificationToken before this runs. It used to arrive as a path segment
// on GET /auth/verify/{token}, which put a live credential into this service's
// own request log — measured, twice per request, as `url=` and `path=` — as
// well as into the browser history and Referer of whoever clicked the link.
func (ref *AuthnHandler) confirmVerification(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "confirmVerification")
	defer span.End()

	token, err := getTokenFromContext(ctx)
	if err != nil {
		_ = o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "Invalid or expired token")
		return
	}

	if err := ref.service.VerifyUser(ctx, token); err != nil {
		if _, ok := errors.AsType[*domain.InvalidJWTError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusUnauthorized, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}

		// A second click on the same link. It used to fall through to the
		// 500 branch, whose body then carried the domain's message; now that
		// a 500 says nothing, the case needs the status it always deserved.
		if _, ok := errors.AsType[*domain.UserAlreadyVerifiedError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
			return
		}

		_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
		_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
		_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
		_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
		if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user verified")

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.AuthnUserVerifiedSuccessfully)
}

// reVerifyUser Re-verify a user using the JWT token.
//
//	@Id				019822af-b448-7576-8a41-41b83b3239f0
//	@Summary		Resend verification email
//	@Description	Request a new verification email for unverified account.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			body	body		payload.ReVerifyUserRequest	true	"Email address for verification"
//	@Success		200		{object}	payload.HTTPMessage			"Verification email sent if account exists"
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid request body or email format"
//	@Failure		401		{object}	payload.HTTPMessage			"Invalid or expired token"
//	@Failure		413		{object}	payload.HTTPMessage			"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage			"Body not declared as application/json"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error during email send"
//	@Router			/auth/verify [post]
func (ref *AuthnHandler) reVerifyUser(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "reVerifyUser")
	defer span.End()

	var req payload.ReVerifyUserRequest
	if err := decodeJSONBody(r, &req); err != nil {
		errorType := &domain.InvalidRequestError{Message: "failed to decode request"}
		e := o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
		respond.WriteDecodeError(w, r, e)
		return
	}

	if err := req.Validate(); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	if err := ref.service.ReVerifyUser(ctx, req.Email); err != nil {
		if _, ok := errors.AsType[*domain.InvalidJWTError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusUnauthorized, e.Error())
			return
		}

		// gracefully handle the case where the user is not found, securely
		// without exposing any information about the user
		if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
			// gratefully handle the case where the user is not found
			_ = o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusOK, domain.AuthnUserVerificationEmailSent)
			return
		}

		_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
		_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
		_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
		_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
		if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user re-verification email sent")

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.AuthnUserVerificationEmailSent)
}

// logout Log out the current user
//
//	@Id				019822af-b448-7562-a27f-0d02884f3477
//	@Summary		Logout user
//	@Description	End the session. The access token this request was authorised with is revoked immediately and stops working. The refresh token is revoked too when it is supplied in the body -- without it, that token stays valid until it expires and can still mint new access tokens, so pass it.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Success		200		{object}	payload.HTTPMessage			"Logout successful"
//	@Failure		400		{object}	payload.HTTPMessage			"Malformed request or invalid stored session data"
//	@Failure		401		{object}	payload.HTTPMessage			"Invalid or missing access token. An ALREADY-REVOKED access token is accepted here, unlike everywhere else: logging out twice must succeed, because two tabs logging out at once is ordinary"
//	@Failure		403		{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		413		{object}	payload.HTTPMessage			"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage			"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error during logout"
//	@Param			body	body		payload.LogoutUserRequest	false	"Refresh token to revoke. Omitting it leaves the refresh token valid"
//	@Router			/auth/logout [delete]
//	@Security		AccessToken
func (ref *AuthnHandler) logout(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "logout")
	defer span.End()

	// get the user id from the context
	jwtClaims, ok := r.Context().Value(middleware.JwtClaims).(map[string]any)
	if !ok {
		errorMsg := errors.New(domain.AuthnFailedToGetUserIDFromContext)
		_ = o11y.RecordError(ctx, span, start, errorMsg, ref.metrics, attrs)
		respond.WriteInternalError(w, r, errorMsg)
		return
	}

	userIDSub, ok := jwtClaims["sub"].(string)
	if !ok {
		errorMsg := errors.New(domain.AuthnFailedToGetUserIDFromContext)
		_ = o11y.RecordError(ctx, span, start, errorMsg, ref.metrics, attrs)
		respond.WriteInternalError(w, r, errorMsg)
		return
	}

	userID, err := parseUUIDQueryParams(userIDSub)
	if err != nil {
		errorMsg := errors.New(domain.AuthnFailedToParseUserIDFromContext)
		_ = o11y.RecordError(ctx, span, start, errorMsg, ref.metrics, attrs)
		respond.WriteInternalError(w, r, errorMsg)
		return
	}

	// The body is optional: DELETE with no body has always been valid here, and
	// a decode failure must not stop someone logging out. What it costs is that
	// the refresh token is not revoked, which the service logs.
	var req payload.LogoutUserRequest
	if r.Body != nil {
		if err := decodeJSONBody(r, &req); err != nil && !errors.Is(err, io.EOF) {
			slog.Debug("logout: could not decode the request body, continuing without a refresh token", "error", err)
		}
	}

	// The access token this request was authorised with, taken from the claims
	// the middleware verified rather than from anything the caller sent. A
	// personal access token has no denylist entry — deleting its row is what
	// revokes it — so it is deliberately left alone here.
	accessJTI, accessExpiresAt := revocableAccessToken(jwtClaims)

	input := &domain.LogoutUserInput{
		UserID:               userID,
		RefreshToken:         req.RefreshToken,
		AccessTokenJTI:       accessJTI,
		AccessTokenExpiresAt: accessExpiresAt,
	}

	_, err = ref.service.LogoutUser(ctx, input)
	if err != nil {
		// The caller supplied something we could not revoke — unreadable, not a
		// refresh token, or somebody else's. 400 rather than 500: nothing is
		// broken here, the request was wrong, and the session is still live.
		if _, ok := errors.AsType[*domain.InvalidRefreshTokenError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
		_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
		_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
		_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
		if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	slog.Debug("user logged out", "userID", userID)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.AuthnUserLoggedOutSuccessfully)

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.AuthnUserLoggedOutSuccessfully)
}

// refreshAccessToken Retrieve a new access token using the refresh token.
//
//	@Id				019822af-b448-756a-92b2-791a0e748162
//	@Summary		Refresh access token
//	@Description	Obtain new access and refresh tokens using valid refresh token. The token spent is the one in the Authorization header; the body is optional and, if it carries a token, it must be the same one.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			body	body		payload.RefreshTokenRequest		false	"Optional, and must match the Authorization header when present"
//	@Success		200		{object}	payload.RefreshTokenResponse	"New access and refresh tokens issued"
//	@Failure		400		{object}	payload.HTTPMessage				"Malformed body, or a refresh_token that disagrees with the Authorization header"
//	@Failure		401		{object}	payload.HTTPMessage				"Refresh token is invalid or expired, or the account it was issued for is disabled or no longer exists"
//	@Failure		403		{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		413		{object}	payload.HTTPMessage				"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage				"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage				"Internal server error during token refresh"
//	@Router			/auth/refresh [post]
//	@Security		RefreshToken
func (ref *AuthnHandler) refreshAccessToken(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "refreshAccessToken")
	defer span.End()

	// The token to spend is the one the middleware verified, which is the one
	// in the Authorization header.
	//
	// This handler used to decode a SECOND token out of the request body and
	// act on that instead, so the token that was authorised and the token that
	// was refreshed were two different values and the validated one was thrown
	// away. Nothing enforced that they matched.
	refreshToken, err := getTokenFromContext(ctx)
	if err != nil {
		// Reaching here means the middleware did not run, which is a wiring
		// mistake rather than anything the caller did. The caller is told the
		// same thing every refused token gets; the detail is on the span.
		_ = o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "Invalid or expired token")
		return
	}

	// The body is still read, and is still allowed to carry the token, because
	// every existing client sends it in both places. It may no longer DISAGREE:
	// a request that authorises with one token and asks to spend another is a
	// client bug, and answering it by quietly picking one is how the two got
	// out of step in the first place.
	var req payload.RefreshTokenRequest
	if err := decodeJSONBody(r, &req); err != nil && !errors.Is(err, io.EOF) {
		errorType := &domain.InvalidRequestError{Message: "failed to decode request"}
		e := o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
		respond.WriteDecodeError(w, r, e)
		return
	}

	if req.RefreshToken != "" && req.RefreshToken != refreshToken {
		errorType := &domain.InvalidRequestError{
			Message: "the refresh_token in the body is not the token this request was authorised with",
		}
		e := o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	input := &domain.RefreshAccessTokenInput{
		RefreshToken: refreshToken,
	}

	out, err := ref.service.RefreshAccessToken(ctx, input)
	if err != nil {
		// InvalidRefreshTokenError covers everything the service rejects about
		// the token itself — a missing jti, the wrong token_type, an account
		// that has since been disabled or deleted. None of those are server
		// faults, but the type was mapped nowhere, so all of them answered 500.
		_, isInvalidJWT := errors.AsType[*domain.InvalidJWTError](err)
		_, isInvalidRefresh := errors.AsType[*domain.InvalidRefreshTokenError](err)
		if isInvalidJWT || isInvalidRefresh {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusUnauthorized, e.Error())
			return
		}

		_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
		_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
		_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
		_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
		if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	outResponse := &payload.RefreshTokenResponse{
		AccessToken:  out.AccessToken,
		RefreshToken: out.RefreshToken,
		TokenType:    out.TokenType,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.AuthnAccessTokenRefreshedSuccessfully)
}

// recoverPassword endpoint uses a token to verify the user's identity and initiate the password recovery process.
//
//	@Id				01991917-2720-7589-971b-cce23bf8a74b
//	@Summary		Initiate password recovery
//	@Description	Request a password reset email with secure token.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			body	body		payload.RecoverPasswordRequest	true	"Email address for password recovery"
//	@Success		200		{object}	payload.HTTPMessage				"Accepted. Answered the same way whether or not an account exists, is disabled, or authenticates through an identity provider — deliberately, so this endpoint cannot be used to discover which addresses have accounts"
//	@Failure		400		{object}	payload.HTTPMessage				"Invalid request body or email format"
//	@Failure		413		{object}	payload.HTTPMessage				"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage				"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage				"This address has been asked about too often; Retry-After says when. Keyed on the submitted address, so an address with no account is throttled exactly like a real one"
//	@Failure		500		{object}	payload.HTTPMessage				"Internal server error during password recovery"
//	@Router			/auth/password/recover [post]
func (ref *AuthnHandler) recoverPassword(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "recoverPassword")
	defer span.End()

	var req payload.RecoverPasswordRequest
	if err := decodeJSONBody(r, &req); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteDecodeError(w, r, e)
		return
	}

	if err := req.Validate(); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	input := &domain.RecoverPasswordInput{
		Email: req.Email,
	}

	if err := ref.service.RecoverPassword(ctx, input); err != nil {
		_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
		_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
		_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
		_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
		if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		// The per-address budget is spent. Answering 429 here does not
		// reintroduce the account oracle this endpoint was just cleared of:
		// the budget is keyed on the SUBMITTED address, before anything is
		// looked up, so an address with no account is throttled exactly like a
		// real one.
		if throttled, ok := errors.AsType[*domain.TooManyRecoveryRequestsError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(throttled.RetryAfter)))
			respond.WriteJSONMessage(w, r, http.StatusTooManyRequests, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.AuthnPasswordRecoveryEmailSent)

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.AuthnPasswordRecoveryEmailSent)
}

// resetPassword endpoint allows users to reset their password using a valid token.
//
//	@Id				01991917-2720-758d-8104-94a0368acecb
//	@Summary		Reset password
//	@Description	Set new password using the token from password recovery email.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			body	body		payload.ResetPasswordRequest	true	"New password to set"
//	@Success		200		{object}	payload.HTTPMessage				"Password reset successful"
//	@Failure		400		{object}	payload.HTTPMessage				"Invalid request body or password validation error"
//	@Failure		401		{object}	payload.HTTPMessage				"Invalid, expired, or already used reset token"
//	@Failure		403		{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		413		{object}	payload.HTTPMessage				"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage				"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage				"Internal server error during password reset"
//	@Router			/auth/password/reset [post]
//	@Security		ResetPasswordToken
func (ref *AuthnHandler) resetPassword(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "resetPassword")
	defer span.End()

	var req payload.ResetPasswordRequest
	if err := decodeJSONBody(r, &req); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteDecodeError(w, r, e)
		return
	}

	if err := req.Validate(); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	// get the user id from the context
	jwtClaims, ok := r.Context().Value(middleware.JwtClaims).(map[string]any)
	if !ok {
		errorMsg := domain.AuthnFailedToGetUserIDFromContext
		_ = o11y.RecordError(ctx, span, start, errors.New(errorMsg), ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, errorMsg)
		return
	}

	userID, ok := jwtClaims["sub"].(string)
	if !ok {
		errorMsg := domain.AuthnFailedToGetUserIDFromContext
		_ = o11y.RecordError(ctx, span, start, errors.New(errorMsg), ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, errorMsg)
		return
	}

	// convert userID to UUID
	userUUID, err := parseUUIDQueryParams(userID)
	if err != nil {
		errorMsg := domain.AuthnFailedToParseUserIDFromContext
		_ = o11y.RecordError(ctx, span, start, errors.New(errorMsg), ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, errorMsg)
		return
	}

	// The token's jti and expiry make the reset single-use. The middleware
	// verified the token; the claims are the ones it put on the context.
	jti, _ := uuid.Parse(fmt.Sprint(jwtClaims["jti"]))
	tokenExpiresAt := time.Time{}
	if exp, ok := jwtClaims["exp"].(float64); ok {
		tokenExpiresAt = time.Unix(int64(exp), 0)
	}

	input := &domain.ResetPasswordInput{
		UserID:         userUUID,
		Password:       req.Password,
		TokenID:        jti,
		TokenExpiresAt: tokenExpiresAt,
	}

	if err := ref.service.ResetPassword(ctx, input); err != nil {
		_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
		_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
		_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
		_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
		if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.AuthnPasswordResetSuccessfully)

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.AuthnPasswordResetSuccessfully)
}

// retryAfterSeconds renders a delay for the Retry-After header, which is whole
// seconds and must never be zero or negative — a client reading "0" would retry
// immediately and simply be refused again.
func retryAfterSeconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}

	return int((d + time.Second - 1) / time.Second)
}

// revocableAccessToken pulls the jti and exp of the access token a request was
// authorised with out of the claims the middleware verified.
//
// It returns zeroes for anything that is not an ordinary access token — a
// personal access token is governed by its own row, and revoking it here would
// put a year-long entry in a denylist whose reload window is measured against
// the access-token lifetime, where it would be missed on the next reload and
// silently come back to life.
//
// It also returns zeroes rather than an error when a claim is missing or
// malformed: this runs on a token that has already been verified, so a missing
// claim is not an attack, and refusing to log somebody out because their token
// was shaped unusually would be the wrong trade in the wrong direction.
func revocableAccessToken(claims map[string]any) (jti uuid.UUID, expiresAt time.Time) {
	if tokenType, _ := claims["token_type"].(string); tokenType != domain.TokenTypeAccess.String() {
		return uuid.Nil(), time.Time{}
	}

	raw, ok := claims["jti"].(string)
	if !ok {
		return uuid.Nil(), time.Time{}
	}

	parsed, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil(), time.Time{}
	}

	// exp is a JSON number, so it arrives as a float64 through map[string]any.
	exp, ok := claims["exp"].(float64)
	if !ok {
		return uuid.Nil(), time.Time{}
	}

	return parsed, time.Unix(int64(exp), 0)
}
