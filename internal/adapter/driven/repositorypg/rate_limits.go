package repositorypg

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
	"time"

	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
	"github.com/slashdevops/go-rest-api-service-template/pkg/cslog"
)

// Constraint names this repository matches on.
//
// A CONSTRAINT NAME REFERENCED FROM GO IS A CONTRACT. Renaming one in a later
// migration without changing the constant here does not break the build and does
// not fail a test that only exercises the happy path -- it turns a documented
// 409 or 403 back into a 500, discovered by a user. TestConstraintNamesExistInTheMigration
// parses 00015_rate_limits.sql and fails if any of these disappears.
const (
	constraintRateLimitName          = "unique_rate_limit_name"
	constraintRateLimitWindowPeriod  = "unique_rate_limit_window_period"
	constraintRateLimitStrategyCheck = "chk_rate_limits_strategy"
	constraintRateLimitScopeCheck    = "chk_rate_limits_scope"
	constraintRateLimitAudienceCheck = "chk_rate_limits_audience"
	constraintRateLimitKindCheck     = "chk_rate_limits_target_kind"
	constraintRateLimitTargetCheck   = "chk_rate_limits_global_target"
	constraintRateLimitMethodsCheck  = "chk_rate_limits_methods"
)

type RateLimitsRepositoryConfig struct {
	DB              *pgxpool.Pool
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
	MaxPingTimeout  time.Duration
	MaxQueryTimeout time.Duration
}

// RateLimitsRepository is a PostgreSQL store for rate-limit rules.
type RateLimitsRepository struct {
	db              *pgxpool.Pool
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
	maxPingTimeout  time.Duration
	maxQueryTimeout time.Duration
}

// NewRateLimitsRepository creates a new RateLimitsRepository.
func NewRateLimitsRepository(conf RateLimitsRepositoryConfig) (*RateLimitsRepository, error) {
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

	ref := &RateLimitsRepository{
		db:              conf.DB,
		maxPingTimeout:  conf.MaxPingTimeout,
		maxQueryTimeout: conf.MaxQueryTimeout,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "RateLimits",
			Action: "NewRateLimitsRepository",
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

// DriverName returns the name of the driver.
func (ref *RateLimitsRepository) DriverName() string {
	return sql.Drivers()[0]
}

// PingContext verifies the connection is alive.
func (ref *RateLimitsRepository) PingContext(ctx context.Context) error {
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

// Insert stores a rule and its windows.
//
// Both in ONE transaction, because a rule with no windows has no budget. A
// half-written rule is not merely incomplete -- the loader skips it, so the
// endpoint it names silently loses the protection an operator believes they
// just configured.
func (ref *RateLimitsRepository) Insert(ctx context.Context, input *domain.CreateRateLimitInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "Insert")
	defer cancel()
	defer span.End()

	if input == nil {
		return o11y.RecordError(ctx, span, start, &domain.InvalidInputError{Message: "input is nil"}, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	tx, err := ref.db.Begin(ctx)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			slog.Error("repository.RateLimits.Insert", "what", "rollback failed", "error", rbErr)
		}
	}()

	query := `
        INSERT INTO rate_limits (id, name, description, target_kind, target, methods, scope, audience, strategy, enabled)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, COALESCE($10, TRUE));
    `

	cslog.Trace(ctx, "repository.RateLimits.Insert", "query", prettyPrint(query, input.ID.String(), input.Name))

	if _, err := tx.Exec(ctx, query,
		input.ID,
		input.Name,
		input.Description,
		string(input.TargetKind),
		input.Target,
		input.Methods,
		string(input.Scope),
		string(input.Audience),
		string(input.Strategy),
		input.Enabled,
	); err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	if err := ref.insertWindows(ctx, tx, input.ID, input.Windows, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := tx.Commit(ctx); err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	slog.Debug("repository.RateLimits.Insert", "rate_limit_id", input.ID.String(), "windows", len(input.Windows))
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "rate limit inserted successfully", attribute.String("rate_limit.id", input.ID.String()))

	return nil
}

// UpdateByID replaces a rule and its window set.
//
// The windows are deleted and re-inserted rather than merged. A partial update
// of a SET is the shape that produces two windows on one period, or a budget
// that changed in a way nobody wrote down -- and unique_rate_limit_window_period
// would then reject the write with an error about a duplicate, which reads like
// a bug in the API rather than in the request.
func (ref *RateLimitsRepository) UpdateByID(ctx context.Context, input *domain.UpdateRateLimitInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "UpdateByID")
	defer cancel()
	defer span.End()

	if input == nil {
		return o11y.RecordError(ctx, span, start, &domain.InvalidInputError{Message: "input is nil"}, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("rate_limit_id", input.ID.String()))

	tx, err := ref.db.Begin(ctx)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			slog.Error("repository.RateLimits.UpdateByID", "what", "rollback failed", "error", rbErr)
		}
	}()

	query := `
        UPDATE rate_limits
        SET name        = $2,
            description = $3,
            target_kind = $4,
            target      = $5,
            methods     = $6,
            scope       = $7,
            audience    = $8,
            strategy    = $9,
            enabled     = COALESCE($10, enabled),
            updated_at  = CURRENT_TIMESTAMP
        WHERE id = $1;
    `

	cslog.Trace(ctx, "repository.RateLimits.UpdateByID", "query", prettyPrint(query, input.ID.String()))

	tag, err := tx.Exec(ctx, query,
		input.ID,
		input.Name,
		input.Description,
		string(input.TargetKind),
		input.Target,
		input.Methods,
		string(input.Scope),
		string(input.Audience),
		string(input.Strategy),
		input.Enabled,
	)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	if tag.RowsAffected() == 0 {
		return o11y.RecordError(ctx, span, start, &domain.RateLimitNotFoundError{ID: input.ID}, ref.metrics, attrs)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM rate_limit_windows WHERE rate_limits_id = $1;`, input.ID); err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	if err := ref.insertWindows(ctx, tx, input.ID, input.Windows, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := tx.Commit(ctx); err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	slog.Debug("repository.RateLimits.UpdateByID", "rate_limit_id", input.ID.String())
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "rate limit updated successfully", attribute.String("rate_limit.id", input.ID.String()))

	return nil
}

// insertWindows writes a rule's window set. Period is stored in SECONDS, which
// is what the CHECK constraint bounds; the domain carries a time.Duration
// because that is what the limiter needs and what the API renders.
func (ref *RateLimitsRepository) insertWindows(ctx context.Context, tx pgx.Tx, ruleID uuid.UUID, windows []domain.RateLimitWindow, input any) error {
	const query = `
        INSERT INTO rate_limit_windows (id, rate_limits_id, requests, period_seconds, burst)
        VALUES ($1, $2, $3, $4, $5);
    `

	for _, w := range windows {
		id := w.ID
		if id == uuid.Nil() {
			id = uuid.NewV7()
		}

		if _, err := tx.Exec(ctx, query, id, ruleID, w.Requests, int(w.Period.Seconds()), w.Burst); err != nil {
			return ref.handlePgError(err, input)
		}
	}

	return nil
}

// DeleteByID removes a rule. Its windows go with it through ON DELETE CASCADE.
func (ref *RateLimitsRepository) DeleteByID(ctx context.Context, input *domain.DeleteRateLimitInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "DeleteByID")
	defer cancel()
	defer span.End()

	if input == nil {
		return o11y.RecordError(ctx, span, start, &domain.InvalidInputError{Message: "input is nil"}, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("rate_limit_id", input.ID.String()))

	tag, err := ref.db.Exec(ctx, `DELETE FROM rate_limits WHERE id = $1;`, input.ID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	if tag.RowsAffected() == 0 {
		return o11y.RecordError(ctx, span, start, &domain.RateLimitNotFoundError{ID: input.ID}, ref.metrics, attrs)
	}

	slog.Debug("repository.RateLimits.DeleteByID", "rate_limit_id", input.ID.String())
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "rate limit deleted successfully", attribute.String("rate_limit.id", input.ID.String()))

	return nil
}

// SelectByID returns one rule with its windows.
func (ref *RateLimitsRepository) SelectByID(ctx context.Context, id uuid.UUID) (*domain.RateLimit, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByID")
	defer cancel()
	defer span.End()

	span.SetAttributes(attribute.String("rate_limit_id", id.String()))

	const query = `
        SELECT id, name, description, target_kind, target, methods, scope, audience, strategy,
               enabled, system, created_at, updated_at, serial_id
        FROM rate_limits
        WHERE id = $1;
    `

	cslog.Trace(ctx, "repository.RateLimits.SelectByID", "query", prettyPrint(query, id.String()))

	var item domain.RateLimit

	var kind, target, scope, audience, strategy string

	err := ref.db.QueryRow(ctx, query, id).Scan(
		&item.ID, &item.Name, &item.Description, &kind, &target, &item.Methods,
		&scope, &audience, &strategy, &item.Enabled, &item.System,
		&item.CreatedAt, &item.UpdatedAt, &item.SerialID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, o11y.RecordError(ctx, span, start, &domain.RateLimitNotFoundError{ID: id}, ref.metrics, attrs)
		}

		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, id), ref.metrics, attrs)
	}

	item.TargetKind = domain.RateLimitTargetKind(kind)
	item.Target = target
	item.Scope = domain.RateLimitScope(scope)
	item.Audience = domain.RateLimitAudience(audience)
	item.Strategy = domain.RateLimitStrategy(strategy)

	windows, err := ref.selectWindows(ctx, []uuid.UUID{id})
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	item.Windows = windows[id]

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "rate limit found", attribute.String("rate_limit.id", id.String()))

	return &item, nil
}

// SelectAll returns every ENABLED rule with its windows, for the mirror.
//
// No paginator, deliberately. The mirror answers every request from this set, so
// a page-shaped answer would mean a rule that exists, is enabled, and is not
// enforced because it fell off page two -- with nothing anywhere to say so.
func (ref *RateLimitsRepository) SelectAll(ctx context.Context) ([]domain.RateLimit, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectAll")
	defer cancel()
	defer span.End()

	const query = `
        SELECT id, name, description, target_kind, target, methods, scope, audience, strategy,
               enabled, system, created_at, updated_at, serial_id
        FROM rate_limits
        WHERE enabled = TRUE
        ORDER BY serial_id;
    `

	cslog.Trace(ctx, "repository.RateLimits.SelectAll", "query", prettyPrint(query))

	rows, err := ref.db.Query(ctx, query)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}
	defer rows.Close()

	items := make([]domain.RateLimit, 0)
	ids := make([]uuid.UUID, 0)

	for rows.Next() {
		var item domain.RateLimit

		var kind, target, scope, audience, strategy string

		if err := rows.Scan(
			&item.ID, &item.Name, &item.Description, &kind, &target, &item.Methods,
			&scope, &audience, &strategy, &item.Enabled, &item.System,
			&item.CreatedAt, &item.UpdatedAt, &item.SerialID,
		); err != nil {
			return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
		}

		item.TargetKind = domain.RateLimitTargetKind(kind)
		item.Target = target
		item.Scope = domain.RateLimitScope(scope)
		item.Audience = domain.RateLimitAudience(audience)
		item.Strategy = domain.RateLimitStrategy(strategy)

		items = append(items, item)
		ids = append(ids, item.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	windows, err := ref.selectWindows(ctx, ids)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	for i := range items {
		items[i].Windows = windows[items[i].ID]
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "rate limits loaded", attribute.Int("rate_limits.count", len(items)))

	return items, nil
}

// selectWindows fetches the windows for a set of rules in one query.
//
// One query rather than one per rule: the mirror reloads on a ticker and on
// every write, so an N+1 here is N+1 round trips on a path that runs
// unattended.
func (ref *RateLimitsRepository) selectWindows(ctx context.Context, ruleIDs []uuid.UUID) (map[uuid.UUID][]domain.RateLimitWindow, error) {
	out := make(map[uuid.UUID][]domain.RateLimitWindow, len(ruleIDs))

	if len(ruleIDs) == 0 {
		return out, nil
	}

	const query = `
        SELECT id, rate_limits_id, requests, period_seconds, burst, system, created_at, updated_at, serial_id
        FROM rate_limit_windows
        WHERE rate_limits_id = ANY($1)
        ORDER BY period_seconds;
    `

	rows, err := ref.db.Query(ctx, query, ruleIDs)
	if err != nil {
		return nil, ref.handlePgError(err, nil)
	}
	defer rows.Close()

	for rows.Next() {
		var w domain.RateLimitWindow

		var seconds int

		if err := rows.Scan(&w.ID, &w.RateLimit, &w.Requests, &seconds, &w.Burst, &w.System, &w.CreatedAt, &w.UpdatedAt, &w.SerialID); err != nil {
			return nil, ref.handlePgError(err, nil)
		}

		w.Period = time.Duration(seconds) * time.Second
		out[w.RateLimit] = append(out[w.RateLimit], w)
	}

	if err := rows.Err(); err != nil {
		return nil, ref.handlePgError(err, nil)
	}

	return out, nil
}

// Select returns a page of rules, with filter, sort, fields and the paginator.
func (ref *RateLimitsRepository) Select(ctx context.Context, input *domain.SelectRateLimitsInput) (*domain.SelectRateLimitsOutput, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "Select")
	defer cancel()
	defer span.End()

	if input == nil {
		return nil, o11y.RecordError(ctx, span, start, &domain.InvalidInputError{Message: "input is nil"}, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	sqlFieldsPrefix := "rl."
	fieldsArray := []string{
		"id", "name", "description", "target_kind", "target", "methods",
		"scope", "audience", "strategy", "enabled", "system",
		"created_at", "updated_at", "serial_id",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.RateLimitsFilterFields)
		filterQuery = fmt.Sprintf("WHERE (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "rl.serial_id DESC, rl.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.RateLimitsSortFields)
	}

	queryTemplate := `
        WITH rl AS (
            SELECT
                {{.QueryColumns}}
            FROM rate_limits AS rl
            {{ .QueryWhere }}
            ORDER BY {{.QueryInternalSort}}
            LIMIT {{.QueryLimit}}
        ) SELECT * FROM rl ORDER BY {{.QueryExternalSort}}
    `

	var queryValues struct {
		QueryColumns      template.HTML
		QueryWhere        template.HTML
		QueryInternalSort string
		QueryExternalSort string
		QueryLimit        int
	}

	queryValues.QueryColumns = template.HTML(fieldsStr)
	queryValues.QueryWhere = template.HTML(filterQuery)
	queryValues.QueryLimit = input.Paginator.Limit + 1
	queryValues.QueryInternalSort = "rl.serial_id DESC, rl.id DESC"
	queryValues.QueryExternalSort = sortQuery

	tokenDirection, id, serial, err := domain.GetPaginatorDirection(input.Paginator.NextToken, input.Paginator.PrevToken)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("rl", tokenDirection, id, serial, filterQuery, false)

	var tpl bytes.Buffer

	t := template.Must(template.New("query").Parse(queryTemplate))
	if err := t.Execute(&tpl, queryValues); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.RateLimits.Select", "query", prettyPrint(query))

	rows, err := ref.db.Query(ctx, query)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.RateLimit

	for rows.Next() {
		var item domain.RateLimit

		var raw rateLimitRawEnums

		scanFields := ref.buildScanFields(&item, &raw, input.Fields)

		if err := rows.Scan(scanFields...); err != nil {
			return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
		}

		raw.apply(&item)
		fetchedItems = append(fetchedItems, item)
	}

	if err := rows.Err(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	hasMore := len(fetchedItems) > input.Paginator.Limit

	displayItems := fetchedItems
	if hasMore {
		displayItems = fetchedItems[:input.Paginator.Limit]
	}

	outLen := len(displayItems)
	if outLen == 0 {
		slog.Warn("repository.RateLimits.Select", "what", "no rate limits found")

		return &domain.SelectRateLimitsOutput{
			Items:     make([]domain.RateLimit, 0),
			Paginator: domain.Paginator{},
		}, nil
	}

	// The windows come back in a second query rather than an array_agg. A rule
	// carries at most five, so the join would multiply the page by five rows and
	// then have to be de-duplicated -- and a LIMIT applied to the joined result
	// would silently truncate a rule's windows rather than the page.
	ids := make([]uuid.UUID, 0, outLen)
	for i := range displayItems {
		ids = append(ids, displayItems[i].ID)
	}

	windows, err := ref.selectWindows(ctx, ids)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	for i := range displayItems {
		displayItems[i].Windows = windows[displayItems[i].ID]
	}

	repoFoundMoreForNextQuery := false
	repoFoundMoreForPrevQuery := false

	switch tokenDirection {
	case domain.TokenDirectionNext:
		repoFoundMoreForPrevQuery = true
		repoFoundMoreForNextQuery = hasMore
	case domain.TokenDirectionPrev:
		repoFoundMoreForNextQuery = true
		repoFoundMoreForPrevQuery = hasMore
	default:
		repoFoundMoreForNextQuery = hasMore
	}

	minSerialItem := displayItems[0]
	maxSerialItem := displayItems[0]

	for _, item := range displayItems {
		if item.SerialID < minSerialItem.SerialID {
			minSerialItem = item
		}

		if item.SerialID > maxSerialItem.SerialID {
			maxSerialItem = item
		}
	}

	nextToken, prevToken := domain.GetTokens(
		outLen,
		maxSerialItem.ID,
		maxSerialItem.SerialID,
		minSerialItem.ID,
		minSerialItem.SerialID,
		tokenDirection,
		repoFoundMoreForNextQuery,
		repoFoundMoreForPrevQuery,
	)

	ret := &domain.SelectRateLimitsOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "rate limits found", attribute.Int("rate_limits.count", outLen))

	return ret, nil
}

// rateLimitRawEnums holds the columns that are strings in Postgres and typed
// constants in the domain. Scanning straight into the domain type would work --
// they are string kinds -- but it would also accept ANY string, so a hand-edited
// row would flow into the limiter and be discovered as a panic or a wrong
// strategy rather than as a bad row.
type rateLimitRawEnums struct {
	kind     string
	scope    string
	audience string
	strategy string
}

func (r *rateLimitRawEnums) apply(item *domain.RateLimit) {
	if r.kind != "" {
		item.TargetKind = domain.RateLimitTargetKind(r.kind)
	}

	if r.scope != "" {
		item.Scope = domain.RateLimitScope(r.scope)
	}

	if r.audience != "" {
		item.Audience = domain.RateLimitAudience(r.audience)
	}

	if r.strategy != "" {
		item.Strategy = domain.RateLimitStrategy(r.strategy)
	}
}

// buildScanFields maps requested fields to scan targets.
//
// The default branch must list EVERY column in fieldsArray, in the same order.
// It is a []any, so a column added to one and forgotten in the other fails at
// run time with a scan-arity error, not at compile time --
// TestScanFieldsCoverEveryColumn is what makes that a build failure instead.
func (ref *RateLimitsRepository) buildScanFields(item *domain.RateLimit, raw *rateLimitRawEnums, requestedFields string) []any {
	if requestedFields == "" {
		return []any{
			&item.ID, &item.Name, &item.Description, &raw.kind, &item.Target, &item.Methods,
			&raw.scope, &raw.audience, &raw.strategy, &item.Enabled, &item.System,
			&item.CreatedAt, &item.UpdatedAt, &item.SerialID,
		}
	}

	scanFields := make([]any, 0)

	var idFound bool

	for field := range strings.SplitSeq(requestedFields, ",") {
		switch strings.TrimSpace(field) {
		case domain.FieldID:
			scanFields = append(scanFields, &item.ID)
			idFound = true
		case domain.FieldName:
			scanFields = append(scanFields, &item.Name)
		case domain.FieldDescription:
			scanFields = append(scanFields, &item.Description)
		case domain.FieldTargetKind:
			scanFields = append(scanFields, &raw.kind)
		case domain.FieldTarget:
			scanFields = append(scanFields, &item.Target)
		case domain.FieldMethods:
			scanFields = append(scanFields, &item.Methods)
		case domain.FieldScope:
			scanFields = append(scanFields, &raw.scope)
		case domain.FieldAudience:
			scanFields = append(scanFields, &raw.audience)
		case domain.FieldStrategy:
			scanFields = append(scanFields, &raw.strategy)
		case domain.FieldEnabled:
			scanFields = append(scanFields, &item.Enabled)
		case domain.FieldSystem:
			scanFields = append(scanFields, &item.System)
		case domain.FieldCreatedAt:
			scanFields = append(scanFields, &item.CreatedAt)
		case domain.FieldUpdatedAt:
			scanFields = append(scanFields, &item.UpdatedAt)
		}
	}

	// The paginator keys on (serial_id, id), so both are always selected even
	// when the caller did not ask for them -- otherwise a partial-fields query
	// returns a page that cannot be paged.
	if !idFound {
		scanFields = append(scanFields, &item.ID)
	}

	scanFields = append(scanFields, &item.SerialID)

	return scanFields
}

// handlePgError turns a Postgres error into the domain error the handler maps to
// a status code.
//
//nolint:gocognit // a flat switch over constraint names is the clearest form; splitting it hides which code maps where.
func (ref *RateLimitsRepository) handlePgError(err error, input any) error {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok {
		return err
	}

	switch pgErr.Code {
	case "23505": // unique violation
		switch {
		case strings.Contains(pgErr.ConstraintName, constraintRateLimitName):
			return &domain.RateLimitAlreadyExistsError{Name: rateLimitNameOf(input)}
		case strings.Contains(pgErr.ConstraintName, constraintRateLimitWindowPeriod):
			// Validation catches this first, so reaching here means two writers
			// raced on the same rule. Reporting it as a duplicate NAME would
			// send the operator to rename a rule that is not the problem.
			return &domain.InvalidRateLimitTargetError{
				Reason: "two windows on the same period; the rule was changed concurrently, retry the update",
			}
		case strings.Contains(pgErr.Message, "_pkey"):
			return &domain.RateLimitAlreadyExistsError{ID: rateLimitIDOf(input)}
		}

	case "23514": // check violation -- a value validation should have caught
		switch {
		case strings.Contains(pgErr.ConstraintName, constraintRateLimitStrategyCheck):
			return &domain.InvalidRateLimitStrategyError{Valid: rateLimitStrategyNames()}
		case strings.Contains(pgErr.ConstraintName, constraintRateLimitScopeCheck),
			strings.Contains(pgErr.ConstraintName, constraintRateLimitAudienceCheck),
			strings.Contains(pgErr.ConstraintName, constraintRateLimitKindCheck),
			strings.Contains(pgErr.ConstraintName, constraintRateLimitTargetCheck),
			strings.Contains(pgErr.ConstraintName, constraintRateLimitMethodsCheck):
			return &domain.InvalidRateLimitTargetError{
				Reason: "the rule violates " + pgErr.ConstraintName + "; this should have been caught in validation, please report it",
			}
		}

	case "23503": // foreign key violation
		return &domain.RateLimitNotFoundError{ID: rateLimitIDOf(input)}

	case "P0001": // raised by fn_restrict_delete_update_on_system
		if strings.Contains(pgErr.Message, "updated") || strings.Contains(pgErr.Message, "deleted") {
			return &domain.SystemRateLimitError{RateLimitID: rateLimitIDOf(input)}
		}

	case "22021":
		return &domain.InvalidByteSequenceError{Message: pgErr.Message}
	}

	return err
}

func rateLimitStrategyNames() []string {
	strategies := domain.RateLimitStrategies()

	out := make([]string, 0, len(strategies))
	for _, s := range strategies {
		out = append(out, string(s))
	}

	return out
}

func rateLimitIDOf(input any) uuid.UUID {
	switch v := input.(type) {
	case *domain.CreateRateLimitInput:
		return v.ID
	case *domain.UpdateRateLimitInput:
		return v.ID
	case *domain.DeleteRateLimitInput:
		return v.ID
	case uuid.UUID:
		return v
	default:
		return uuid.Nil()
	}
}

func rateLimitNameOf(input any) string {
	switch v := input.(type) {
	case *domain.CreateRateLimitInput:
		return v.Name
	case *domain.UpdateRateLimitInput:
		return v.Name
	default:
		return ""
	}
}
