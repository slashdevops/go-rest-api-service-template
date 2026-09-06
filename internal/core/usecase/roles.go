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

type RolesServiceConf struct {
	// Guard refuses a grant the caller does not hold. Required: a service
	// that can widen permissions without it is the escalation path.
	Guard         *GrantGuard
	Repository    repository.Roles
	CacheService  cache.Cache
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

type RolesService struct {
	guard           *GrantGuard
	repository      repository.Roles
	cacheService    cache.Cache
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewRolesService creates a new RolesService.
func NewRolesService(conf RolesServiceConf) (*RolesService, error) {
	if conf.Guard == nil {
		return nil, &domain.InvalidInputError{Message: "Guard is nil, but it is required for RolesService"}
	}

	if conf.Repository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "Repository is nil, but it is required for RolesService"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for RolesService"}
	}

	ref := &RolesService{
		guard:        conf.Guard,
		repository:   conf.Repository,
		cacheService: conf.CacheService,
		ot:           conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Roles",
			Action: "NewRolesService",
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

// GetByID returns the roles with the specified ID.
func (ref *RolesService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetByID")
	defer span.End()

	span.SetAttributes(attribute.String("roles.id", id.String()))

	if !domain.IsUUIDV7(id) {
		invalidErr := &domain.InvalidRoleIDError{Message: "invalid role ID. It is nil"}
		return nil, o11y.RecordError(ctx, span, start, invalidErr, ref.metrics, attrs)
	}

	var out *domain.Role
	var err error

	roleFetcher := func(ctx context.Context) (*domain.Role, []cache.Identifier, error) {
		out, err := ref.repository.SelectByID(ctx, id)
		if err != nil {
			return nil, nil, err
		}

		return out, nil, nil
	}

	if ref.cacheService == nil {
		slog.Debug("service.Roles.GetByID", "cache", "disabled")

		out, err = ref.repository.SelectByID(ctx, id)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	} else {

		cacheKey := cache.Identifier{
			Type: "role",
			ID:   id.String(),
		}

		slog.Debug("service.Roles.GetByID", "cache", "enabled", "cache_key", cacheKey.String())
		out, err = cache.GetTyped[*domain.Role](ctx, ref.cacheService, cacheKey, roleFetcher)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "role found successfully", attribute.String("role.id", out.ID.String()))

	return out, nil
}

// Create inserts a new roles into the database.
func (ref *RolesService) Create(ctx context.Context, input *domain.CreateRoleInput) error {
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

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.Insert(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "role created successfully",
		attribute.String("role.id", input.ID.String()),
		attribute.String("role.name", input.Name))

	return nil
}

// UpdateByID updates the roles with the specified ID.
func (ref *RolesService) UpdateByID(ctx context.Context, input *domain.UpdateRoleInput) error {
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

	if err := ref.repository.UpdateByID(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.Debug("service.Roles.UpdateByID", "what", "removing cache", "id", fmt.Sprintf("role:%s", input.ID.String()))

		cacheKey := cache.Identifier{
			Type: "role",
			ID:   input.ID.String(),
		}

		if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
			slog.Warn("service.Roles.UpdateByID", "what", "failed to invalidate cache", slog.Any("error", err), "role_id", input.ID.String())
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "role updated successfully",
		attribute.String("role.id", input.ID.String()))

	return nil
}

// DeleteByID deletes the roles with the specified ID.
func (ref *RolesService) DeleteByID(ctx context.Context, input *domain.DeleteRoleInput) error {
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
		slog.Debug("service.Roles.DeleteByID", "what", "removing cache", "id", fmt.Sprintf("role:%s", input.ID.String()))

		cacheKey := cache.Identifier{
			Type: "role",
			ID:   input.ID.String(),
		}

		if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
			slog.Warn("service.Roles.DeleteByID", "what", "failed to invalidate cache", slog.Any("error", err), "role_id", input.ID.String())
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "role deleted successfully",
		attribute.String("role.id", input.ID.String()))

	return nil
}

// List returns a list of models.
func (ref *RolesService) List(ctx context.Context, input *domain.ListRolesInput) (*domain.ListRolesOutput, error) {
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

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "roles listed successfully",
		attribute.Int("count", len(out.Items)))

	return out, nil
}

// LinkUsers links users to a user.
func (ref *RolesService) LinkUsers(ctx context.Context, input *domain.LinkUsersToRoleInput) error {
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

	// Assigning a role hands out everything the role holds.
	if err := ref.guard.CheckRoles(ctx, input.CallerID, []uuid.UUID{input.RoleID}); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("roles.id", input.RoleID.String()))

	if err := ref.repository.LinkUsers(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.Debug("service.Roles.LinkUsers", "what", "removing cache", "id", fmt.Sprintf("role:%s", input.RoleID.String()))

		cacheKeys := []cache.Identifier{
			{
				Type: "role",
				ID:   input.RoleID.String(),
			},
		}

		for _, userID := range input.UserIDs {

			userCacheKey := cache.Identifier{
				Type: "user",
				ID:   userID.String(),
			}

			authzCacheKey := authzCacheKey(userID)

			cacheKeys = append(cacheKeys, userCacheKey, authzCacheKey)
		}

		for _, cacheKey := range cacheKeys {
			slog.Debug("service.Roles.LinkUsers", "what", "removing cache", "id", cacheKey.String())

			if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
				slog.Warn("service.Roles.LinkUsers", "what", "failed to invalidate cache", slog.Any("error", err), "role_id", input.RoleID.String())
			}
		}

	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "users linked to role successfully",
		attribute.String("role.id", input.RoleID.String()))

	return nil
}

// UnlinkUsers unlinks users from a role.
func (ref *RolesService) UnlinkUsers(ctx context.Context, input *domain.UnlinkUsersFromRoleInput) error {
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

	span.SetAttributes(attribute.String("roles.id", input.RoleID.String()))

	if err := ref.repository.UnlinkUsers(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.Debug("service.Roles.UnlinkUsers", "what", "removing cache", "id", fmt.Sprintf("role:%s", input.RoleID.String()))

		cacheKeys := []cache.Identifier{
			{
				Type: "role",
				ID:   input.RoleID.String(),
			},
		}

		for _, userID := range input.UserIDs {
			userCacheKey := cache.Identifier{
				Type: "user",
				ID:   userID.String(),
			}

			authzCacheKey := authzCacheKey(userID)

			cacheKeys = append(cacheKeys, userCacheKey, authzCacheKey)
		}

		for _, cacheKey := range cacheKeys {
			slog.Debug("service.Roles.UnlinkUsers", "what", "removing cache", "id", cacheKey.String())

			if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
				slog.Warn("service.Roles.UnlinkUsers", "what", "failed to invalidate cache", slog.Any("error", err), "id", cacheKey.String())
			}
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "users unlinked from role successfully",
		attribute.String("role.id", input.RoleID.String()))

	return nil
}

// LinkPolicies links permission to a role.
func (ref *RolesService) LinkPolicies(ctx context.Context, input *domain.LinkPoliciesToRoleInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "LinkPolicies")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.guard.CheckPolicies(ctx, input.CallerID, input.PolicyIDs); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("roles.id", input.RoleID.String()))

	if err := ref.repository.LinkPolicies(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.Debug("service.Roles.LinkPolicies", "what", "removing cache", "id", fmt.Sprintf("role:%s", input.RoleID.String()))

		cacheKey := cache.Identifier{
			Type: "role",
			ID:   input.RoleID.String(),
		}

		if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
			slog.Warn("service.Roles.LinkPolicies", "what", "failed to invalidate cache", slog.Any("error", err), "role_id", input.RoleID.String())
		}

		for _, policyID := range input.PolicyIDs {
			slog.Debug("service.Roles.LinkPolicies", "what", "removing cache", "id", fmt.Sprintf("policy:%s", policyID.String()))

			policyCacheKey := cache.Identifier{
				Type: "policy",
				ID:   policyID.String(),
			}

			if err := ref.cacheService.Invalidate(ctx, policyCacheKey); err != nil {
				slog.Warn("service.Roles.LinkPolicies", "what", "failed to invalidate policy cache", slog.Any("error", err), "policy_id", policyID.String())
			}
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "policies linked to role successfully",
		attribute.String("role.id", input.RoleID.String()))

	return nil
}

// UnlinkPolicies unlinks permission from a role.
func (ref *RolesService) UnlinkPolicies(ctx context.Context, input *domain.UnlinkPoliciesFromRoleInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "UnlinkPolicies")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("roles.id", input.RoleID.String()))

	if err := ref.repository.UnlinkPolicies(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.Debug("service.Roles.UnlinkPolicies", "what", "removing cache", "id", fmt.Sprintf("role:%s", input.RoleID.String()))

		cacheKey := cache.Identifier{
			Type: "role",
			ID:   input.RoleID.String(),
		}

		if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
			slog.Warn("service.Roles.UnlinkPolicies", "what", "failed to invalidate cache", slog.Any("error", err), "role_id", input.RoleID.String())
		}

		// The same keys LinkPolicies invalidates; see Policies.UnlinkRoles.
		for _, policyID := range input.PolicyIDs {
			if err := ref.cacheService.Invalidate(ctx, cache.Identifier{Type: "policy", ID: policyID.String()}); err != nil {
				slog.Warn("service.Roles.UnlinkPolicies", "what", "failed to invalidate cache", slog.Any("error", err), "policy_id", policyID.String())
			}
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "policies unlinked from role successfully",
		attribute.String("role.id", input.RoleID.String()))

	return nil
}

// ListByUserID returns a list of roles for a user.
func (ref *RolesService) ListByUserID(ctx context.Context, userID uuid.UUID, input *domain.ListRolesInput) (*domain.ListRolesOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ListByUserID")
	defer span.End()

	span.SetAttributes(attribute.String("user.id", userID.String()))

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	out, err := ref.repository.SelectByUserID(ctx, userID, input)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "roles found by user ID",
		attribute.String("user.id", userID.String()),
		attribute.Int("count", len(out.Items)))

	return out, nil
}

// ListByPolicyID returns a list of roles for a policy.
func (ref *RolesService) ListByPolicyID(ctx context.Context, policyID uuid.UUID, input *domain.ListRolesInput) (*domain.ListRolesOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ListByPolicyID")
	defer span.End()

	span.SetAttributes(attribute.String("policy.id", policyID.String()))

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	out, err := ref.repository.SelectByPolicyID(ctx, policyID, input)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "roles found by policy ID",
		attribute.String("policy.id", policyID.String()),
		attribute.Int("count", len(out.Items)))

	return out, nil
}
