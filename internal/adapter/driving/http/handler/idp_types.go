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

// IDPTypesHandlerConf is the configuration struct for the IDPTypesHandler.
type IDPTypesHandlerConf struct {
	Service       driving.IDPTypes
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// IDPTypesHandler is the handler that will handle the authentication of idp types.
type IDPTypesHandler struct {
	service         driving.IDPTypes
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewIDPTypesHandler creates a new IDPTypesHandler.
func NewIDPTypesHandler(conf IDPTypesHandlerConf) (*IDPTypesHandler, error) {
	if conf.Service == nil {
		return nil, &domain.InvalidServiceError{Message: "NewIDPTypesHandler is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &IDPTypesHandler{
		service:       conf.Service,
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "IDPTypes",
			Action: "NewIDPTypesHandler",
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

// RegisterRoutes registers the routes for the IDPTypesHandler.
func (ref *IDPTypesHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /auth/idp_types", mdw.ThenFunc(ref.list))
	mux.Handle("GET /auth/idp_types/{idp_type_id}", mdw.ThenFunc(ref.getByID))
}

// getByID Get an Identity Provider (IDP) by ID
//
//	@ID				0198f1e2-14ff-767c-971c-3904e0f2c484
//	@Summary		Get IDP type
//	@Description	Retrieve a specific Identity Provider Type by its unique identifier
//	@Tags			Authentication,Identity Provider Types
//	@Produce		json
//	@Param			idp_type_id	path		string						true	"IDP type unique identifier (UUID v7)"
//	@Success		200			{object}	payload.IDPTypesResponse	"IDP type details retrieved successfully"
//	@Failure		400			{object}	payload.HTTPMessage			"Invalid request - malformed UUID or invalid parameters"
//	@Failure		401			{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage			"IDP type not found"
//	@Failure		429			{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/auth/idp_types/{idp_type_id} [get]
//	@Security		AccessToken
func (ref *IDPTypesHandler) getByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "getByID")
	defer span.End()

	idpTypeID, err := parseUUIDQueryParams(r.PathValue("idp_type_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	out, err := ref.service.GetByID(ctx, idpTypeID)
	if err != nil {
		if _, ok := errors.AsType[*domain.IDPTypesNotFoundError](err); ok {
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

	outResponse := &payload.IDPTypesResponse{
		ID:             out.ID,
		Name:           out.Name,
		Description:    out.Description,
		Scopes:         out.Scopes,
		UserInfoAPIURL: out.UserInfoAPIURL,
		Kind:           out.Kind.String(),
		IssuerHint:     out.IssuerHint,
		System:         out.System,
		CreatedAt:      out.CreatedAt,
		UpdatedAt:      out.UpdatedAt,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	slog.Debug("handler.IDPTypes.getByID", "idp.name", outResponse.Name)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "IDP found",
		attribute.String("idp.id", outResponse.ID.String()),
		attribute.String("idp.name", outResponse.Name),
	)
}

// list handles the list request for all Identity Provider Types.
//
//	@Id				0198f1e2-14ff-7678-afbe-9a627b0eaabd
//	@Summary		List IDP types
//	@Description	Retrieve a paginated list of all Identity Provider Types available for authentication.
//	@Tags			Authentication,Identity Provider Types
//	@Produce		json
//	@Param			sort		query		string							false	"Sort by fields (comma-separated with ASC/DESC)"
//	@Param			filter		query		string							false	"Filter expression for querying results"
//	@Param			fields		query		string							false	"Comma-separated list of fields to return"
//	@Param			next_token	query		string							false	"Pagination cursor for next page"
//	@Param			prev_token	query		string							false	"Pagination cursor for previous page"
//	@Param			limit		query		int								false	"Maximum number of items to return (default: 20, max: 100)"
//	@Success		200			{object}	payload.ListIDPTypesResponse	"Paginated list of IDP types retrieved successfully"
//	@Failure		400			{object}	payload.HTTPMessage				"Invalid request - malformed query parameters"
//	@Failure		401			{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/auth/idp_types [get]
//	@Security		AccessToken
func (ref *IDPTypesHandler) list(w http.ResponseWriter, r *http.Request) {
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

	input := &domain.ListIDPTypesInput{
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

	outResponse := &payload.ListIDPTypesResponse{
		Items:     make([]payload.IDPTypesResponse, len(out.Items)),
		Paginator: out.Paginator,
	}

	for i, item := range out.Items {
		outResponse.Items[i] = payload.IDPTypesResponse{
			ID:             item.ID,
			Name:           item.Name,
			Description:    item.Description,
			Scopes:         item.Scopes,
			UserInfoAPIURL: item.UserInfoAPIURL,
			Kind:           item.Kind.String(),
			IssuerHint:     item.IssuerHint,
			System:         item.System,
			CreatedAt:      item.CreatedAt,
			UpdatedAt:      item.UpdatedAt,
		}
	}

	// Generate the next and previous pages
	location := fmt.Sprintf("http://%s%s", r.Host, r.URL.Path)
	outResponse.Paginator.GeneratePages(location)

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "List IDP Types",
		attribute.Int("idp_types.count", len(outResponse.Items)))
}
