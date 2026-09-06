package handler

import (
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

// ResourcesLimitsHandlerConf represents the configuration for the resources limits handler.
type ResourcesLimitsHandlerConf struct {
	Service       driving.ResourcesLimits
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// ResourcesLimitsHandler represents the handler for the resources limits.
type ResourcesLimitsHandler struct {
	service         driving.ResourcesLimits
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewResourcesLimitsHandler creates a new ResourcesLimitsHandler.
func NewResourcesLimitsHandler(conf ResourcesLimitsHandlerConf) (*ResourcesLimitsHandler, error) {
	if conf.Service == nil {
		return nil, &domain.InvalidServiceError{Message: "driving.ResourcesLimits is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &ResourcesLimitsHandler{
		service:       conf.Service,
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "ResourcesLimits",
			Action: "NewResourcesLimitsHandler",
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
func (ref *ResourcesLimitsHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /resources_limits", mdw.ThenFunc((ref.list)))
	mux.Handle("GET /me/resources_limits", mdw.ThenFunc(ref.statusForMe))
	mux.Handle("GET /projects/{project_id}/resources_limits", mdw.ThenFunc(ref.statusForProject))
}

// writeScopeStatus renders a scope status, shared by both status endpoints.
func (ref *ResourcesLimitsHandler) writeScopeStatus(w http.ResponseWriter, status *domain.ResourcesLimitsScopeStatus) error {
	out := payload.ResourcesLimitsStatusResponse{
		ScopeType: status.ScopeType.String(),
		ScopeID:   status.ScopeID,
		Resources: make([]payload.ResourceUsageStatusResponse, len(status.Resources)),
	}

	for i, resource := range status.Resources {
		out.Resources[i] = payload.ResourceUsageStatusResponse{
			ResourceType: resource.ResourceType.String(),
			Usage:        resource.Status.CurrentUsage,
			SoftLimit:    resource.Status.SoftLimit,
			HardLimit:    resource.Status.HardLimit,
			CanCreate:    resource.Status.CanCreate,
			SoftReached:  resource.Status.SoftLimitReached,
			Tampered:     resource.Status.TamperDetected,
		}
	}

	return respond.WriteJSONData(w, http.StatusOK, out)
}

// statusForMe Get the calling user's resource limits and usage
//
//	@ID				01a01117-dba9-74da-bd70-d1acc3842ffa
//	@Summary		Get my resource limits
//	@Description	Retrieve the limits that apply to the calling user and how much of each has been consumed. Read-only: limits are not editable through the API.
//	@Tags			Resources Limits
//	@Produce		json
//	@Success		200	{object}	payload.ResourcesLimitsStatusResponse	"Limits and usage for the calling user"
//	@Failure		400	{object}	payload.HTTPMessage						"Missing or malformed user context"
//	@Failure		401	{object}	payload.HTTPMessage						"Missing or invalid authentication"
//	@Failure		403	{object}	payload.HTTPMessage						"Insufficient permissions"
//	@Failure		429	{object}	payload.HTTPMessage						"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500	{object}	payload.HTTPMessage						"Internal server error"
//	@Router			/me/resources_limits [get]
//	@Security		AccessToken
func (ref *ResourcesLimitsHandler) statusForMe(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "statusForMe")
	defer span.End()

	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	status, err := ref.service.StatusByScope(ctx, domain.ResourcesLimitsScope{
		Type: domain.ResourcesLimitsScopeTypeUser,
		ID:   &userID,
	})
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	if err := ref.writeScopeStatus(w, status); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "my resource limits retrieved successfully")
}

// statusForProject Get a project's resource limits and usage
//
//	@ID				01a01117-dba9-763d-8f4e-968072dbdb52
//	@Summary		Get project resource limits
//	@Description	Retrieve the limits that apply to a project and how much of each has been consumed. Read-only: limits are not editable through the API.
//	@Tags			Resources Limits
//	@Produce		json
//	@Param			project_id	path		string									true	"Project ID"	format(uuid)
//	@Success		200			{object}	payload.ResourcesLimitsStatusResponse	"Limits and usage for the project"
//	@Failure		400			{object}	payload.HTTPMessage						"Invalid project ID format"
//	@Failure		401			{object}	payload.HTTPMessage						"Missing or invalid authentication"
//	@Failure		403			{object}	payload.HTTPMessage						"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage						"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		404			{object}	payload.HTTPMessage						"Project not found, or the caller is not a member of it"
//	@Failure		500			{object}	payload.HTTPMessage						"Internal server error"
//	@Router			/projects/{project_id}/resources_limits [get]
//	@Security		AccessToken
func (ref *ResourcesLimitsHandler) statusForProject(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "statusForProject")
	defer span.End()

	projectID, err := parseUUIDQueryParams(r.PathValue("project_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	status, err := ref.service.StatusByScope(ctx, domain.ResourcesLimitsScope{
		Type: domain.ResourcesLimitsScopeTypeProject,
		ID:   &projectID,
	})
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	if err := ref.writeScopeStatus(w, status); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "project resource limits retrieved successfully",
		attribute.String("project.id", projectID.String()))
}

// list Return a paginated list of resources limits
//
//	@ID				01994754-5db8-7904-80f3-91417f2a4003
//	@Summary		List resource limits
//	@Description	Retrieve a paginated list of resource limits in the system with optional filtering, sorting, and field selection
//	@Tags			Resources Limits
//	@Produce		json
//	@Param			sort		query		string								false	"Comma-separated list of fields to sort by"
//	@Param			filter		query		string								false	"Filter expression for querying resources limits"
//	@Param			fields		query		string								false	"Comma-separated list of fields to return"
//	@Param			next_token	query		string								false	"Pagination token for next page"
//	@Param			prev_token	query		string								false	"Pagination token for previous page"
//	@Param			limit		query		int									false	"Maximum number of items to return"
//	@Success		200			{object}	payload.ListResourcesLimitsResponse	"Resources limits list retrieved successfully"
//	@Failure		400			{object}	payload.HTTPMessage					"Invalid request - malformed parameters or invalid filter syntax"
//	@Failure		401			{object}	payload.HTTPMessage					"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage					"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage					"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage					"Internal server error"
//	@Router			/resources_limits [get]
//	@Security		AccessToken
func (ref *ResourcesLimitsHandler) list(w http.ResponseWriter, r *http.Request) {
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

	input := &domain.ListResourcesLimitsInput{
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

	outResponse := &payload.ListResourcesLimitsResponse{
		Items:     make([]payload.ResourcesLimitsResponse, len(out.Items)),
		Paginator: out.Paginator,
	}

	for i, item := range out.Items {
		outResponse.Items[i] = payload.ResourcesLimitsResponse{
			ID:           item.ID,
			ScopeType:    item.ScopeType,
			ScopeID:      item.ScopeID,
			ResourceType: item.ResourceType,
			Usage:        item.Usage,
			SoftLimit:    item.SoftLimit,
			HardLimit:    item.HardLimit,
			CreatedAt:    item.CreatedAt,
			UpdatedAt:    item.UpdatedAt,
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

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "List resources limits",
		attribute.Int("resources_limits.count", len(outResponse.Items)))
}
