package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"uuid"

	"go.opentelemetry.io/otel/attribute"

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
	// RecoverPasswordOf, narrowed to the one method this handler needs.
	Recoverer     PasswordRecoverer
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// PasswordRecoverer sends an account a reset link, to the address its row
// has, saying nothing about whether one could be sent.
type PasswordRecoverer interface {
	RecoverPasswordOf(ctx context.Context, userID uuid.UUID) error
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

	metrics, err := o11y.NewLayerMetrics(ref.ot.Metrics.Meter, ref.metricsPrefix)
	if err != nil {
		return nil, err
	}

	ref.metrics = metrics

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
	mux.Handle("GET /auth/idps/{idp_id}/identities", mdw.ThenFunc(ref.listByIDPID))

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
		ID:              out.ID,
		CreatedAt:       out.CreatedAt,
		UpdatedAt:       out.UpdatedAt,
		Disabled:        out.Disabled,
		DisabledByAdmin: out.DisabledByAdmin,
		Verified:        out.Verified,
		Admin:           out.Admin,
		LocalAccount:    out.LocalAccount,
		FirstName:       out.FirstName,
		LastName:        out.LastName,
		Email:           out.Email,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, userResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user retrieved",
		attribute.String("user.id", userID.String()),
	)

	slog.DebugContext(r.Context(), "handler.Users.getByID", "user.email", out.Email)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.UsersUserFound,
		attribute.String("user.id", out.ID.String()),
		attribute.String("user.email", out.Email),
	)
}

// create Create a new user
//
//	@ID				01982303-f0f9-7e78-bddf-389144c4beaf
//	@Summary		Create user
//	@Description	Create a new user account with specified details. It is stored disabled, for an administrator to enable, and verified: the administrator vouches for the address and no verification mail is sent.
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
		ID:        req.ID,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Email:     req.Email,
		Password:  req.Password,
		// Stored disabled, and enabled by an administrator: no mail is sent
		// for an account made here. And stored verified: the administrator
		// vouches for the address, as an identity provider does for its
		// subject, so the public verification link does not apply to it. The
		// row says whose doing its `disabled` is, so that no link opens it
		// whatever else changes.
		Disabled:        new(true),
		DisabledByAdmin: true,
		Verified:        new(true),
		LocalAccount:    new(true),
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
//	@Description	Update a user account by ID. Only the administrator account (the admin flag, not the Administrator role) changes an e-mail address, grants or takes the admin flag, or changes disabled, disabled_by_admin or local_account on an account that has the flag; anyone else gets a 403. Values equal to the stored ones are accepted.
//	@Tags			Users
//	@Accept			json
//	@Produce		json
//	@Param			user_id	path		string						true	"User unique identifier"	Format(uuid)
//	@Param			body	body		payload.UpdateUserRequest	true	"User update details including name, email, or profile changes"
//	@Success		200		{object}	payload.HTTPMessage			"User updated successfully"
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid user ID or request body; or disabled and disabled_by_admin changing the account in two directions"
//	@Failure		401		{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage			"Insufficient permissions; a change of e-mail address or of the admin flag, or of disabled, disabled_by_admin or local_account on an administrator account, by an account that is not one; the last enabled administrator account; or the seeded account's admin flag"
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

	// The body may carry an address, and a change of address is held to
	// who asks.
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

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
		ID:              userID,
		CallerID:        callerID,
		FirstName:       req.FirstName,
		LastName:        req.LastName,
		Email:           req.Email,
		Disabled:        req.Disabled,
		DisabledByAdmin: req.DisabledByAdmin,
		LocalAccount:    req.LocalAccount,
		Admin:           req.Admin,
	}

	if err := ref.service.UpdateByID(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.UserAlreadyExistsError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.AdministratorOnlyError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusForbidden, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.AdministratorProtectedError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusForbidden, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}

		// A body that pulls the account in two directions: disabled and the
		// administrator's lock (UsersService.lockMayChange). That refusal
		// alone: any other InvalidInputError from here is a fault.
		if refusal, ok := errors.AsType[*domain.InvalidInputError](err); ok && refusal.Field == domain.FieldDisabledByAdmin {
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
	respond.SetLocation(w, r)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.UsersUserUpdatedSuccessfully,
		attribute.String("user.id", input.ID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.UsersUserUpdatedSuccessfully)
}

// deleteByID Delete a user
//
//	@ID				01982303-f0f9-7dda-84da-cedd68bca775
//	@Summary		Delete user
//	@Description	Permanently remove user account by ID. An account with the admin flag is deleted only by one that has it, the last one that can sign in by nobody, and the seeded administrator by nobody.
//	@Tags			Users
//	@Produce		json
//	@Param			user_id	path		string				true	"User unique identifier"	Format(uuid)
//	@Success		200		{object}	payload.HTTPMessage	"User deleted successfully"
//	@Failure		400		{object}	payload.HTTPMessage	"Invalid user ID format"
//	@Failure		401		{object}	payload.HTTPMessage	"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage	"Insufficient permissions; or an administrator account, by an account that is not one; the last enabled one; or the seeded administrator"
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

	// Who asks: an account with the `admin` flag is deleted only by one that
	// has it. A request that reaches here with no caller is a route wired
	// without its middleware.
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	input := &domain.DeleteUserInput{
		ID:       userID,
		CallerID: callerID,
	}

	if err := ref.service.DeleteByID(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.AdministratorProtectedError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusForbidden, e.Error())
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
//	@Param			filter		query		string						false	"Filter expression. Example: first_name='John' AND disabled=false"
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
		writeListError(w, r, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs))
		return
	}

	outResponse := &payload.ListUsersResponse{
		Items:     make([]payload.UserResponse, len(outList.Items)),
		Paginator: outList.Paginator,
	}

	for i, user := range outList.Items {
		outResponse.Items[i] = payload.UserResponse{
			ID:              user.ID,
			CreatedAt:       user.CreatedAt,
			UpdatedAt:       user.UpdatedAt,
			Disabled:        user.Disabled,
			DisabledByAdmin: user.DisabledByAdmin,
			Verified:        user.Verified,
			Admin:           user.Admin,
			LocalAccount:    user.LocalAccount,
			FirstName:       user.FirstName,
			LastName:        user.LastName,
			Email:           user.Email,
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
//	@Param			filter		query		string						false	"Filter expression. Example: first_name='John' AND disabled=false"
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
		writeListError(w, r, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs))
		return
	}

	outResponse := &payload.ListUsersResponse{
		Items:     make([]payload.UserResponse, len(outList.Items)),
		Paginator: outList.Paginator,
	}

	for i, user := range outList.Items {
		outResponse.Items[i] = payload.UserResponse{
			ID:              user.ID,
			CreatedAt:       user.CreatedAt,
			UpdatedAt:       user.UpdatedAt,
			Disabled:        user.Disabled,
			DisabledByAdmin: user.DisabledByAdmin,
			Verified:        user.Verified,
			Admin:           user.Admin,
			LocalAccount:    user.LocalAccount,
			FirstName:       user.FirstName,
			LastName:        user.LastName,
			Email:           user.Email,
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

// listByIDPID List the accounts linked to an identity provider
//
//	@ID				01a15479-0c52-7737-a03d-3910bf262442
//	@Summary		List a provider's accounts
//	@Description	Retrieve paginated list of the accounts linked to an Identity Provider, each with when it was linked and only_way_in: true for the accounts with no password and an identity at no other provider, which keep it from being deleted (409). The subject is not shown.
//	@Tags			Identity Providers (IDPs),Users
//	@Produce		json
//	@Param			idp_id		path		string							true	"Identity Provider UUID"	Format(uuid)
//	@Param			sort		query		string							false	"Sort fields of the account (comma-separated). Example: first_name ASC, created_at DESC"
//	@Param			filter		query		string							false	"Filter expression on the account's fields. Example: first_name='John' AND disabled=false"
//	@Param			fields		query		string							false	"The account's fields to return (comma-separated); linked_at and only_way_in are always returned. Example: id,email"
//	@Param			next_token	query		string							false	"Next page cursor for pagination"
//	@Param			prev_token	query		string							false	"Previous page cursor for pagination"
//	@Param			limit		query		int								false	"Maximum number of results per page"
//	@Success		200			{object}	payload.ListIDPAccountsResponse	"Paginated list of the accounts linked to the provider"
//	@Failure		400			{object}	payload.HTTPMessage				"Invalid provider ID or query parameters"
//	@Failure		401			{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage				"IDP not found"
//	@Failure		429			{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/auth/idps/{idp_id}/identities [get]
//	@Security		AccessToken
func (ref *UsersHandler) listByIDPID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "listByIDPID")
	defer span.End()

	idpID, err := parseUUIDQueryParams(r.PathValue("idp_id"))
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

	span.SetAttributes(attribute.String("idp.id", idpID.String()))

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

	outList, err := ref.service.ListByIDPID(ctx, idpID, input)
	if err != nil {
		// A provider that is not there is a 404, which the lists' own
		// writeListError does not know: a read of what a parent holds says
		// when there is no parent.
		if isErr[*domain.IDPNotFoundError](err) {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}

		writeListError(w, r, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs))
		return
	}

	outResponse := &payload.ListIDPAccountsResponse{
		Items:     make([]payload.IDPAccountResponse, len(outList.Items)),
		Paginator: outList.Paginator,
	}

	for i, account := range outList.Items {
		user := account.User
		outResponse.Items[i] = payload.IDPAccountResponse{
			ID:              user.ID,
			CreatedAt:       user.CreatedAt,
			UpdatedAt:       user.UpdatedAt,
			Disabled:        user.Disabled,
			DisabledByAdmin: user.DisabledByAdmin,
			Verified:        user.Verified,
			Admin:           user.Admin,
			LocalAccount:    user.LocalAccount,
			FirstName:       user.FirstName,
			LastName:        user.LastName,
			Email:           user.Email,
			LinkedAt:        account.LinkedAt,
			OnlyWayIn:       account.OnlyWayIn,
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

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "a provider's accounts listed",
		attribute.Int("accounts.count", len(outResponse.Items)),
		attribute.String("idp.id", idpID.String()))
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
//	@Param			filter		query		string						false	"Filter expression. Example: first_name='John' AND disabled=false"
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
		writeListError(w, r, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs))
		return
	}

	outResponse := &payload.ListUsersResponse{
		Items:     make([]payload.UserResponse, len(outList.Items)),
		Paginator: outList.Paginator,
	}

	for i, user := range outList.Items {
		outResponse.Items[i] = payload.UserResponse{
			ID:              user.ID,
			CreatedAt:       user.CreatedAt,
			UpdatedAt:       user.UpdatedAt,
			Disabled:        user.Disabled,
			DisabledByAdmin: user.DisabledByAdmin,
			Verified:        user.Verified,
			Admin:           user.Admin,
			LocalAccount:    user.LocalAccount,
			FirstName:       user.FirstName,
			LastName:        user.LastName,
			Email:           user.Email,
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
		respond.WriteInternalError(w, r, e)
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

	slog.DebugContext(r.Context(), "handler.Users.linkRoles", "user.id", userID.String())

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
//	@Description	Remove role associations from user within project. An account with the admin flag is stripped of a role only by an account that has the flag.
//	@Tags			Users,Roles
//	@Accept			json
//	@Produce		json
//	@Param			user_id	path		string								true	"User unique identifier"	Format(uuid)
//	@Param			body	body		payload.UnlinkRolesFromUserRequest	true	"Role IDs to unlink with project context"
//	@Success		200		{object}	payload.HTTPMessage					"Roles unlinked from user successfully"
//	@Failure		400		{object}	payload.HTTPMessage					"Invalid user ID or request body"
//	@Failure		401		{object}	payload.HTTPMessage					"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage					"Insufficient permissions; or an administrator account, by an account that is not one"
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

	// Who asks: an account with the `admin` flag is stripped of a role only by one that
	// has it. A request that reaches here with no caller is a route wired
	// without its middleware.
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	input := &domain.UnlinkRolesFromUsersInput{
		UserID:   userID,
		CallerID: callerID,
		RoleIDs:  req.RoleIDs,
	}

	if err := ref.service.UnlinkRoles(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.AdministratorProtectedError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusForbidden, e.Error())
			return
		}

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

	slog.DebugContext(r.Context(), "handler.Users.unlinkRoles", "user.id", userID.String())

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
//	@Description	Associate projects with a user. A caller links a user only to a project the caller is linked to; another account's project and one that does not exist are the same 400.
//	@Tags			Users,Projects
//	@Accept			json
//	@Produce		json
//	@Param			user_id	path		string								true	"User unique identifier"	Format(uuid)
//	@Param			user	body		payload.LinkProjectsToUserRequest	true	"Project IDs to link"
//	@Success		200		{object}	payload.HTTPMessage					"Projects linked to user successfully"
//	@Failure		400		{object}	payload.HTTPMessage					"Invalid user ID or request body, or a project that is not one of the caller's"
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

	// The projects come from the body, and are held to who asks.
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

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
		CallerID:   callerID,
		ProjectIDs: req.ProjectIDs,
	}

	if err := ref.service.LinkProjects(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.ProjectNotOfCallerError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

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
//	@Description	Remove project associations from user. A caller unlinks a user only from a project the caller is linked to; another account's project and one that does not exist are the same 400.
//	@Tags			Users,Projects
//	@Accept			json
//	@Produce		json
//	@Param			user_id	path		string									true	"User unique identifier"	Format(uuid)
//	@Param			body	body		payload.UnlinkProjectsFromUserRequest	true	"Project IDs to unlink"
//	@Success		200		{object}	payload.HTTPMessage						"Projects unlinked from user successfully"
//	@Failure		400		{object}	payload.HTTPMessage						"Invalid user ID or request body, or a project that is not one of the caller's"
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

	// The projects come from the body, and are held to who asks.
	callerID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

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
		CallerID:   callerID,
		ProjectIDs: req.ProjectIDs,
	}

	if err := ref.service.UnlinkProjects(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.ProjectNotOfCallerError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
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

	slog.DebugContext(r.Context(), "handler.Users.unlinkProjects", "user.id", userID.String())

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
		slog.WarnContext(r.Context(), "unrecognised permission shape, sending it untyped",
			"user.id", userID.String(), "error", err)
	}

	if err := respond.WriteJSONData(w, http.StatusOK, body); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	slog.DebugContext(r.Context(), "handler.Users.selectAuthz", "user.id", userID.String())

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "User authorization retrieved",
		attribute.String("user.id", userID.String()))
}

// resetPassword Send a password reset email
//
//	@ID				01a07662-d5ca-7c8f-aa04-0a29d6cad3bd
//	@Summary		Send password reset email
//	@Description	Email the account holder a password-reset link, to the address it has now. PUT /users/{user_id} used to accept a new password outright, so a grant on it was a takeover of any account; a reset link needs the mailbox as well. Answers 202 whether or not the address can be reached, and says nothing about why.
//	@Tags			Users
//	@Produce		json
//	@Param			user_id	path		string				true	"User unique identifier"	Format(uuid)
//	@Success		202		{object}	payload.HTTPMessage	"Reset email requested"
//	@Failure		400		{object}	payload.HTTPMessage	"Invalid user ID format, or a stored address the recovery refuses"
//	@Failure		401		{object}	payload.HTTPMessage	"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage	"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage	"User not found"
//	@Failure		429		{object}	payload.HTTPMessage	"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store; or the address's recovery budget is spent"
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

	// The recovery reads the account and its address from the repository,
	// not from the cache: a corrected address is the one mailed. It answers
	// alike for a disabled or federated account; the reason is on its span
	// and in its log, not in this response. An account that is not there is
	// its 404.
	if err := ref.recoverer.RecoverPasswordOf(ctx, userID); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)

		switch {
		case isErr[*domain.UserNotFoundError](err):
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
		case isErr[*domain.TooManyRecoveryRequestsError](err):
			// The address's budget is the public route's; so is the header
			// that says when it is back.
			throttled, _ := errors.AsType[*domain.TooManyRecoveryRequestsError](err)
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(throttled.RetryAfter)))
			respond.WriteJSONMessage(w, r, http.StatusTooManyRequests, e.Error())
		case isErr[*domain.ValidationError](err), isErr[*domain.ValidationErrors](err), isErr[*domain.InvalidInputError](err):
			// A stored address the recovery's rule refuses is ValidateEmail's
			// ValidationError, singular.
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		default:
			respond.WriteInternalError(w, r, e)
		}

		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "password reset requested",
		attribute.String("user.id", userID.String()),
	)
	respond.WriteJSONMessage(w, r, http.StatusAccepted, "password reset email requested")
}
