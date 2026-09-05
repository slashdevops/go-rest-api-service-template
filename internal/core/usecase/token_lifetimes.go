package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/changenotify"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// TokenLifetimesServiceConf configures [TokenLifetimesService].
type TokenLifetimesServiceConf struct {
	Repository repository.TokenLifetimes

	// Mirror is this replica's in-memory copy. Required: a PUT that the
	// replica serving it cannot see is the bug operators report as "the
	// change did nothing", because the next login they try still gets the
	// old lifetime.
	Mirror TokenLifetimesSet

	// Notifier tells the other replicas a write happened. nil is supported
	// and means ticker-only propagation, which is what cache.enabled=false
	// gets.
	Notifier changenotify.Notifier

	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// TokenLifetimesService is the use case behind GET and PUT /auth/token_lifetimes.
//
// It is deliberately small. The row is a singleton with two numbers, so there
// is no listing, no creation and no deletion -- only reading it and replacing
// it. What the service adds over the repository is the two things a plain
// write would leave out: the write is applied to the serving replica's mirror
// before the response is sent, and the other replicas are told.
type TokenLifetimesService struct {
	repository      repository.TokenLifetimes
	mirror          TokenLifetimesSet
	notifier        changenotify.Notifier
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewTokenLifetimesService creates the service.
func NewTokenLifetimesService(conf TokenLifetimesServiceConf) (*TokenLifetimesService, error) {
	if conf.Repository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "Repository is nil, but it is required for TokenLifetimesService"}
	}

	if conf.Mirror == nil {
		return nil, &domain.InvalidInputError{Message: "Mirror is nil, but it is required for TokenLifetimesService; a write this replica cannot see is the bug this exists to prevent"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for TokenLifetimesService"}
	}

	ref := &TokenLifetimesService{
		repository: conf.Repository,
		mirror:     conf.Mirror,
		notifier:   conf.Notifier,
		ot:         conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "TokenLifetimes",
			Action: "NewTokenLifetimesService",
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
		metric.WithDescription(fmt.Sprintf("Duration of %s calls", AppLayer)),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	ref.metrics = &o11y.LayerMetrics{Counter: callsCounter, Histogram: callsDuration}

	return ref, nil
}

// Get returns the stored row.
//
// The ROW, not the mirror. The row is the source of truth: this replica's
// mirror may lag it by up to a reload interval, and another replica's is not
// visible from here. Answering from the row means GET never disagrees with what
// a PUT just wrote -- and this is an admin call, not a hot path, so the one
// indexed read costs nothing that matters.
func (ref *TokenLifetimesService) Get(ctx context.Context) (*domain.TokenLifetimes, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "Get")
	defer span.End()

	out, err := ref.repository.Get(ctx)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "token lifetimes read",
		attribute.String("access_token_duration", out.AccessTokenDuration.String()),
		attribute.String("refresh_token_duration", out.RefreshTokenDuration.String()),
	)

	return out, nil
}

// Update validates, writes, applies locally and notifies.
//
// Only the validation and the write can fail the call. A mirror reload or a
// notify failure after a successful write is logged, never returned:
// reporting an error would tell the caller their change was not saved when it
// was, and the reload ticker carries it either way.
func (ref *TokenLifetimesService) Update(ctx context.Context, input *domain.UpdateTokenLifetimesInput) (*domain.TokenLifetimes, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "Update")
	defer span.End()

	if input == nil {
		return nil, o11y.RecordError(ctx, span, start, &domain.InvalidInputError{Message: "input is nil"}, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	out, err := ref.repository.Update(ctx, input)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	ref.applyLocally(ctx)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "token lifetimes updated",
		attribute.String("access_token_duration", out.AccessTokenDuration.String()),
		attribute.String("refresh_token_duration", out.RefreshTokenDuration.String()),
		attribute.String("updated_by", input.UpdatedBy.String()),
	)

	return out, nil
}

// applyLocally refreshes the serving replica's mirror after a write, then tells
// the other replicas.
//
// This replica first, so it is never the last to know about its own write --
// the operator who saves and immediately signs in again must get the new
// lifetime, not the old one for up to a reload interval.
func (ref *TokenLifetimesService) applyLocally(ctx context.Context) {
	if err := ref.mirror.Reload(ctx); err != nil {
		slog.Warn("token lifetimes written but the local mirror could not be refreshed",
			"error", err,
			"consequence", "this replica keeps issuing with the previous lifetimes until the next scheduled reload",
		)
	}

	if ref.notifier == nil {
		return
	}

	if err := ref.notifier.Notify(ctx); err != nil {
		slog.Warn("token lifetimes written but other replicas could not be notified",
			"error", err,
			"consequence", "they apply it within authn.token.lifetimes.reload.interval instead of at once",
		)
	}
}
