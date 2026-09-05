package repositorypg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
	"github.com/slashdevops/go-rest-api-service-template/pkg/cslog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type RevokedTokensRepositoryConfig struct {
	DB              *pgxpool.Pool
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
	MaxPingTimeout  time.Duration
	MaxQueryTimeout time.Duration
}

// RevokedTokensRepository is a PostgreSQL store for the token denylist.
type RevokedTokensRepository struct {
	db              *pgxpool.Pool
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
	maxPingTimeout  time.Duration
	maxQueryTimeout time.Duration
}

// NewRevokedTokensRepository creates a new RevokedTokensRepository.
func NewRevokedTokensRepository(conf RevokedTokensRepositoryConfig) (*RevokedTokensRepository, error) {
	if conf.DB == nil {
		return nil, &domain.InvalidDBConfigurationError{Message: "invalid database configuration. It is nil"}
	}

	if conf.MaxPingTimeout < domain.ValidDatabaseMinPingTimeout || conf.MaxPingTimeout > domain.ValidDatabaseMaxPingTimeout {
		return nil, &domain.InvalidDBMaxPingTimeoutError{
			Message: fmt.Sprintf(
				"invalid max ping timeout. It must be between %d and %d",
				domain.ValidDatabaseMinPingTimeout,
				domain.ValidDatabaseMaxPingTimeout,
			),
		}
	}

	if conf.MaxQueryTimeout < domain.ValidDatabaseMinQueryTimeout || conf.MaxQueryTimeout > domain.ValidDatabaseMaxQueryTimeout {
		return nil, &domain.InvalidDBMaxQueryTimeoutError{
			Message: fmt.Sprintf(
				"invalid max query timeout. It must be between %d and %d",
				domain.ValidDatabaseMinQueryTimeout,
				domain.ValidDatabaseMaxQueryTimeout,
			),
		}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "invalid OpenTelemetry configuration. It is nil"}
	}

	ref := &RevokedTokensRepository{
		db:              conf.DB,
		maxPingTimeout:  conf.MaxPingTimeout,
		maxQueryTimeout: conf.MaxQueryTimeout,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "RevokedTokens",
			Action: "NewRevokedTokensRepository",
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

	ref.metrics = &o11y.LayerMetrics{
		Counter:   callsCounter,
		Histogram: callsDuration,
	}

	return ref, nil
}

// PingContext verifies a connection to the repository is still alive.
func (ref *RevokedTokensRepository) PingContext(ctx context.Context) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxPingTimeout, ref.metricsMetadata, "PingContext")
	defer cancel()
	defer span.End()

	if err := ref.db.Ping(ctx); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	return nil
}

// Revoke implements [repository.RevokedTokens].
//
// ON CONFLICT DO NOTHING because revoking twice is a normal thing to do — two
// tabs logging out at once, or a client retrying — and neither should be an
// error. The first revocation is the one that counts; a later one cannot make
// the token any more revoked.
func (ref *RevokedTokensRepository) Revoke(ctx context.Context, jti, userID uuid.UUID, tokenType domain.TokenType, expiresAt time.Time) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "Revoke")
	defer cancel()
	defer span.End()

	if jti == uuid.Nil() {
		errorValue := &domain.InvalidInputError{Message: "jti is empty"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if !tokenType.IsValid() {
		errorValue := &domain.InvalidInputError{Message: "token type is invalid"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(
		attribute.String("revoked_tokens.jti", jti.String()),
		attribute.String("revoked_tokens.token_type", tokenType.String()),
	)

	query := `
        INSERT INTO revoked_tokens (jti, users_id, token_type, expires_at)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (jti) DO NOTHING;
    `

	cslog.Trace(ctx, "repository.RevokedTokens.Revoke", "query", prettyPrint(query))

	if _, err := ref.db.Exec(ctx, query, jti, userID, tokenType.String(), expiresAt); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "token revoked")

	return nil
}

// Rotate implements [repository.RevokedTokens].
//
// ON CONFLICT DO NOTHING, and deliberately not DO UPDATE: two requests racing on
// the same refresh token both reach here, and the successor recorded first is
// the one the chain actually went to. Overwriting it would leave the earlier
// successor unreachable from the walk — still valid, and no longer revocable by
// detecting the replay that produced it.
func (ref *RevokedTokensRepository) Rotate(ctx context.Context, oldJTI, newJTI, userID uuid.UUID, expiresAt time.Time) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "Rotate")
	defer cancel()
	defer span.End()

	if oldJTI == uuid.Nil() || newJTI == uuid.Nil() {
		errorValue := &domain.InvalidInputError{Message: "jti is empty"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(
		attribute.String("revoked_tokens.jti", oldJTI.String()),
		attribute.String("revoked_tokens.replaced_by", newJTI.String()),
	)

	// Only a refresh token is ever rotated, so the type is fixed here rather
	// than asked for.
	query := `
        INSERT INTO revoked_tokens (jti, users_id, token_type, expires_at, replaced_by)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (jti) DO NOTHING;
    `

	cslog.Trace(ctx, "repository.RevokedTokens.Rotate", "query", prettyPrint(query))

	if _, err := ref.db.Exec(ctx, query, oldJTI, userID, domain.TokenTypeRefresh.String(), expiresAt, newJTI); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "token rotated")

	return nil
}

// Consume implements [repository.RevokedTokens].
//
// RETURNING with ON CONFLICT DO NOTHING is what makes this atomic: the row comes
// back only when this statement inserted it, so "did I spend it" and "was it
// already spent" are the same question answered once, under the primary key's
// own lock. A SELECT followed by an INSERT would let two concurrent callbacks
// both decide they were first.
func (ref *RevokedTokensRepository) Consume(ctx context.Context, jti, userID uuid.UUID, tokenType domain.TokenType, expiresAt time.Time) (bool, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "Consume")
	defer cancel()
	defer span.End()

	if jti == uuid.Nil() {
		errorValue := &domain.InvalidInputError{Message: "jti is empty"}
		return false, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if !tokenType.IsValid() {
		errorValue := &domain.InvalidInputError{Message: "token type is invalid"}
		return false, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(
		attribute.String("revoked_tokens.jti", jti.String()),
		attribute.String("revoked_tokens.token_type", tokenType.String()),
	)

	query := `
        INSERT INTO revoked_tokens (jti, users_id, token_type, expires_at)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (jti) DO NOTHING
        RETURNING jti;
    `

	cslog.Trace(ctx, "repository.RevokedTokens.Consume", "query", prettyPrint(query))

	var spent uuid.UUID
	if err := ref.db.QueryRow(ctx, query, jti, userID, tokenType.String(), expiresAt).Scan(&spent); err != nil {
		// No row means the conflict fired: something had already spent this
		// jti. That is an answer, not a failure.
		if errors.Is(err, pgx.ErrNoRows) {
			span.SetAttributes(attribute.Bool("revoked_tokens.first_use", false))
			o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "token had already been spent")

			return false, nil
		}

		return false, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.Bool("revoked_tokens.first_use", true))
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "token spent")

	return true, nil
}

// Get implements [repository.RevokedTokens].
//
// The expires_at predicate means an expired row answers "no record". It would be
// harmless to answer with one — the token is refused for being expired anyway —
// but this keeps the question honest and lets the sweep run on its own schedule
// without changing any answer.
func (ref *RevokedTokensRepository) Get(ctx context.Context, jti uuid.UUID) (*domain.TokenRevocation, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "Get")
	defer cancel()
	defer span.End()

	if jti == uuid.Nil() {
		errorValue := &domain.InvalidInputError{Message: "jti is empty"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// COALESCE rather than a nullable scan target: the domain reads an unset
	// successor as uuid.Nil(), which is the same thing said in Go.
	query := `
        SELECT jti, users_id, expires_at, revoked_at,
               COALESCE(replaced_by, '00000000-0000-0000-0000-000000000000'::uuid)
        FROM revoked_tokens
        WHERE jti = $1 AND expires_at > NOW();
    `

	cslog.Trace(ctx, "repository.RevokedTokens.Get", "query", prettyPrint(query))

	var record domain.TokenRevocation
	if err := ref.db.QueryRow(ctx, query, jti).Scan(
		&record.JTI, &record.UserID, &record.ExpiresAt, &record.RevokedAt, &record.ReplacedBy,
	); err != nil {
		// No row is the ordinary case: the token has neither been revoked nor
		// spent. It is not an error and must not be reported as one.
		if errors.Is(err, pgx.ErrNoRows) {
			o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "token is not revoked")

			return nil, nil
		}

		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.Bool("revoked_tokens.rotated", record.Rotated()))
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "revocation checked")

	return &record, nil
}

// RevokeChain implements [repository.RevokedTokens].
//
// One statement, because the walk and the insert must see the same snapshot: a
// concurrent rotation between reading the tip and writing it would move the tip
// and leave the token that replaced it alive — the precise outcome this call
// exists to prevent.
//
// The CYCLE clause is insurance, not a live concern. A successor jti is freshly
// generated at rotation and a row is written once (Rotate never overwrites a
// successor), so a loop cannot form; but a recursive walk that can hang belongs
// nowhere near a path an attacker reaches by replaying a token they stole.
func (ref *RevokedTokensRepository) RevokeChain(ctx context.Context, jti, userID uuid.UUID, expiresAt time.Time) (uuid.UUID, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "RevokeChain")
	defer cancel()
	defer span.End()

	if jti == uuid.Nil() {
		errorValue := &domain.InvalidInputError{Message: "jti is empty"}
		return uuid.Nil(), o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("revoked_tokens.jti", jti.String()))

	// The tip is the successor that has no row of its own: every link before it
	// was spent on the refresh that produced the next one, so the tip is the
	// only token in the chain nobody has used yet.
	query := `
        WITH RECURSIVE chain AS (
            SELECT r.jti, r.replaced_by
            FROM revoked_tokens r
            WHERE r.jti = $1
          UNION ALL
            SELECT r.jti, r.replaced_by
            FROM revoked_tokens r
            JOIN chain c ON r.jti = c.replaced_by
        ) CYCLE jti SET is_cycle USING path,
        tip AS (
            SELECT c.replaced_by AS jti
            FROM chain c
            WHERE c.replaced_by IS NOT NULL
              AND NOT c.is_cycle
              AND NOT EXISTS (SELECT 1 FROM revoked_tokens r WHERE r.jti = c.replaced_by)
            LIMIT 1
        )
        INSERT INTO revoked_tokens (jti, users_id, token_type, expires_at)
        SELECT tip.jti, $2, $3, $4 FROM tip
        ON CONFLICT (jti) DO NOTHING
        RETURNING jti;
    `

	cslog.Trace(ctx, "repository.RevokedTokens.RevokeChain", "query", prettyPrint(query))

	// The tip of a rotation chain is always a refresh token.
	var tip uuid.UUID
	if err := ref.db.QueryRow(ctx, query, jti, userID, domain.TokenTypeRefresh.String(), expiresAt).Scan(&tip); err != nil {
		// No row returned means the chain had already been fully revoked —
		// somebody logged out, or a concurrent replay got here first. There is
		// nothing left to revoke, which is the outcome the caller wanted.
		if errors.Is(err, pgx.ErrNoRows) {
			o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "token chain was already fully revoked")

			return uuid.Nil(), nil
		}

		return uuid.Nil(), o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("revoked_tokens.chain_tip", tip.String()))
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "token chain revoked")

	return tip, nil
}

// DeleteExpired implements [repository.RevokedTokens].
func (ref *RevokedTokensRepository) DeleteExpired(ctx context.Context) (int64, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "DeleteExpired")
	defer cancel()
	defer span.End()

	query := `
        DELETE FROM revoked_tokens WHERE expires_at <= NOW();
    `

	cslog.Trace(ctx, "repository.RevokedTokens.DeleteExpired", "query", prettyPrint(query))

	result, err := ref.db.Exec(ctx, query)
	if err != nil {
		return 0, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	removed := result.RowsAffected()
	span.SetAttributes(attribute.Int64("revoked_tokens.removed", removed))
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "expired revocations removed")

	return removed, nil
}

// SelectUnexpiredJTIs implements repository.RevokedTokens.
func (ref *RevokedTokensRepository) SelectUnexpiredJTIs(ctx context.Context, tokenType domain.TokenType) ([]uuid.UUID, error) {
	start := time.Now()

	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectUnexpiredJTIs")
	defer cancel()
	defer span.End()

	if !tokenType.IsValid() {
		errorValue := &domain.InvalidInputError{Message: "token type is invalid"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("revoked_tokens.token_type", tokenType.String()))

	// By TYPE, with no horizon: exactly the rows of this kind that are still
	// refused. For access tokens idx_revoked_tokens_access_expires_at serves it
	// as a range scan over the handful of rows that matter. NOW() from the
	// database clock, not the caller's, so the bound cannot drift with it.
	query := `
        SELECT jti
        FROM revoked_tokens
        WHERE token_type = $1
          AND expires_at > NOW();
    `

	cslog.Trace(ctx, "repository.RevokedTokens.SelectUnexpiredJTIs", "query", prettyPrint(query, tokenType.String()))

	rows, err := ref.db.Query(ctx, query, tokenType.String())
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}
	defer rows.Close()

	jtis := make([]uuid.UUID, 0, 64)

	for rows.Next() {
		var jti uuid.UUID
		if err := rows.Scan(&jti); err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		jtis = append(jtis, jti)
	}

	// Checked explicitly: rows.Next returning false does not distinguish the
	// end of the result set from a read that failed halfway. Silently returning
	// the partial set would be a mirror that is quietly missing revocations.
	if err := rows.Err(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.Int("revoked_tokens.unexpired", len(jtis)))
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "unexpired revocations listed")

	return jtis, nil
}

// compile-time check that the adapter satisfies the port.
var _ repository.RevokedTokens = (*RevokedTokensRepository)(nil)
