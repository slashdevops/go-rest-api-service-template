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
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/policy"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
	"github.com/slashdevops/go-rest-api-service-template/pkg/cslog"
)

//go:generate go tool mockgen -package=mocks -destination=../../../mocks/service/authz.go -source=authz.go UsersServiceConsumer

// UsersServiceConsumer represents the service for the user.
type UsersServiceConsumer interface {
	SelectAuthz(ctx context.Context, userID uuid.UUID) (map[string]any, error)
}

type AuthzServiceConf struct {
	UserService   UsersServiceConsumer
	PolicyEngine  policy.Engine
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

type AuthzService struct {
	userService     UsersServiceConsumer
	policyEngine    policy.Engine
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

func NewAuthzService(conf AuthzServiceConf) (*AuthzService, error) {
	if conf.UserService == nil {
		return nil, &domain.InvalidUserServiceError{Message: "UserService cannot be nil"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for AuthzService"}
	}

	if conf.PolicyEngine == nil {
		return nil, &domain.InvalidPolicyEngineError{Message: "PolicyEngine cannot be nil"}
	}

	ref := &AuthzService{
		userService:  conf.UserService,
		policyEngine: conf.PolicyEngine,
		ot:           conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Authz",
			Action: "NewAuthzService",
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
		metric.WithDescription(fmt.Sprintf("Duration of %s service calls", AppLayer)),
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

func (ref *AuthzService) IsAuthorized(ctx context.Context, userID uuid.UUID, requestAction, requestResource string) (bool, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "IsAuthorized")
	defer span.End()

	span.SetAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("action", requestAction),
		attribute.String("resource", requestResource),
	)

	if userID == uuid.Nil() {
		errorType := &domain.InvalidUserIDError{Message: "user ID cannot be empty"}
		return false, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	userAuthPermissions, err := ref.userService.SelectAuthz(ctx, userID)
	if err != nil {
		return false, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	allowed, err := ref.policyEngine.IsAllowed(ctx, policy.Decision{
		UserID:      userID.String(),
		Action:      requestAction,
		Resource:    requestResource,
		Permissions: userAuthPermissions,
	})
	cslog.Trace(ctx, "service.Authz.IsAuthorized", "allowed", allowed)
	if err != nil {
		slog.Warn("service.Authz.IsAuthorized", "policy_error", err)
		return false, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// The decision is a label on the counter, not only a span attribute:
	// a rising deny rate is what a dashboard alerts on, and a span is not a
	// dashboard.
	o11y.RecordSuccess(ctx, span, start, ref.metrics, append(attrs, attribute.Bool("authorized", allowed)), "Authorization check completed")
	return allowed, nil
}
