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
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

type ProductsServiceConf struct {
	Repository      repository.Products
	ResourcesLimits ResourcesLimitsServiceConsumer
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
}

type ProductsService struct {
	repository      repository.Products
	resourcesLimits ResourcesLimitsServiceConsumer
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewProductsService creates a new ProductsService.
func NewProductsService(conf ProductsServiceConf) (*ProductsService, error) {
	if conf.Repository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "Repository is nil, but it is required for ProductsService"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for ProductsService"}
	}

	ref := &ProductsService{
		repository:      conf.Repository,
		resourcesLimits: conf.ResourcesLimits,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Products",
			Action: "NewProductsService",
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

// Products are deliberately NOT cached.
//
// The repository answers GetByIDByProjectID differently depending on whether
// the caller belongs to the project, so the result is a function of the CALLER
// as well as the product. That leaves no good key: one without the user id
// serves a member's row to a non-member, and one with it cannot be invalidated
// on write, because nothing can enumerate the users who have read a product.
//
// The layered implementation this was ported from did not cache products
// either, so this costs nothing that existed before. If products ever need a
// cache, the way to earn it is to move the membership check out of the row
// query -- resolve membership once, cache that, and key the row on the project
// alone.

// GetByIDByProjectID returns the product with the given ID within a project.
func (ref *ProductsService) GetByIDByProjectID(ctx context.Context, id, projectID, userID uuid.UUID) (*domain.Product, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetByIDByProjectID")
	defer span.End()

	span.SetAttributes(
		attribute.String("products.id", id.String()),
		attribute.String("products.project_id", projectID.String()),
	)

	if !domain.IsUUIDV7(id) {
		invalidErr := &domain.InvalidProductError{Message: "invalid product ID. It is nil"}
		return nil, o11y.RecordError(ctx, span, start, invalidErr, ref.metrics, attrs)
	}

	if !domain.IsUUIDV7(projectID) {
		invalidErr := &domain.InvalidProjectIDError{Message: "invalid project ID. It is nil"}
		return nil, o11y.RecordError(ctx, span, start, invalidErr, ref.metrics, attrs)
	}

	if !domain.IsUUIDV7(userID) {
		invalidErr := &domain.InvalidUserIDError{Message: "invalid user ID. It is nil"}
		return nil, o11y.RecordError(ctx, span, start, invalidErr, ref.metrics, attrs)
	}

	out, err := ref.repository.SelectByIDByProjectID(ctx, id, projectID, userID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "product found successfully",
		attribute.String("product.id", out.ID.String()))

	return out, nil
}

// CreateByProjectID inserts a new product into the project.
func (ref *ProductsService) CreateByProjectID(ctx context.Context, input *domain.CreateProductInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "CreateByProjectID")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(
		attribute.String("products.name", input.Name),
		attribute.String("products.project_id", input.ProjectID.String()),
	)

	var err error
	input.ID, err = domain.EnsureUUIDV7(input.ID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Products are counted against the project that owns them, which is the
	// scope the `products` row in resources_limits is seeded for.
	rlScope := domain.ResourcesLimitsScope{
		Type: domain.ResourcesLimitsScopeTypeProject,
		ID:   &input.ProjectID,
	}

	resourceLimitParams := ResourceLimitCheckParams{
		Ctx:             ctx,
		ResourcesLimits: ref.resourcesLimits,
		Scope:           rlScope,
		ResourceType:    domain.ResourcesLimitsResourceTypeProducts,
	}

	// Reserve the slot before creating, and give it back if creation fails.
	if err := ReserveResourceSlot(resourceLimitParams); err != nil {
		return err
	}

	if err := ref.repository.InsertByProjectID(ctx, input); err != nil {
		ReleaseResourceSlot(resourceLimitParams)
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	WarnOnSoftLimit(resourceLimitParams)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "product created successfully",
		attribute.String("product.id", input.ID.String()),
		attribute.String("product.name", input.Name))

	return nil
}

// UpdateByIDByProjectID updates a product within a project.
func (ref *ProductsService) UpdateByIDByProjectID(ctx context.Context, input *domain.UpdateProductInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "UpdateByIDByProjectID")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(
		attribute.String("products.id", input.ID.String()),
		attribute.String("products.project_id", input.ProjectID.String()),
	)

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.UpdateByIDByProjectID(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "product updated successfully",
		attribute.String("product.id", input.ID.String()))

	return nil
}

// DeleteByIDByProjectID removes a product from a project.
func (ref *ProductsService) DeleteByIDByProjectID(ctx context.Context, input *domain.DeleteProductInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "DeleteByIDByProjectID")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(
		attribute.String("products.id", input.ID.String()),
		attribute.String("products.project_id", input.ProjectID.String()),
	)

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.DeleteByIDByProjectID(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// The counter is decremented only after the delete succeeded. Doing it
	// first would let a failed delete hand back a slot the row still occupies.
	if ref.resourcesLimits != nil {
		rlScope := domain.ResourcesLimitsScope{
			Type: domain.ResourcesLimitsScopeTypeProject,
			ID:   &input.ProjectID,
		}

		if err := ref.resourcesLimits.DecrementUsage(ctx, rlScope, domain.ResourcesLimitsResourceTypeProducts); err != nil {
			slog.Warn("service.Products.DeleteByIDByProjectID",
				"what", "failed to decrement usage", slog.Any("error", err),
				"project_id", input.ProjectID.String())
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "product deleted successfully",
		attribute.String("product.id", input.ID.String()))

	return nil
}

// ListByProjectID returns the products belonging to one project.
func (ref *ProductsService) ListByProjectID(ctx context.Context, projectID, userID uuid.UUID, input *domain.ListProductsInput) (*domain.ListProductsOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ListByProjectID")
	defer span.End()

	span.SetAttributes(attribute.String("products.project_id", projectID.String()))

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if !domain.IsUUIDV7(projectID) {
		invalidErr := &domain.InvalidProjectIDError{Message: "invalid project ID. It is nil"}
		return nil, o11y.RecordError(ctx, span, start, invalidErr, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Lists are not cached: the key would have to carry the filter, sort,
	// fields and pagination token, which makes it effectively unique per
	// request and the cache pure overhead.
	out, err := ref.repository.SelectByProjectID(ctx, projectID, userID, input)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.ProductsFound,
		attribute.Int("products.count", len(out.Items)))

	return out, nil
}

// List returns products across every project the caller can see.
func (ref *ProductsService) List(ctx context.Context, input *domain.ListProductsInput) (*domain.ListProductsOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "List")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	out, err := ref.repository.Select(ctx, input)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, domain.ProductsFound,
		attribute.Int("products.count", len(out.Items)))

	return out, nil
}
