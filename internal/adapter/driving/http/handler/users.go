package handler

import (
	"context"
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

// UsersHandlerConf represents the configuration for the user handler.
type UsersHandlerConf struct {
	Service driving.Users
	// Recoverer sends the password-reset email. It is the authn service's
	// RecoverPassword, narrowed to the one method this handler needs.
	Recoverer     PasswordRecoverer
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// PasswordRecoverer sends a reset link to an address, saying nothing about
// whether the address exists.
type PasswordRecoverer interface {
	RecoverPassword(ctx context.Context, input *domain.RecoverPasswordInput) error
}

// UsersHandler represents the handler for the user.
type UsersHandler struct {
	service         driving.Users
	recoverer       PasswordRecoverer
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewUsersHandler creates a new UsersHandler.
func NewUsersHandler(conf UsersHandlerConf) (*UsersHandler, error) {
	if conf.Service == nil {
		return nil, &domain.InvalidServiceError{Message: "driving.Users is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &UsersHandler{
		service:       conf.Service,
		recoverer:     conf.Recoverer,
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Users",
			Action: "NewUsersHandler",
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
func (ref *UsersHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /users", mdw.ThenFunc((ref.list)))
	mux.Handle("POST /users", mdw.ThenFunc(ref.create))
	mux.Handle("GET /users/{user_id}", mdw.ThenFunc(ref.getByID))
	mux.Handle("PUT /users/{user_id}", mdw.ThenFunc(ref.updateByID))
	mux.Handle("DELETE /users/{user_id}", mdw.ThenFunc(ref.deleteByID))
	mux.Handle("POST /users/{user_id}/password/reset", mdw.ThenFunc(ref.resetPassword))

	// link/unlink roles to user
	mux.Handle("POST /users/{user_id}/roles", mdw.ThenFunc(ref.linkRoles))
	mux.Handle("DELETE /users/{user_id}/roles", mdw.ThenFunc(ref.unlinkRoles))

	// link/unlink projects to user
	mux.Handle("POST /users/{user_id}/projects", mdw.ThenFunc(ref.linkProjects))
	mux.Handle("DELETE /users/{user_id}/projects", mdw.ThenFunc(ref.unlinkProjects))

	// select authz
	mux.Handle("GET /users/{user_id}/authz", mdw.ThenFunc(ref.selectAuthz))

	// list users by role
	mux.Handle("GET /roles/{role_id}/users", mdw.ThenFunc(ref.listByRoleID))

	// list users by project
	mux.Handle("GET /projects/{project_id}/users", mdw.ThenFunc(ref.listByProjectID))
}

// getByID Get a user by ID
//
//	@ID				01982303-f0f9-7e25-85f4-4d9d47622702
//	@Summary		Get user
//	@Description	Retrieve user account by unique identifier.
//	@Tags			Users
//	@Produce		json
//	@Param			user_id	path		string					true	"User unique identifier"	Format(uuid)
//	@Success		200		{object}	payload.UserResponse	"User details retrieved successfully"
//	@Failure		400		{object}	payload.HTTPMessage		"Invalid user ID format"
//	@Failure		401		{object}	payload.HTTPMessage		"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage		"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage		"User not found"
//	@Failure		429		{object}	payload.HTTPMessage		"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage		"Internal server error"
//	@Router			/users/{user_id} [get]
//	@Security		AccessToken
func (ref *UsersHandler) getByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "getByID")
	defer span.End()

	userID, err := parseUUIDQueryParams(r.PathValue("user_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	out, err := ref.service.GetByID(ctx, userID)
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
		respond.WriteInternalError(w, r, e)
		return
	}

	userResponse := payload.UserResponse{
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

	if err := respond.WriteJSONData(w, http.StatusOK, userResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user retrieved",
		attribute.String("user.id", userID.String()),
	)

	slog.Debug("handler.Users.getByID", "user.email", out.Email)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.UsersUserFound,
		attribute.String("user.id", out.ID.String()),
		attribute.String("user.email", out.Email),
	)
}

// create Create a new user
//
//	@ID				01982303-f0f9-7e78-bddf-389144c4beaf
//	@Summary		Create user
//	@Description	Create a new user account with specified details.
//	@Tags			Users
//	@Accept			json
//	@Produce		json
//	@Param			body	body		payload.CreateUserRequest	true	"User creation details including email, name, and profile"
//	@Success		201		{object}	payload.HTTPMessage			"User account created successfully"
//	@Header			201		{string}	Location					"/users/{id}"	"URI of the created user resource"
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid request body or validation error"
//	@Failure		401		{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		409		{object}	payload.HTTPMessage			"User with email already exists"
//	@Failure		413		{object}	payload.HTTPMessage			"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage			"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error during user creation"
//	@Router			/users [post]
//	@Security		AccessToken
func (ref *UsersHandler) create(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "create")
	defer span.End()

	var req payload.CreateUserRequest
	if err := decodeJSONBody(r, &req); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteDecodeError(w, r, e)
		return
	}

	var err error
	req.ID, err = domain.EnsureUUIDV7(req.ID)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	if err := req.Validate(); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	input := &domain.CreateUserInput{
		ID:           req.ID,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Email:        req.Email,
		Password:     req.Password,
		Disabled:     new(true), // users are disabled by default until they verify their email
		LocalAccount: new(true),
	}

	if err := ref.service.Create(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.ResourcesLimitsHardLimitReachedError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.UserAlreadyExistsError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.InvalidEmailError](err); ok {
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

	// Location header is required for RESTful APIs
	respond.SetLocation(w, r, input.ID.String())

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.UsersUserCreatedSuccessfully,
		attribute.String("user.id", input.ID.String()),
		attribute.String("user.email", input.Email))

	respond.WriteJSONMessage(w, r, http.StatusCreated, domain.UsersUserCreatedSuccessfully)
}

// updateByID Update a user
//
//	@ID				01982303-f0f9-7e3c-a186-f186a3418768
//	@Summary		Update user
//	@Description	Update existing user account details by ID.
//	@Tags			Users
//	@Accept			json
//	@Produce		json
//	@Param			user_id	path		string						true	"User unique identifier"	Format(uuid)
//	@Param			body	body		payload.UpdateUserRequest	true	"User update details including name, email, or profile changes"
//	@Success		200		{object}	payload.HTTPMessage			"User updated successfully"
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid user ID or request body"
//	@Failure		401		{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage			"User not found"
//	@Failure		409		{object}	payload.HTTPMessage			"Email already in use by another user"
//	@Failure		413		{object}	payload.HTTPMessage			"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage			"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error during update"
//	@Router			/users/{user_id} [put]
//	@Security		AccessToken
func (ref *UsersHandler) updateByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "updateByID")
	defer span.End()

	userID, err := parseUUIDQueryParams(r.PathValue("user_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.UpdateUserRequest
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

	input := &domain.UpdateUserInput{
		ID:           userID,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Email:        req.Email,
		Disabled:     req.Disabled,
		LocalAccount: req.LocalAccount,
	}

	if err := ref.service.UpdateByID(ctx, input); err != nil {
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
		respond.WriteInternalError(w, r, e)
		return
	}

	// Location header is required for RESTful APIs
	respond.SetLocation(w, r)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.UsersUserUpdatedSuccessfully,
		attribute.String("user.id", input.ID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.UsersUserUpdatedSuccessfully)
}

// deleteByID Delete a user
//
//	@ID				01982303-f0f9-7dda-84da-cedd68bca775
//	@Summary		Delete user
//	@Description	Permanently remove user account by ID.
//	@Tags			Users
//	@Produce		json
//	@Param			user_id	path		string				true	"User unique identifier"	Format(uuid)
//	@Success		200		{object}	payload.HTTPMessage	"User deleted successfully"
//	@Failure		400		{object}	payload.HTTPMessage	"Invalid user ID format"
//	@Failure		401		{object}	payload.HTTPMessage	"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage	"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage	"User not found"
//	@Failure		429		{object}	payload.HTTPMessage	"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage	"Internal server error during deletion"
//	@Router			/users/{user_id} [delete]
//	@Security		AccessToken
func (ref *UsersHandler) deleteByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "deleteByID")
	defer span.End()

	userID, err := parseUUIDQueryParams(r.PathValue("user_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	input := &domain.DeleteUserInput{
		ID: userID,
	}

	if err := ref.service.DeleteByID(ctx, input); err != nil {
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
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.UsersUserDeletedSuccessfully,
		attribute.String("user.id", userID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.UsersUserDeletedSuccessfully)
}

// list Return a paginated list of users
//
//	@ID				01982303-f0f9-7daf-809b-4f7880ca9e40
//	@Summary		List users
//	@Description	Retrieve paginated list of users with optional filtering and sorting.
//	@Tags			Users
//	@Produce		json
//	@Param			sort		query		string						false	"Sort fields (comma-separated). Example: first_name ASC, created_at DESC"
//	@Param			filter		query		string						false	"Filter expression. Example: id=1 AND first_name='John'"
//	@Param			fields		query		string						false	"Fields to return (comma-separated). Example: id,first_name,last_name"
//	@Param			next_token	query		string						false	"Next page cursor for pagination"
//	@Param			prev_token	query		string						false	"Previous page cursor for pagination"
//	@Param			limit		query		int							false	"Maximum number of results per page"
//	@Success		200			{object}	payload.ListUsersResponse	"Paginated list of users"
//	@Failure		400			{object}	payload.HTTPMessage			"Invalid query parameters or filter expression"
//	@Failure		401			{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/users [get]
//	@Security		AccessToken
func (ref *UsersHandler) list(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "list")
	defer span.End()

	// parse the query parameters
	params := map[string]any{
		"sort":      r.URL.Query().Get("sort"),
		"filter":    r.URL.Query().Get("filter"),
		"fields":    r.URL.Query().Get("fields"),
		"nextToken": r.URL.Query().Get("next_token"),
		"prevToken": r.URL.Query().Get("prev_token"),
		"limit":     r.URL.Query().Get("limit"),
	}

	sort, filter, fields, nextToken, prevToken, limit, err := parseListQueryParams(params)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	input := &domain.ListUsersInput{
		Sort:   sort,
		Filter: filter,
		Fields: fields,
		Paginator: domain.Paginator{
			NextToken: nextToken,
			PrevToken: prevToken,
			Limit:     limit,
		},
	}

	outList, err := ref.service.List(ctx, input)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, httpStatusForListError(err), e.Error())
		return
	}

	outResponse := &payload.ListUsersResponse{
		Items:     make([]payload.UserResponse, len(outList.Items)),
		Paginator: outList.Paginator,
	}

	for i, user := range outList.Items {
		outResponse.Items[i] = payload.UserResponse{
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
	}

	// Generate the next and previous pages
	location := fmt.Sprintf("http://%s%s", r.Host, r.URL.Path)
	outResponse.Paginator.GeneratePages(location)

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "List users",
		attribute.Int("users.count", len(outResponse.Items)))
}

// listByRoleID List the users linked to a role
//
//	@ID				01982303-f0fa-7027-9b77-197b693d0e5a
//	@Summary		List users by role
//	@Description	Retrieve paginated list of users assigned to specific role.
//	@Tags			Users,Roles
//	@Produce		json
//	@Param			role_id		path		string						true	"Role unique identifier"	Format(uuid)
//	@Param			sort		query		string						false	"Sort fields (comma-separated). Example: first_name ASC, created_at DESC"
//	@Param			filter		query		string						false	"Filter expression. Example: id=1 AND first_name='John'"
//	@Param			fields		query		string						false	"Fields to return (comma-separated). Example: id,first_name,last_name"
//	@Param			next_token	query		string						false	"Next page cursor for pagination"
//	@Param			prev_token	query		string						false	"Previous page cursor for pagination"
//	@Param			limit		query		int							false	"Maximum number of results per page"
//	@Success		200			{object}	payload.ListUsersResponse	"Paginated list of users with specified role"
//	@Failure		400			{object}	payload.HTTPMessage			"Invalid role ID or query parameters"
//	@Failure		401			{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/roles/{role_id}/users [get]
//	@Security		AccessToken
func (ref *UsersHandler) listByRoleID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "listByRoleID")
	defer span.End()

	// Get project ID
	roleID, err := parseUUIDQueryParams(r.PathValue("role_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	// parse the query parameters
	params := map[string]any{
		"sort":      r.URL.Query().Get("sort"),
		"filter":    r.URL.Query().Get("filter"),
		"fields":    r.URL.Query().Get("fields"),
		"nextToken": r.URL.Query().Get("next_token"),
		"prevToken": r.URL.Query().Get("prev_token"),
		"limit":     r.URL.Query().Get("limit"),
	}

	sort, filter, fields, nextToken, prevToken, limit, err := parseListQueryParams(params)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	span.SetAttributes(attribute.String("role.id", roleID.String()))

	input := &domain.ListUsersInput{
		Sort:   sort,
		Filter: filter,
		Fields: fields,
		Paginator: domain.Paginator{
			NextToken: nextToken,
			PrevToken: prevToken,
			Limit:     limit,
		},
	}

	outList, err := ref.service.ListByRoleID(ctx, roleID, input)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, httpStatusForListError(err), e.Error())
		return
	}

	outResponse := &payload.ListUsersResponse{
		Items:     make([]payload.UserResponse, len(outList.Items)),
		Paginator: outList.Paginator,
	}

	for i, user := range outList.Items {
		outResponse.Items[i] = payload.UserResponse{
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
	}

	// Generate the next and previous pages
	location := fmt.Sprintf("http://%s%s", r.Host, r.URL.Path)
	outResponse.Paginator.GeneratePages(location)

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "List users by role ID",
		attribute.Int("users.count", len(outResponse.Items)),
		attribute.String("role.id", roleID.String()))
}

// listByProjectID List the users linked to a project
//
//	@ID				01987096-b4a1-7e8a-8a38-98148daa27a2
//	@Summary		List users by project
//	@Description	Retrieve paginated list of users assigned to specific project.
//	@Tags			Users,Projects
//	@Produce		json
//	@Param			project_id	path		string						true	"Project unique identifier"	Format(uuid)
//	@Param			sort		query		string						false	"Sort fields (comma-separated). Example: first_name ASC, created_at DESC"
//	@Param			filter		query		string						false	"Filter expression. Example: id=1 AND first_name='John'"
//	@Param			fields		query		string						false	"Fields to return (comma-separated). Example: id,first_name,last_name"
//	@Param			next_token	query		string						false	"Next page cursor for pagination"
//	@Param			prev_token	query		string						false	"Previous page cursor for pagination"
//	@Param			limit		query		int							false	"Maximum number of results per page"
//	@Success		200			{object}	payload.ListUsersResponse	"Paginated list of users in specified project"
//	@Failure		400			{object}	payload.HTTPMessage			"Invalid project ID or query parameters"
//	@Failure		401			{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		404			{object}	payload.HTTPMessage			"Project not found, or the caller is not a member of it"
//	@Failure		500			{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/projects/{project_id}/users [get]
//	@Security		AccessToken
func (ref *UsersHandler) listByProjectID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "listByProjectID")
	defer span.End()

	// Get project ID
	projectID, err := parseUUIDQueryParams(r.PathValue("project_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	// parse the query parameters
	params := map[string]any{
		"sort":      r.URL.Query().Get("sort"),
		"filter":    r.URL.Query().Get("filter"),
		"fields":    r.URL.Query().Get("fields"),
		"nextToken": r.URL.Query().Get("next_token"),
		"prevToken": r.URL.Query().Get("prev_token"),
		"limit":     r.URL.Query().Get("limit"),
	}

	sort, filter, fields, nextToken, prevToken, limit, err := parseListQueryParams(params)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	span.SetAttributes(attribute.String("project.id", projectID.String()))

	input := &domain.ListUsersInput{
		Sort:   sort,
		Filter: filter,
		Fields: fields,
		Paginator: domain.Paginator{
			NextToken: nextToken,
			PrevToken: prevToken,
			Limit:     limit,
		},
	}

	outList, err := ref.service.ListByProjectID(ctx, projectID, input)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, httpStatusForListError(err), e.Error())
		return
	}

	outResponse := &payload.ListUsersResponse{
		Items:     make([]payload.UserResponse, len(outList.Items)),
		Paginator: outList.Paginator,
	}

	for i, user := range outList.Items {
		outResponse.Items[i] = payload.UserResponse{
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
	}

	// Generate the next and previous pages
	location := fmt.Sprintf("http://%s%s", r.Host, r.URL.Path)
	outResponse.Paginator.GeneratePages(location)

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "List users by project ID",
		attribute.Int("users.count", len(outResponse.Items)),
		attribute.String("project.id", projectID.String()))
}

// linkRoles Link roles to user
//
//	@ID				01982303-f0fa-700f-a042-0a487ed3c9fb
//	@Summary		Link roles to user
//	@Description	Associate multiple roles with user within project.
//	@Tags			Users,Roles
//	@Accept			json
//	@Produce		json
//	@Param			user_id	path		string							true	"User unique identifier"	Format(uuid)
//	@Param			user	body		payload.LinkRolesToUserRequest	true	"Role IDs to link with project context"
//	@Success		200		{object}	payload.HTTPMessage				"Roles linked to user successfully"
//	@Failure		400		{object}	payload.HTTPMessage				"Invalid user ID or request body"
//	@Failure		401		{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage				"User not found"
//	@Failure		409		{object}	payload.HTTPMessage				"Role already linked to user"
//	@Failure		413		{object}	payload.HTTPMessage				"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage				"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage				"Internal server error during role linking"
//	@Router			/users/{user_id}/roles [post]
//	@Security		AccessToken
func (ref *UsersHandler) linkRoles(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "linkRoles")
	defer span.End()

	userID, err := parseUUIDQueryParams(r.PathValue("user_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.LinkRolesToUserRequest
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

	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusUnauthorized, e.Error())
		return
	}

	input := &domain.LinkRolesToUserInput{
		CallerID: callerID,
		UserID:   userID,
		RoleIDs:  req.RoleIDs,
	}

	if err := ref.service.LinkRoles(ctx, input); err != nil {
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

		// The caller does not hold what they tried to hand out.
		if _, ok := errors.AsType[*domain.GrantNotHeldError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusForbidden, e.Error())
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

	slog.Debug("handler.Users.linkRoles", "user.id", userID.String())

	// Location header is required for RESTful APIs
	respond.SetLocation(w, r)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.UsersRoleLinkedToUserSuccessfully,
		attribute.String("user.id", userID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.UsersRoleLinkedToUserSuccessfully)
}

// unlinkRoles Unlink roles from user
//
//	@ID				01982303-f0f9-7f23-b203-7079222718d0
//	@Summary		Unlink roles from user
//	@Description	Remove role associations from user within project.
//	@Tags			Users,Roles
//	@Accept			json
//	@Produce		json
//	@Param			user_id	path		string								true	"User unique identifier"	Format(uuid)
//	@Param			body	body		payload.UnlinkRolesFromUserRequest	true	"Role IDs to unlink with project context"
//	@Success		200		{object}	payload.HTTPMessage					"Roles unlinked from user successfully"
//	@Failure		400		{object}	payload.HTTPMessage					"Invalid user ID or request body"
//	@Failure		401		{object}	payload.HTTPMessage					"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage					"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage					"User not found"
//	@Failure		409		{object}	payload.HTTPMessage					"Conflict"
//	@Failure		413		{object}	payload.HTTPMessage					"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage					"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage					"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage					"Internal server error during role unlinking"
//	@Router			/users/{user_id}/roles [delete]
//	@Security		AccessToken
func (ref *UsersHandler) unlinkRoles(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "unlinkRoles")
	defer span.End()

	userID, err := parseUUIDQueryParams(r.PathValue("user_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.UnlinkRolesFromUserRequest
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

	input := &domain.UnlinkRolesFromUsersInput{
		UserID:  userID,
		RoleIDs: req.RoleIDs,
	}

	if err := ref.service.UnlinkRoles(ctx, input); err != nil {
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
		respond.WriteInternalError(w, r, e)
		return
	}

	slog.Debug("handler.Users.unlinkRoles", "user.id", userID.String())

	// Location header is required for RESTful APIs
	respond.SetLocation(w, r)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.UsersRoleUnlinkedFromUserSuccessfully,
		attribute.String("user.id", userID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.UsersRoleUnlinkedFromUserSuccessfully)
}

// linkProjects Link projects to user
//
//	@ID				01986f44-3a65-7a2d-8c68-8c579be0aae7
//	@Summary		Link projects to user
//	@Description	Associate multiple projects with user.
//	@Tags			Users,Projects
//	@Accept			json
//	@Produce		json
//	@Param			user_id	path		string								true	"User unique identifier"	Format(uuid)
//	@Param			user	body		payload.LinkProjectsToUserRequest	true	"Project IDs to link"
//	@Success		200		{object}	payload.HTTPMessage					"Projects linked to user successfully"
//	@Failure		400		{object}	payload.HTTPMessage					"Invalid user ID or request body"
//	@Failure		401		{object}	payload.HTTPMessage					"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage					"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage					"User not found"
//	@Failure		409		{object}	payload.HTTPMessage					"One or more projects already linked to user"
//	@Failure		413		{object}	payload.HTTPMessage					"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage					"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage					"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage					"Internal server error during project linking"
//	@Router			/users/{user_id}/projects [post]
//	@Security		AccessToken
func (ref *UsersHandler) linkProjects(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "linkProjects")
	defer span.End()

	userID, err := parseUUIDQueryParams(r.PathValue("user_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.LinkProjectsToUserRequest
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

	input := &domain.LinkProjectsToUserInput{
		UserID:     userID,
		ProjectIDs: req.ProjectIDs,
	}

	if err := ref.service.LinkProjects(ctx, input); err != nil {
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

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	// Location header is required for RESTful APIs
	respond.SetLocation(w, r)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.UsersProjectsLinkedToUserSuccessfully,
		attribute.String("user.id", userID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.UsersProjectsLinkedToUserSuccessfully)
}

// unlinkProjects Unlink projects from user
//
//	@ID				01986f44-3a65-7a25-afe8-fdd6ae4572c4
//	@Summary		Unlink projects from user
//	@Description	Remove project associations from user.
//	@Tags			Users,Projects
//	@Accept			json
//	@Produce		json
//	@Param			user_id	path		string									true	"User unique identifier"	Format(uuid)
//	@Param			body	body		payload.UnlinkProjectsFromUserRequest	true	"Project IDs to unlink"
//	@Success		200		{object}	payload.HTTPMessage						"Projects unlinked from user successfully"
//	@Failure		400		{object}	payload.HTTPMessage						"Invalid user ID or request body"
//	@Failure		401		{object}	payload.HTTPMessage						"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage						"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage						"User not found"
//	@Failure		413		{object}	payload.HTTPMessage						"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage						"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage						"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage						"Internal server error during project unlinking"
//	@Router			/users/{user_id}/projects [delete]
//	@Security		AccessToken
func (ref *UsersHandler) unlinkProjects(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "unlinkProjects")
	defer span.End()

	userID, err := parseUUIDQueryParams(r.PathValue("user_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.UnlinkProjectsFromUserRequest
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

	input := &domain.UnlinkProjectsFromUserInput{
		UserID:     userID,
		ProjectIDs: req.ProjectIDs,
	}

	if err := ref.service.UnlinkProjects(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	slog.Debug("handler.Users.unlinkProjects", "user.id", userID.String())

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.UsersProjectsUnlinkedFromUserSuccessfully,
		attribute.String("user.id", userID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.UsersProjectsUnlinkedFromUserSuccessfully)
}

// selectAuthz Get user authorization
//
//	@ID				01982303-f0fa-7089-9875-cd42f8e1a3d6
//	@Summary		Get user authorization
//	@Description	Retrieve user authorization permissions and roles for access control decisions.
//	@Tags			Users,Authorization
//	@Produce		json
//	@Param			user_id	path		string						true	"User unique identifier"	Format(uuid)
//	@Success		200		{object}	payload.UserAuthzResponse	"The user's effective permissions, keyed by category. Note the extra nesting compared with /me/authz, which strips the outer \"permissions\" level"
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid user ID format"
//	@Failure		401		{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage			"User not found"
//	@Failure		429		{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/users/{user_id}/authz [get]
//	@Security		AccessToken
func (ref *UsersHandler) selectAuthz(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "selectAuthz")
	defer span.End()

	userID, err := parseUUIDQueryParams(r.PathValue("user_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	outResponse, err := ref.service.SelectAuthz(ctx, userID)
	if err != nil {
		if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	// Typed for the wire, and only for the wire: the map stays generic through
	// the core because OPA takes it as Rego input.
	//
	// A shape this does not recognise is logged and sent as-is rather than
	// failed. The endpoint's job is to answer what a user may do; refusing to
	// answer because the body could not be typed would be the wrong trade, and
	// the WARN is what makes the drift visible instead of silent.
	var body any = outResponse

	if typed, err := payload.NewUserAuthzResponse(outResponse); err == nil {
		body = typed
	} else {
		slog.Warn("handler.Users.selectAuthz: unrecognised permission shape, sending it untyped",
			"user.id", userID.String(), "error", err)
	}

	if err := respond.WriteJSONData(w, http.StatusOK, body); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	slog.Debug("handler.Users.selectAuthz", "user.id", userID.String())

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "User authorization retrieved",
		attribute.String("user.id", userID.String()))
}

// resetPassword Send a password reset email
//
//	@ID				01a07662-d5ca-7c8f-aa04-0a29d6cad3bd
//	@Summary		Send password reset email
//	@Description	Email the account holder a password-reset link. PUT /users/{user_id} used to accept a new password outright, so a grant on it was a takeover of any account; a reset link needs the mailbox as well. Answers 202 whether or not the address can be reached, and says nothing about why.
//	@Tags			Users
//	@Produce		json
//	@Param			user_id	path		string				true	"User unique identifier"	Format(uuid)
//	@Success		202		{object}	payload.HTTPMessage	"Reset email requested"
//	@Failure		400		{object}	payload.HTTPMessage	"Invalid user ID format"
//	@Failure		401		{object}	payload.HTTPMessage	"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage	"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage	"User not found"
//	@Failure		429		{object}	payload.HTTPMessage	"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage	"Internal server error"
//	@Router			/users/{user_id}/password/reset [post]
//	@Security		AccessToken
func (ref *UsersHandler) resetPassword(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "resetPassword")
	defer span.End()

	userID, err := parseUUIDQueryParams(r.PathValue("user_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	user, err := ref.service.GetByID(ctx, userID)
	if err != nil {
		if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	// RecoverPassword answers alike for a disabled or federated account; the
	// reason is on its span and in its log, not in this response.
	if err := ref.recoverer.RecoverPassword(ctx, &domain.RecoverPasswordInput{Email: user.Email}); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "password reset requested",
		attribute.String("user.id", userID.String()),
	)
	respond.WriteJSONMessage(w, r, http.StatusAccepted, "password reset email requested")
}
