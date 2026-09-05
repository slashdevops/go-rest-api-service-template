package usecase

import (
	"context"
	"errors"
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
)

type ProjectsServiceConf struct {
	Repository      repository.Projects
	CacheService    cache.Cache
	ResourcesLimits ResourcesLimitsServiceConsumer
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
}

type ProjectsService struct {
	repository      repository.Projects
	cacheService    cache.Cache
	resourcesLimits ResourcesLimitsServiceConsumer
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewProjectsService creates a new ProjectsService.
func NewProjectsService(conf ProjectsServiceConf) (*ProjectsService, error) {
	if conf.Repository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "Repository is nil, but it is required for ProjectsService"}
	}

	if conf.ResourcesLimits == nil {
		return nil, &domain.InvalidResourcesLimitsError{Message: "ResourcesLimits is nil, but it is required for ProjectsService"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for ProjectsService"}
	}

	ref := &ProjectsService{
		repository:      conf.Repository,
		cacheService:    conf.CacheService,
		resourcesLimits: conf.ResourcesLimits,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Projects",
			Action: "NewProjectsService",
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

// Create inserts a new projects into the database.
func (ref *ProjectsService) Create(ctx context.Context, input *domain.CreateProjectInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "Create")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("projects.name", input.Name))

	var err error
	input.ID, err = domain.EnsureUUIDV7(input.ID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// validate the projects input
	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Check resource limits
	rlScope := domain.ResourcesLimitsScope{
		Type: domain.ResourcesLimitsScopeTypeUser,
		ID:   &input.UserID, // specific user ID
	}

	resourceLimitParams := ResourceLimitCheckParams{
		Ctx:             ctx,
		ResourcesLimits: ref.resourcesLimits,
		Scope:           rlScope,
		ResourceType:    domain.ResourcesLimitsResourceTypeProjects,
	}

	// Reserve the slot before creating, and give it back if creation fails.
	if err := ReserveResourceSlot(resourceLimitParams); err != nil {
		return err
	}

	if err := ref.repository.Insert(ctx, input); err != nil {
		ReleaseResourceSlot(resourceLimitParams)
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	WarnOnSoftLimit(resourceLimitParams)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "project created successfully",
		attribute.String("project.id", input.ID.String()),
		attribute.String("project.name", input.Name))

	return nil
}

// UpdateByID updates the projects with the specified ID.
func (ref *ProjectsService) UpdateByID(ctx context.Context, input *domain.UpdateProjectInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "UpdateByID")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("projects.id", input.ID.String()))

	if err := ref.repository.UpdateByID(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {

		cacheKey := cache.Identifier{
			Type: "project",
			ID:   input.ID.String(),
		}

		slog.Debug("service.Projects.UpdateByID", "what", "invalidate cache", "cache_key", cacheKey.String())

		if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
			slog.Warn("service.Projects.UpdateByID", "what", "failed to invalidate cache", slog.Any("error", err), "project.id", input.ID.String(), "cache_key", cacheKey.String())
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "project updated successfully",
		attribute.String("project.id", input.ID.String()))

	return nil
}

// DeleteByID deletes the projects with the specified ID.
func (ref *ProjectsService) DeleteByID(ctx context.Context, input *domain.DeleteProjectInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "DeleteByID")
	defer span.End()

	span.SetAttributes(attribute.String("projects.id", input.ID.String()))

	if input.ID == uuid.Nil() {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := ref.repository.DeleteByID(ctx, input); err != nil {
		// Deleting a project that is not there stays a success — this endpoint
		// is idempotent and TestProjectDelete/delete_project_not_found pins that
		// behaviour. What must not happen is the decrement below: nothing was
		// removed, so nothing may be given back, or a caller could lower their
		// own usage at will by deleting ids that never existed and then create
		// past their limit indefinitely.
		if _, ok := errors.AsType[*domain.ProjectNotFoundError](err); ok {
			o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "project delete was a no-op",
				attribute.String("project.id", input.ID.String()))

			return nil
		}

		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// decrement resource usage
	userScope := domain.ResourcesLimitsScope{
		Type: domain.ResourcesLimitsScopeTypeUser,
		ID:   &input.UserID, // specific user ID
	}

	if err := ref.resourcesLimits.DecrementUsage(ctx, userScope, domain.ResourcesLimitsResourceTypeProjects); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.Debug("service.Projects.DeleteByID", "what", "invalidate cache", "project.id", input.ID.String())

		cacheKey := cache.Identifier{
			Type: "project",
			ID:   input.ID.String(),
		}
		if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
			slog.Warn("service.Projects.DeleteByID", "what", "failed to invalidate cache", slog.Any("error", err), "project.id", input.ID.String())
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "project deleted successfully",
		attribute.String("project.id", input.ID.String()))

	return nil
}

// GetByIDByUserID returns the projects with the specified ID.
func (ref *ProjectsService) GetByIDByUserID(ctx context.Context, id, userID uuid.UUID) (*domain.Project, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetByIDByUserID")
	defer span.End()

	span.SetAttributes(attribute.String("projects.id", id.String()))

	if !domain.IsUUIDV7(id) {
		errorType := &domain.InvalidProjectIDError{Message: "project ID is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	var out *domain.Project
	var err error

	projectFetcher := func(ctx context.Context) (*domain.Project, []cache.Identifier, error) {
		out, err := ref.repository.SelectByIDByUserID(ctx, id, userID)
		if err != nil {
			return nil, nil, err
		}

		dependencies := []cache.Identifier{
			{
				Type: "user",
				ID:   userID.String(),
			},
		}

		return out, dependencies, nil
	}

	if ref.cacheService == nil {
		slog.Debug("service.Projects.GetByIDByUserID", "cache", "disabled")

		out, err = ref.repository.SelectByIDByUserID(ctx, id, userID)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	} else {
		cacheKey := cache.Identifier{
			Type: "project",
			ID:   id.String(),
		}
		slog.Debug("service.Projects.GetByIDByUserID", "cache", "enabled")
		out, err = cache.GetTyped[*domain.Project](ctx, ref.cacheService, cacheKey, projectFetcher)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "project found successfully", attribute.String("project.id", out.ID.String()))

	return out, nil
}

// ListByUserID returns a list of projects for the specified user ID.
func (ref *ProjectsService) ListByUserID(ctx context.Context, input *domain.ListProjectsInput) (*domain.ListProjectsOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ListByUserID")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	out, err := ref.repository.SelectByUserID(ctx, input)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs,
		"projects found successfully", attribute.Int("count", len(out.Items)))

	return out, nil
}

// List returns a list of models.
func (ref *ProjectsService) List(ctx context.Context, input *domain.ListProjectsInput) (*domain.ListProjectsOutput, error) {
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

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "projects listed successfully",
		attribute.Int("count", len(out.Items)))

	return out, nil
}

func (ref *ProjectsService) LinkUsers(ctx context.Context, input *domain.LinkUsersToProjectInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "LinkUsers")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.LinkUsers(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.Debug("service.Projects.LinkUsers", "what", "invalidate cache", "project.id", input.ProjectID.String())

		cacheKey := cache.Identifier{
			Type: "project",
			ID:   input.ProjectID.String(),
		}
		if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
			slog.Warn("service.Projects.LinkUsers", "what", "failed to invalidate cache", slog.Any("error", err), "project.id", input.ProjectID.String())
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "users linked to project successfully",
		attribute.String("project.id", input.ProjectID.String()))

	return nil
}

func (ref *ProjectsService) UnlinkUsers(ctx context.Context, input *domain.UnlinkUsersFromProjectInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "UnlinkUsers")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.UnlinkUsers(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.Debug("service.Projects.UnlinkUsers", "what", "invalidate cache", "project.id", input.ProjectID.String())

		cacheKey := cache.Identifier{
			Type: "project",
			ID:   input.ProjectID.String(),
		}
		if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
			slog.Warn("service.Projects.UnlinkUsers", "what", "failed to invalidate cache", slog.Any("error", err), "project.id", input.ProjectID.String())
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "users unlinked from project successfully",
		attribute.String("project.id", input.ProjectID.String()))

	return nil
}
