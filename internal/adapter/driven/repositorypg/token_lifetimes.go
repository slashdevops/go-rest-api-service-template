package repositorypg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
	"github.com/slashdevops/go-rest-api-service-template/pkg/cslog"
)

// Constraint names this repository matches on.
//
// A CONSTRAINT NAME REFERENCED FROM GO IS A CONTRACT: renaming one in a later
// migration without changing the constant here turns a documented 400 into a
// 500. TestTokenLifetimesConstraintNamesExistInTheMigration parses
// 00016_authn_token_lifetimes.sql and fails if any of these disappears.
const (
	constraintTokenLifetimesAccess  = "chk_authn_token_lifetimes_access"
	constraintTokenLifetimesRefresh = "chk_authn_token_lifetimes_refresh"
	constraintTokenLifetimesOrder   = "chk_authn_token_lifetimes_order"
)

type TokenLifetimesRepositoryConfig struct {
	DB              *pgxpool.Pool
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
	MaxPingTimeout  time.Duration
	MaxQueryTimeout time.Duration
}

// TokenLifetimesRepository is the PostgreSQL store for the one row of
// authn_token_lifetimes.
//
// The column holds SECONDS, the domain holds a time.Duration, and the
// conversion happens here and nowhere else -- the same arrangement as
// rate_limit_windows.period_seconds.
type TokenLifetimesRepository struct {
	db              *pgxpool.Pool
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
	maxPingTimeout  time.Duration
	maxQueryTimeout time.Duration
}

// NewTokenLifetimesRepository creates a new TokenLifetimesRepository.
func NewTokenLifetimesRepository(conf TokenLifetimesRepositoryConfig) (*TokenLifetimesRepository, error) {
	if conf.DB == nil {
		return nil, &domain.InvalidDBConfigurationError{Message: "invalid database configuration. It is nil"}
	}

	if conf.MaxPingTimeout < domain.ValidDatabaseMinPingTimeout || conf.MaxPingTimeout > domain.ValidDatabaseMaxPingTimeout {
		return nil, &domain.InvalidDBMaxPingTimeoutError{
			Message: fmt.Sprintf("invalid max ping timeout. It must be between %d and %d",
				domain.ValidDatabaseMinPingTimeout, domain.ValidDatabaseMaxPingTimeout),
		}
	}

	if conf.MaxQueryTimeout < domain.ValidDatabaseMinQueryTimeout || conf.MaxQueryTimeout > domain.ValidDatabaseMaxQueryTimeout {
		return nil, &domain.InvalidDBMaxQueryTimeoutError{
			Message: fmt.Sprintf("invalid max query timeout. It must be between %d and %d",
				domain.ValidDatabaseMinQueryTimeout, domain.ValidDatabaseMaxQueryTimeout),
		}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "invalid OpenTelemetry configuration. It is nil"}
	}

	ref := &TokenLifetimesRepository{
		db:              conf.DB,
		maxPingTimeout:  conf.MaxPingTimeout,
		maxQueryTimeout: conf.MaxQueryTimeout,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "TokenLifetimes",
			Action: "NewTokenLifetimesRepository",
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

// DriverName returns the name of the driver.
func (ref *TokenLifetimesRepository) DriverName() string {
	return sql.Drivers()[0]
}

// PingContext verifies the connection is alive.
func (ref *TokenLifetimesRepository) PingContext(ctx context.Context) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxPingTimeout, ref.metricsMetadata, "PingContext")
	defer cancel()
	defer span.End()

	if err := ref.db.Ping(ctx); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "database ping successful")

	return nil
}

// Get implements [repository.TokenLifetimes].
func (ref *TokenLifetimesRepository) Get(ctx context.Context) (*domain.TokenLifetimes, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "Get")
	defer cancel()
	defer span.End()

	// COALESCE rather than a nullable scan target: the domain reads "nobody
	// has changed it" as uuid.Nil(), which is the same thing said in Go.
	const query = `
        SELECT access_token_seconds, refresh_token_seconds,
               COALESCE(updated_by, '00000000-0000-0000-0000-000000000000'::uuid),
               updated_at
        FROM authn_token_lifetimes
        WHERE singleton = TRUE;
    `

	cslog.Trace(ctx, "repository.TokenLifetimes.Get", "query", prettyPrint(query))

	item, err := scanTokenLifetimes(ref.db.QueryRow(ctx, query))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, o11y.RecordError(ctx, span, start, &domain.TokenLifetimesNotFoundError{}, ref.metrics, attrs)
		}

		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "token lifetimes read",
		attribute.Int64("access_token_seconds", int64(item.AccessTokenDuration.Seconds())),
		attribute.Int64("refresh_token_seconds", int64(item.RefreshTokenDuration.Seconds())),
	)

	return item, nil
}

// Update implements [repository.TokenLifetimes].
//
// RETURNING the row rather than re-reading it: what the caller gets back is
// exactly what this statement stored, under the same snapshot, so a concurrent
// PUT cannot make the response describe somebody else's write.
func (ref *TokenLifetimesRepository) Update(ctx context.Context, input *domain.UpdateTokenLifetimesInput) (*domain.TokenLifetimes, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "Update")
	defer cancel()
	defer span.End()

	if input == nil {
		return nil, o11y.RecordError(ctx, span, start, &domain.InvalidInputError{Message: "input is nil"}, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(
		attribute.Int64("access_token_seconds", int64(input.AccessTokenDuration.Seconds())),
		attribute.Int64("refresh_token_seconds", int64(input.RefreshTokenDuration.Seconds())),
		attribute.String("updated_by", input.UpdatedBy.String()),
	)

	const query = `
        UPDATE authn_token_lifetimes
        SET access_token_seconds  = $1,
            refresh_token_seconds = $2,
            updated_by            = $3,
            updated_at            = NOW()
        WHERE singleton = TRUE
        RETURNING access_token_seconds, refresh_token_seconds,
                  COALESCE(updated_by, '00000000-0000-0000-0000-000000000000'::uuid),
                  updated_at;
    `

	cslog.Trace(ctx, "repository.TokenLifetimes.Update", "query",
		prettyPrint(query, int64(input.AccessTokenDuration.Seconds()), int64(input.RefreshTokenDuration.Seconds()), input.UpdatedBy.String()))

	item, err := scanTokenLifetimes(ref.db.QueryRow(ctx, query,
		int64(input.AccessTokenDuration.Seconds()),
		int64(input.RefreshTokenDuration.Seconds()),
		input.UpdatedBy,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, o11y.RecordError(ctx, span, start, &domain.TokenLifetimesNotFoundError{}, ref.metrics, attrs)
		}

		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "token lifetimes updated")

	return item, nil
}

func scanTokenLifetimes(row pgx.Row) (*domain.TokenLifetimes, error) {
	var (
		item                          domain.TokenLifetimes
		accessSeconds, refreshSeconds int64
		updatedBy                     uuid.UUID
	)

	if err := row.Scan(&accessSeconds, &refreshSeconds, &updatedBy, &item.UpdatedAt); err != nil {
		return nil, err
	}

	item.AccessTokenDuration = time.Duration(accessSeconds) * time.Second
	item.RefreshTokenDuration = time.Duration(refreshSeconds) * time.Second
	item.UpdatedBy = updatedBy

	return &item, nil
}

// handlePgError turns a CHECK violation into the validation error the use case
// would have produced. Reaching one means validation was bypassed or the bounds
// drifted between Go and SQL; either way the caller deserves a 400 that names
// the field, not a 500 that names a constraint.
func (ref *TokenLifetimesRepository) handlePgError(err error) error {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok {
		return err
	}

	if pgErr.Code != "23514" { // check violation
		return err
	}

	var errs domain.ValidationErrors

	switch {
	case strings.Contains(pgErr.ConstraintName, constraintTokenLifetimesAccess):
		errs.AddError(domain.FieldAccessTokenDuration, "refused by the database bounds; this should have been caught in validation, please report it", "OUT_OF_RANGE")
	case strings.Contains(pgErr.ConstraintName, constraintTokenLifetimesRefresh):
		errs.AddError(domain.FieldRefreshTokenDuration, "refused by the database bounds; this should have been caught in validation, please report it", "OUT_OF_RANGE")
	case strings.Contains(pgErr.ConstraintName, constraintTokenLifetimesOrder):
		errs.AddError(domain.FieldRefreshTokenDuration, "must be longer than "+domain.FieldAccessTokenDuration+"; this should have been caught in validation, please report it", "ORDERING")
	default:
		return err
	}

	return &errs
}

// compile-time check that the adapter satisfies the port.
var _ repository.TokenLifetimes = (*TokenLifetimesRepository)(nil)
