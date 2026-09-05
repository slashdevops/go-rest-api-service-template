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
)

type ResourcesServiceMethods interface {
	ListMatches(ctx context.Context, action, resource string, input *domain.ListResourcesInput) (*domain.SelectResourcesOutput, error)
}

type PoliciesServiceConf struct {
	Repository       repository.Policies
	ResourcesService ResourcesServiceMethods
	CacheService     cache.Cache
	OT               *o11y.OpenTelemetry
	MetricsPrefix    string
}

type PoliciesService struct {
	repository       repository.Policies
	resourcesService ResourcesServiceMethods
	cacheService     cache.Cache
	ot               *o11y.OpenTelemetry
	metrics          *o11y.LayerMetrics
	metricsMetadata  o11y.Metadata
	metricsPrefix    string
}

// NewPoliciesService creates a new PoliciesService.
func NewPoliciesService(conf PoliciesServiceConf) (*PoliciesService, error) {
	if conf.Repository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "Repository is nil, but it is required for PoliciesService"}
	}

	if conf.ResourcesService == nil {
		return nil, &domain.InvalidRepositoryError{Message: "ResourcesService is nil, but it is required for PoliciesService"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for PoliciesService"}
	}

	ref := &PoliciesService{
		repository:       conf.Repository,
		resourcesService: conf.ResourcesService,
		cacheService:     conf.CacheService,
		ot:               conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Policies",
			Action: "NewPoliciesService",
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

// Create creates a new policy.
func (ref *PoliciesService) Create(ctx context.Context, input *domain.CreatePolicyInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "Create")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	var err error
	input.ID, err = domain.EnsureUUIDV7(input.ID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	resources, err := ref.resourcesService.ListMatches(
		ctx,
		input.AllowedAction,
		input.AllowedResource,
		&domain.ListResourcesInput{
			Sort: "resource ASC",
			Paginator: domain.Paginator{
				Limit: 1,
			},
		},
	)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if resources == nil || len(resources.Items) == 0 {
		return &domain.ResourceNotFoundError{
			ID:      input.AllowedResource,
			Message: fmt.Sprintf("does not found any resource with action = '%s' and resource as '%s'", input.AllowedAction, input.AllowedResource),
		}
	}

	if len(resources.Items) > 1 {
		return &domain.InvalidResourceIDError{
			ID:      input.AllowedResource,
			Message: fmt.Sprintf("there are more than one resource with action = '%s' and resource as '%s'", input.AllowedAction, input.AllowedResource),
		}
	}

	input.ResourceID = resources.Items[0].ID

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.Insert(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Policy created", attribute.String("policy_id", input.ID.String()), attribute.String("policy.name", input.Name))

	return nil
}

// DeleteByID deletes a policy by ID.
func (ref *PoliciesService) DeleteByID(ctx context.Context, input *domain.DeletePolicyInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "DeleteByID")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.DeleteByID(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.Debug("service.Policies.DeleteByID", "what", "invalidating cache", "policy_id", input.ID.String())

		cacheKey := cache.Identifier{
			Type: "policy",
			ID:   input.ID.String(),
		}

		if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
			slog.Warn("service.Policies.DeleteByID", "what", "failed to invalidate cache", "policy_id", input.ID.String(), "error", err)
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Policy deleted", attribute.String("policy_id", input.ID.String()))
	return nil
}

// UpdateByID updates a policy by ID.
func (ref *PoliciesService) UpdateByID(ctx context.Context, input *domain.UpdatePolicyInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "UpdateByID")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// TODO: the allowed action and resource must be validated by
	// comparing this with the resources table items

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.UpdateByID(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.Debug("service.Policies.UpdateByID", "what", "invalidating cache", "policy_id", input.ID.String())

		cacheKey := cache.Identifier{
			Type: "policy",
			ID:   input.ID.String(),
		}

		if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
			slog.Warn("service.Policies.UpdateByID", "what", "failed to invalidate cache", "policy_id", input.ID.String(), "error", err)
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Policy updated", attribute.String("policy_id", input.ID.String()))
	return nil
}

// GetByID returns a policy by ID.
func (ref *PoliciesService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Policy, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetByID")
	defer span.End()

	if !domain.IsUUIDV7(id) {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	var out *domain.Policy
	var err error

	policyFetcher := func(ctx context.Context) (*domain.Policy, []cache.Identifier, error) {
		out, err := ref.repository.SelectByID(ctx, id)
		if err != nil {
			return nil, nil, err
		}

		return out, nil, nil
	}

	if ref.cacheService == nil {
		slog.Debug("service.Policies.GetByID", "cache", "disabled")

		out, err = ref.repository.SelectByID(ctx, id)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	} else {

		cacheKey := cache.Identifier{
			Type: "policy",
			ID:   id.String(),
		}

		slog.Debug("service.Policies.GetByID", "cache", "enabled", "cache_key", cacheKey.String())
		out, err = cache.GetTyped[*domain.Policy](ctx, ref.cacheService, cacheKey, policyFetcher)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Policy retrieved", attribute.String("policy_id", id.String()))
	return out, nil
}

// List returns a list of policies.
func (ref *PoliciesService) List(ctx context.Context, input *domain.SelectPoliciesInput) (*domain.SelectPoliciesOutput, error) {
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

	policies, err := ref.repository.Select(ctx, input)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Policies listed")
	return policies, nil
}

// LinkRoles links roles to a permission.
func (ref *PoliciesService) LinkRoles(ctx context.Context, input *domain.LinkRolesToPolicyInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "LinkRoles")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.LinkRoles(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.Debug("service.Policies.LinkRoles", "what", "invalidating cache", "policy_id", input.PolicyID.String())

		cacheKey := cache.Identifier{
			Type: "policy",
			ID:   input.PolicyID.String(),
		}

		if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
			slog.Warn("service.Policies.LinkRoles", "what", "failed to invalidate cache", "policy_id", input.PolicyID.String(), "error", err)
		}

		for _, roleID := range input.RoleIDs {
			slog.Debug("service.Policies.LinkRoles", "what", "invalidating cache", "role_id", roleID.String())

			cacheKey := cache.Identifier{
				Type: "role",
				ID:   roleID.String(),
			}

			if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
				slog.Warn("service.Policies.LinkRoles", "what", "failed to invalidate cache", "role_id", roleID.String(), "error", err)
			}
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Roles linked to permission", attribute.String("policy_id", input.PolicyID.String()))
	return nil
}

// UnlinkRoles unlinks roles from a permission.
func (ref *PoliciesService) UnlinkRoles(ctx context.Context, input *domain.UnlinkRolesFromPolicyInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "UnlinkRoles")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.UnlinkRoles(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.Debug("service.Policies.UnlinkRoles", "what", "invalidating cache", "policy_id", input.PolicyID.String())

		cacheKey := cache.Identifier{
			Type: "policy",
			ID:   input.PolicyID.String(),
		}

		if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
			slog.Warn("service.Policies.UnlinkRoles", "what", "failed to invalidate cache", "policy_id", input.PolicyID.String(), "error", err)
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Roles unlinked from permission", attribute.String("policy_id", input.PolicyID.String()))
	return nil
}

// ListByRoleID returns a list of policies by role ID.
func (ref *PoliciesService) ListByRoleID(ctx context.Context, roleID uuid.UUID, input *domain.ListPoliciesInput) (*domain.ListPoliciesOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ListByRoleID")
	defer span.End()

	if roleID == uuid.Nil() {
		errorValue := &domain.InvalidRoleIDError{Message: "roleID is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	out, err := ref.repository.SelectByRoleID(ctx, roleID, input)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Policies listed by role", attribute.String("role_id", roleID.String()))
	return out, nil
}
