package usecase

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

//go:generate go tool mockgen -package=mocks -destination=../../../mocks/service/health_app.go -source=health.go AppHealthProvider

// AppHealthProvider provides detailed health information from the app.
type AppHealthProvider interface {
	GetHealth(ctx context.Context) domain.DetailedHealth
}

type HealthServiceConf struct {
	Repository    repository.Health
	AppHealth     AppHealthProvider
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

type HealthService struct {
	repository      repository.Health
	appHealth       AppHealthProvider
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewHealthService creates a new HealthService.
func NewHealthService(conf HealthServiceConf) (*HealthService, error) {
	if conf.Repository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "Repository is nil, but it is required for HealthService"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for HealthService"}
	}

	ref := &HealthService{
		repository: conf.Repository,
		appHealth:  conf.AppHealth,
		ot:         conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Health",
			Action: "NewHealthService",
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
		metric.WithDescription(fmt.Sprintf("Duration of %s handler calls", AppLayer)),
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

// SetAppHealthProvider sets the app health provider after initialization.
// This is needed to avoid circular dependency during app startup.
func (ref *HealthService) SetAppHealthProvider(provider AppHealthProvider) {
	ref.appHealth = provider
}

// HealthCheck verifies a connection to the repository is still alive.
func (ref *HealthService) HealthCheck(ctx context.Context) (domain.Health, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "HealthCheck")
	defer span.End()

	// database
	dbStatus := domain.StatusUp
	err := ref.repository.PingContext(ctx)
	if err != nil {
		dbStatus = domain.StatusDown
	}

	database := domain.Check{
		Name:   "database",
		Kind:   ref.repository.DriverName(),
		Status: dbStatus,
	}

	// runtime
	//
	// Status only. This check used to carry runtime.MemStats, the Go version,
	// the CPU count and the goroutine count -- on an endpoint that is
	// UNAUTHENTICATED and, being under /health, exempt from the rate limiter.
	//
	// Two problems, and the second is the one that bites. It handed an
	// anonymous caller a version string to match against known advisories and a
	// picture of the process; and it made a 5 KB response available without a
	// token and without a limit, which is a cheap amplifier.
	//
	// The data is not lost -- it moved to /health/detailed, which now requires
	// authentication. A probe only ever reads the verdict.
	rtStatus := domain.StatusUp
	rt := domain.Check{
		Name:   "runtime",
		Kind:   "go",
		Status: rtStatus,
	}

	// and operator
	allStatus := dbStatus && rtStatus

	health := domain.Health{
		Status: allStatus,
		Checks: []domain.Check{
			database,
			rt,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "health check successful")

	return health, err
}

// GetDetailedHealth returns comprehensive health information including component status and metrics.
func (ref *HealthService) GetDetailedHealth(ctx context.Context) (domain.DetailedHealth, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetDetailedHealth")
	defer span.End()

	if ref.appHealth == nil {
		err := fmt.Errorf("app health provider not configured")
		_ = o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		return domain.DetailedHealth{
			Status: "unhealthy",
			Components: map[string]domain.ComponentHealth{
				"app": {
					Status:  "unhealthy",
					Message: "Health provider not configured",
				},
			},
		}, nil
	}

	// Get detailed health from the app
	detailedHealth := ref.appHealth.GetHealth(ctx)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "detailed health check successful")

	return detailedHealth, nil
}

// RuntimeInfo describes the Go process, for the AUTHENTICATED detailed health
// endpoint.
//
// It used to be part of the public status payload. An operator still wants it;
// an anonymous caller has no business with the Go version, and the response it
// produced was a 5 KB unauthenticated body on a rate-limit-exempt path.
func RuntimeInfo() map[string]any {
	mem := &runtime.MemStats{}
	runtime.ReadMemStats(mem)

	return map[string]any{
		"version":       runtime.Version(),
		"num_cpu":       strconv.Itoa(runtime.NumCPU()),
		"num_goroutine": strconv.Itoa(runtime.NumGoroutine()),
		"heap_alloc":    strconv.FormatUint(mem.HeapAlloc, 10),
		"heap_sys":      strconv.FormatUint(mem.HeapSys, 10),
		"gc_cycles":     strconv.FormatUint(uint64(mem.NumGC), 10),
	}
}
