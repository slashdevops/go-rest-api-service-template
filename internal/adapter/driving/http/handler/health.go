package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
	"github.com/slashdevops/go-rest-api-service-template/internal/version"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/handler/health.go -source=health.go HealthService

// HealthService represents the service for the health.
type HealthService interface {
	HealthCheck(ctx context.Context) (payload.Health, error)
	GetDetailedHealth(ctx context.Context) (payload.DetailedHealth, error)
}

// HealthHandlerConf represents the configuration for the HealthHandler.
type HealthHandlerConf struct {
	Service       HealthService
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// HealthHandler represents the handler for the health.
type HealthHandler struct {
	service         HealthService
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(conf HealthHandlerConf) (*HealthHandler, error) {
	if conf.Service == nil {
		return nil, &domain.InvalidServiceError{Message: "HealthService is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &HealthHandler{
		service: conf.Service,
		ot:      conf.OT,
	}

	if conf.MetricsPrefix != "" {
		ref.metricsPrefix = strings.ReplaceAll(conf.MetricsPrefix, "-", "_")
		ref.metricsPrefix += "_"
	}

	ref.metricsMetadata = o11y.Metadata{
		Layer:  AppLayer,
		Domain: "Health",
		Action: "NewHealthHandler",
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
//
// The probe endpoints and the detailed one take DIFFERENT middleware, and the
// split is the point:
//
//   - /health/live and /health/status must answer without a token. An
//     orchestrator has no credentials, and a readiness probe that 401s is a
//     replica that never joins the load balancer.
//   - /health/detailed must NOT. It names every component, its configuration
//     and its timings, and until this split it did so to anyone who asked --
//     on a path that is also exempt from the rate limiter.
func (ref *HealthHandler) RegisterRoutes(mux *http.ServeMux, public middleware.Middleware, authenticated middleware.Middleware) {
	mux.Handle("GET /health/live", public.ThenFunc(ref.getLiveness))
	mux.Handle("GET /health/status", public.ThenFunc(ref.getStatus))
	mux.Handle("GET /health/detailed", authenticated.ThenFunc(ref.getDetailedHealth))
}

// getLiveness Report that the process is alive and serving.
//
//	@ID				01a02ec9-ac6c-77c2-81ad-d6e2f23bcd92
//	@Summary		Liveness probe
//	@Description	Answers 200 whenever this process can serve HTTP. It checks NOTHING else — no database, no cache — on purpose: a liveness probe decides whether to RESTART the process, and restarting cannot fix a dependency that is down. Point a readiness probe at /health/detailed instead, which does reflect dependencies.
//	@Tags			Health
//	@Produce		json
//	@Success		200	{object}	payload.HTTPMessage	"The process is alive and serving requests"
//	@Router			/health/live [get]
func (ref *HealthHandler) getLiveness(w http.ResponseWriter, r *http.Request) {
	// Deliberately does no work at all.
	//
	// This is the probe an orchestrator uses to decide whether to KILL the
	// process, so it must answer from the process itself and nothing else.
	// /health/status looks like a liveness probe and is not safe as one: it
	// pings the database with a five second budget, so a hung Postgres would
	// hang the probe, and the orchestrator would restart a service whose only
	// problem was somewhere else -- turning a database outage into a restart
	// loop on top of a database outage.
	//
	// Reaching this handler is the whole check: the listener accepted, the
	// router matched, and the goroutine ran.
	//
	// One thing it does NOT escape: the IP rate limiter wraps every route
	// under the API prefix, this one included, so a probe source that shares
	// an address with client traffic can be answered 429 -- which every
	// orchestrator reads as a failed probe. At the shipped 100 req/s, burst
	// 300, per client IP, a probe every few seconds from its own address
	// cannot reach that. Give the probe its own source, or raise the limit.
	respond.WriteJSONMessage(w, r, http.StatusOK, "alive")
}

// healthCheckFailedMessage is what a failed health check tells a caller, and it
// is deliberately all it tells them.
//
// Never the underlying error. These endpoints are public and unauthenticated --
// they are registered before the authentication chain, because a probe cannot
// hold a token -- and the pgx failure text names the database user, the
// database, and every address the pool tried:
//
//	failed to connect to `user=username database=go-rest-api-service-template`:
//		[::1]:5432 (localhost): tls error: EOF
//
// That is free reconnaissance for anyone who can reach the port, handed over by
// an endpoint that exists to be polled. The reason still reaches the span and an
// ERROR log, which is where an operator should be reading it from anyway.
//
// Same rule as everywhere else in this service: never forward a library's error
// string into an API response.
const healthCheckFailedMessage = "health check failed"

// getStatus Get the health of the health service
//
//	@ID				01982303-f0f9-7eec-8bf3-84f51fd09b73
//	@Summary		Health summary (diagnostic, not a probe)
//	@Description	A human-readable summary including database connectivity and runtime metrics. NOT a probe: it pings the database inside a five second budget, so it hangs as long as the database does, and when the ping fails it answers 500 and discards the summary — no verdict a probe could act on. Point liveness at /health/live and readiness at /health/detailed.
//	@Tags			Health
//	@Produce		json
//	@Success		200	{object}	payload.Health		"Service health status retrieved successfully"
//	@Failure		500	{object}	payload.HTTPMessage	"The check could not be completed -- currently, the database ping failed. The body carries a fixed message and never the underlying error: this endpoint is public, and the driver's text names the database user, the database and the addresses tried. The reason is on the span and in an ERROR log"
//	@Router			/health/status [get]
func (ref *HealthHandler) getStatus(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "getStatus")
	defer span.End()

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	outResponse, err := ref.service.HealthCheck(ctxWithTimeout)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		slog.Error("health status check failed", "error", e)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, healthCheckFailedMessage)
		return
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		slog.Error("writing the health status response failed", "error", e)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, healthCheckFailedMessage)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "health status checked")
}

// httpStatusForHealth maps the overall health status to a response code.
//
// 206 rather than 200 for degraded so a caller can tell "working, but something
// is wrong" from "working", and 503 for unhealthy so a load balancer or a
// readiness probe can act on it. Both are part of the published contract and
// are declared on the handler; TestEverySwaggerStatusIsDeclared fails if this
// grows a code the annotations do not mention.
func httpStatusForHealth(status string) int {
	switch status {
	case "unhealthy":
		return http.StatusServiceUnavailable
	case "degraded":
		return http.StatusPartialContent
	default:
		return http.StatusOK
	}
}

// getDetailedHealth Get detailed health information for all app components
//
//	@ID				01982304-a1b2-7eec-8bf3-84f51fd09b74
//	@Summary		Readiness probe and detailed health
//	@Description	Per-component health, database pool stats and startup metrics, with the status code carrying the verdict: 200 healthy, 206 degraded, 503 a hard dependency is unreachable. This is the READINESS target — it answers whether this instance should receive traffic. Point liveness at /health/live, which deliberately checks nothing.
//	@Tags			Health
//	@Produce		json
//	@Success		200	{object}	payload.DetailedHealth	"Every component is healthy"
//	@Success		206	{object}	payload.DetailedHealth	"One or more components are DEGRADED; the same payload is returned with the per-component details. A client polling this endpoint must treat 206 as a reachable service, not as a failure. Note that ratelimit_store degraded under the default fail-closed mode means the service is refusing every request with 429 -- reachable, but serving nothing"
//	@Failure		401	{object}	payload.HTTPMessage		"Invalid or expired token. This endpoint names every component, its configuration and its timings, so it requires authentication -- unlike /health/live and /health/status, which an orchestrator must be able to reach without a token"
//	@Failure		403	{object}	payload.HTTPMessage		"Not authorized"
//	@Failure		429	{object}	payload.HTTPMessage		"Too many requests"
//	@Failure		500	{object}	payload.HTTPMessage		"The check could not be completed. The body carries a fixed message and never the underlying error. The reason is on the span and in an ERROR log"
//	@Failure		503	{object}	payload.DetailedHealth	"A HARD dependency is unreachable -- currently the database -- so this instance cannot serve. A load balancer or readiness probe should take it out of rotation. Two components are deliberately excluded from this code and can only reach 206: the cache, because it is fail-open and a request still succeeds without it; and the rate-limit store, because health and version bypass the limiter precisely so its outage cannot evict a replica, and failing readiness would reintroduce that eviction by the other door -- on every replica at once, since they share the store"
//	@Router			/health/detailed [get]
//	@Security		AccessToken
func (ref *HealthHandler) getDetailedHealth(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "getDetailedHealth")
	defer span.End()

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	outResponse, err := ref.service.GetDetailedHealth(ctxWithTimeout)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		slog.Error("detailed health check failed", "error", e)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, healthCheckFailedMessage)
		return
	}

	// The build is stamped here, at the transport, from the binary's own
	// metadata; the use case has no business knowing a git commit.
	outResponse.Build = domain.BuildInfo{
		Version:       version.Version,
		BuildDate:     version.BuildDate,
		GitCommit:     version.GitCommit,
		GitBranch:     version.GitBranch,
		GoVersion:     version.GoVersion,
		GoVersionArch: version.GoVersionArch,
		GoVersionOS:   version.GoVersionOS,
	}

	statusCode := httpStatusForHealth(outResponse.Status)

	if err := respond.WriteJSONData(w, statusCode, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		slog.Error("writing the detailed health response failed", "error", e)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, healthCheckFailedMessage)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "detailed health checked")
}
