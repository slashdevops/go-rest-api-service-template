package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"
	"uuid"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

type IDPTypesServiceConf struct {
	Repository    repository.IDPTypes
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

type IDPTypesService struct {
	repository      repository.IDPTypes
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

func NewIDPTypesService(conf IDPTypesServiceConf) (*IDPTypesService, error) {
	if conf.Repository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "Repository is nil, but it is required for IDPTypesService"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for IDPTypesService"}
	}

	ref := &IDPTypesService{
		repository: conf.Repository,
		ot:         conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "IDPTypes",
			Action: "NewIDPTypesService",
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

func (ref *IDPTypesService) GetByID(ctx context.Context, id uuid.UUID) (*domain.IDPTypes, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetByID")
	defer span.End()

	if !domain.IsUUIDV7(id) {
		errorType := &domain.InvalidInputError{Message: "IDP ID cannot be empty"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("idp.id", id.String()))

	out, err := ref.repository.SelectByID(ctx, id)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "IDP found", attribute.String("idp.name", out.Name))

	return out, nil
}

func (ref *IDPTypesService) GetByName(ctx context.Context, name string) (*domain.IDPTypes, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetByName")
	defer span.End()

	if name == "" {
		errorValue := &domain.InvalidInputError{Message: "name is empty"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("idp.name", name))

	out, err := ref.repository.SelectByName(ctx, name)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "IDP found", attribute.String("idp.name", out.Name))

	return out, nil
}

func (ref *IDPTypesService) List(ctx context.Context, input *domain.ListIDPTypesInput) (*domain.ListIDPTypesOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "List")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is required"}
		_ = o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
		return nil, errorValue
	}

	span.SetAttributes(
		attribute.String("sort", input.Sort),
		attribute.String("fields", input.Fields),
		attribute.String("filter", input.Filter),
		attribute.Int("limit", input.Paginator.Limit),
	)

	out, err := ref.repository.Select(ctx, input)
	if err != nil {
		_ = o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		return nil, err
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "IDPTypes listed successfully",
		attribute.Int("count", len(out.Items)))

	return out, nil
}
