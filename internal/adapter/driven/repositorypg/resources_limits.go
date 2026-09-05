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
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
	"github.com/slashdevops/go-rest-api-service-template/pkg/cslog"
)

type ResourcesLimitsRepositoryConfig struct {
	DB              *pgxpool.Pool
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
	MaxPingTimeout  time.Duration
	MaxQueryTimeout time.Duration
}

// ResourcesLimitsRepository is a PostgreSQL store.
type ResourcesLimitsRepository struct {
	db              *pgxpool.Pool
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
	maxPingTimeout  time.Duration
	maxQueryTimeout time.Duration
}

// NewResourcesLimitsRepository creates a new ResourcesLimitsRepository.
func NewResourcesLimitsRepository(conf ResourcesLimitsRepositoryConfig) (*ResourcesLimitsRepository, error) {
	if conf.DB == nil {
		return nil, &domain.InvalidDBConfigurationError{Message: "invalid database configuration. It is nil"}
	}

	if conf.MaxPingTimeout < domain.ValidDatabaseMinPingTimeout || conf.MaxPingTimeout > domain.ValidDatabaseMaxPingTimeout {
		return nil, &domain.InvalidDBMaxPingTimeoutError{
			Message: fmt.Sprintf("invalid max ping timeout. It must be between %d and %d",
				domain.ValidDatabaseMinPingTimeout,
				domain.ValidDatabaseMaxPingTimeout),
		}
	}

	if conf.MaxQueryTimeout < domain.ValidDatabaseMinQueryTimeout || conf.MaxQueryTimeout > domain.ValidDatabaseMaxQueryTimeout {
		return nil, &domain.InvalidDBMaxQueryTimeoutError{
			Message: fmt.Sprintf("invalid max query timeout. It must be between %d and %d",
				domain.ValidDatabaseMinQueryTimeout,
				domain.ValidDatabaseMaxQueryTimeout),
		}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "invalid OpenTelemetry configuration. It is nil"}
	}

	ref := &ResourcesLimitsRepository{
		db:              conf.DB,
		maxPingTimeout:  conf.MaxPingTimeout,
		maxQueryTimeout: conf.MaxQueryTimeout,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "ResourcesLimits",
			Action: "NewResourcesLimitsRepository",
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

// DriverName returns the name of the driver.
func (ref *ResourcesLimitsRepository) DriverName() string {
	return sql.Drivers()[0]
}

// PingContext verifies a connection to the repository is still alive, establishing a connection if necessary.
func (ref *ResourcesLimitsRepository) PingContext(ctx context.Context) error {
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

// Functions related to handlers

func (ref *ResourcesLimitsRepository) Select(ctx context.Context, input *domain.SelectResourcesLimitsInput) (*domain.SelectResourcesLimitsOutput, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "Select")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// if no fields are provided, select all fields
	sqlFieldsPrefix := "rls."
	fieldsArray := []string{
		"id",
		"scope_type",
		"scope_id",
		"resource_type",
		"COALESCE(rls.usage, 0) AS usage",
		"COALESCE(rl.soft_limit, -1) AS soft_limit",
		"COALESCE(rl.hard_limit, -1) AS hard_limit",
		"created_at",
		"updated_at",
		"serial_id",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.ResourcesLimitsFilterFields)
		filterQuery = fmt.Sprintf("WHERE (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "rls.serial_id DESC, rls.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.ResourcesLimitsSortFields)
	}

	// query template
	queryTemplate := `
        WITH rls AS (
            SELECT
                {{.QueryColumns}}
            FROM resources_usage AS rls
		        -- scope_id is part of the key: without it one usage row fans out against every
		        -- limit row of the same type and reports another scope's limits as its own.
		        LEFT JOIN resources_limits AS rl
		            ON  rls.scope_type = rl.scope_type
		            AND rls.resource_type = rl.resource_type
		            AND rls.scope_id = rl.scope_id
            {{ .QueryWhere }}
            ORDER BY {{.QueryInternalSort}}
            LIMIT {{.QueryLimit}}
        ) SELECT * FROM rls ORDER BY {{.QueryExternalSort}}
    `

	// struct to hold the query values
	var queryValues struct {
		QueryColumns      template.HTML
		QueryWhere        template.HTML
		QueryInternalSort string
		QueryExternalSort string
		QueryLimit        int
	}

	// default values
	queryValues.QueryColumns = template.HTML(fieldsStr)
	queryValues.QueryWhere = template.HTML(filterQuery)
	queryValues.QueryLimit = input.Paginator.Limit + 1 // Fetch one extra item
	queryValues.QueryInternalSort = "rls.serial_id DESC, rls.id DESC"
	queryValues.QueryExternalSort = sortQuery

	tokenDirection, id, serial, err := domain.GetPaginatorDirection(input.Paginator.NextToken, input.Paginator.PrevToken)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("rls", tokenDirection, id, serial, filterQuery, false)

	// render the template on query variable
	var tpl bytes.Buffer
	t := template.Must(template.New("query").Parse(queryTemplate))
	err = t.Execute(&tpl, queryValues)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.ResourcesLimits.Select", "query", prettyPrint(query))

	// execute the query
	rows, err := ref.db.Query(ctx, query)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.ResourcesLimits
	for rows.Next() {
		var item domain.ResourcesLimits

		scanFields := ref.buildScanFields(&item, input.Fields)

		if err := rows.Scan(scanFields...); err != nil {
			return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
		}

		fetchedItems = append(fetchedItems, item)
	}

	if err := rows.Err(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(rows.Err(), nil), ref.metrics, attrs)
	}

	hasMore := len(fetchedItems) > input.Paginator.Limit
	displayItems := fetchedItems
	if hasMore {
		displayItems = fetchedItems[:input.Paginator.Limit]
	}

	outLen := len(displayItems)
	if outLen == 0 {
		return &domain.SelectResourcesLimitsOutput{
			Items:     make([]domain.ResourcesLimits, 0),
			Paginator: domain.Paginator{},
		}, nil
	}

	repoFoundMoreForNextQuery := false
	repoFoundMoreForPrevQuery := false

	switch tokenDirection {
	case domain.TokenDirectionNext: // Used 'next' token to get current page
		repoFoundMoreForPrevQuery = true // Came from a previous page
		repoFoundMoreForNextQuery = hasMore
	case domain.TokenDirectionPrev: // Used 'prev' token to get current page
		repoFoundMoreForNextQuery = true // Came from a next page
		repoFoundMoreForPrevQuery = hasMore
	default: // Initial load (tokenDirection == domain.TokenDirectionInvalid)
		repoFoundMoreForNextQuery = hasMore
		// repoFoundMoreForPrevQuery remains false, GetTokens will handle it
	}

	// Calculate min/max serial_id from entire result set to ensure correct pagination boundaries
	// regardless of external sorting order (e.g., "name ASC")
	minSerialID := displayItems[0].SerialID
	maxSerialID := displayItems[0].SerialID
	minSerialItem := displayItems[0]
	maxSerialItem := displayItems[0]

	for _, item := range displayItems {
		if item.SerialID < minSerialID {
			minSerialID = item.SerialID
			minSerialItem = item
		}
		if item.SerialID > maxSerialID {
			maxSerialID = item.SerialID
			maxSerialItem = item
		}
	}

	nextToken, prevToken := domain.GetTokens(
		outLen,
		maxSerialItem.ID, // Use item with MAX serial for prev token
		maxSerialItem.SerialID,
		minSerialItem.ID, // Use item with MIN serial for next token
		minSerialItem.SerialID,
		tokenDirection,
		repoFoundMoreForNextQuery,
		repoFoundMoreForPrevQuery,
	)

	ret := &domain.SelectResourcesLimitsOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "resources limits selected successfully")

	return ret, nil
}

// buildScanFields creates the scan targets for the result rows based on the requested fields.
func (ref *ResourcesLimitsRepository) buildScanFields(item *domain.ResourcesLimits, requestedFields string) []any {
	scanFields := make([]any, 0)

	if requestedFields == "" {
		// All fields were requested
		return []any{
			&item.ID,
			&item.ScopeType,
			&item.ScopeID,
			&item.ResourceType,
			&item.Usage,
			&item.SoftLimit,
			&item.HardLimit,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.SerialID,
		}
	}

	var idFound bool
	inputFields := strings.SplitSeq(requestedFields, ",")

	for field := range inputFields {
		field = strings.TrimSpace(field)

		switch field {
		case "id":
			scanFields = append(scanFields, &item.ID)
			idFound = true
		case "scope_type":
			scanFields = append(scanFields, &item.ScopeType)
		case "scope_id":
			scanFields = append(scanFields, &item.ScopeID)
		case "resource_type":
			scanFields = append(scanFields, &item.ResourceType)
		case "usage":
			scanFields = append(scanFields, &item.Usage)
		case "soft_limit":
			scanFields = append(scanFields, &item.SoftLimit)
		case "hard_limit":
			scanFields = append(scanFields, &item.HardLimit)
		case "created_at":
			scanFields = append(scanFields, &item.CreatedAt)
		case "updated_at":
			scanFields = append(scanFields, &item.UpdatedAt)

		default:
			slog.Warn("repository.ResourcesLimits.buildScanFields", "what", "field not found", "field", field)
		}
	}

	// Always include ID and SerialID fields for pagination
	if !idFound {
		scanFields = append(scanFields, &item.ID)
	}

	scanFields = append(scanFields, &item.SerialID)
	return scanFields
}

// Functions not related to the handlers

// resolveLimitCTE is the scope-resolution query body, shared verbatim by
// CheckUsage and ReserveUsage so the two can never disagree about which limit
// applies. Parameters are $1 scope_type, $2 resource_type, $3 scope_id, and it
// exposes a single row as `resolved(usage, soft_limit, hard_limit, signature,
// has_usage_row)`.
//
// Three priorities, best (lowest) wins:
//
//  1. a limit row for this exact scope_id
//  2. the default row for the scope type (scope_id = zero UUID), joined to
//     *this* scope's usage — the limit is shared, the counter is not
//  3. -1 sentinels, which the service reads as unlimited
//
// has_usage_row distinguishes "no counter yet" from "counter is zero", which
// the caller needs in order to decide whether there is a signature to verify.
// Without it a scope that has never been used looks identical to one whose
// signature was stripped.
const resolveLimitCTE = `
    WITH ranked_results AS (
        -- PRIORITY 1: a limit matching the specific scope_id.
        SELECT
            COALESCE(ru.usage, 0) AS usage,
            rl.soft_limit,
            rl.hard_limit,
            COALESCE(ru.signature, '') AS signature,
            (ru.scope_id IS NOT NULL) AS has_usage_row,
            1 AS priority
        FROM resources_limits AS rl
        LEFT JOIN resources_usage AS ru
            ON  ru.scope_type = rl.scope_type
            AND ru.resource_type = rl.resource_type
            AND ru.scope_id = rl.scope_id
        WHERE rl.scope_type = $1
        AND rl.resource_type = $2
        AND rl.scope_id = $3

        UNION ALL

        -- PRIORITY 2: the default row for the scope type.
        -- IMPORTANT: the limit row is the shared default (scope_id = zero UUID) but the counter is
        -- the caller's own, so this join must use $3. It previously used a hardcoded UUID left over
        -- from debugging, which made every scope that relied on the default limit read usage = 0
        -- forever: the limit resolved, the usage never did, and nothing was ever enforced.
        SELECT
            COALESCE(ru.usage, 0) AS usage,
            rl.soft_limit,
            rl.hard_limit,
            COALESCE(ru.signature, '') AS signature,
            (ru.scope_id IS NOT NULL) AS has_usage_row,
            2 AS priority
        FROM resources_limits AS rl
        LEFT JOIN resources_usage AS ru
            ON  ru.scope_type = rl.scope_type
            AND ru.resource_type = rl.resource_type
            AND ru.scope_id = $3 -- the caller's scope, NOT the default row's scope_id
        WHERE rl.scope_type = $1
        AND rl.resource_type = $2
        AND rl.scope_id = '00000000-0000-0000-0000-000000000000'::uuid

        UNION ALL

        -- PRIORITY 3: nothing configured. Reads as unlimited, which is wrong under
        -- licensing and is scheduled to become a free tier instead.
        SELECT -1, -1, -1, ''::bytea, FALSE, 3
    ),
    resolved AS (
        SELECT usage, soft_limit, hard_limit, signature, has_usage_row
        FROM ranked_results
        ORDER BY priority
        LIMIT 1
    )`

// resourceCountQuery is how one resource type is counted for a scope during
// reconciliation.
type resourceCountQuery struct {
	// sql counts the resources. When scoped, $1 is the scope id.
	sql string

	// scoped reports whether sql takes the scope id. System-wide resources
	// count everything and take no parameter.
	scoped bool
}

// resourceCountQueries maps a resource type to the source of truth its counter
// is supposed to mirror.
//
// Two rules are load-bearing here, and getting either wrong makes reconciliation
// worse than no reconciliation:
//
// **System rows are excluded.** Rows with system = TRUE are seeded by migrations
// and never went through a creation path, so no increment ever counted them.
// Counting them now would invent usage out of nothing — the seeded model
// catalogue alone is 60 rows in the default project, which a recount would
// otherwise charge to that project's products limit.
//
// **The count must mirror what the increment counted**, not what seems
// reasonable. Where the two disagree the increment wins, because that is what
// enforcement has been doing; see the projects note below.
var resourceCountQueries = map[domain.ResourcesLimitsResourceType]resourceCountQuery{
	domain.ResourcesLimitsResourceTypeUsers: {
		sql: `SELECT count(*) FROM users;`,
	},
	domain.ResourcesLimitsResourceTypeIDPs: {
		sql: `SELECT count(*) FROM idps;`,
	},

	// NOTE: projects are counted by *membership*, because the schema records no
	// owner — creation links the creator into projects_users and nothing marks
	// them as the creator afterwards. The two agree until a project is shared:
	// a user linked to someone else's project then counts it against their own
	// limit, and a creator who is unlinked stops counting theirs. Making this
	// exact needs a created_by column; until then a recount can move a counter
	// that increments would not have.
	domain.ResourcesLimitsResourceTypeProjects: {
		sql: `
            SELECT count(*)
            FROM projects_users AS pu
            JOIN projects AS p ON p.id = pu.projects_id
            WHERE pu.users_id = $1 AND NOT p.system;
        `,
		scoped: true,
	},

	domain.ResourcesLimitsResourceTypeProducts: {
		sql:    `SELECT count(*) FROM products WHERE projects_id = $1 AND NOT system;`,
		scoped: true,
	},
}

// validateScope applies the checks shared by the usage mutators and normalises
// a nil scope ID to the zero UUID used for system-level rows.
func (ref *ResourcesLimitsRepository) validateScope(scope *domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType) error {
	if scope.Type == "" {
		return &domain.InvalidScopeTypeError{Message: "invalid scope type. It must not be empty"}
	}

	if scope.ID != nil && *scope.ID == uuid.Nil() {
		return &domain.InvalidScopeTypeError{Message: "scope ID must not be nil for specific scopes"}
	}

	if resourceType == "" {
		return &domain.InvalidResourceTypeError{Message: "invalid resource type. It must not be empty"}
	}

	if scope.ID == nil {
		// For system level (scope_id IS NULL), we treat it as the zero UUID internally
		scope.ID = &uuid.UUID{}
	}

	return nil
}

// mutateUsage runs a counter statement and the signature write for its result
// inside one transaction, and returns the new usage.
//
// Both statements must share a transaction. The row lock taken by the counter
// statement is held until commit, so a concurrent caller blocks instead of
// interleaving its signature write with ours. When these were two separate
// calls the last signature to land could belong to an earlier counter value,
// which permanently invalidated the row — see [repository.ResourcesLimitsSigner].
func (ref *ResourcesLimitsRepository) mutateUsage(
	ctx context.Context,
	scope domain.ResourcesLimitsScope,
	resourceType domain.ResourcesLimitsResourceType,
	sign repository.ResourcesLimitsSigner,
	counterQuery string,
	counterArgs []any,
) (int, error) {
	tx, txErr := ref.db.Begin(ctx)
	if txErr != nil {
		return 0, ref.handlePgError(txErr, nil)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			slog.Error("repository.ResourcesLimits.mutateUsage", "what", "rollback failed", "error", err)
		}
	}()

	var newUsage int
	if err := tx.QueryRow(ctx, counterQuery, counterArgs...).Scan(&newUsage); err != nil {
		// A decrement against a counter already at zero matches no row. That is
		// not an error: the counter is where it should be.
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}

		return 0, ref.handlePgError(err, nil)
	}

	signature, err := sign(newUsage)
	if err != nil {
		return 0, err
	}

	const signatureQuery = `
        UPDATE resources_usage
        SET signature = $1
        WHERE scope_type = $2 AND resource_type = $3 AND scope_id = $4;
    `

	if _, err := tx.Exec(ctx, signatureQuery, signature, scope.Type.String(), resourceType.String(), scope.ID); err != nil {
		return 0, ref.handlePgError(err, nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, ref.handlePgError(err, nil)
	}

	return newUsage, nil
}

// ReserveUsage claims one unit of a resource for a scope, or refuses because
// the hard limit is reached. It is the write path that enforcement runs on.
//
// Everything happens in one transaction, in this order:
//
//  1. resolve the limit and lock the usage row (FOR UPDATE)
//  2. verify the stored signature, if there is a row to verify
//  3. refuse with [domain.ResourcesLimitsHardLimitReachedError] if the counter
//     is already at the hard limit
//  4. increment, sign the new value, store it
//
// Steps 1-4 in one transaction is the whole point. The previous shape —
// CheckUsage in the use-case, then a separate increment after the insert — left
// a window in which N concurrent callers all read the same "usage < hard_limit"
// and all proceeded, overshooting the limit by up to N-1. Taking the row lock in
// step 1 and holding it to commit closes that window: concurrent reservations
// queue, and each sees the previous one's result.
//
// The caller reserves *before* creating the resource and releases with
// [ResourcesLimitsRepository.DecrementUsage] if the creation then fails. That
// ordering is deliberate: an interrupted create leaves the counter one too high,
// which refuses a later request, rather than one too low, which would let a
// tenant past their limit. Over-counting is repaired by reconciliation;
// under-counting silently sells capacity that was not licensed.
func (ref *ResourcesLimitsRepository) ReserveUsage(
	ctx context.Context,
	scope domain.ResourcesLimitsScope,
	resourceType domain.ResourcesLimitsResourceType,
	verify repository.ResourcesLimitsVerifier,
	sign repository.ResourcesLimitsSigner,
) (int, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ReserveUsage")
	defer span.End()

	if err := ref.validateScope(&scope, resourceType); err != nil {
		return 0, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	tx, txErr := ref.db.Begin(ctx)
	if txErr != nil {
		return 0, o11y.RecordError(ctx, span, start, ref.handlePgError(txErr, nil), ref.metrics, attrs)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			slog.Error("repository.ResourcesLimits.ReserveUsage", "what", "rollback failed", "error", err)
		}
	}()

	// Step 1: resolve the applicable limits. This reads shared rows and takes no
	// lock — FOR UPDATE cannot be applied here anyway, because the usage side of
	// the resolution is an outer join and PostgreSQL refuses to lock the
	// nullable side of one (SQLSTATE 0A000).
	resolveQuery := resolveLimitCTE + `
        SELECT soft_limit, hard_limit FROM resolved;
    `

	cslog.Trace(ctx, "repository.ResourcesLimits.ReserveUsage", "query",
		prettyPrint(resolveQuery, scope.Type.String(), resourceType.String(), scope.ID.String()))

	var softLimit, hardLimit int
	if err := tx.QueryRow(ctx, resolveQuery, scope.Type.String(), resourceType.String(), scope.ID).
		Scan(&softLimit, &hardLimit); err != nil {
		return 0, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	// Step 2: lock this scope's counter, on its own so the lock is legal. The
	// row may not exist yet; that is handled in step 4.
	const lockQuery = `
        SELECT usage, signature
        FROM resources_usage
        WHERE scope_type = $1 AND resource_type = $2 AND scope_id = $3
        FOR UPDATE;
    `

	var (
		usage       int
		signature   []byte
		hasUsageRow = true
	)

	if err := tx.QueryRow(ctx, lockQuery, scope.Type.String(), resourceType.String(), scope.ID).
		Scan(&usage, &signature); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
		}

		hasUsageRow = false
	}

	// Step 3: verify, but only when a counter actually exists. A scope that has
	// never been used has nothing to verify, and treating that as tampering
	// would refuse every tenant's first creation.
	if hasUsageRow && verify != nil {
		if err := verify(usage, signature); err != nil {
			return 0, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	// Step 4: increment, guarded by the hard limit in the statement itself.
	//
	// The guard is repeated here rather than only tested in Go because of the
	// first-insert race: when no row exists there is nothing for step 2 to lock,
	// so two callers can both find the counter absent. ON CONFLICT DO UPDATE
	// re-evaluates its WHERE against the live row while holding the row lock the
	// unique index gives it, so the second caller sees the first one's value.
	// No rows returned means the limit is reached.
	//
	// $4 is the hard limit and $5 the unlimited sentinel, passed rather than
	// written literally so Go and SQL cannot disagree about its value.
	const incrementQuery = `
        INSERT INTO resources_usage (scope_type, scope_id, resource_type, usage)
        SELECT $1, $2, $3, 1
        WHERE $4::int = $5::int OR $4::int >= 1
        ON CONFLICT (scope_type, scope_id, resource_type)
        DO UPDATE SET usage = resources_usage.usage + 1
        WHERE $4::int = $5::int OR resources_usage.usage < $4::int
        RETURNING usage;
    `

	limitReached := func() error {
		return &domain.ResourcesLimitsHardLimitReachedError{
			Message: fmt.Sprintf(
				"hard limit reached: scope type %s, scope ID %v, resource type %s. soft limit %d, hard limit %d, current usage %d",
				scope.Type, scope.ID, resourceType, softLimit, hardLimit, usage),
			Resource: resourceType.String(),
			Limit:    int64(hardLimit),
			Current:  int64(usage),
		}
	}

	var newUsage int
	if err := tx.QueryRow(ctx, incrementQuery, scope.Type.String(), scope.ID, resourceType.String(), hardLimit, domain.ResourcesLimitsUnlimited).
		Scan(&newUsage); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, o11y.RecordError(ctx, span, start, limitReached(), ref.metrics, attrs)
		}

		return 0, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	newSignature, err := sign(newUsage)
	if err != nil {
		return 0, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	const signatureQuery = `
        UPDATE resources_usage
        SET signature = $1
        WHERE scope_type = $2 AND resource_type = $3 AND scope_id = $4;
    `

	if _, err := tx.Exec(ctx, signatureQuery, newSignature, scope.Type.String(), resourceType.String(), scope.ID); err != nil {
		return 0, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "repository.ResourcesLimits.ReserveUsage",
		attribute.Int("usage", newUsage), attribute.Int("hard_limit", hardLimit))

	return newUsage, nil
}

// RecountUsage recomputes a counter from the resource table it tracks and stores
// the corrected value with a fresh signature.
//
// The whole thing runs in one transaction with the counter row locked, so a
// reservation racing with a repair either waits for the corrected value or is
// counted by the recount. Without the lock a create landing mid-recount could be
// counted by neither.
func (ref *ResourcesLimitsRepository) RecountUsage(
	ctx context.Context,
	scope domain.ResourcesLimitsScope,
	resourceType domain.ResourcesLimitsResourceType,
	sign repository.ResourcesLimitsSigner,
) (*domain.ResourcesLimitsRecountOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "RecountUsage")
	defer span.End()

	if err := ref.validateScope(&scope, resourceType); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	countQuery, ok := resourceCountQueries[resourceType]
	if !ok {
		// A resource type with no counting rule cannot be reconciled. Refusing
		// is the safe answer: silently leaving the counter alone would report a
		// successful repair that never happened.
		errorType := &domain.InvalidResourceTypeError{
			Message: fmt.Sprintf("resource type %q has no reconciliation query", resourceType),
		}

		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	tx, txErr := ref.db.Begin(ctx)
	if txErr != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(txErr, nil), ref.metrics, attrs)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			slog.Error("repository.ResourcesLimits.RecountUsage", "what", "rollback failed", "error", err)
		}
	}()

	// Lock the counter first, so nothing moves it while we count.
	const lockQuery = `
        SELECT usage
        FROM resources_usage
        WHERE scope_type = $1 AND resource_type = $2 AND scope_id = $3
        FOR UPDATE;
    `

	out := &domain.ResourcesLimitsRecountOutput{HadUsageRow: true}

	if err := tx.QueryRow(ctx, lockQuery, scope.Type.String(), resourceType.String(), scope.ID).
		Scan(&out.Previous); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
		}

		out.HadUsageRow = false
	}

	countArgs := []any{}
	if countQuery.scoped {
		countArgs = append(countArgs, scope.ID)
	}

	cslog.Trace(ctx, "repository.ResourcesLimits.RecountUsage", "query", prettyPrint(countQuery.sql, countArgs...))

	if err := tx.QueryRow(ctx, countQuery.sql, countArgs...).Scan(&out.Actual); err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	// Write the corrected counter even when it did not change, because the
	// signature may be the thing that is wrong — that is the case a recount is
	// most often called to repair.
	const upsertQuery = `
        INSERT INTO resources_usage (scope_type, scope_id, resource_type, usage)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (scope_type, scope_id, resource_type)
        DO UPDATE SET usage = EXCLUDED.usage;
    `

	if _, err := tx.Exec(ctx, upsertQuery, scope.Type.String(), scope.ID, resourceType.String(), out.Actual); err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	signature, err := sign(out.Actual)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	const signatureQuery = `
        UPDATE resources_usage
        SET signature = $1
        WHERE scope_type = $2 AND resource_type = $3 AND scope_id = $4;
    `

	if _, err := tx.Exec(ctx, signatureQuery, signature, scope.Type.String(), resourceType.String(), scope.ID); err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "repository.ResourcesLimits.RecountUsage",
		attribute.Int("previous", out.Previous), attribute.Int("actual", out.Actual))

	return out, nil
}

// SelectTrackedScopes returns every scope that has a usage row.
func (ref *ResourcesLimitsRepository) SelectTrackedScopes(ctx context.Context) ([]domain.ResourcesLimitsTrackedScope, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectTrackedScopes")
	defer cancel()
	defer span.End()

	const query = `
        SELECT scope_type, scope_id, resource_type
        FROM resources_usage
        ORDER BY scope_type, resource_type, scope_id;
    `

	rows, err := ref.db.Query(ctx, query)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}
	defer rows.Close()

	var scopes []domain.ResourcesLimitsTrackedScope
	for rows.Next() {
		var (
			scopeType    string
			scopeID      uuid.UUID
			resourceType string
		)

		if err := rows.Scan(&scopeType, &scopeID, &resourceType); err != nil {
			return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
		}

		scopes = append(scopes, domain.ResourcesLimitsTrackedScope{
			ScopeType:    domain.ResourcesLimitsScopeType(scopeType),
			ScopeID:      scopeID,
			ResourceType: domain.ResourcesLimitsResourceType(resourceType),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "repository.ResourcesLimits.SelectTrackedScopes",
		attribute.Int("count", len(scopes)))

	return scopes, nil
}

// IncrementUsage increments the usage of a resource type for a given scope and
// stores the matching signature atomically. It returns the new usage.
func (ref *ResourcesLimitsRepository) IncrementUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType, sign repository.ResourcesLimitsSigner) (int, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "IncrementUsage")
	defer span.End()

	if err := ref.validateScope(&scope, resourceType); err != nil {
		return 0, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	const query = `
        INSERT INTO resources_usage (scope_type, scope_id, resource_type, usage)
        VALUES ($1, $2, $3, 1)
        ON CONFLICT (scope_type, scope_id, resource_type)
        DO UPDATE SET usage = resources_usage.usage + 1
        RETURNING usage;
    `

	cslog.Trace(ctx, "repository.ResourcesLimits.IncrementUsage", "query", prettyPrint(query, scope.Type.String(), scope.ID.String(), resourceType.String()))

	newUsage, err := ref.mutateUsage(ctx, scope, resourceType, sign, query,
		[]any{scope.Type.String(), scope.ID, resourceType.String()})
	if err != nil {
		return 0, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "repository.ResourcesLimits.IncrementUsage")

	return newUsage, nil
}

// DecrementUsage lowers the usage of a resource type for a given scope, never
// below zero, and stores the matching signature atomically.
func (ref *ResourcesLimitsRepository) DecrementUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType, sign repository.ResourcesLimitsSigner) (int, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "DecrementUsage")
	defer span.End()

	if err := ref.validateScope(&scope, resourceType); err != nil {
		return 0, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	const query = `
        UPDATE resources_usage
        SET usage = usage - 1
        WHERE
            scope_type = $1
            AND resource_type = $2
            AND scope_id = $3
            AND usage > 0
        RETURNING usage;
    `

	cslog.Trace(ctx, "repository.ResourcesLimits.DecrementUsage", "query", prettyPrint(query, scope.Type.String(), resourceType.String(), scope.ID.String()))

	newUsage, err := ref.mutateUsage(ctx, scope, resourceType, sign, query,
		[]any{scope.Type.String(), resourceType.String(), scope.ID})
	if err != nil {
		return 0, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "repository.ResourcesLimits.DecrementUsage")

	return newUsage, nil
}

func (ref *ResourcesLimitsRepository) CheckUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType) (*domain.ResourcesLimitsCheckUsageOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "CheckUsage")
	defer span.End()

	if scope.Type == "" {
		errorType := &domain.InvalidScopeTypeError{Message: "invalid scope type. It must not be empty"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	if scope.ID != nil && *scope.ID == uuid.Nil() {
		errorType := &domain.InvalidScopeTypeError{Message: "scope ID must not be nil for specific scopes"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	if resourceType == "" {
		errorType := &domain.InvalidResourceTypeError{Message: "invalid resource type. It must not be empty"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	if scope.ID == nil {
		// For system level (scope_id IS NULL), we treat it as the zero UUID internally
		scope.ID = &uuid.UUID{}
	}

	query := resolveLimitCTE + `
        SELECT usage, soft_limit, hard_limit, signature, has_usage_row FROM resolved;
    `

	cslog.Trace(ctx, "repository.ResourcesLimits.CheckUsage", "query", prettyPrint(query, scope.Type.String(), resourceType.String(), scope.ID.String()))

	// Check the usage in the database
	var check domain.ResourcesLimitsCheckUsageOutput
	err := ref.db.QueryRow(ctx, query,
		scope.Type.String(),
		resourceType.String(),
		scope.ID,
	).Scan(&check.Usage, &check.SoftLimit, &check.HardLimit, &check.Signature, &check.HasUsageRow)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "repository.ResourcesLimits.CheckUsage")

	return &check, nil
}

// handlePgError maps PostgreSQL errors to domain-specific errors.
// Returns the appropriate domain error or the original error if no mapping exists.
func (ref *ResourcesLimitsRepository) handlePgError(err error, input any) error {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case "23505": // Unique violation
			if strings.Contains(pgErr.Message, "_pkey") {
				switch v := input.(type) {
				case uuid.UUID:
					return &domain.ResourcesLimitsAlreadyExistsError{Message: fmt.Sprintf("resource limit with ID %s already exists", v.String())}
				}
			}

		case "23503": // Foreign key violation
			return &domain.ResourcesLimitsForeignKeyError{Message: pgErr.Message}

		case "22021": // invalid byte sequence for encoding
			return &domain.InvalidByteSequenceError{Message: pgErr.Message}
		case "08P01": // invalid message format
			return &domain.InvalidMessageFormatError{Message: pgErr.Message}

		case "42703": // undefined column
			return &domain.UndefinedColumnError{Message: pgErr.Message}
		case "42804": // datatype mismatch - operator class does not accept data type
			return &domain.DatatypeMismatchError{Message: pgErr.Message}
		}
	}

	return err
}
