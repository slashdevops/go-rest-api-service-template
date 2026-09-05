package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driving"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// MeHandlerConf represents the configuration for the user handler.
type MeHandlerConf struct {
	UserService   driving.Users
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// MeHandler represents the handler for the user.
type MeHandler struct {
	userService     driving.Users
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewMeHandler creates a new MeHandler.
func NewMeHandler(conf MeHandlerConf) (*MeHandler, error) {
	if conf.UserService == nil {
		return nil, &domain.InvalidServiceError{Message: "MeService is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &MeHandler{
		userService:   conf.UserService,
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Me",
			Action: "NewMeHandler",
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

// RegisterRoutes registers the routes on the mux.
func (ref *MeHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /me/authz", mdw.ThenFunc(ref.authz))
	mux.Handle("GET /me", mdw.ThenFunc(ref.getByID))
	mux.Handle("PUT /me", mdw.ThenFunc(ref.update))
}

// authz retrieves the authorization information of the authenticated user.
//
//	@ID				0199489b-f2f0-719e-b860-3b7ea6a86a1a
//	@Summary		Get authorization info
//	@Description	Retrieve the authorization details and permissions for the currently authenticated user
//	@Tags			Me,Authorization
//	@Produce		json
//	@Success		200	{object}	payload.GetAuthenticatedUserResponse	"Authorization information retrieved successfully"
//	@Failure		400	{object}	payload.HTTPMessage						"Invalid request format or parameters"
//	@Failure		401	{object}	payload.HTTPMessage						"Unauthorized - invalid or missing authentication"
//	@Failure		403	{object}	payload.HTTPMessage						"Insufficient permissions"
//	@Failure		404	{object}	payload.HTTPMessage						"User not found"
//	@Failure		429	{object}	payload.HTTPMessage						"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500	{object}	payload.HTTPMessage						"Internal server error"
//	@Router			/me/authz [get]
//	@Security		AccessToken
func (ref *MeHandler) authz(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "authz")
	defer span.End()

	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusUnauthorized, e.Error())
		return
	}

	user, err := ref.userService.GetByID(ctx, userID)
	if err != nil {
		if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
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
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	perm, err := ref.userService.SelectAuthz(ctx, userID)
	if err != nil {
		if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
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
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	if perm == nil {
		e := o11y.RecordError(ctx, span, start, fmt.Errorf("permissions not found for user %s", userID), ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	if perm["permissions"] == nil {
		e := o11y.RecordError(ctx, span, start, fmt.Errorf("permissions field is missing for user %s", userID), ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	if _, ok := perm["permissions"].(map[string]any); !ok {
		e := o11y.RecordError(ctx, span, start, fmt.Errorf("permissions field is not a list for user %s", userID), ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	permissions, ok := perm["permissions"].(map[string]any)
	if !ok {
		e := o11y.RecordError(ctx, span, start, fmt.Errorf("permissions field is not a []any for user %s", userID), ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	userResponse := payload.UserResponse{
		ID:           user.ID,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
		Disabled:     user.Disabled,
		Admin:        user.Admin,
		LocalAccount: user.LocalAccount,
		FirstName:    user.FirstName,
		LastName:     user.LastName,
		Email:        user.Email,
	}

	// Typed for the wire; the map stays generic through the core because OPA
	// takes it as Rego input. A shape this does not recognise is logged at ERROR
	// and sent empty rather than failing the request: this set tells the client
	// which controls to offer and nothing else -- CheckAuthz decides every
	// request from the same data, server-side -- so an empty one hides controls
	// the caller may use and can never reveal one they may not.
	typedPermissions, err := payload.NewAuthzPermissions(permissions)
	if err != nil {
		slog.Error("handler.Me.authz: unrecognised permission shape, sending an empty set",
			"user.id", userID.String(), "error", err)
	}

	outResponse := &payload.GetAuthenticatedUserResponse{
		Account:     userResponse,
		Permissions: typedPermissions,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "User information retrieved",
		attribute.String("user.id", userID.String()))
}

// update updates the authenticated user information.
//
//	@ID				0199489b-f2f0-718e-a94d-b05a296eb818
//	@Summary		Update authenticated user
//	@Description	Update the profile information for the currently authenticated user
//	@Tags			Me
//	@Accept			json
//	@Produce		json
//	@Param			body	body		payload.UpdateMeRequest	true	"User update request payload"
//	@Success		200		{object}	payload.HTTPMessage		"User updated successfully"
//	@Failure		400		{object}	payload.HTTPMessage		"Invalid request format or validation failed"
//	@Failure		401		{object}	payload.HTTPMessage		"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage		"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage		"User not found"
//	@Failure		409		{object}	payload.HTTPMessage		"User already exists (duplicate email)"
//	@Failure		429		{object}	payload.HTTPMessage		"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage		"Internal server error"
//	@Router			/me [put]
//	@Security		AccessToken
func (ref *MeHandler) update(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "update")
	defer span.End()

	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.UpdateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	if err := req.Validate(); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	input := &domain.UpdateUserInput{
		ID:        userID,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Password:  req.Password,
	}

	if err := ref.userService.UpdateByID(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.UserAlreadyExistsError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
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
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	// Location header is required for RESTful APIs
	w.Header().Set("Location", fmt.Sprintf("%s%s", r.Header.Get("Origin"), r.RequestURI))

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.UsersUserUpdatedSuccessfully,
		attribute.String("user.id", input.ID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.UsersUserUpdatedSuccessfully)
}

// getByID retrieves the authenticated user information.
//
//	@ID				0199489b-f2f0-718a-a0cb-de8752ea864f
//	@Summary		Get authenticated user
//	@Description	Retrieve the profile information for the currently authenticated user
//	@Tags			Me
//	@Produce		json
//	@Success		200	{object}	payload.UserResponse	"User information retrieved successfully"
//	@Failure		400	{object}	payload.HTTPMessage		"Invalid request format or parameters"
//	@Failure		401	{object}	payload.HTTPMessage		"Missing or invalid authentication token"
//	@Failure		403	{object}	payload.HTTPMessage		"Insufficient permissions"
//	@Failure		404	{object}	payload.HTTPMessage		"User not found"
//	@Failure		429	{object}	payload.HTTPMessage		"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500	{object}	payload.HTTPMessage		"Internal server error"
//	@Router			/me [get]
//	@Security		AccessToken
func (ref *MeHandler) getByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "getByID")
	defer span.End()

	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	out, err := ref.userService.GetByID(ctx, userID)
	if err != nil {
		if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
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
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	outResponse := payload.UserResponse{
		ID:           out.ID,
		CreatedAt:    out.CreatedAt,
		UpdatedAt:    out.UpdatedAt,
		Disabled:     out.Disabled,
		Admin:        out.Admin,
		LocalAccount: out.LocalAccount,
		FirstName:    out.FirstName,
		LastName:     out.LastName,
		Email:        out.Email,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	slog.Debug("handler.Me.getByID", "user.email", outResponse.Email)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.UsersUserFound,
		attribute.String("user.id", outResponse.ID.String()),
		attribute.String("user.email", outResponse.Email),
	)
}
