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
	"go.opentelemetry.io/otel/trace"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driving"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// RateLimitsHandlerConf represents the configuration for the RateLimitsHandler.
type RateLimitsHandlerConf struct {
	Service       driving.RateLimits
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// RateLimitsHandler serves the rate-limit rule CRUD.
type RateLimitsHandler struct {
	service         driving.RateLimits
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewRateLimitsHandler creates a new RateLimitsHandler.
func NewRateLimitsHandler(conf RateLimitsHandlerConf) (*RateLimitsHandler, error) {
	if conf.Service == nil {
		return nil, &domain.InvalidServiceError{Message: "driving.RateLimits is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &RateLimitsHandler{
		service:       conf.Service,
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "RateLimits",
			Action: "NewRateLimitsHandler",
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
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	ref.metrics = &o11y.LayerMetrics{Counter: callsCounter, Histogram: callsDuration}

	return ref, nil
}

// bucketKeyDescription is the answer to "10 per minute per WHAT", returned in
// every effective-rules response.
//
// A constant, not a literal at the call site, because
// TestBucketKeyStringNamesEveryComponentOfTheRealKey asserts it against the key
// the middleware actually builds.
//
// It needs that guard because it drifted once. It read "(rule_id, scope_key) --
// one budget per rule", which was true when written and stopped being true when
// several windows per rule were added. It then said "window_id" for a while,
// which was accurate and described a BUG: a bucket keyed on the window id was
// reset by every edit, because PUT remints the window set. The key is the
// window's parameters now, so a rule carrying 10/s and 300/min holds two buckets
// and each survives an edit that leaves its numbers alone.
//
// The half that was always right is the one the field exists for: the VERB is
// not in the key, so a methods={GET,POST} rule shares one budget across both.
// This comment is deliberately not on the payload field -- swag turns a field's
// doc comment into the published API description, and a client developer needs
// the contract, not its history.
const bucketKeyDescription = "(rule_id, window parameters, scope_key) — one budget per window, shared across the rule's verbs, and kept across an edit that does not change the numbers"

// RegisterRoutes registers the rate-limit routes.
//
// /rate_limits/effective is registered BEFORE /rate_limits/{rate_limit_id}
// only for readability -- ServeMux prefers the more specific literal pattern
// regardless of registration order, so "effective" is never parsed as an id.
func (ref *RateLimitsHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /rate_limits", mdw.ThenFunc(ref.list))
	mux.Handle("GET /rate_limits/effective", mdw.ThenFunc(ref.effective))
	mux.Handle("POST /rate_limits", mdw.ThenFunc(ref.create))
	mux.Handle("GET /rate_limits/{rate_limit_id}", mdw.ThenFunc(ref.getByID))
	mux.Handle("PUT /rate_limits/{rate_limit_id}", mdw.ThenFunc(ref.updateByID))
	mux.Handle("DELETE /rate_limits/{rate_limit_id}", mdw.ThenFunc(ref.deleteByID))
}

// list List rate-limit rules
//
//	@ID				01a03a46-16d4-7831-9c94-a7975a9c4334
//	@Summary		List rate limits
//	@Description	List the rate-limit rules, with filtering, sorting, partial fields and pagination
//	@Tags			RateLimits
//	@Accept			json
//	@Produce		json
//	@Param			sort		query		string							false	"Sort expression, for example 'name ASC'"
//	@Param			filter		query		string							false	"Filter expression"
//	@Param			fields		query		string							false	"Comma-separated fields to return"
//	@Param			next_token	query		string							false	"Pagination token for the next page"
//	@Param			prev_token	query		string							false	"Pagination token for the previous page"
//	@Param			limit		query		int								false	"Maximum items per page"
//	@Success		200			{object}	payload.ListRateLimitsResponse	"Rate limits retrieved successfully"
//	@Failure		400			{object}	payload.HTTPMessage				"Invalid query parameters"
//	@Failure		401			{object}	payload.HTTPMessage				"Invalid or expired token"
//	@Failure		403			{object}	payload.HTTPMessage				"Not authorized"
//	@Failure		429			{object}	payload.HTTPMessage				"Too many requests"
//	@Failure		500			{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/rate_limits [get]
//	@Security		AccessToken
func (ref *RateLimitsHandler) list(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "list")
	defer span.End()

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

	input := &domain.SelectRateLimitsInput{
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
		ref.writeQueryError(ctx, w, r, span, start, attrs, err)

		return
	}

	items := make([]payload.RateLimitResponse, 0, len(out.Items))
	for i := range out.Items {
		items = append(items, payload.ToRateLimitResponse(&out.Items[i]))
	}

	response := payload.ListRateLimitsResponse{
		Items:     items,
		Paginator: out.Paginator,
		Enforcing: ref.service.Enforcing(),
	}

	if err := respond.WriteJSONData(w, http.StatusOK, response); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "list rate limits", attribute.Int("rate_limits.count", len(items)))
}

// getByID Get a rate-limit rule by ID
//
//	@ID				01a03a46-16d4-7af9-9f96-d9dc094afd80
//	@Summary		Get rate limit
//	@Description	Retrieve a rate-limit rule and its windows by unique identifier
//	@Tags			RateLimits
//	@Accept			json
//	@Produce		json
//	@Param			rate_limit_id	path		string						true	"Unique rate limit identifier"	Format(uuid)
//	@Success		200				{object}	payload.RateLimitResponse	"Rate limit retrieved successfully"
//	@Failure		400				{object}	payload.HTTPMessage			"Invalid rate limit ID format"
//	@Failure		401				{object}	payload.HTTPMessage			"Invalid or expired token"
//	@Failure		403				{object}	payload.HTTPMessage			"Not authorized"
//	@Failure		404				{object}	payload.HTTPMessage			"Rate limit not found"
//	@Failure		429				{object}	payload.HTTPMessage			"Too many requests"
//	@Failure		500				{object}	payload.HTTPMessage			"Internal server error"
//	@Router			/rate_limits/{rate_limit_id} [get]
//	@Security		AccessToken
func (ref *RateLimitsHandler) getByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "getByID")
	defer span.End()

	// parseUUIDQueryParams, not uuid.Parse: it already derives a readable reason
	// rather than forwarding the standard library's flat "invalid uuid" into
	// this API's contract.
	id, err := parseUUIDQueryParams(r.PathValue("rate_limit_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return
	}

	out, err := ref.service.GetByID(ctx, id)
	if err != nil {
		ref.writeGetError(ctx, w, r, span, start, attrs, err)

		return
	}

	if err := respond.WriteJSONData(w, http.StatusOK, payload.ToRateLimitResponse(out)); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "get rate limit", attribute.String("rate_limit.id", id.String()))
}

// create Create a rate-limit rule
//
//	@ID				01a03a46-16d4-7ad9-b646-0bc67824b38c
//	@Summary		Create rate limit
//	@Description	Create a rate-limit rule. The target is validated against the endpoint catalogue, so a rule for a route this service does not serve is refused rather than silently protecting nothing
//	@Tags			RateLimits
//	@Accept			json
//	@Produce		json
//	@Param			body	body		payload.CreateRateLimitRequest	true	"Rate limit creation request payload"
//	@Success		201		{object}	payload.HTTPMessage				"Rate limit created successfully"
//	@Failure		400		{object}	payload.HTTPMessage				"Invalid request body, unknown strategy, or a target no route matches"
//	@Failure		401		{object}	payload.HTTPMessage				"Invalid or expired token"
//	@Failure		403		{object}	payload.HTTPMessage				"Not authorized"
//	@Failure		409		{object}	payload.HTTPMessage				"A rate limit with that name already exists"
//	@Failure		413		{object}	payload.HTTPMessage				"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage				"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage				"Too many requests"
//	@Failure		500		{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/rate_limits [post]
//	@Security		AccessToken
func (ref *RateLimitsHandler) create(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "create")
	defer span.End()

	var req payload.CreateRateLimitRequest
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

	windows, err := payload.ToRateLimitWindows(req.Windows)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return
	}

	input := &domain.CreateRateLimitInput{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		TargetKind:  domain.RateLimitTargetKind(req.TargetKind),
		Target:      req.Target,
		Methods:     req.Methods,
		Scope:       domain.RateLimitScope(req.Scope),
		Audience:    audienceOrDefault(req.Audience),
		Strategy:    strategyOrDefault(req.Strategy),
		Enabled:     req.Enabled,
		Windows:     windows,
	}

	if err := ref.service.Create(ctx, input); err != nil {
		ref.writeCreateError(ctx, w, r, span, start, attrs, err)

		return
	}

	respond.SetLocation(w, r, input.ID.String())
	respond.WriteJSONMessage(w, r, http.StatusCreated, domain.RateLimitsRateLimitCreatedSuccessfully)

	slog.Debug("handler.RateLimits.create: called", "rate_limit.id", input.ID.String())
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "create rate limit",
		attribute.String("rate_limit.id", input.ID.String()),
		attribute.String("rate_limit.strategy", string(input.Strategy)),
	)
}

// updateByID Replace a rate-limit rule
//
//	@ID				01a03a46-16d4-7b0a-af95-6805d68a37d3
//	@Summary		Update rate limit
//	@Description	Replace a rate-limit rule. The window set is replaced in full, not merged
//	@Tags			RateLimits
//	@Accept			json
//	@Produce		json
//	@Param			rate_limit_id	path		string							true	"Unique rate limit identifier"	Format(uuid)
//	@Param			body			body		payload.UpdateRateLimitRequest	true	"Rate limit update request payload"
//	@Success		200				{object}	payload.HTTPMessage				"Rate limit updated successfully"
//	@Failure		400				{object}	payload.HTTPMessage				"Invalid request body, unknown strategy, or a target no route matches"
//	@Failure		401				{object}	payload.HTTPMessage				"Invalid or expired token"
//	@Failure		403				{object}	payload.HTTPMessage				"Not authorized, or the rule is system-managed"
//	@Failure		404				{object}	payload.HTTPMessage				"Rate limit not found"
//	@Failure		409				{object}	payload.HTTPMessage				"A rate limit with that name already exists"
//	@Failure		413				{object}	payload.HTTPMessage				"Request body larger than http.server.max.body.bytes"
//	@Failure		415				{object}	payload.HTTPMessage				"Body not declared as application/json"
//	@Failure		429				{object}	payload.HTTPMessage				"Too many requests"
//	@Failure		500				{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/rate_limits/{rate_limit_id} [put]
//	@Security		AccessToken
func (ref *RateLimitsHandler) updateByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "updateByID")
	defer span.End()

	// parseUUIDQueryParams, not uuid.Parse: it already derives a readable reason
	// rather than forwarding the standard library's flat "invalid uuid" into
	// this API's contract.
	id, err := parseUUIDQueryParams(r.PathValue("rate_limit_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return
	}

	var req payload.UpdateRateLimitRequest
	if err := decodeJSONBody(r, &req); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteDecodeError(w, r, e)

		return
	}

	windows, err := payload.ToRateLimitWindows(req.Windows)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return
	}

	input := &domain.UpdateRateLimitInput{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		TargetKind:  domain.RateLimitTargetKind(req.TargetKind),
		Target:      req.Target,
		Methods:     req.Methods,
		Scope:       domain.RateLimitScope(req.Scope),
		Audience:    audienceOrDefault(req.Audience),
		Strategy:    strategyOrDefault(req.Strategy),
		Enabled:     req.Enabled,
		Windows:     windows,
	}

	if err := ref.service.UpdateByID(ctx, input); err != nil {
		ref.writeUpdateError(ctx, w, r, span, start, attrs, err)

		return
	}

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.RateLimitsRateLimitUpdatedSuccessfully)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "update rate limit", attribute.String("rate_limit.id", id.String()))
}

// deleteByID Delete a rate-limit rule
//
//	@ID				01a03a46-16d4-7b1a-913f-e9e50f9acfa7
//	@Summary		Delete rate limit
//	@Description	Delete a rate-limit rule. Its windows are removed with it
//	@Tags			RateLimits
//	@Accept			json
//	@Produce		json
//	@Param			rate_limit_id	path		string				true	"Unique rate limit identifier"	Format(uuid)
//	@Success		200				{object}	payload.HTTPMessage	"Rate limit deleted successfully"
//	@Failure		400				{object}	payload.HTTPMessage	"Invalid rate limit ID format"
//	@Failure		401				{object}	payload.HTTPMessage	"Invalid or expired token"
//	@Failure		403				{object}	payload.HTTPMessage	"Not authorized, or the rule is system-managed"
//	@Failure		404				{object}	payload.HTTPMessage	"Rate limit not found"
//	@Failure		429				{object}	payload.HTTPMessage	"Too many requests"
//	@Failure		500				{object}	payload.HTTPMessage	"Internal server error"
//	@Router			/rate_limits/{rate_limit_id} [delete]
//	@Security		AccessToken
func (ref *RateLimitsHandler) deleteByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "deleteByID")
	defer span.End()

	// parseUUIDQueryParams, not uuid.Parse: it already derives a readable reason
	// rather than forwarding the standard library's flat "invalid uuid" into
	// this API's contract.
	id, err := parseUUIDQueryParams(r.PathValue("rate_limit_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return
	}

	if err := ref.service.DeleteByID(ctx, &domain.DeleteRateLimitInput{ID: id}); err != nil {
		ref.writeDeleteError(ctx, w, r, span, start, attrs, err)

		return
	}

	respond.WriteJSONMessage(w, r, http.StatusOK, domain.RateLimitsRateLimitDeletedSuccessfully)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "delete rate limit", attribute.String("rate_limit.id", id.String()))
}

// effective Show which rules apply to a request
//
//	@ID				01a03a46-16d4-7b2b-8932-ef9694d8f940
//	@Summary		Effective rate limits
//	@Description	Resolve which rules apply to a method and endpoint, one per scope, most specific first. Resolved with the same function the limiter uses, so it cannot disagree with what is enforced
//	@Tags			RateLimits
//	@Accept			json
//	@Produce		json
//	@Param			method			query		string								true	"HTTP method, uppercase"	Enums(GET,POST,PUT,PATCH,DELETE,OPTIONS,HEAD)
//	@Param			endpoint		query		string								true	"Route template, for example /projects/{project_id}/products"
//	@Param			authenticated	query		bool								false	"Whether to resolve as an authenticated caller. Defaults to true"
//	@Success		200				{object}	payload.EffectiveRateLimitsResponse	"Effective rules resolved"
//	@Failure		400				{object}	payload.HTTPMessage					"method or endpoint missing or invalid"
//	@Failure		401				{object}	payload.HTTPMessage					"Invalid or expired token"
//	@Failure		403				{object}	payload.HTTPMessage					"Not authorized"
//	@Failure		429				{object}	payload.HTTPMessage					"Too many requests"
//	@Failure		500				{object}	payload.HTTPMessage					"Internal server error"
//	@Router			/rate_limits/effective [get]
//	@Security		AccessToken
func (ref *RateLimitsHandler) effective(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "effective")
	defer span.End()

	method := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("method")))
	endpoint := strings.TrimSpace(r.URL.Query().Get("endpoint"))

	// method is REQUIRED. Without it the answer is ambiguous, because naming a
	// verb beats * at every tier and an endpoint usually has rules for both --
	// so a default would quietly answer a different question than the one asked.
	if method == "" {
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, "method is required, and must be one of "+domain.GetValidActions())

		return
	}

	if err := domain.IsValidAction(method); err != nil {
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, err.Error())

		return
	}

	if endpoint == "" {
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, "endpoint is required, and must be a route template such as /projects/{project_id}/generate")

		return
	}

	// Defaults to true: an authenticated caller sees both `any` and `auth`
	// rules, which is the superset. Defaulting to guest would hide every rule
	// written for authenticated traffic, which is most of them.
	authenticated := r.URL.Query().Get("authenticated") != "false"

	matches, err := ref.service.Effective(ctx, domain.RateLimitRequest{
		Method:        method,
		Pattern:       endpoint,
		Authenticated: authenticated,
	})
	if err != nil {
		ref.writeQueryError(ctx, w, r, span, start, attrs, err)

		return
	}

	entries := make([]payload.EffectiveRateLimitEntry, 0, len(matches))

	for _, m := range matches {
		resp := payload.ToRateLimitResponse(m.Rule)
		entries = append(entries, payload.EffectiveRateLimitEntry{
			RuleID:   m.Rule.ID,
			Name:     m.Rule.Name,
			Scope:    string(m.Rule.Scope),
			Strategy: string(m.Rule.Strategy),
			Windows:  resp.Windows,
			Why:      m.Why,
		})
	}

	response := payload.EffectiveRateLimitsResponse{
		Method:    method,
		Endpoint:  endpoint,
		Effective: entries,
		Enforcing: ref.service.Enforcing(),
		BucketKey: bucketKeyDescription,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, response); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "effective rate limits",
		attribute.String("method", method),
		attribute.String("endpoint", endpoint),
		attribute.Int("matches", len(entries)),
	)
}

// The error mapping is deliberately NOT one shared helper.
//
// It was, and TestEverySwaggerStatusIsDeclared rejected it: the guard follows
// called functions, so a single helper makes EVERY method look capable of
// writing every code it maps -- list would appear to return 404 and 409. The
// repo's rule is explicit that documenting a code a handler cannot return is
// worse than silence, because it invents a branch for every generated client.
//
// So each shape maps only what its methods can actually produce. The cost is
// four small functions; the benefit is that the annotations and the code cannot
// drift apart without the guard noticing.

// writeQueryError handles a read that cannot 404: list and effective. A missing
// page is an empty page, not a not-found.
func (ref *RateLimitsHandler) writeQueryError(
	ctx context.Context, w http.ResponseWriter, r *http.Request,
	span trace.Span, start time.Time, attrs []attribute.KeyValue, err error,
) {
	e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)

	if isBadRequest(err) {
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return
	}

	respond.WriteInternalError(w, r, e)
}

// writeGetError handles a read of one rule, which can 404.
func (ref *RateLimitsHandler) writeGetError(
	ctx context.Context, w http.ResponseWriter, r *http.Request,
	span trace.Span, start time.Time, attrs []attribute.KeyValue, err error,
) {
	e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)

	switch {
	case isType[*domain.RateLimitNotFoundError](err):
		respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
	case isBadRequest(err):
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
	default:
		respond.WriteInternalError(w, r, e)
	}
}

// writeCreateError handles a create: no 404 (nothing is being looked up) and no
// 403 (the system trigger guards UPDATE and DELETE, never INSERT).
func (ref *RateLimitsHandler) writeCreateError(
	ctx context.Context, w http.ResponseWriter, r *http.Request,
	span trace.Span, start time.Time, attrs []attribute.KeyValue, err error,
) {
	e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)

	switch {
	case isType[*domain.RateLimitAlreadyExistsError](err):
		respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
	case isBadRequest(err):
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
	default:
		respond.WriteInternalError(w, r, e)
	}
}

// writeUpdateError handles an update: a missing rule, a name that now collides,
// and the system trigger's refusal.
func (ref *RateLimitsHandler) writeUpdateError(
	ctx context.Context, w http.ResponseWriter, r *http.Request,
	span trace.Span, start time.Time, attrs []attribute.KeyValue, err error,
) {
	e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)

	switch {
	case isType[*domain.RateLimitNotFoundError](err):
		respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
	case isType[*domain.RateLimitAlreadyExistsError](err):
		respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
	case isType[*domain.SystemRateLimitError](err):
		respond.WriteJSONMessage(w, r, http.StatusForbidden, e.Error())
	case isBadRequest(err):
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
	default:
		respond.WriteInternalError(w, r, e)
	}
}

// writeDeleteError handles a delete. NO 409: a delete cannot collide with
// another rule's name, and declaring a code that cannot happen invents a branch
// for every generated client. TestEverySwaggerStatusIsDeclared caught exactly
// that when update and delete shared one helper.
func (ref *RateLimitsHandler) writeDeleteError(
	ctx context.Context, w http.ResponseWriter, r *http.Request,
	span trace.Span, start time.Time, attrs []attribute.KeyValue, err error,
) {
	e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)

	switch {
	case isType[*domain.RateLimitNotFoundError](err):
		respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
	case isType[*domain.SystemRateLimitError](err):
		respond.WriteJSONMessage(w, r, http.StatusForbidden, e.Error())
	case isBadRequest(err):
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
	default:
		respond.WriteInternalError(w, r, e)
	}
}

// isBadRequest reports whether an error is the caller's fault.
func isBadRequest(err error) bool {
	return isType[*domain.InvalidRateLimitTargetError](err) ||
		isType[*domain.InvalidRateLimitStrategyError](err) ||
		isType[*domain.ValidationErrors](err) ||
		isType[*domain.InvalidInputError](err) ||
		isType[*domain.InvalidByteSequenceError](err) ||
		isType[*domain.InvalidMessageFormatError](err) ||
		isType[*domain.UndefinedColumnError](err) ||
		isType[*domain.DatatypeMismatchError](err)
}

func isType[T error](err error) bool {
	_, ok := errors.AsType[T](err)

	return ok
}

// audienceOrDefault applies the column default when the caller omits it.
func audienceOrDefault(in string) domain.RateLimitAudience {
	if in == "" {
		return domain.RateLimitAudienceAny
	}

	return domain.RateLimitAudience(in)
}

// strategyOrDefault applies the column default when the caller omits it.
//
// It defaults ONLY an empty string. An unrecognised value is passed through
// unchanged so validation refuses it and names the valid ones -- silently
// defaulting a typo would enforce a limiter the operator did not ask for, with
// nothing anywhere to say so.
func strategyOrDefault(in string) domain.RateLimitStrategy {
	if in == "" {
		return domain.RateLimitStrategyTokenBucket
	}

	return domain.RateLimitStrategy(in)
}
