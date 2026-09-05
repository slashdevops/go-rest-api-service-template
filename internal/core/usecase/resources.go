package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"uuid"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/cache"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
	"github.com/slashdevops/go-rest-api-service-template/pkg/cslog"
)

type ResourcesServiceConf struct {
	Repository    repository.Resources
	CacheService  cache.Cache
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

type ResourcesService struct {
	repository      repository.Resources
	cacheService    cache.Cache
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewResourcesService creates a new ResourcesService.
func NewResourcesService(conf ResourcesServiceConf) (*ResourcesService, error) {
	if conf.Repository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "Repository is nil, but it is required for ResourcesService"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for ResourcesService"}
	}

	ref := &ResourcesService{
		repository:   conf.Repository,
		cacheService: conf.CacheService,
		ot:           conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Resources",
			Action: "NewResourcesService",
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

// GetByID returns the Resources with the specified ID.
func (ref *ResourcesService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Resource, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetByID")
	defer span.End()

	span.SetAttributes(
		attribute.String("Resources.id", id.String()),
	)

	if !domain.IsUUIDV7(id) {
		errorType := &domain.InvalidResourceIDError{ID: id.String(), Message: "ID is empty"}
		slog.Error("service.Resources.GetByID", "error", errorType)
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	var out *domain.Resource
	var err error

	resourceFetcher := func(ctx context.Context) (*domain.Resource, []cache.Identifier, error) {
		out, err := ref.repository.SelectByID(ctx, id)
		if err != nil {
			return nil, nil, err
		}

		// FIXME: add dependencies to cache invalidation when resource changes
		// these could provided from the repository layer as well.

		return out, nil, nil
	}

	if ref.cacheService == nil {
		slog.Debug("service.Resources.GetByID", "cache", "disabled")

		out, _, err = resourceFetcher(ctx)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	} else {

		cacheKey := cache.Identifier{
			Type: "resource",
			ID:   id.String(),
		}

		slog.Debug("service.Resources.GetByID", "cache", "enabled")
		out, err = cache.GetTyped[*domain.Resource](ctx, ref.cacheService, cacheKey, resourceFetcher)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Resources found", attribute.String("resources.method", out.Action))
	return out, nil
}

// List returns a list of resources.
func (ref *ResourcesService) List(ctx context.Context, input *domain.ListResourcesInput) (*domain.ListResourcesOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "List")
	defer span.End()

	span.SetAttributes(
		attribute.String("sort", input.Sort),
		attribute.String("fields", input.Fields),
		attribute.String("filter", input.Filter),
		attribute.Int("limit", input.Paginator.Limit),
	)

	out, err := ref.repository.Select(ctx, input)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Resources found")

	return out, nil
}

// ListMatches returns a list of policies that match the given action and resource.
func (ref *ResourcesService) ListMatches(ctx context.Context, action, resource string, input *domain.ListResourcesInput) (*domain.ListResourcesOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ListMatches")
	defer span.End()

	if err := domain.IsValidAction(action); err != nil {
		errType := &domain.InvalidActionError{Action: action}
		return nil, o11y.RecordError(ctx, span, start, errType, ref.metrics, attrs)
	}

	if err := domain.IsValidResource(resource); err != nil {
		errType := &domain.InvalidResourceError{Resource: resource, Message: "resource  cannot be empty"}
		return nil, o11y.RecordError(ctx, span, start, errType, ref.metrics, attrs)
	}

	cslog.Trace(ctx, "service.Resources.ListMatches", "action", action, "resource", resource)

	var out *domain.SelectResourcesOutput
	var err error

	switch {
	// This case handles when both action and resource are "*"
	case action == "*" && resource == "*":
		out, err = ref.repository.Select(ctx, &domain.SelectResourcesInput{
			Filter:    fmt.Sprintf("action = '%s' AND resource = '%s'", action, resource),
			Paginator: input.Paginator,
		})
		if err != nil || out == nil || len(out.Items) == 0 {
			return nil, &domain.ResourceNotFoundError{
				Message: fmt.Sprintf("does not found exist any resource with action = '%s' and resource as '%s'", action, resource),
			}
		}

		cslog.Trace(ctx, "service.Resources.ListMatches", "matched_resources_count", len(out.Items), "action", action, "resource", resource, "case", "action=* resource=*")

		// This case handles when action is "*" and resource is a specific value (not "*" or empty)
	case action == "*" && resource != "*" && resource != "" && resource != "/":
		resourceWithWildcard := convertToSQLRegex(resource)

		out, err = ref.repository.Select(ctx, &domain.SelectResourcesInput{
			Filter:    fmt.Sprintf("resource ~ '%s'", resourceWithWildcard),
			Paginator: input.Paginator,
		})
		if err != nil || out == nil || len(out.Items) == 0 {
			return nil, &domain.ResourceNotFoundError{
				Message: fmt.Sprintf("does not found exist any resource with action = '%s' and resource as '%s'", action, resource),
			}
		}

		cslog.Trace(ctx, "service.Resources.ListMatches", "matched_resources_count", len(out.Items), "action", action, "resource", resource, "case", "action=* resource=specific")

		// This case handles when action is a specific value (not "*") and resource is a specific value (not "*")
	case action != "*" && resource != "*" && resource != "" && resource != "/":
		resourceWithWildcard := convertToSQLRegex(resource)

		out, err = ref.repository.Select(ctx, &domain.SelectResourcesInput{
			Filter:    fmt.Sprintf("action = '%s' AND resource ~ '%s'", action, resourceWithWildcard),
			Paginator: input.Paginator,
		})
		if err != nil || out == nil || len(out.Items) == 0 {
			return nil, &domain.ResourceNotFoundError{
				Message: fmt.Sprintf("does not found exist any resource with action = '%s' and resource as '%s'", action, resource),
			}
		}

		cslog.Trace(ctx, "service.Resources.ListMatches", "matched_resources_count", len(out.Items), "action", action, "resource", resource, "case", "action=specific resource=specific")

		// This case handles when action is a specific value (not "*") and resource is "*"
	case action != "*" && resource == "*":
		out, err = ref.repository.Select(ctx, &domain.SelectResourcesInput{
			Filter:    fmt.Sprintf("action = '%s' AND resource = '%s'", action, resource),
			Paginator: input.Paginator,
		})
		if err != nil || out == nil || len(out.Items) == 0 {
			return nil, &domain.ResourceNotFoundError{
				Message: fmt.Sprintf("does not found exist any resource with action = '%s' and resource as '%s'", action, resource),
			}
		}

		cslog.Trace(ctx, "service.Resources.ListMatches", "matched_resources_count", len(out.Items), "action", action, "resource", resource, "case", "action=specific resource=*")

	default:
		return nil, &domain.ResourceNotFoundError{
			Message: fmt.Sprintf("does not found exist any resource with action = '%s' and resource as '%s'", action, resource),
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Resources matched")

	cslog.Trace(ctx, "service.Resources.ListMatches", "total_matched_resources", len(out.Items), "action", action, "resource", resource)

	return out, nil
}
