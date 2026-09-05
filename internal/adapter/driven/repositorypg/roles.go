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

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
	"github.com/slashdevops/go-rest-api-service-template/pkg/cslog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// RolesRepositoryConfig is the configuration for the RolesRepository.
type RolesRepositoryConfig struct {
	DB              *pgxpool.Pool
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
	MaxPingTimeout  time.Duration
	MaxQueryTimeout time.Duration
}

// RolesRepository is a PostgreSQL store.
type RolesRepository struct {
	db              *pgxpool.Pool
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
	maxPingTimeout  time.Duration
	maxQueryTimeout time.Duration
}

// NewRolesRepository creates a new RolesRepository.
func NewRolesRepository(conf RolesRepositoryConfig) (*RolesRepository, error) {
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

	ref := &RolesRepository{
		db:              conf.DB,
		maxPingTimeout:  conf.MaxPingTimeout,
		maxQueryTimeout: conf.MaxQueryTimeout,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Roles",
			Action: "NewRolesRepository",
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
func (ref *RolesRepository) DriverName() string {
	return sql.Drivers()[0]
}

// PingContext verifies a connection to the repository is still alive, establishing a connection if necessary.
func (ref *RolesRepository) PingContext(ctx context.Context) error {
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

// Insert a new role into the database.
func (ref *RolesRepository) Insert(ctx context.Context, input *domain.InsertRoleInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "Insert")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := `
        INSERT INTO roles (id, name, description)
        VALUES ($1, $2, $3);
    `

	cslog.Trace(ctx, "repository.Roles.Insert", "query", prettyPrint(query, input.ID.String(), input.Name, input.Description))

	_, err := ref.db.Exec(ctx, query,
		input.ID.String(),
		input.Name,
		input.Description,
	)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	slog.Debug("repository.Roles.Insert", "role.id", input.ID)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "role inserted successfully",
		attribute.String("role.id", input.ID.String()),
	)

	return nil
}

// UpdateByID updates the role with the specified ID.
func (ref *RolesRepository) UpdateByID(ctx context.Context, input *domain.UpdateRoleInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "UpdateByID")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("role.id", input.ID.String()))

	// Values go through $n placeholders, never string interpolation.
	//
	// This used to build the SET clause with fmt.Sprintf("name='%s'", …) and
	// interpolate the id into the WHERE, which was a reachable SQL injection:
	// role-name validation forbids control characters, HTML, script tags and
	// null bytes, but permits an apostrophe, so a name of
	//
	//     x', description='injected
	//
	// closed the literal and appended an attacker-controlled assignment. Every
	// other repository in this package already used placeholders; this was the
	// only one that did not.
	//
	// COALESCE(NULLIF($n, ''), col) keeps the partial-update semantics: a nil or
	// empty argument leaves the existing column value untouched. Same shape as
	// ProjectsRepository.UpdateByID.
	args := []any{input.ID}

	if input.Name != nil && *input.Name != "" {
		args = append(args, *input.Name)
	} else {
		args = append(args, nil)
	}

	if input.Description != nil && *input.Description != "" {
		args = append(args, *input.Description)
	} else {
		args = append(args, nil)
	}

	// CURRENT_TIMESTAMP rather than a Go-side timestamp: the database is the
	// clock every other column in this table is stamped from, and it removes the
	// MarshalText error path entirely.
	query := `
        UPDATE roles
        SET
            name        = COALESCE(NULLIF($2, ''), name),
            description = COALESCE(NULLIF($3, ''), description),
            updated_at  = CURRENT_TIMESTAMP
        WHERE id = $1;
        `

	cslog.Trace(ctx, "repository.Roles.UpdateByID", "query", prettyPrint(query))

	result, err := ref.db.Exec(ctx, query, args...)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	if result.RowsAffected() == 0 {
		errorType := &domain.RoleNotFoundError{RoleID: input.ID.String()}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "role updated successfully",
		attribute.String("role.id", input.ID.String()),
	)

	return nil
}

// DeleteByID deletes the role with the specified ID.
func (ref *RolesRepository) DeleteByID(ctx context.Context, input *domain.DeleteRoleInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "DeleteByID")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	queryString := `
        DELETE FROM roles WHERE id = $1;
    `

	cslog.Trace(ctx, "repository.Roles.Delete", "query", prettyPrint(queryString))

	result, err := ref.db.Exec(ctx, queryString, input.ID.String())
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	if result.RowsAffected() == 0 {
		// grateful return user was deleted, security reason, but log and record error
		errorType := &domain.RoleNotFoundError{RoleID: input.ID.String()}
		e := o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
		slog.Error("repository.Roles.DeleteByID", "error", e, "role.id", input.ID.String())

		return nil
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "role deleted successfully",
		attribute.String("role.id", input.ID.String()),
	)

	return nil
}

// SelectByID returns the role and its policies with the specified ID.
func (ref *RolesRepository) SelectByID(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByID")
	defer cancel()
	defer span.End()

	if !domain.IsUUIDV7(id) {
		invalidErr := &domain.InvalidRoleIDError{Message: "invalid role ID"}
		return nil, o11y.RecordError(ctx, span, start, invalidErr, ref.metrics, attrs)
	}

	query := `
        SELECT
            rls.id,
            rls.name,
            rls.description,
            rls.system,
            rls.auto_assign,
            rls.created_at,
            rls.updated_at
        FROM roles AS rls
        WHERE rls.id = $1
        GROUP BY rls.id;
    `

	cslog.Trace(ctx, "repository.Roles.SelectByID", "query", prettyPrint(query))

	row := ref.db.QueryRow(ctx, query, id.String())

	var item domain.Role

	if err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Description,
		&item.System,
		&item.AutoAssign,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errorType := &domain.RoleNotFoundError{RoleID: id.String()}
			return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
		}

		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "role selected successfully",
		attribute.String("role.id", id.String()),
	)

	return &item, nil
}

func (ref *RolesRepository) Select(ctx context.Context, input *domain.SelectRolesInput) (*domain.SelectRolesOutput, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "Select")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// if no fields are provided, select all fields
	sqlFieldsPrefix := "rls."
	fieldsArray := []string{
		"id",
		"name",
		"description",
		"system",
		"auto_assign",
		"created_at",
		"updated_at",
		"serial_id",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.RolesFilterFields)
		filterQuery = fmt.Sprintf("WHERE (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "rls.serial_id DESC, rls.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.RolesSortFields)
	}

	// query template
	queryTemplate := `
        WITH rls AS (
            SELECT
                {{.QueryColumns}}
            FROM roles AS rls
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
	cslog.Trace(ctx, "repository.Roles.Select", "query", prettyPrint(query))

	// execute the query
	rows, err := ref.db.Query(ctx, query)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.Role
	for rows.Next() {
		var item domain.Role

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
		return &domain.SelectRolesOutput{
			Items:     make([]domain.Role, 0),
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

	ret := &domain.SelectRolesOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "roles selected successfully")

	return ret, nil
}

// SelectByPolicyID selects the roles by policy ID.
func (ref *RolesRepository) SelectByPolicyID(ctx context.Context, policyID uuid.UUID, input *domain.SelectRolesInput) (*domain.SelectRolesOutput, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByPolicyID")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if policyID == uuid.Nil() {
		invalidErr := &domain.InvalidPolicyIDError{Message: "invalid policy ID"}
		return nil, o11y.RecordError(ctx, span, start, invalidErr, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// if no fields are provided, select all fields
	sqlFieldsPrefix := "rls."
	fieldsArray := []string{
		"id",
		"name",
		"description",
		"system",
		"auto_assign",
		"created_at",
		"updated_at",
		"serial_id",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.RolesFilterFields)
		filterQuery = fmt.Sprintf("AND (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "rls.serial_id DESC, rls.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.RolesSortFields)
	}

	// query template
	queryTemplate := `
        WITH rls AS (
            SELECT
                {{.QueryColumns}}
            FROM roles AS rls
                -- policies
                LEFT JOIN roles_policies AS rp ON rls.id = rp.roles_id
                LEFT JOIN policies AS p ON rp.policies_id = p.id
            WHERE p.id = $1
            {{ .QueryWhere }}
            GROUP BY rls.id
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

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("rls", tokenDirection, id, serial, filterQuery, true)

	// render the template on query variable
	var tpl bytes.Buffer
	t := template.Must(template.New("query").Parse(queryTemplate))
	err = t.Execute(&tpl, queryValues)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.Roles.SelectByPolicyID", "query", prettyPrint(query))

	// execute the query
	rows, err := ref.db.Query(ctx, query, policyID.String())
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.Role
	for rows.Next() {
		var item domain.Role

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
		slog.Warn("repository.Roles.SelectByPolicyID", "what", "no roles found")
		return &domain.SelectRolesOutput{
			Items:     make([]domain.Role, 0),
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

	ret := &domain.SelectRolesOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "roles selected successfully",
		attribute.String("policy.id", policyID.String()),
	)

	return ret, nil
}

func (ref *RolesRepository) SelectByUserID(ctx context.Context, userID uuid.UUID, input *domain.SelectRolesInput) (*domain.SelectRolesOutput, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByUserID")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if userID == uuid.Nil() {
		invalidErr := &domain.InvalidUserIDError{Message: "invalid user ID"}
		return nil, o11y.RecordError(ctx, span, start, invalidErr, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// if no fields are provided, select all fields
	sqlFieldsPrefix := "rls."
	fieldsArray := []string{
		"id",
		"name",
		"description",
		"system",
		"auto_assign",
		"created_at",
		"updated_at",
		"serial_id",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.RolesFilterFields)
		filterQuery = fmt.Sprintf("AND (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "rls.serial_id DESC, rls.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.RolesSortFields)
	}

	// query template
	queryTemplate := `
        WITH rls AS (
            SELECT
                {{.QueryColumns}}
            FROM roles AS rls
                -- users
                LEFT JOIN users_roles AS ur ON rls.id = ur.roles_id
                LEFT JOIN users AS u ON ur.users_id = u.id
            WHERE u.id = $1
            {{ .QueryWhere }}
            GROUP BY rls.id
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

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("rls", tokenDirection, id, serial, filterQuery, true)

	// render the template on query variable
	var tpl bytes.Buffer
	t := template.Must(template.New("query").Parse(queryTemplate))
	err = t.Execute(&tpl, queryValues)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.Roles.SelectByUserID", "query", prettyPrint(query))

	// execute the query
	rows, err := ref.db.Query(ctx, query, userID.String())
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.Role
	for rows.Next() {
		var item domain.Role

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
		slog.Warn("repository.Roles.SelectByUserID", "what", "no roles found")
		return &domain.SelectRolesOutput{
			Items:     make([]domain.Role, 0),
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

	ret := &domain.SelectRolesOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "roles selected successfully",
		attribute.String("user.id", userID.String()),
	)

	return ret, nil
}

// LinkUsers links the users to the role.
func (ref *RolesRepository) LinkUsers(ctx context.Context, input *domain.LinkUsersToRoleInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "LinkUsers")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Prepare arrays for UNNEST
	roleIDs := make([]string, len(input.UserIDs))
	userIDs := make([]string, len(input.UserIDs))
	for i, userID := range input.UserIDs {
		roleIDs[i] = input.RoleID.String() // Ensure RoleID is converted to string (e.g., if it's a UUID)
		userIDs[i] = userID.String()       // Ensure userID is converted to string (e.g., if it's a UUID)
	}

	query := `
        -- insert the new users
        INSERT INTO users_roles (roles_id, users_id)
        SELECT * FROM UNNEST($1::uuid[], $2::uuid[]) -- Use appropriate type casting for your UUIDs
        ON CONFLICT (roles_id, users_id)
        DO UPDATE SET updated_at = NOW();
    `

	cslog.Trace(ctx, "repository.Roles.LinkUsers", "query", prettyPrint(query, roleIDs, userIDs))

	// Pass the arrays as parameters
	_, err := ref.db.Exec(ctx, query, roleIDs, userIDs)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "users linked successfully")

	return nil
}

// UnlinkUsers unlinks the users from the role.
func (ref *RolesRepository) UnlinkUsers(ctx context.Context, input *domain.UnlinkUsersFromRoleInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "UnlinkUsers")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Prepare the user IDs for the parameterized query.
	// Assuming UserIDs are UUIDs, convert them to string slices.
	userIDs := make([]string, len(input.UserIDs))
	for i, userID := range input.UserIDs {
		userIDs[i] = userID.String() // Ensure your UUID type has a .String() method
	}

	// Use a parameterized query with ANY() for the IN clause.
	query := `
        DELETE FROM users_roles
        WHERE roles_id = $1 AND users_id IN (SELECT unnest($2::uuid[]));
    `

	cslog.Trace(ctx, "repository.Roles.UnlinkUsers", "query", prettyPrint(query, input.RoleID.String(), userIDs))

	// Execute the query with parameters.
	// Ensure input.RoleID is converted to its string representation if it's a UUID type.
	_, err := ref.db.Exec(ctx, query, input.RoleID.String(), userIDs)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "users unlinked successfully")

	return nil
}

// LinkPolicies links the policies to the role.
func (ref *RolesRepository) LinkPolicies(ctx context.Context, input *domain.LinkPoliciesToRoleInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "LinkPolicies")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Prepare arrays for UNNEST
	roleIDs := make([]string, len(input.PolicyIDs))
	policyIDs := make([]string, len(input.PolicyIDs))
	for i, policyID := range input.PolicyIDs {
		roleIDs[i] = input.RoleID.String() // Ensure RoleID is converted to string (e.g., if it's a UUID)
		policyIDs[i] = policyID.String()   // Ensure PolicyID is converted to string (e.g., if it's a UUID)
	}

	query := `
        -- insert the new policies
        INSERT INTO roles_policies (roles_id, policies_id)
        SELECT * FROM UNNEST($1::uuid[], $2::uuid[]) -- Use appropriate type casting for your UUIDs
        ON CONFLICT (roles_id, policies_id)
        DO UPDATE SET updated_at = NOW();
    `

	cslog.Trace(ctx, "repository.Roles.LinkPolicies", "query", prettyPrint(query, roleIDs, policyIDs))

	// Pass the arrays as parameters
	_, err := ref.db.Exec(ctx, query, roleIDs, policyIDs)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "policies linked successfully")

	return nil
}

// UnlinkPolicies unlinks the policies from the role.
func (ref *RolesRepository) UnlinkPolicies(ctx context.Context, input *domain.UnlinkPoliciesFromRoleInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "UnlinkPolicies")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Prepare the policy IDs for the parameterized query.
	// Assuming PolicyIDs are UUIDs, convert them to string slices.
	policyIDs := make([]string, len(input.PolicyIDs))
	for i, policyID := range input.PolicyIDs {
		policyIDs[i] = policyID.String()
	}

	// Use parameterized query with ANY() for the IN clause.
	query := `
        DELETE FROM roles_policies
        WHERE roles_id = $1  AND policies_id IN (SELECT unnest($2::uuid[]));
    `

	cslog.Trace(ctx, "repository.Roles.UnlinkPolicies", "query", prettyPrint(query, input.RoleID.String(), policyIDs))

	// Execute the query with parameters.
	// Ensure input.RoleID is converted to its string representation if it's a UUID type.
	_, err := ref.db.Exec(ctx, query, input.RoleID.String(), policyIDs)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "policies unlinked successfully")

	return nil
}

// handlePgError maps PostgreSQL errors to domain-specific errors.
// Returns the appropriate domain error or the original error if no mapping exists.
func (ref *RolesRepository) handlePgError(err error, input any) error {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case "23505": // Unique violation
			if strings.Contains(pgErr.Message, "_pkey") {
				switch v := input.(type) {
				case *domain.InsertRoleInput:
					return &domain.RoleAlreadyExistsError{ID: v.ID}
				case *domain.UpdateRoleInput:
					return &domain.RoleAlreadyExistsError{ID: v.ID}
				case uuid.UUID:
					return &domain.RoleAlreadyExistsError{ID: v}
				}
			}

			if strings.Contains(pgErr.Message, "name") {
				switch v := input.(type) {
				case *domain.InsertRoleInput:
					return &domain.RoleAlreadyExistsError{Name: v.Name}
				case *domain.UpdateRoleInput:
					if v.Name != nil {
						return &domain.RoleAlreadyExistsError{Name: *v.Name}
					}
				}
			}
		case "P0001": // Raised exception
			if strings.Contains(pgErr.Message, "updated") || strings.Contains(pgErr.Message, "deleted") {
				switch v := input.(type) {
				case *domain.UpdateRoleInput:
					return &domain.SystemRoleError{RoleID: v.ID}
				case *domain.DeleteRoleInput:
					return &domain.SystemRoleError{RoleID: v.ID}
				case uuid.UUID:
					return &domain.SystemRoleError{RoleID: v}
				}
			}
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

func (ref *RolesRepository) buildScanFields(item *domain.Role, requestedFields string) []any {
	scanFields := make([]any, 0)

	if requestedFields == "" {
		// All fields were requested
		return []any{
			&item.ID,
			&item.Name,
			&item.Description,
			&item.System,
			&item.AutoAssign,
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
		case "name":
			scanFields = append(scanFields, &item.Name)
		case "description":
			scanFields = append(scanFields, &item.Description)
		case "system":
			scanFields = append(scanFields, &item.System)
		case "auto_assign":
			scanFields = append(scanFields, &item.AutoAssign)
		case "created_at":
			scanFields = append(scanFields, &item.CreatedAt)
		case "updated_at":
			scanFields = append(scanFields, &item.UpdatedAt)

		default:
			slog.Warn("repository.Roles.buildScanFields", "what", "field not found", "field", field)
		}
	}

	// Always include ID and SerialID fields for pagination
	if !idFound {
		scanFields = append(scanFields, &item.ID)
	}

	scanFields = append(scanFields, &item.SerialID)
	return scanFields
}
