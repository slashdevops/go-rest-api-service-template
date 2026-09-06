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

// ResourcesHandlerConf represents the configuration for the ResourcesHandler.
type ResourcesHandlerConf struct {
	Service       driving.Resources
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// ResourcesHandler represents the handler for the resources.
type ResourcesHandler struct {
	service         driving.Resources
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewResourcesHandler creates a new ResourcesHandler.
func NewResourcesHandler(conf ResourcesHandlerConf) (*ResourcesHandler, error) {
	if conf.Service == nil {
		return nil, &domain.InvalidServiceError{Message: "driving.Resources is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &ResourcesHandler{
		service:       conf.Service,
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Resources",
			Action: "NewResourcesHandler",
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
func (ref *ResourcesHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /resources", mdw.ThenFunc(ref.list))
	mux.Handle("GET /resources/{resource_id}", mdw.ThenFunc(ref.getByID))

	mux.Handle("GET /resources/matches", mdw.ThenFunc(ref.listMatches))
}

// getByID retrieves a specific resource by its unique identifier
//
//	@ID				019822c9-9775-71b1-a2c6-deac83cf2519
//	@Summary		Get resource
//	@Description	Retrieve detailed information about a specific system resource configuration using its unique identifier.
//	@Tags			Resources
//	@Accept			json
//	@Produce		json
//	@Param			resource_id	path		string						true	"Unique resource identifier (UUID v7)"	Format(uuid)
//	@Success		200			{object}	payload.ResourceResponse	"Resource details retrieved successfully"
//	@Failure		400			{object}	payload.HTTPMessage			"Invalid resource ID format or malformed request"
//	@Failure		401			{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage			"Resource not found"
//	@Failure		429			{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/resources/{resource_id} [get]
//	@Security		AccessToken
func (ref *ResourcesHandler) getByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "getByID")
	defer span.End()

	resourceID, err := parseUUIDQueryParams(r.PathValue("resource_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	out, err := ref.service.GetByID(ctx, resourceID)
	if err != nil {
		if _, ok := errors.AsType[*domain.ResourceNotFoundError](err); ok {
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

	outResponse := payload.ResourceResponse{
		ID:          out.ID,
		Name:        out.Name,
		Description: out.Description,
		Action:      out.Action,
		Resource:    out.Resource,
		System:      out.System,
		CreatedAt:   out.CreatedAt,
		UpdatedAt:   out.UpdatedAt,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	slog.Debug("handler.Resources.getByID", "id", outResponse.ID.String())
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Resources found",
		attribute.String("resource.id", outResponse.ID.String()))
}

// list returns a paginated collection of system resources
//
//	@ID				01982303-f0f9-7ee0-aa66-0f756c3c8bec
//	@Summary		List resources
//	@Description	Retrieve a paginated list of all available system resources.
//	@Tags			Resources
//	@Accept			json
//	@Produce		json
//	@Param			sort		query		string							false	"Sort order: comma-separated fields with ASC/DESC. Example: name ASC,created_at DESC"
//	@Param			filter		query		string							false	"Filter expression using SQL-like syntax. Example: action='read' AND system=true"
//	@Param			fields		query		string							false	"Comma-separated list of fields to include in response. Example: id,name,action,resource"
//	@Param			next_token	query		string							false	"Pagination cursor for fetching the next page of results"
//	@Param			prev_token	query		string							false	"Pagination cursor for fetching the previous page of results"
//	@Param			limit		query		int								false	"Maximum number of items to return per page (default: varies by configuration)"
//	@Success		200			{object}	payload.ListResourcesResponse	"Paginated list of resources retrieved successfully"
//	@Failure		400			{object}	payload.HTTPMessage				"Invalid query parameters or malformed filter/sort expression"
//	@Failure		401			{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/resources [get]
//	@Security		AccessToken
func (ref *ResourcesHandler) list(w http.ResponseWriter, r *http.Request) {
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

	input := &domain.ListResourcesInput{
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

	outResponse := &payload.ListResourcesResponse{
		Items:     make([]payload.ResourceResponse, len(out.Items)),
		Paginator: out.Paginator,
	}

	for i, resource := range out.Items {
		outResponse.Items[i] = payload.ResourceResponse{
			ID:          resource.ID,
			Name:        resource.Name,
			Description: resource.Description,
			Action:      resource.Action,
			Resource:    resource.Resource,
			System:      resource.System,
			CreatedAt:   resource.CreatedAt,
			UpdatedAt:   resource.UpdatedAt,
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

	slog.Debug("handler.Resources.list: called", "resources", len(outResponse.Items))
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "list resources",
		attribute.Int("resources.count", len(outResponse.Items)))
}

// listMatches returns resources matching specific action and resource policy patterns
//
//	@ID				01982303-f0f9-7e44-aeb1-63934913e601
//	@Summary		Find resources by action and pattern
//	@Description	Retrieve a paginated list of resources that match the specified action and resource policy patterns.
//	@Tags			Resources
//	@Accept			json
//	@Produce		json
//	@Param			action		query		string							true	"Action pattern to match (e.g., 'read', 'write', 'delete' or wildcard patterns)"
//	@Param			resource	query		string							true	"Resource pattern to match (e.g., 'users/*', 'projects/123' or wildcard patterns)"
//	@Param			sort		query		string							false	"Sort order: comma-separated fields with ASC/DESC. Example: name ASC,created_at DESC"
//	@Param			fields		query		string							false	"Comma-separated list of fields to include in response. Example: id,name,action,resource"
//	@Param			next_token	query		string							false	"Pagination cursor for fetching the next page of results"
//	@Param			prev_token	query		string							false	"Pagination cursor for fetching the previous page of results"
//	@Param			limit		query		int								false	"Maximum number of items to return per page (default: varies by configuration)"
//	@Success		200			{object}	payload.ListResourcesResponse	"Matching resources retrieved successfully"
//	@Failure		400			{object}	payload.HTTPMessage				"Invalid action/resource pattern or malformed query parameters"
//	@Failure		401			{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage				"No resources found matching the specified patterns"
//	@Failure		429			{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/resources/matches [get]
//	@Security		AccessToken
func (ref *ResourcesHandler) listMatches(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "listMatches")
	defer span.End()

	action := r.URL.Query().Get("action")
	if err := domain.IsValidAction(action); err != nil {
		errType := &domain.InvalidActionError{Action: action}
		e := o11y.RecordError(ctx, span, start, errType, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	resource := r.URL.Query().Get("resource")
	if err := domain.IsValidResource(resource); err != nil {
		errType := &domain.InvalidResourceError{Resource: resource}
		e := o11y.RecordError(ctx, span, start, errType, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	params := map[string]any{
		"sort":      r.URL.Query().Get("sort"),
		"filter":    "", // this is disabled because the filter is not supported for this endpoint
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

	input := &domain.ListResourcesInput{
		Sort:   sort,
		Filter: filter,
		Fields: fields,
		Paginator: domain.Paginator{
			NextToken: nextToken,
			PrevToken: prevToken,
			Limit:     limit,
		},
	}

	out, err := ref.service.ListMatches(ctx, action, resource, input)
	if err != nil {
		if _, ok := errors.AsType[*domain.ResourceNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}

		_, isInvalidAction := errors.AsType[*domain.InvalidActionError](err)
		_, isInvalidResource := errors.AsType[*domain.InvalidResourceError](err)
		_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
		_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
		_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
		_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
		if isInvalidAction || isInvalidResource || isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		// Malformed sort/filter/fields surface as a domain ValidationError → 400.
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, httpStatusForListError(err), e.Error())
		return
	}

	outResponse := &payload.ListResourcesResponse{
		Items:     make([]payload.ResourceResponse, len(out.Items)),
		Paginator: out.Paginator,
	}

	for i, resource := range out.Items {
		outResponse.Items[i] = payload.ResourceResponse{
			ID:          resource.ID,
			Name:        resource.Name,
			Description: resource.Description,
			Action:      resource.Action,
			Resource:    resource.Resource,
			System:      resource.System,
			CreatedAt:   resource.CreatedAt,
			UpdatedAt:   resource.UpdatedAt,
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

	slog.Debug("handler.Resources.listMatches: called", "resources", len(outResponse.Items))
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "list resources by action and resource",
		attribute.Int("resources.count", len(outResponse.Items)),
		attribute.String("action", action),
		attribute.String("resource", resource))
}
