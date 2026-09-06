package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"uuid"

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

// TokenLifetimesHandlerConf represents the configuration for the TokenLifetimesHandler.
type TokenLifetimesHandlerConf struct {
	Service       driving.TokenLifetimes
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// TokenLifetimesHandler serves GET and PUT /auth/token_lifetimes.
//
// Two verbs, deliberately. The row is a singleton seeded by migration, so
// there is nothing to create or delete; "reset to defaults" is a PUT of the
// defaults every GET returns.
type TokenLifetimesHandler struct {
	service         driving.TokenLifetimes
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewTokenLifetimesHandler creates a new TokenLifetimesHandler.
func NewTokenLifetimesHandler(conf TokenLifetimesHandlerConf) (*TokenLifetimesHandler, error) {
	if conf.Service == nil {
		return nil, &domain.InvalidServiceError{Message: "driving.TokenLifetimes is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &TokenLifetimesHandler{
		service:       conf.Service,
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "TokenLifetimes",
			Action: "NewTokenLifetimesHandler",
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

// RegisterRoutes registers the token-lifetimes routes.
func (ref *TokenLifetimesHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /auth/token_lifetimes", mdw.ThenFunc(ref.get))
	mux.Handle("PUT /auth/token_lifetimes", mdw.ThenFunc(ref.update))
}

// get Read the token lifetimes
//
//	@ID				01a072df-d81d-78c8-a72b-6ea167cd50b2
//	@Summary		Get token lifetimes
//	@Description	Read how long the access and refresh tokens issued from now on will live, the bounds a change is validated against, the shipped defaults, and who last changed them
//	@Tags			Authn
//	@Produce		json
//	@Success		200	{object}	payload.TokenLifetimesResponse	"The stored lifetimes, with bounds and defaults"
//	@Failure		401	{object}	payload.HTTPMessage				"Invalid or expired token"
//	@Failure		403	{object}	payload.HTTPMessage				"Not authorized"
//	@Failure		429	{object}	payload.HTTPMessage				"Too many requests"
//	@Failure		500	{object}	payload.HTTPMessage				"Internal server error, including a missing row, which the migration seeds and nothing may delete"
//	@Router			/auth/token_lifetimes [get]
//	@Security		AccessToken
func (ref *TokenLifetimesHandler) get(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "get")
	defer span.End()

	out, err := ref.service.Get(ctx)
	if err != nil {
		// No 404: the row is seeded by migration and the service refuses to
		// start without it, so a missing row at request time is a fault.
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	if err := respond.WriteJSONData(w, http.StatusOK, payload.ToTokenLifetimesResponse(out)); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "get token lifetimes",
		attribute.String("access_token_duration", out.AccessTokenDuration.String()),
		attribute.String("refresh_token_duration", out.RefreshTokenDuration.String()),
	)
}

// update Replace the token lifetimes
//
//	@ID				01a072df-d81d-7995-ad3e-92a0a4d0a4cf
//	@Summary		Update token lifetimes
//	@Description	Replace the access and refresh token lifetimes. Applies to tokens issued from now on; a token already issued keeps the expiry it was signed with. The change reaches every replica within its reload interval, or at once where a change signal is available
//	@Tags			Authn
//	@Accept			json
//	@Produce		json
//	@Param			body	body		payload.UpdateTokenLifetimesRequest	true	"Both lifetimes as Go duration strings"
//	@Success		200		{object}	payload.TokenLifetimesResponse		"The stored lifetimes after the change"
//	@Failure		400		{object}	payload.HTTPMessage					"A duration that does not parse, a value outside its bounds, or a refresh lifetime not longer than the access lifetime"
//	@Failure		401		{object}	payload.HTTPMessage					"Invalid or expired token"
//	@Failure		403		{object}	payload.HTTPMessage					"Not authorized"
//	@Failure		413		{object}	payload.HTTPMessage					"Request body larger than http.server.max.body.bytes"
//	@Failure		415		{object}	payload.HTTPMessage					"Body not declared as application/json"
//	@Failure		429		{object}	payload.HTTPMessage					"Too many requests"
//	@Failure		500		{object}	payload.HTTPMessage					"Internal server error"
//	@Router			/auth/token_lifetimes [put]
//	@Security		AccessToken
func (ref *TokenLifetimesHandler) update(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "update")
	defer span.End()

	// The caller comes from the token the middleware verified, never from the
	// body: the change is attributed to whoever the request was authorised as.
	updatedBy, err := callerFromContext(r)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	var req payload.UpdateTokenLifetimesRequest
	if err := decodeJSONBody(r, &req); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteDecodeError(w, r, e)

		return
	}

	input, err := payload.ToUpdateTokenLifetimesInput(req)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return
	}

	input.UpdatedBy = updatedBy

	out, err := ref.service.Update(ctx, input)
	if err != nil {
		ref.writeTokenLifetimesUpdateError(ctx, w, r, span, start, attrs, err)

		return
	}

	if err := respond.WriteJSONData(w, http.StatusOK, payload.ToTokenLifetimesResponse(out)); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "update token lifetimes",
		attribute.String("access_token_duration", out.AccessTokenDuration.String()),
		attribute.String("refresh_token_duration", out.RefreshTokenDuration.String()),
		attribute.String("updated_by", updatedBy.String()),
	)
}

// writeTokenLifetimesUpdateError maps a failed update. No 404 (the row is a singleton the
// service will not run without) and no 409 (there is nothing to collide with).
func (ref *TokenLifetimesHandler) writeTokenLifetimesUpdateError(
	ctx context.Context, w http.ResponseWriter, r *http.Request,
	span trace.Span, start time.Time, attrs []attribute.KeyValue, err error,
) {
	e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)

	switch {
	case isType[*domain.ValidationErrors](err), isType[*domain.InvalidInputError](err):
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
	default:
		respond.WriteInternalError(w, r, e)
	}
}

// callerFromContext reads the subject of the token the middleware verified.
func callerFromContext(r *http.Request) (uuid.UUID, error) {
	jwtClaims, ok := r.Context().Value(middleware.JwtClaims).(map[string]any)
	if !ok {
		return uuid.Nil(), errors.New(domain.AuthnFailedToGetUserIDFromContext)
	}

	subject, ok := jwtClaims["sub"].(string)
	if !ok {
		return uuid.Nil(), errors.New(domain.AuthnFailedToGetUserIDFromContext)
	}

	id, err := parseUUIDQueryParams(subject)
	if err != nil {
		return uuid.Nil(), errors.New(domain.AuthnFailedToParseUserIDFromContext)
	}

	return id, nil
}
