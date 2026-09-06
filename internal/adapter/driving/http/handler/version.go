package handler

import (
	"fmt"
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

// VersionHandlerConf represents the configuration for the version handler.
type VersionHandlerConf struct {
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// VersionHandler represents the handler for the version of the service.
type VersionHandler struct {
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewVersionHandler returns a new instance of VersionHandler.
func NewVersionHandler(conf VersionHandlerConf) (*VersionHandler, error) {
	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is not configured"}
	}

	handler := &VersionHandler{
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Version",
			Action: "NewVersionHandler",
		},
	}

	if conf.MetricsPrefix != "" {
		handler.metricsPrefix = strings.ReplaceAll(conf.MetricsPrefix, "-", "_")
		handler.metricsPrefix += "_"
	}

	callsCounter, err := handler.ot.Metrics.Meter.Int64Counter(
		fmt.Sprintf("%s%s", handler.metricsPrefix, MetricCallsCounterName),
		metric.WithDescription(fmt.Sprintf("Total number of %s calls", AppLayer)),
	)
	if err != nil {
		return nil, err
	}

	callsDuration, err := handler.ot.Metrics.Meter.Float64Histogram(
		fmt.Sprintf("%s%s", handler.metricsPrefix, MetricDurationHistogramName),
		metric.WithDescription(fmt.Sprintf("Duration of %s calls", AppLayer)),
		metric.WithUnit("s"), // Seconds
	)
	if err != nil {
		return nil, err
	}

	handler.metrics = &o11y.LayerMetrics{
		Counter:   callsCounter,
		Histogram: callsDuration,
	}

	return handler, nil
}

// RegisterRoutes registers the routes for the version of the service.
func (ref *VersionHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /version", mdw.ThenFunc(ref.get))
}

// get returns the version of the service
//
//	@ID				01982303-f0f9-7dee-90c1-ca400b3d7b91
//	@Summary		Get service version
//	@Description	Retrieve the current version and build information of the service
//	@Tags			Version
//	@Produce		json
//	@Success		200	{object}	payload.Version		"Service version information retrieved successfully"
//	@Failure		500	{object}	payload.HTTPMessage	"Internal server error"
//	@Router			/version [get]
func (ref *VersionHandler) get(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "get")
	defer span.End()

	w.Header().Set("Content-Type", "application/json")

	// Only the version. The commit, branch and Go version identify the build
	// against published advisories and belong on the authenticated
	// /health/detailed answer, which /health/status already learned.
	outResponse := payload.Version{
		Version: version.Version,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)
		return
	}
}
