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

// PoliciesHandlerConf represents the configuration for the PoliciesHandler.
type PoliciesHandlerConf struct {
	Service       driving.Policies
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// PoliciesHandler represents the handler for the policies.
type PoliciesHandler struct {
	service         driving.Policies
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewPoliciesHandler creates a new PoliciesHandler.
func NewPoliciesHandler(conf PoliciesHandlerConf) (*PoliciesHandler, error) {
	if conf.Service == nil {
		return nil, &domain.InvalidServiceError{Message: "driving.Policies is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &PoliciesHandler{
		service:       conf.Service,
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Policies",
			Action: "NewPoliciesHandler",
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
func (ref *PoliciesHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /policies", mdw.ThenFunc(ref.list))
	mux.Handle("GET /policies/{policy_id}", mdw.ThenFunc(ref.getByID))
	mux.Handle("POST /policies", mdw.ThenFunc(ref.create))
	mux.Handle("PUT /policies/{policy_id}", mdw.ThenFunc(ref.updateByID))
	mux.Handle("DELETE /policies/{policy_id}", mdw.ThenFunc(ref.deleteByID))

	// link/unlink roles to/from a policy
	mux.Handle("POST /policies/{policy_id}/roles", mdw.ThenFunc(ref.linkRoles))
	mux.Handle("DELETE /policies/{policy_id}/roles", mdw.ThenFunc(ref.unlinkRoles))

	// list policies by role id
	mux.Handle("GET /roles/{role_id}/policies", mdw.ThenFunc(ref.listByRoleID))
}

// getByID Get a policy by ID
//
//	@ID				01982303-f0f9-7e30-ab3f-9220a73b02eb
//	@Summary		Get policy
//	@Description	Retrieve a specific authorization policy by its unique identifier
//	@Tags			Policies
//	@Accept			json
//	@Produce		json
//	@Param			policy_id	path		string					true	"Unique policy identifier"	Format(uuid)
//	@Success		200			{object}	payload.PolicyResponse	"Policy retrieved successfully"
//	@Failure		400			{object}	payload.HTTPMessage		"Invalid policy ID format"
//	@Failure		401			{object}	payload.HTTPMessage		"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage		"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage		"Policy not found"
//	@Failure		429			{object}	payload.HTTPMessage		"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage		"Internal server error"
//	@Router			/policies/{policy_id} [get]
//	@Security		AccessToken
func (ref *PoliciesHandler) getByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "getByID")
	defer span.End()

	policyID, err := parseUUIDQueryParams(r.PathValue("policy_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	out, err := ref.service.GetByID(ctx, policyID)
	if err != nil {
		if _, ok := errors.AsType[*domain.PolicyNotFoundError](err); ok {
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

	ourResponse := payload.PolicyResponse{
		ID:          out.ID,
		Name:        out.Name,
		Description: out.Description,
		System:      out.System,
		Resource: payload.ResourceResponse{
			ID:       out.Resource.ID,
			Name:     out.Resource.Name,
			Action:   out.Resource.Action,
			Resource: out.Resource.Resource,
		},
		AllowedAction:   out.AllowedAction,
		AllowedResource: out.AllowedResource,
		CreatedAt:       out.CreatedAt,
		UpdatedAt:       out.UpdatedAt,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, ourResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}

	slog.Debug("handler.Policies.getByID: called", "policy.id", ourResponse.ID)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "get policy",
		attribute.String("policy.id", ourResponse.ID.String()))
}

// create Create a new policy
//
//	@ID				01982303-f0f9-7e38-ab68-486c8a2e819b
//	@Summary		Create policy
//	@Description	Create a new authorization policy with specified permissions
//	@Tags			Policies
//	@Accept			json
//	@Produce		json
//	@Param			body	body		payload.CreatePolicyRequest	true							"Policy creation request payload"
//	@Success		201		{object}	payload.HTTPMessage			"Policy created successfully"	{Location: "/policies/{policy_id}"}
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid request body or validation error"
//	@Failure		401		{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403		{object}	payload.HTTPMessage			"Insufficient permissions"
//	@Failure		404		{object}	payload.HTTPMessage			"One or more referenced resources not found"
//	@Failure		409		{object}	payload.HTTPMessage			"Policy already exists"
//	@Failure		413		{object}	payload.HTTPMessage			"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage			"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/policies [post]
//	@Security		AccessToken
func (ref *PoliciesHandler) create(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "create")
	defer span.End()

	var req payload.CreatePolicyRequest
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

	input := &domain.CreatePolicyInput{
		ID:              req.ID,
		Name:            req.Name,
		Description:     req.Description,
		AllowedAction:   req.AllowedAction,
		AllowedResource: req.AllowedResource,
	}

	if err := ref.service.Create(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.PolicyAlreadyExistsError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
			return
		}

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

	slog.Debug("handler.Policies.create: called", "policy.id", input.ID.String())
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "create policy",
		attribute.String("policy.id", input.ID.String()))

	// Location header is required for RESTful APIs
	respond.SetLocation(w, r, input.ID.String())
	respond.WriteJSONMessage(w, r, http.StatusCreated, domain.PoliciesPolicyCreatedSuccessfully)
}

// updateByID Update a policy by ID
//
//	@ID				01982303-f0f9-7ec1-8f39-98e77141c05c
//	@Summary		Update policy
//	@Description	Update an existing authorization policy by its unique identifier
//	@Tags			Policies
//	@Accept			json
//	@Produce		json
//	@Param			policy_id	path		string						true							"Unique policy identifier"	Format(uuid)
//	@Param			body		body		payload.UpdatePolicyRequest	true							"Policy update request payload"
//	@Success		200			{object}	payload.HTTPMessage			"Policy updated successfully"	{Location: "/policies/{policy_id}"}
//	@Failure		400			{object}	payload.HTTPMessage			"Invalid request body or validation error"
//	@Failure		401			{object}	payload.HTTPMessage			"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage			"System policies cannot be modified"
//	@Failure		404			{object}	payload.HTTPMessage			"Policy not found"
//	@Failure		409			{object}	payload.HTTPMessage			"Policy name already in use"
//	@Failure		413			{object}	payload.HTTPMessage			"Request body larger than http.server.max.body.bytes"
//	@Failure		415			{object}	payload.HTTPMessage			"Body not declared as application/json"
//	@Failure		429			{object}	payload.HTTPMessage			"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/policies/{policy_id} [put]
//	@Security		AccessToken
func (ref *PoliciesHandler) updateByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "updateByID")
	defer span.End()

	policyID, err := parseUUIDQueryParams(r.PathValue("policy_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.UpdatePolicyRequest
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

	input := &domain.UpdatePolicyInput{
		ID:              policyID,
		Name:            req.Name,
		Description:     req.Description,
		AllowedAction:   req.AllowedAction,
		AllowedResource: req.AllowedResource,
	}

	if err := ref.service.UpdateByID(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.PolicyAlreadyExistsError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.PolicyNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}

		if _, ok := errors.AsType[*domain.SystemPolicyError](err); ok {
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

	slog.Debug("handler.Policies.updateByID: called", "policy.id", input.ID.String())
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "update policy",
		attribute.String("policy.id", input.ID.String()))

	// Location header is required for RESTful APIs
	respond.SetLocation(w, r)
	respond.WriteJSONMessage(w, r, http.StatusOK, domain.PoliciesPolicyUpdatedSuccessfully)
}

// deleteByID Delete a policy by ID
//
//	@ID				01982303-f0f9-7f13-a03b-ed306ff7d06b
//	@Summary		Delete policy
//	@Description	Permanently remove an authorization policy by its unique identifier
//	@Tags			Policies
//	@Accept			json
//	@Produce		json
//	@Param			policy_id	path		string				true	"Unique policy identifier"	Format(uuid)
//	@Success		200			{object}	payload.HTTPMessage	"Policy deleted successfully"
//	@Failure		400			{object}	payload.HTTPMessage	"Invalid policy ID format"
//	@Failure		401			{object}	payload.HTTPMessage	"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage	"System policies cannot be deleted"
//	@Failure		429			{object}	payload.HTTPMessage	"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage	"Internal server error"
//	@Router			/policies/{policy_id} [delete]
//	@Security		AccessToken
func (ref *PoliciesHandler) deleteByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "deleteByID")
	defer span.End()

	policyID, err := parseUUIDQueryParams(r.PathValue("policy_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	input := &domain.DeletePolicyInput{
		ID: policyID,
	}

	if err := ref.service.DeleteByID(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.PolicyNotFoundError](err); ok {
			// gracefully handle the case where the policy is not found
			respond.WriteJSONMessage(w, r, http.StatusOK, domain.PoliciesPolicyDeletedSuccessfully)
			return
		}

		if _, ok := errors.AsType[*domain.SystemPolicyError](err); ok {
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

	slog.Debug("handler.Policies.deleteByID: called", "policy.id", input.ID.String())
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "delete policy",
		attribute.String("policy.id", input.ID.String()))

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.PoliciesPolicyDeletedSuccessfully)
}

// list Retrieves a paginated list of all the policies in the system
//
//	@ID				01982303-f0f9-7ee4-968d-ba2078a272fc
//	@Summary		List policies
//	@Description	Retrieve a paginated list of all authorization policies in the system
//	@Tags			Policies
//	@Accept			json
//	@Produce		json
//	@Param			sort		query		string							false	"Sort by fields (e.g., 'name ASC, created_at DESC')"
//	@Param			filter		query		string							false	"Filter conditions (e.g., 'name LIKE policy%')"
//	@Param			fields		query		string							false	"Comma-separated fields to return (e.g., 'id,name,description')"
//	@Param			next_token	query		string							false	"Token for next page of results"
//	@Param			prev_token	query		string							false	"Token for previous page of results"
//	@Param			limit		query		int								false	"Maximum number of items per page"
//	@Success		200			{object}	payload.ListPoliciesResponse	"Policies retrieved successfully"
//	@Failure		400			{object}	payload.HTTPMessage				"Invalid query parameters"
//	@Failure		401			{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/policies [get]
//	@Security		AccessToken
func (ref *PoliciesHandler) list(w http.ResponseWriter, r *http.Request) {
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

	input := &domain.ListPoliciesInput{
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

	outResponse := &payload.ListPoliciesResponse{
		Items:     make([]payload.PolicyResponse, len(out.Items)),
		Paginator: out.Paginator,
	}

	for i, policy := range out.Items {
		outResponse.Items[i] = payload.PolicyResponse{
			ID:          policy.ID,
			Name:        policy.Name,
			Description: policy.Description,
			System:      policy.System,
			Resource: payload.ResourceResponse{
				ID:   policy.Resource.ID,
				Name: policy.Resource.Name,
			},
			AllowedAction:   policy.AllowedAction,
			AllowedResource: policy.AllowedResource,
			CreatedAt:       policy.CreatedAt,
			UpdatedAt:       policy.UpdatedAt,
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

	slog.Debug("handler.Policies.list: called", "policies.count", len(outResponse.Items))
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "list policy",
		attribute.Int("policies.count", len(outResponse.Items)))
}

// linkRoles Link roles to policy
//
//	@ID				01982303-f0f9-7e0d-ab27-f75b3a03ef46
//	@Summary		Link roles to policy
//	@Description	Associate multiple roles with a specific policy for authorization
//	@Tags			Policies,Roles
//	@Accept			json
//	@Produce		json
//	@Param			policy_id	path		string								true									"Unique policy identifier"	Format(uuid)
//	@Param			body		body		payload.LinkRolesToPolicyRequest	true									"Roles linking request payload"
//	@Success		200			{object}	payload.HTTPMessage					"Roles linked to policy successfully"	{Location: "/policies/{policy_id}/roles/{policy_id}"}
//	@Failure		400			{object}	payload.HTTPMessage					"Invalid request body or validation error"
//	@Failure		401			{object}	payload.HTTPMessage					"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage					"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage					"Policy not found"
//	@Failure		413			{object}	payload.HTTPMessage					"Request body larger than http.server.max.body.bytes"
//	@Failure		415			{object}	payload.HTTPMessage					"Body not declared as application/json"
//	@Failure		429			{object}	payload.HTTPMessage					"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage					"Internal server error"
//	@Router			/policies/{policy_id}/roles [post]
//	@Security		AccessToken
func (ref *PoliciesHandler) linkRoles(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "linkRoles")
	defer span.End()

	policyID, err := parseUUIDQueryParams(r.PathValue("policy_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.LinkRolesToPolicyRequest
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

	input := &domain.LinkRolesToPolicyInput{
		PolicyID: policyID,
		RoleIDs:  req.RoleIDs,
	}

	if err := ref.service.LinkRoles(ctx, input); err != nil {
		_, isPolicyMissing := errors.AsType[*domain.PolicyNotFoundError](err)
		_, isRoleMissing := errors.AsType[*domain.RoleNotFoundError](err)
		if isPolicyMissing || isRoleMissing {
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

	slog.Debug("handler.Policies.linkRoles: called", "policy_id", policyID.String())
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "link roles to policy",
		attribute.String("policy.id", policyID.String()))

	// Location header is not needed for this endpoint
	respond.SetLocation(w, r, policyID.String())
	respond.WriteJSONMessage(w, r, http.StatusOK, domain.PoliciesRolesLinkedSuccessfully)
}

// unlinkRoles Unlink roles from policy
//
//	@ID				01982303-f0f9-7ed4-9630-b9af3e3b6f17
//	@Summary		Unlink roles from policy
//	@Description	Remove role associations from a specific policy
//	@Tags			Policies,Roles
//	@Accept			json
//	@Produce		json
//	@Param			policy_id	path		string									true										"Unique policy identifier"	Format(uuid)
//	@Param			body		body		payload.UnlinkRolesFromPolicyRequest	true										"Roles unlinking request payload"
//	@Success		200			{object}	payload.HTTPMessage						"Roles unlinked from policy successfully"	{Location: "/policies/{policy_id}/roles/{policy_id}"}
//	@Failure		400			{object}	payload.HTTPMessage						"Invalid request body or validation error"
//	@Failure		401			{object}	payload.HTTPMessage						"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage						"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage						"Policy not found"
//	@Failure		413			{object}	payload.HTTPMessage						"Request body larger than http.server.max.body.bytes"
//	@Failure		415			{object}	payload.HTTPMessage						"Body not declared as application/json"
//	@Failure		429			{object}	payload.HTTPMessage						"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage						"Internal server error"
//	@Router			/policies/{policy_id}/roles [delete]
//	@Security		AccessToken
func (ref *PoliciesHandler) unlinkRoles(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "unlinkRoles")
	defer span.End()

	policyID, err := parseUUIDQueryParams(r.PathValue("policy_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	var req payload.UnlinkRolesFromPolicyRequest
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

	input := &domain.UnlinkRolesFromPolicyInput{
		PolicyID: policyID,
		RoleIDs:  req.RoleIDs,
	}

	if err := ref.service.UnlinkRoles(ctx, input); err != nil {
		if _, ok := errors.AsType[*domain.PolicyNotFoundError](err); ok {
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

	slog.Debug("handler.Policies.unlinkRoles: called", "policy_id", policyID.String())
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "unlink roles from policy",
		attribute.String("policy.id", policyID.String()))

	// Location header is not needed for this endpoint
	respond.SetLocation(w, r, policyID.String())
	respond.WriteJSONMessage(w, r, http.StatusOK, domain.PoliciesRolesUnlinkedSuccessfully)
}

// listByRoleID List policies by role ID
//
//	@ID				01982303-f0fa-7036-9474-482fc8e5843d
//	@Summary		List policies by role
//	@Description	Retrieve a paginated list of policies associated with a specific role
//	@Tags			Policies,Roles
//	@Accept			json
//	@Produce		json
//	@Param			role_id		path		string							true	"Unique role identifier"	Format(uuid)
//	@Param			sort		query		string							false	"Sort by fields (e.g., 'name ASC, created_at DESC')"
//	@Param			filter		query		string							false	"Filter conditions (e.g., 'name LIKE policy%')"
//	@Param			fields		query		string							false	"Comma-separated fields to return (e.g., 'id,name,description')"
//	@Param			next_token	query		string							false	"Token for next page of results"
//	@Param			prev_token	query		string							false	"Token for previous page of results"
//	@Param			limit		query		int								false	"Maximum number of items per page"
//	@Success		200			{object}	payload.ListPoliciesResponse	"Policies retrieved successfully"
//	@Failure		400			{object}	payload.HTTPMessage				"Invalid query parameters"
//	@Failure		401			{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage				"No policies found for the given role"
//	@Failure		429			{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/roles/{role_id}/policies [get]
//	@Security		AccessToken
func (ref *PoliciesHandler) listByRoleID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "listByRoleID")
	defer span.End()

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

	input := &domain.ListPoliciesInput{
		Sort:   sort,
		Filter: filter,
		Fields: fields,
		Paginator: domain.Paginator{
			NextToken: nextToken,
			PrevToken: prevToken,
			Limit:     limit,
		},
	}

	out, err := ref.service.ListByRoleID(ctx, roleID, input)
	if err != nil {
		if _, ok := errors.AsType[*domain.PolicyNotFoundError](err); ok {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, httpStatusForListError(err), e.Error())
		return
	}

	outResponse := &payload.ListPoliciesResponse{
		Items:     make([]payload.PolicyResponse, len(out.Items)),
		Paginator: out.Paginator,
	}

	for i, policy := range out.Items {
		outResponse.Items[i] = payload.PolicyResponse{
			ID:          policy.ID,
			Name:        policy.Name,
			Description: policy.Description,
			System:      policy.System,
			Resource: payload.ResourceResponse{
				ID:   policy.Resource.ID,
				Name: policy.Resource.Name,
			},
			AllowedAction:   policy.AllowedAction,
			AllowedResource: policy.AllowedResource,
			CreatedAt:       policy.CreatedAt,
			UpdatedAt:       policy.UpdatedAt,
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

	slog.Debug("handler.Policies.listByRoleID: called", "policies.count", len(outResponse.Items))
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "list policies by role ID",
		attribute.Int("policies.count", len(outResponse.Items)),
		attribute.String("role.id", roleID.String()))
}
