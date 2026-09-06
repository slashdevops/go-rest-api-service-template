package handler

import (
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

// RolesHandlerConf represents the handler for the roles.
type RolesHandlerConf struct {
	Service       driving.Roles
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// RolesHandler represents the handler for the roles.
type RolesHandler struct {
	service         driving.Roles
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewRolesHandler creates a new roleHandler.
func NewRolesHandler(conf RolesHandlerConf) (*RolesHandler, error) {
	if conf.Service == nil {
		return nil, &domain.InvalidServiceError{Message: "driving.Roles is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &RolesHandler{
		service:       conf.Service,
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Roles",
			Action: "NewRolesHandler",
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
func (ref *RolesHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /roles", mdw.ThenFunc(ref.list))
	mux.Handle("GET /roles/{role_id}", mdw.ThenFunc(ref.getByID))
	mux.Handle("POST /roles", mdw.ThenFunc(ref.create))
	mux.Handle("PUT /roles/{role_id}", mdw.ThenFunc(ref.updateByID))
	mux.Handle("DELETE /roles/{role_id}", mdw.ThenFunc(ref.deleteByID))

	// link/unlink role to users
	mux.Handle("POST /roles/{role_id}/users", mdw.ThenFunc(ref.linkUsers))
	mux.Handle("DELETE /roles/{role_id}/users", mdw.ThenFunc(ref.unlinkUsers))

	// Link and unlink policies to/from a role
	mux.Handle("POST /roles/{role_id}/policies", mdw.ThenFunc(ref.linkPolicies))
	mux.Handle("DELETE /roles/{role_id}/policies", mdw.ThenFunc(ref.unlinkPolicies))

	// list roles by user id
	mux.Handle("GET /users/{user_id}/roles", mdw.ThenFunc(ref.listByUserID))

	// list roles by policy id
	mux.Handle("GET /policies/{policy_id}/roles", mdw.ThenFunc(ref.listByPolicyID))
}

// getByID Get a role by its ID
//
//	@ID				01982303-f0f9-7dde-ab91-1ab138a8b6c5
//	@Summary		Get role
//	@Description	Retrieve role configuration by unique identifier
//	@Tags			Roles
//	@Accept			json
//	@Produce		json
//	@Param			role_id	path		string					true	"Role unique identifier"	Format(uuid)
//	@Success		200		{object}	payload.RoleResponse	"Role retrieved successfully"
//	@Failure		400		{object}	payload.HTTPMessage		"Invalid role ID format or malformed request"
//	@Failure		401		{object}	payload.HTTPMessage		"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage		"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage		"Role not found"
//	@Failure		429		{object}	payload.HTTPMessage		"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage		"Internal server error"
//	@Router			/roles/{role_id} [get]
//	@Security		AccessToken
func (ref *RolesHandler) getByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "getByID")
	defer span.End()

	roleID, err := parseUUIDQueryParams(r.PathValue("role_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	out, err := ref.service.GetByID(ctx, roleID)
	if err != nil {
		if _, ok := errors.AsType[*domain.RoleNotFoundError](err); ok {
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

	outResponse := &payload.RoleResponse{
		CreatedAt:   out.CreatedAt,
		UpdatedAt:   out.UpdatedAt,
		System:      out.System,
		AutoAssign:  out.AutoAssign,
		Name:        out.Name,
		Description: out.Description,
		ID:          out.ID,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	slog.Debug("handler.Roles.getByID: called", "role.id", outResponse.ID.String())
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "get role",
		attribute.String("role.id", outResponse.ID.String()))
}

// create Create a role
//
//	@ID				01982303-f0f9-7eff-825d-622a05ef4435
//	@Summary		Create role
//	@Description	Create new role with specified permissions and access levels
//	@Tags			Roles
//	@Accept			json
//	@Produce		json
//	@Param			body	body		payload.CreateRoleRequest	true	"Role creation payload"
//	@Success		201		{object}	payload.HTTPMessage			"Role created successfully"
//	@Header			201		{string}	Location					"URL of created role resource"
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid request payload or validation failed"
//	@Failure		401		{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		409		{object}	payload.HTTPMessage			"Role already exists"
//	@Failure		413		{object}	payload.HTTPMessage			"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage			"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/roles [post]
//	@Security		AccessToken
func (ref *RolesHandler) create(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "create")
	defer span.End()

	var req payload.CreateRoleRequest
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

	input := &domain.CreateRoleInput{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
	}

	if err := ref.service.Create(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.RoleAlreadyExistsError](err); ok {
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

	slog.Debug("handler.Roles.create", "name", input.Name)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Role created", attribute.String("role.id", input.ID.String()))

	// Location header is required for RESTful APIs
	respond.SetLocation(w, r, input.ID.String())
	respond.WriteJSONMessage(w, r, http.StatusCreated, domain.RolesRoleCreatedSuccessfully)
}

// updateByID Update a role
//
//	@ID				01982303-f0f9-7e92-bd69-076fc1cd4a6e
//	@Summary		Update role
//	@Description	Update existing role configuration by identifier
//	@Tags			Roles
//	@Accept			json
//	@Produce		json
//	@Param			role_id	path		string						true	"Role unique identifier"	Format(uuid)
//	@Param			body	body		payload.UpdateRoleRequest	true	"Role update payload"
//	@Success		200		{object}	payload.HTTPMessage			"Role updated successfully"
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid request payload or validation failed"
//	@Failure		401		{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage			"Role not found"
//	@Failure		409		{object}	payload.HTTPMessage			"Role with same name already exists"
//	@Failure		413		{object}	payload.HTTPMessage			"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage			"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/roles/{role_id} [put]
//	@Security		AccessToken
func (ref *RolesHandler) updateByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "updateByID")
	defer span.End()

	roleID, err := parseUUIDQueryParams(r.PathValue("role_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.UpdateRoleRequest
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

	input := &domain.UpdateRoleInput{
		ID:          roleID,
		Name:        req.Name,
		Description: req.Description,
	}

	if err := ref.service.UpdateByID(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.RoleAlreadyExistsError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.RoleNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}
		// A system role refuses UPDATE and DELETE alike -- the shared
		// tr_restrict_delete_update_on_system_* trigger rejects both -- and that
		// is a permanent, explainable refusal, not a fault. It answered 500,
		// which tells a client to retry something that can never succeed and
		// pages whoever is on call for an error nobody can fix.
		//
		// 403 is what the other guards of this kind already return; see
		// SystemProjectError in projects.go and SystemPolicyError in policies.go.
		if _, ok := errors.AsType[*domain.SystemRoleError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusForbidden, e.Error())
			return
		}

		_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
		_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
		_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
		_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
		if isInvalidMsgFmt || isInvalidByteSeq || isUndefCol || isDtMismatch {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	slog.Debug("handler.Roles.updateByID", "role.id", input.ID.String())
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Role updated",
		attribute.String("role.id", input.ID.String()))

	// Location header is required for RESTful APIs
	respond.SetLocation(w, r)
	respond.WriteJSONMessage(w, r, http.StatusOK, domain.RolesRoleUpdatedSuccessfully)
}

// deleteByID Delete a role
//
//	@ID				01982303-f0fa-7007-aaf6-462c0b8702ec
//	@Summary		Delete role
//	@Description	Permanently remove role and all associations
//	@Tags			Roles
//	@Accept			json
//	@Produce		json
//	@Param			role_id	path		string				true	"Role unique identifier"	Format(uuid)
//	@Success		200		{object}	payload.HTTPMessage	"Role deleted successfully"
//	@Failure		400		{object}	payload.HTTPMessage	"Invalid role ID format"
//	@Failure		401		{object}	payload.HTTPMessage	"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage	"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage	"Role not found"
//	@Failure		429		{object}	payload.HTTPMessage	"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage	"Internal server error"
//	@Router			/roles/{role_id} [delete]
//	@Security		AccessToken
func (ref *RolesHandler) deleteByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "deleteByID")
	defer span.End()

	roleID, err := parseUUIDQueryParams(r.PathValue("role_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	input := &domain.DeleteRoleInput{
		ID: roleID,
	}

	if err := ref.service.DeleteByID(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.RoleNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}
		// A system role refuses UPDATE and DELETE alike -- the shared
		// tr_restrict_delete_update_on_system_* trigger rejects both -- and that
		// is a permanent, explainable refusal, not a fault. It answered 500,
		// which tells a client to retry something that can never succeed and
		// pages whoever is on call for an error nobody can fix.
		//
		// 403 is what the other guards of this kind already return; see
		// SystemProjectError in projects.go and SystemPolicyError in policies.go.
		if _, ok := errors.AsType[*domain.SystemRoleError](err); ok {
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

	slog.Debug("handler.Roles.deleteByID", "id", input.ID.String())
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Role deleted",
		attribute.String("role.id", input.ID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.RolesRoleDeletedSuccessfully)
}

// list Retrieves a paginated list of all the roles in the system
//
//	@ID				01982303-f0fa-703a-92fa-be272044b2e3
//	@Summary		List roles
//	@Description	Retrieve paginated list of roles with filtering and sorting
//	@Tags			Roles
//	@Accept			json
//	@Produce		json
//	@Param			sort		query		string						false	"Comma-separated sort fields with direction (e.g., name ASC, created_at DESC)"
//	@Param			filter		query		string						false	"Filter expression (e.g., name='admin' AND system=true)"
//	@Param			fields		query		string						false	"Comma-separated field names to include in response"
//	@Param			next_token	query		string						false	"Pagination token for next page"
//	@Param			prev_token	query		string						false	"Pagination token for previous page"
//	@Param			limit		query		int							false	"Maximum number of items per page"
//	@Success		200			{object}	payload.ListRolesResponse	"Paginated list of roles retrieved successfully"
//	@Failure		400			{object}	payload.HTTPMessage			"Invalid query parameters or pagination tokens"
//	@Failure		401			{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/roles [get]
//	@Security		AccessToken
func (ref *RolesHandler) list(w http.ResponseWriter, r *http.Request) {
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

	input := &domain.ListRolesInput{
		Sort:   sort,
		Filter: filter,
		Fields: fields,
		Paginator: domain.Paginator{
			NextToken: nextToken,
			PrevToken: prevToken,
			Limit:     limit,
		},
	}

	out, err := ref.service.List(ctx, input)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, httpStatusForListError(err), e.Error())
		return
	}

	outResponse := &payload.ListRolesResponse{
		Items:     make([]payload.RoleResponse, len(out.Items)),
		Paginator: out.Paginator,
	}

	for i, role := range out.Items {
		outResponse.Items[i] = payload.RoleResponse{
			CreatedAt:   role.CreatedAt,
			UpdatedAt:   role.UpdatedAt,
			System:      role.System,
			AutoAssign:  role.AutoAssign,
			Name:        role.Name,
			Description: role.Description,
			ID:          role.ID,
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

	slog.Debug("handler.Roles.list: called", "roles.count", len(outResponse.Items))
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "list role",
		attribute.Int("roles.count", len(outResponse.Items)))
}

// linkUsers Link users to a role
//
//	@ID				01982303-f0f9-7e2c-a026-5aec9fbbe375
//	@Summary		Link users to role
//	@Description	Associate multiple users with role for authorization
//	@Tags			Roles,Users
//	@Accept			json
//	@Produce		json
//	@Param			role_id	path		string							true	"Role unique identifier"	Format(uuid)
//	@Param			body	body		payload.LinkUsersToRoleRequest	true	"User IDs to link with role"
//	@Success		200		{object}	payload.HTTPMessage				"Users linked to role successfully"
//	@Failure		400		{object}	payload.HTTPMessage				"Invalid request payload or role ID format"
//	@Failure		401		{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage				"Role not found"
//	@Failure		413		{object}	payload.HTTPMessage				"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage				"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/roles/{role_id}/users [post]
//	@Security		AccessToken
func (ref *RolesHandler) linkUsers(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "linkUsers")
	defer span.End()

	roleID, err := parseUUIDQueryParams(r.PathValue("role_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.LinkUsersToRoleRequest
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

	input := &domain.LinkUsersToRoleInput{
		CallerID: callerID,
		RoleID:   roleID,
		UserIDs:  req.UserIDs,
	}

	if err := ref.service.LinkUsers(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.RoleNotFoundError](err); ok {
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

	slog.Debug("handler.Roles.linkUsers", "role.id", input.RoleID)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Users linked to role",
		attribute.String("role.id", input.RoleID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.RolesUsersLinkedSuccessfully)
}

// unlinkUsers Unlink users from a role
//
//	@ID				01982303-f0f9-7e02-8bf7-c240927de056
//	@Summary		Unlink users from role
//	@Description	Remove user associations from role
//	@Tags			Roles,Users
//	@Accept			json
//	@Produce		json
//	@Param			role_id	path		string								true	"Role unique identifier"	Format(uuid)
//	@Param			body	body		payload.UnlinkUsersFromRoleRequest	true	"User IDs to unlink from role"
//	@Success		200		{object}	payload.HTTPMessage					"Users unlinked from role successfully"
//	@Failure		400		{object}	payload.HTTPMessage					"Invalid request payload or role ID format"
//	@Failure		401		{object}	payload.HTTPMessage					"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage					"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage					"Role not found"
//	@Failure		413		{object}	payload.HTTPMessage					"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage					"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage					"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage					"Internal server error"
//	@Router			/roles/{role_id}/users [delete]
//	@Security		AccessToken
func (ref *RolesHandler) unlinkUsers(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "unlinkUsers")
	defer span.End()

	roleID, err := parseUUIDQueryParams(r.PathValue("role_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.UnlinkUsersFromRoleRequest
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

	input := &domain.UnlinkUsersFromRoleInput{
		RoleID:  roleID,
		UserIDs: req.UserIDs,
	}

	if err := ref.service.UnlinkUsers(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.RoleNotFoundError](err); ok {
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

	slog.Debug("handler.Roles.unlinkUsers", "role.id", input.RoleID)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Users unlinked from role",
		attribute.String("role.id", input.RoleID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.RolesUsersUnlinkedSuccessfully)
}

// linkPolicies Link policies to a role
//
//	@ID				01982303-f0f9-7edc-bbff-c8fc5dcba075
//	@Summary		Link policies to role
//	@Description	Associate multiple policies with role for authorization
//	@Tags			Roles,Policies
//	@Accept			json
//	@Produce		json
//	@Param			role_id	path		string								true	"Role unique identifier"	Format(uuid)
//	@Param			body	body		payload.LinkPoliciesToRoleRequest	true	"Policy IDs to link with role"
//	@Success		200		{object}	payload.HTTPMessage					"Policies linked to role successfully"
//	@Failure		400		{object}	payload.HTTPMessage					"Invalid request payload or role ID format"
//	@Failure		401		{object}	payload.HTTPMessage					"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage					"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage					"Role not found"
//	@Failure		413		{object}	payload.HTTPMessage					"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage					"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage					"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage					"Internal server error"
//	@Router			/roles/{role_id}/policies [post]
//	@Security		AccessToken
func (ref *RolesHandler) linkPolicies(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "linkPolicies")
	defer span.End()

	roleID, err := parseUUIDQueryParams(r.PathValue("role_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.LinkPoliciesToRoleRequest
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

	input := &domain.LinkPoliciesToRoleInput{
		CallerID:  callerID,
		RoleID:    roleID,
		PolicyIDs: req.PolicyIDs,
	}

	if err := ref.service.LinkPolicies(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.RoleNotFoundError](err); ok {
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

	slog.Debug("handler.Roles.linkPolicies", "role.id", input.RoleID)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Policies linked to role",
		attribute.String("role.id", input.RoleID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.RolesPoliciesLinkedSuccessfully)
}

// unlinkPolicies Unlink policies from a role
//
//	@ID				01982303-f0f9-7efb-a786-89d7d9db40ee
//	@Summary		Unlink policies from role
//	@Description	Remove policy associations from role
//	@Tags			Roles,Policies
//	@Accept			json
//	@Produce		json
//	@Param			role_id	path		string									true	"Role unique identifier"	Format(uuid)
//	@Param			body	body		payload.UnlinkPoliciesFromRoleRequest	true	"Policy IDs to unlink from role"
//	@Success		200		{object}	payload.HTTPMessage						"Policies unlinked from role successfully"
//	@Failure		400		{object}	payload.HTTPMessage						"Invalid request payload or role ID format"
//	@Failure		401		{object}	payload.HTTPMessage						"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage						"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage						"Role not found"
//	@Failure		413		{object}	payload.HTTPMessage						"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage						"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage						"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage						"Internal server error"
//	@Router			/roles/{role_id}/policies [delete]
//	@Security		AccessToken
func (ref *RolesHandler) unlinkPolicies(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "unlinkPolicies")
	defer span.End()

	roleID, err := parseUUIDQueryParams(r.PathValue("role_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.UnlinkPoliciesFromRoleRequest
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

	input := &domain.UnlinkPoliciesFromRoleInput{
		RoleID:    roleID,
		PolicyIDs: req.PolicyIDs,
	}

	if err := ref.service.UnlinkPolicies(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.RoleNotFoundError](err); ok {
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

	slog.Debug("handler.Roles.unlinkPolicies", "role.id", input.RoleID)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Policies unlinked from role",
		attribute.String("role.id", input.RoleID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.RolesPoliciesUnlinkedSuccessfully)
}

// listByUserID List roles by user ID
//
//	@ID				01982303-f0f9-7e6b-9d17-9b9076785bd6
//	@Summary		List roles by user
//	@Description	Retrieve paginated list of roles assigned to user
//	@Tags			Roles,Users
//	@Accept			json
//	@Produce		json
//	@Param			user_id		path		string						true	"User unique identifier"	Format(uuid)
//	@Param			sort		query		string						false	"Comma-separated sort fields with direction (e.g., name ASC, created_at DESC)"
//	@Param			filter		query		string						false	"Filter expression (e.g., name='admin' AND system=true)"
//	@Param			fields		query		string						false	"Comma-separated field names to include in response"
//	@Param			next_token	query		string						false	"Pagination token for next page"
//	@Param			prev_token	query		string						false	"Pagination token for previous page"
//	@Param			limit		query		int							false	"Maximum number of items per page"
//	@Success		200			{object}	payload.ListRolesResponse	"Paginated list of user roles retrieved successfully"
//	@Failure		400			{object}	payload.HTTPMessage			"Invalid user ID or query parameters"
//	@Failure		401			{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/users/{user_id}/roles [get]
//	@Security		AccessToken
func (ref *RolesHandler) listByUserID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "listByUserID")
	defer span.End()

	userID, err := parseUUIDQueryParams(r.PathValue("user_id"))
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

	input := &domain.ListRolesInput{
		Sort:   sort,
		Filter: filter,
		Fields: fields,
		Paginator: domain.Paginator{
			NextToken: nextToken,
			PrevToken: prevToken,
			Limit:     limit,
		},
	}

	out, err := ref.service.ListByUserID(ctx, userID, input)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, httpStatusForListError(err), e.Error())
		return
	}

	outResponse := &payload.ListRolesResponse{
		Items:     make([]payload.RoleResponse, len(out.Items)),
		Paginator: out.Paginator,
	}

	for i, role := range out.Items {
		outResponse.Items[i] = payload.RoleResponse{
			CreatedAt:   role.CreatedAt,
			UpdatedAt:   role.UpdatedAt,
			System:      role.System,
			AutoAssign:  role.AutoAssign,
			Name:        role.Name,
			Description: role.Description,
			ID:          role.ID,
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

	slog.Debug("handler.Roles.listByUserID: called", "roles.count", len(outResponse.Items))
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "list role by user ID",
		attribute.Int("roles.count", len(outResponse.Items)),
		attribute.String("user.id", userID.String()))
}

// listByPolicyID List roles by policy ID
//
//	@ID				01982303-f0f9-7ef0-ad95-3cb214216ef1
//	@Summary		List roles by policy
//	@Description	Retrieve paginated list of roles associated with policy
//	@Tags			Roles,Policies
//	@Accept			json
//	@Produce		json
//	@Param			policy_id	path		string						true	"Policy unique identifier"	Format(uuid)
//	@Param			sort		query		string						false	"Comma-separated sort fields with direction (e.g., name ASC, created_at DESC)"
//	@Param			filter		query		string						false	"Filter expression (e.g., name='admin' AND system=true)"
//	@Param			fields		query		string						false	"Comma-separated field names to include in response"
//	@Param			next_token	query		string						false	"Pagination token for next page"
//	@Param			prev_token	query		string						false	"Pagination token for previous page"
//	@Param			limit		query		int							false	"Maximum number of items per page"
//	@Success		200			{object}	payload.ListRolesResponse	"Paginated list of policy roles retrieved successfully"
//	@Failure		400			{object}	payload.HTTPMessage			"Invalid policy ID or query parameters"
//	@Failure		401			{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/policies/{policy_id}/roles [get]
//	@Security		AccessToken
func (ref *RolesHandler) listByPolicyID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "listByPolicyID")
	defer span.End()

	policyID, err := parseUUIDQueryParams(r.PathValue("policy_id"))
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

	input := &domain.ListRolesInput{
		Sort:   sort,
		Filter: filter,
		Fields: fields,
		Paginator: domain.Paginator{
			NextToken: nextToken,
			PrevToken: prevToken,
			Limit:     limit,
		},
	}

	out, err := ref.service.ListByPolicyID(ctx, policyID, input)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, httpStatusForListError(err), e.Error())
		return
	}

	outResponse := &payload.ListRolesResponse{
		Items:     make([]payload.RoleResponse, len(out.Items)),
		Paginator: out.Paginator,
	}

	for i, role := range out.Items {
		outResponse.Items[i] = payload.RoleResponse{
			CreatedAt:   role.CreatedAt,
			UpdatedAt:   role.UpdatedAt,
			System:      role.System,
			AutoAssign:  role.AutoAssign,
			Name:        role.Name,
			Description: role.Description,
			ID:          role.ID,
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

	slog.Debug("handler.Roles.listByPolicyID: called", "roles.count", len(outResponse.Items))
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "list role by policy ID",
		attribute.Int("roles.count", len(out.Items)),
		attribute.String("policy.id", policyID.String()))
}
