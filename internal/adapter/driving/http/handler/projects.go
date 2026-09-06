package handler

import (
	"errors"
	"fmt"
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

// ProjectsHandlerConf represents the handler for the projects.
type ProjectsHandlerConf struct {
	Service       driving.Projects
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// ProjectsHandler represents the handler for the projects.
type ProjectsHandler struct {
	service         driving.Projects
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewProjectsHandler creates a new projectHandler.
func NewProjectsHandler(conf ProjectsHandlerConf) (*ProjectsHandler, error) {
	if conf.Service == nil {
		return nil, &domain.InvalidServiceError{Message: "driving.Projects is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &ProjectsHandler{
		service:       conf.Service,
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Projects",
			Action: "NewProjectsHandler",
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
func (ref *ProjectsHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /projects", mdw.ThenFunc(ref.list))
	mux.Handle("GET /projects/{project_id}", mdw.ThenFunc(ref.getByIDByUserID))
	mux.Handle("PUT /projects/{project_id}", mdw.ThenFunc(ref.updateByID))
	mux.Handle("DELETE /projects/{project_id}", mdw.ThenFunc(ref.deleteByID))
	mux.Handle("POST /projects", mdw.ThenFunc(ref.create))

	mux.Handle("POST /projects/{project_id}/users", mdw.ThenFunc(ref.linkUsers))
	mux.Handle("DELETE /projects/{project_id}/users", mdw.ThenFunc(ref.unlinkUsers))

	mux.Handle("GET /users/{user_id}/projects", mdw.ThenFunc(ref.listByUserID))
}

// create Create a project
//
//	@ID				01982303-f0f9-7e63-92ba-141813745a7d
//	@Summary		Create project
//	@Description	Create a new project with specified configuration.
//	@Tags			Projects
//	@Accept			json
//	@Produce		json
//	@Param			body	body		payload.CreateProjectRequest	true	"Project configuration including name, description, and settings"
//	@Success		201		{object}	payload.HTTPMessage				"Project created successfully"
//	@Header			201		{string}	Location						"/projects/{id}"	"URI of the created project resource"
//	@Failure		400		{object}	payload.HTTPMessage				"Invalid request body or validation error"
//	@Failure		401		{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage				"Owning user not found"
//	@Failure		409		{object}	payload.HTTPMessage				"Project with name already exists"
//	@Failure		413		{object}	payload.HTTPMessage				"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage				"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage				"Internal server error during project creation"
//	@Router			/projects [post]
//	@Security		AccessToken
func (ref *ProjectsHandler) create(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "create")
	defer span.End()

	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.CreateProjectRequest
	if err := decodeJSONBody(r, &req); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteDecodeError(w, r, e)
		return
	}

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

	input := &domain.CreateProjectInput{
		ID:          req.ID,
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		Disabled:    false,
	}

	if err := ref.service.Create(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.ResourcesLimitsHardLimitReachedError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.ProjectAlreadyExistsError](err); ok {
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
	respond.SetLocation(w, r, input.ID.String())

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Project created",
		attribute.String("project.id", input.ID.String()))
	respond.WriteJSONMessage(w, r, http.StatusCreated, domain.ProjectsProjectCreatedSuccessfully)
}

// getByIDByUserID Get a project by ID and User ID
//
//	@ID				01982303-f0f9-7dfa-966c-4b9ce4133a33
//	@Summary		Get project
//	@Description	Retrieve project details by unique identifier.
//	@Tags			Projects
//	@Produce		json
//	@Param			project_id	path		string					true	"Project unique identifier"	Format(uuid)
//	@Success		200			{object}	payload.ProjectResponse	"Project details retrieved successfully"
//	@Failure		400			{object}	payload.HTTPMessage		"Invalid project ID format"
//	@Failure		401			{object}	payload.HTTPMessage		"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage		"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage		"Project not found"
//	@Failure		429			{object}	payload.HTTPMessage		"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage		"Internal server error"
//	@Router			/projects/{project_id} [get]
//	@Security		AccessToken
func (ref *ProjectsHandler) getByIDByUserID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "getByIDByUserID")
	defer span.End()

	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	projectID, err := parseUUIDQueryParams(r.PathValue("project_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	out, err := ref.service.GetByIDByUserID(ctx, projectID, userID)
	if err != nil {
		if _, ok := errors.AsType[*domain.ProjectNotFoundError](err); ok {
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

	outResponse := payload.ProjectResponse{
		ID:          out.ID,
		Name:        out.Name,
		Description: out.Description,
		Disabled:    out.Disabled,
		System:      out.System,
		CreatedAt:   out.CreatedAt,
		UpdatedAt:   out.UpdatedAt,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "get project",
		attribute.String("project.id", outResponse.ID.String()))
}

// updateByID Update a project by ID
//
//	@ID				01982303-f0f9-7db3-991f-2b7943b5328c
//	@Summary		Update project
//	@Description	Update existing project details by ID.
//	@Tags			Projects
//	@Accept			json
//	@Produce		json
//	@Param			project_id	path		string							true	"Project unique identifier"	Format(uuid)
//	@Param			body		body		payload.UpdateProjectRequest	true	"Project update details including name, description, or settings"
//	@Success		200			{object}	payload.HTTPMessage				"Project updated successfully"
//	@Failure		400			{object}	payload.HTTPMessage				"Invalid project ID or request body"
//	@Failure		401			{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage				"System projects cannot be modified"
//	@Failure		404			{object}	payload.HTTPMessage				"Project or owning user not found"
//	@Failure		409			{object}	payload.HTTPMessage				"Project name already in use"
//	@Failure		413			{object}	payload.HTTPMessage				"Request body larger than http.server.max.body.bytes"
//	@Failure		415			{object}	payload.HTTPMessage				"Body not declared as application/json"
//	@Failure		429			{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage				"Internal server error during update"
//	@Router			/projects/{project_id} [put]
//	@Security		AccessToken
func (ref *ProjectsHandler) updateByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "updateByID")
	defer span.End()

	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	projectID, err := parseUUIDQueryParams(r.PathValue("project_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.UpdateProjectRequest
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

	input := &domain.UpdateProjectInput{
		ID:          projectID,
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		Disabled:    req.Disabled,
	}

	if err := ref.service.UpdateByID(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.ProjectAlreadyExistsError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
			return
		}

		_, isProjectNotFound := errors.AsType[*domain.ProjectNotFoundError](err)
		_, isUserNotFound := errors.AsType[*domain.UserNotFoundError](err)
		if isProjectNotFound || isUserNotFound {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.SystemProjectError](err); ok {
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

		e := o11y.RecordError(ctx, span, start, &domain.InternalServerError{Message: "failed to update project"}, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	// Location header is required for RESTful APIs
	respond.SetLocation(w, r)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Project updated",
		attribute.String("project.id", input.ID.String()))
	respond.WriteJSONMessage(w, r, http.StatusOK, domain.ProjectsProjectUpdatedSuccessfully)
}

// deleteByID Delete a project by id
//
//	@ID				01982303-f0f9-7e9f-9bb9-81d42a9eb30a
//	@Summary		Delete project
//	@Description	Permanently remove project and all associated data.
//	@Tags			Projects
//	@Produce		json
//	@Param			project_id	path		string				true	"Project unique identifier"	Format(uuid)
//	@Success		200			{object}	payload.HTTPMessage	"Project deleted successfully"
//	@Failure		400			{object}	payload.HTTPMessage	"Invalid project ID format"
//	@Failure		401			{object}	payload.HTTPMessage	"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage	"System projects cannot be deleted"
//	@Failure		404			{object}	payload.HTTPMessage	"Project not found"
//	@Failure		429			{object}	payload.HTTPMessage	"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage	"Internal server error during deletion"
//	@Router			/projects/{project_id} [delete]
//	@Security		AccessToken
func (ref *ProjectsHandler) deleteByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "deleteByID")
	defer span.End()

	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	id, err := parseUUIDQueryParams(r.PathValue("project_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	input := &domain.DeleteProjectInput{
		ID:     id,
		UserID: userID,
	}

	if err := ref.service.DeleteByID(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.ProjectNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.SystemProjectError](err); ok {
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

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Project deleted",
		attribute.String("project.id", input.ID.String()))
	respond.WriteJSONMessage(w, r, http.StatusOK, domain.ProjectsProjectDeletedSuccessfully)
}

// listByUserID Retrieve a paginated list of projects for a specific user
//
//	@ID				019870ff-37f6-737e-8efb-e39730ef6952
//	@Summary		List projects by user
//	@Description	Retrieve paginated list of projects accessible to specific user.
//	@Tags			Projects,Users
//	@Produce		json
//	@Param			user_id		path		string							true	"User unique identifier"	Format(uuid)
//	@Param			sort		query		string							false	"Sort fields (comma-separated). Example: name ASC, created_at DESC"
//	@Param			filter		query		string							false	"Filter expression. Example: name LIKE '%test%'"
//	@Param			fields		query		string							false	"Fields to return (comma-separated). Example: id,name,description"
//	@Param			next_token	query		string							false	"Next page cursor for pagination"
//	@Param			prev_token	query		string							false	"Previous page cursor for pagination"
//	@Param			limit		query		int								false	"Maximum number of results per page"
//	@Success		200			{object}	payload.ListProjectsResponse	"Paginated list of user's projects"
//	@Failure		400			{object}	payload.HTTPMessage				"Invalid user ID or query parameters"
//	@Failure		401			{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/users/{user_id}/projects [get]
//	@Security		AccessToken
func (ref *ProjectsHandler) listByUserID(w http.ResponseWriter, r *http.Request) {
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

	input := &domain.ListProjectsInput{
		UserID: userID,
		Sort:   sort,
		Filter: filter,
		Fields: fields,
		Paginator: domain.Paginator{
			NextToken: nextToken,
			PrevToken: prevToken,
			Limit:     limit,
		},
	}

	out, err := ref.service.ListByUserID(ctx, input)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, httpStatusForListError(err), e.Error())
		return
	}

	outResponse := &payload.ListProjectsResponse{
		Items:     make([]payload.ProjectResponse, len(out.Items)),
		Paginator: out.Paginator,
	}

	for i, project := range out.Items {
		outResponse.Items[i] = payload.ProjectResponse{
			ID:          project.ID,
			Name:        project.Name,
			Description: project.Description,
			Disabled:    project.Disabled,
			System:      project.System,
			CreatedAt:   project.CreatedAt,
			UpdatedAt:   project.UpdatedAt,
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

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Projects retrieved successfully",
		attribute.String("user.id", input.UserID.String()))
}

// list Return a paginated list of Project
//
//	@ID				01982303-f0f9-7dbf-9688-6ef0150502e9
//	@Summary		List projects
//	@Description	Retrieve paginated list of accessible projects for authenticated user.
//	@Tags			Projects
//	@Produce		json
//	@Param			sort		query		string							false	"Sort fields (comma-separated). Example: name ASC, created_at DESC"
//	@Param			filter		query		string							false	"Filter expression. Example: name LIKE '%test%'"
//	@Param			fields		query		string							false	"Fields to return (comma-separated). Example: id,name,description"
//	@Param			next_token	query		string							false	"Next page cursor for pagination"
//	@Param			prev_token	query		string							false	"Previous page cursor for pagination"
//	@Param			limit		query		int								false	"Maximum number of results per page"
//	@Success		200			{object}	payload.ListProjectsResponse	"Paginated list of projects"
//	@Failure		400			{object}	payload.HTTPMessage				"Invalid query parameters or filter expression"
//	@Failure		401			{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/projects [get]
//	@Security		AccessToken
func (ref *ProjectsHandler) list(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "list")
	defer span.End()

	userID, err := getUserIDFromContext(ctx)
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

	input := &domain.ListProjectsInput{
		UserID: userID,
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

	outResponse := &payload.ListProjectsResponse{
		Items:     make([]payload.ProjectResponse, len(out.Items)),
		Paginator: out.Paginator,
	}

	for i, project := range out.Items {
		outResponse.Items[i] = payload.ProjectResponse{
			ID:          project.ID,
			Name:        project.Name,
			Description: project.Description,
			Disabled:    project.Disabled,
			System:      project.System,
			CreatedAt:   project.CreatedAt,
			UpdatedAt:   project.UpdatedAt,
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

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "list project",
		attribute.Int("project.count", len(outResponse.Items)))
}

// linkUsers Link users to a project
//
//	@ID				01986f44-3a65-7a21-9c2b-392f2b0eacf7
//	@Summary		Link users to project
//	@Description	Associate multiple users with project.
//	@Tags			Projects,Users
//	@Accept			json
//	@Produce		json
//	@Param			project_id	path		string								true	"Project unique identifier"	Format(uuid)
//	@Param			body		body		payload.LinkUsersToProjectRequest	true	"User IDs to link to project"
//	@Success		200			{object}	payload.HTTPMessage					"Users linked to project successfully"
//	@Failure		400			{object}	payload.HTTPMessage					"Invalid project ID or request body"
//	@Failure		401			{object}	payload.HTTPMessage					"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage					"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage					"Project not found"
//	@Failure		409			{object}	payload.HTTPMessage					"One or more users are already linked to the project"
//	@Failure		413			{object}	payload.HTTPMessage					"Request body larger than http.server.max.body.bytes"
//	@Failure		415			{object}	payload.HTTPMessage					"Body not declared as application/json"
//	@Failure		429			{object}	payload.HTTPMessage					"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage					"Internal server error during user linking"
//	@Router			/projects/{project_id}/users [post]
//	@Security		AccessToken
func (ref *ProjectsHandler) linkUsers(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "linkUsers")
	defer span.End()

	projectID, err := parseUUIDQueryParams(r.PathValue("project_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.LinkUsersToProjectRequest
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

	input := &domain.LinkUsersToProjectInput{
		ProjectID: projectID,
		UserIDs:   req.UserIDs,
	}

	if err := ref.service.LinkUsers(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.ProjectAlreadyExistsError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
			return
		}

		_, isProjectNotFound := errors.AsType[*domain.ProjectNotFoundError](err)
		_, isUserNotFound := errors.AsType[*domain.UserNotFoundError](err)
		if isProjectNotFound || isUserNotFound {
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
	respond.SetLocation(w, r, projectID.String(), "users")

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "link users to project",
		attribute.String("project.id", projectID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.ProjectsUsersLinkedSuccessfully)
}

// unlinkUsers Unlink users from a project
//
//	@ID				01986f44-3a65-7a19-a92d-e6100dd80807
//	@Summary		Unlink users from project
//	@Description	Remove user associations from project.
//	@Tags			Projects,Users
//	@Accept			json
//	@Produce		json
//	@Param			project_id	path		string									true	"Project unique identifier"	Format(uuid)
//	@Param			body		body		payload.UnlinkUsersFromProjectRequest	true	"User IDs to unlink from project"
//	@Success		200			{object}	payload.HTTPMessage						"Users unlinked from project successfully"
//	@Failure		400			{object}	payload.HTTPMessage						"Invalid project ID or request body"
//	@Failure		401			{object}	payload.HTTPMessage						"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage						"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage						"Project or user not found"
//	@Failure		413			{object}	payload.HTTPMessage						"Request body larger than http.server.max.body.bytes"
//	@Failure		415			{object}	payload.HTTPMessage						"Body not declared as application/json"
//	@Failure		429			{object}	payload.HTTPMessage						"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage						"Internal server error during user unlinking"
//	@Router			/projects/{project_id}/users [delete]
//	@Security		AccessToken
func (ref *ProjectsHandler) unlinkUsers(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "unlinkUsers")
	defer span.End()

	projectID, err := parseUUIDQueryParams(r.PathValue("project_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.UnlinkUsersFromProjectRequest
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

	input := &domain.UnlinkUsersFromProjectInput{
		ProjectID: projectID,
		UserIDs:   req.UserIDs,
	}

	if err := ref.service.UnlinkUsers(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.ProjectNotFoundError](err); ok {
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
	respond.SetLocation(w, r, projectID.String(), "users")

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "unlink users from project",
		attribute.String("project.id", projectID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.ProjectsUsersUnlinkedSuccessfully)
}
