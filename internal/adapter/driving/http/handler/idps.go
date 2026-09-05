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

// IDPsHandlerConf is the configuration struct for the IDPsHandler.
type IDPsHandlerConf struct {
	Service       driving.IDPs
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// IDPsHandler is the handler that will handle the authentication of idps.
type IDPsHandler struct {
	service         driving.IDPs
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewIDPsHandler creates a new IDPsHandler.
func NewIDPsHandler(conf IDPsHandlerConf) (*IDPsHandler, error) {
	if conf.Service == nil {
		return nil, &domain.InvalidServiceError{Message: "driving.IDPs is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &IDPsHandler{
		service:       conf.Service,
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "IDPs",
			Action: "NewIDPsHandler",
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

// RegisterRoutes registers the routes for the IDPsHandler.
func (ref *IDPsHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /auth/idps", mdw.ThenFunc(ref.list))
	mux.Handle("POST /auth/idps", mdw.ThenFunc(ref.create))
	mux.Handle("GET /auth/idps/{idp_id}", mdw.ThenFunc(ref.getByID))
	mux.Handle("PUT /auth/idps/{idp_id}", mdw.ThenFunc(ref.updateByID))
	mux.Handle("DELETE /auth/idps/{idp_id}", mdw.ThenFunc(ref.deleteByID))
}

// getByID Get an IDP by ID
//
//	@ID				0198e7ea-3755-7a3d-8baa-36126e5d1c48
//	@Summary		Get IDP
//	@Description	Retrieve a specific Identity Provider configuration by its unique identifier
//	@Tags			Authentication,Identity Providers (IDPs)
//	@Accept			json
//	@Produce		json
//	@Param			idp_id	path		string				true	"Identity Provider UUID"	Format(uuid)
//	@Success		200		{object}	payload.IDPResponse	"IDP configuration retrieved successfully"
//	@Failure		400		{object}	payload.HTTPMessage	"Invalid UUID format or malformed request"
//	@Failure		401		{object}	payload.HTTPMessage	"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage	"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage	"IDP not found"
//	@Failure		429		{object}	payload.HTTPMessage	"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage	"Internal server error"
//	@Router			/auth/idps/{idp_id} [get]
//	@Security		AccessToken
func (ref *IDPsHandler) getByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "getByID")
	defer span.End()

	idpID, err := parseUUIDQueryParams(r.PathValue("idp_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	out, err := ref.service.GetByID(ctx, idpID)
	if err != nil {
		if _, ok := errors.AsType[*domain.IDPNotFoundError](err); ok {
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

	outResponse := payload.IDPResponse{
		ID: out.ID,
		IDPType: payload.IDPTypesResponse{
			ID:   out.IDPType.ID,
			Name: out.IDPType.Name,
		},
		Name:                out.Name,
		Description:         out.Description,
		CallbackURL:         out.CallbackURL,
		LoginRedirectURL:    out.LoginRedirectURL,
		RegisterRedirectURL: out.RegisterRedirectURL,
		Logo:                out.Logo,
		ClientID:            out.ClientID,
		CreatedAt:           out.CreatedAt,
		UpdatedAt:           out.UpdatedAt,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.IDPsIDPFound,
		attribute.String("idp.id", out.ID.String()),
		attribute.String("idp.name", out.Name),
	)
}

// create Create a new IDP
//
//	@ID				0198e7ea-3755-7a39-9dfc-717d83facf02
//	@Summary		Create IDP
//	@Description	Register a new Identity Provider with authentication configuration
//	@Tags			Authentication,Identity Providers (IDPs)
//	@Accept			json
//	@Produce		json
//	@Param			body	body		payload.CreateIDPRequest	true	"IDP configuration details"
//	@Success		201		{object}	payload.HTTPMessage			"IDP created successfully"
//	@Header			201		{string}	Location					"URL of the newly created IDP resource (/auth/idps/{idp_id})"
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid request body, validation failure, or malformed UUID"
//	@Failure		401		{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		409		{object}	payload.HTTPMessage			"IDP with this name or configuration already exists"
//	@Failure		429		{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/auth/idps [post]
//	@Security		AccessToken
func (ref *IDPsHandler) create(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "create")
	defer span.End()

	var req payload.CreateIDPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var err error
	req.ID, err = domain.EnsureUUIDV7(req.ID)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	if err := req.Validate(); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	input := &domain.CreateIDPInput{
		ID:                  req.ID,
		IDPTypeID:           req.IDPTypeID,
		Name:                req.Name,
		Description:         req.Description,
		CallbackURL:         req.CallbackURL,
		LoginRedirectURL:    req.LoginRedirectURL,
		RegisterRedirectURL: req.RegisterRedirectURL,
		Logo:                req.Logo,
		ClientID:            req.ClientID,
		ClientSecret:        req.ClientSecret,
	}

	if err := ref.service.Create(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.ResourcesLimitsHardLimitReachedError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.IDPAlreadyExistsError](err); ok {
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
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	slog.Debug("handler.IDPs.create", "idp.name", input.Name)

	// Location header is required for RESTful APIs
	w.Header().Set("Location", fmt.Sprintf("%s%s/%s", r.Header.Get("Origin"), r.RequestURI, input.ID.String()))

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.IDPsIDPCreatedSuccessfully,
		attribute.String("idp.id", input.ID.String()),
		attribute.String("idp.name", input.Name))

	respond.WriteJSONMessage(w, r, http.StatusCreated, domain.IDPsIDPCreatedSuccessfully)
}

// updateByID Update an IDP
//
//	@ID				0198e7ea-3755-7a35-9e30-6a9392e8e7a1
//	@Summary		Update IDP
//	@Description	Modify an existing Identity Provider configuration
//	@Tags			Authentication,Identity Providers (IDPs)
//	@Accept			json
//	@Produce		json
//	@Param			idp_id	path		string						true	"Identity Provider UUID"	Format(uuid)
//	@Param			body	body		payload.UpdateIDPRequest	true	"Updated IDP configuration"
//	@Success		200		{object}	payload.HTTPMessage			"IDP updated successfully"
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid UUID format, request body, or validation failure"
//	@Failure		401		{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage			"IDP not found"
//	@Failure		409		{object}	payload.HTTPMessage			"IDP name already exists"
//	@Failure		429		{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/auth/idps/{idp_id} [put]
//	@Security		AccessToken
func (ref *IDPsHandler) updateByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "updateByID")
	defer span.End()

	idpID, err := parseUUIDQueryParams(r.PathValue("idp_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.UpdateIDPRequest
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

	input := &domain.UpdateIDPInput{
		ID:                  idpID,
		IDPTypeID:           req.IDPTypeID,
		Name:                req.Name,
		Description:         req.Description,
		CallbackURL:         req.CallbackURL,
		LoginRedirectURL:    req.LoginRedirectURL,
		RegisterRedirectURL: req.RegisterRedirectURL,
		Logo:                req.Logo,
		ClientID:            req.ClientID,
		ClientSecret:        req.ClientSecret,
	}

	if err := ref.service.UpdateByID(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.IDPNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.IDPAlreadyExistsError](err); ok {
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
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	// Location header is required for RESTful APIs
	w.Header().Set("Location", fmt.Sprintf("%s%s", r.Header.Get("Host"), r.URL.Path))

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.IDPsIDPUpdatedSuccessfully,
		attribute.String("idp.id", input.ID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.IDPsIDPUpdatedSuccessfully)
}

// deleteByID Delete an IDP
//
//	@ID				0198e7ea-3755-7a2d-9ab0-83ccef188e37
//	@Summary		Delete IDP
//	@Description	Permanently remove an Identity Provider configuration
//	@Tags			Authentication,Identity Providers (IDPs)
//	@Accept			json
//	@Produce		json
//	@Param			idp_id	path		string				true	"Identity Provider UUID"	Format(uuid)
//	@Success		200		{object}	payload.HTTPMessage	"IDP deleted successfully"
//	@Failure		400		{object}	payload.HTTPMessage	"Invalid UUID format or malformed request"
//	@Failure		401		{object}	payload.HTTPMessage	"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage	"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage	"IDP not found"
//	@Failure		429		{object}	payload.HTTPMessage	"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage	"Internal server error"
//	@Router			/auth/idps/{idp_id} [delete]
//	@Security		AccessToken
func (ref *IDPsHandler) deleteByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "deleteByID")
	defer span.End()

	idpID, err := parseUUIDQueryParams(r.PathValue("idp_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	input := &domain.DeleteIDPInput{
		ID: idpID,
	}

	if err := ref.service.DeleteByID(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.IDPNotFoundError](err); ok {
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

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.IDPsIDPDeletedSuccessfully,
		attribute.String("idp.id", input.ID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.IDPsIDPDeletedSuccessfully)
}

// list handles the list request for all IDPs.
//
//	@Id				0198e7ea-3755-7a29-90ed-13245b54f074
//	@Summary		List IDPs
//	@Description	Retrieve paginated list of Identity Providers with optional filtering and sorting
//	@Tags			Authentication,Identity Providers (IDPs)
//	@Accept			json
//	@Produce		json
//	@Param			sort		query		string						false	"Sort order (e.g., name ASC, created_at DESC)"
//	@Param			filter		query		string						false	"Filter expression (e.g., idp_type='oauth2' AND name LIKE 'Google%')"
//	@Param			fields		query		string						false	"Comma-separated fields to return (e.g., id,name,idp_type)"
//	@Param			next_token	query		string						false	"Pagination token for next page"
//	@Param			prev_token	query		string						false	"Pagination token for previous page"
//	@Param			limit		query		int							false	"Maximum number of items per page (default: system-defined)"
//	@Success		200			{object}	payload.ListIDPsResponse	"List of IDPs with pagination metadata"
//	@Failure		400			{object}	payload.HTTPMessage			"Invalid query parameters, filter syntax, or sort fields"
//	@Failure		401			{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/auth/idps [get]
//	@Security		AccessToken
func (ref *IDPsHandler) list(w http.ResponseWriter, r *http.Request) {
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

	input := &domain.ListIDPsInput{
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

	outResponse := payload.ListIDPsResponse{
		Items:     make([]payload.IDPResponse, len(out.Items)),
		Paginator: out.Paginator,
	}

	for i, idp := range out.Items {
		outResponse.Items[i] = payload.IDPResponse{
			ID: idp.ID,
			IDPType: payload.IDPTypesResponse{
				ID:   idp.IDPType.ID,
				Name: idp.IDPType.Name,
			},
			Name:                idp.Name,
			Description:         idp.Description,
			CallbackURL:         idp.CallbackURL,
			LoginRedirectURL:    idp.LoginRedirectURL,
			RegisterRedirectURL: idp.RegisterRedirectURL,
			Logo:                idp.Logo,
			ClientID:            idp.ClientID,
			CreatedAt:           idp.CreatedAt,
			UpdatedAt:           idp.UpdatedAt,
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

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "List IDPs",
		attribute.Int("idps.count", len(outResponse.Items)))
}
