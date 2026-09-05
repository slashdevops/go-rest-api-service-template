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

type PoliciesRepositoryConfig struct {
	DB              *pgxpool.Pool
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
	MaxPingTimeout  time.Duration
	MaxQueryTimeout time.Duration
}

// PoliciesRepository is a PostgreSQL store.
type PoliciesRepository struct {
	db              *pgxpool.Pool
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
	maxPingTimeout  time.Duration
	maxQueryTimeout time.Duration
}

// NewPoliciesRepository creates a new PoliciesRepository.
func NewPoliciesRepository(conf PoliciesRepositoryConfig) (*PoliciesRepository, error) {
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

	ref := &PoliciesRepository{
		db:              conf.DB,
		maxPingTimeout:  conf.MaxPingTimeout,
		maxQueryTimeout: conf.MaxQueryTimeout,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Policies",
			Action: "NewPoliciesRepository",
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
func (ref *PoliciesRepository) DriverName() string {
	return sql.Drivers()[0]
}

// PingContext verifies a connection to the repository is still alive, establishing a connection if necessary.
func (ref *PoliciesRepository) PingContext(ctx context.Context) error {
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

// Insert inserts a new policy into the repository.
func (ref *PoliciesRepository) Insert(ctx context.Context, input *domain.CreatePolicyInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "Insert")
	defer cancel()
	defer span.End()

	if input == nil {
		errorType := &domain.InvalidPolicyIDError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := `
        INSERT INTO policies (id, resources_id, name, description, allowed_action, allowed_resource)
        VALUES ($1, $2, $3, $4, $5, $6);
    `

	cslog.Trace(ctx, "repository.Policies.Insert", "query", prettyPrint(query, input.ID.String(), input.ResourceID.String(), input.Name, input.Description, input.AllowedAction, input.AllowedResource))

	_, err := ref.db.Exec(ctx, query,
		input.ID,
		input.ResourceID,
		input.Name,
		input.Description,
		input.AllowedAction,
		input.AllowedResource,
	)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	slog.Debug("repository.Policies.Insert", "policy_id", input.ID.String())
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "policy inserted successfully", attribute.String("policy.id", input.ID.String()))

	return nil
}

// UpdateByID updates a policy in the repository.
func (ref *PoliciesRepository) UpdateByID(ctx context.Context, input *domain.UpdatePolicyInput) error {
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

	span.SetAttributes(attribute.String("policy_id", input.ID.String()))

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

	if input.AllowedAction != nil && *input.AllowedAction != "" {
		args = append(args, *input.AllowedAction)
	} else {
		args = append(args, nil)
	}

	if input.AllowedResource != nil && *input.AllowedResource != "" {
		args = append(args, *input.AllowedResource)
	} else {
		args = append(args, nil)
	}

	query := `
        UPDATE policies SET
            name             = COALESCE(NULLIF($2, ''), name),
            description      = COALESCE(NULLIF($3, ''), description),
            allowed_action   = COALESCE(NULLIF($4, ''), allowed_action),
            allowed_resource = COALESCE(NULLIF($5, ''), allowed_resource),
            updated_at       = CURRENT_TIMESTAMP
        WHERE id = $1;
    `

	cslog.Trace(ctx, "repository.Policies.UpdateByID", "query", prettyPrint(query))

	result, err := ref.db.Exec(ctx, query, args...)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	if result.RowsAffected() == 0 {
		errorType := &domain.PolicyNotFoundError{Message: "policy not found"}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "policy updated successfully", attribute.String("policy.id", input.ID.String()))

	return nil
}

// DeleteByID deletes a policy from the repository.
func (ref *PoliciesRepository) DeleteByID(ctx context.Context, input *domain.DeletePolicyInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "DeleteByID")
	defer cancel()
	defer span.End()

	if input == nil {
		errorType := &domain.InvalidPolicyIDError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	query := `
        DELETE FROM policies WHERE id = $1;
    `

	cslog.Trace(ctx, "repository.Policies.DeleteByID", "query", prettyPrint(query, input.ID.String()))

	result, err := ref.db.Exec(ctx, query, input.ID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	if result.RowsAffected() == 0 {
		// grateful return user was deleted, security reason, but log and record error
		errorType := &domain.PolicyNotFoundError{Message: "policy not found"}
		e := o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
		slog.Error("repository.Policies.DeleteByID", "error", e, "policy.id", input.ID.String())

		return nil
	}

	slog.Debug("repository.Policies.DeleteByID", "policy_id", input.ID.String())
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "policy deleted successfully",
		attribute.String("policy.id", input.ID.String()),
	)

	return nil
}

// SelectByID returns the resource with the specified ID.
func (ref *PoliciesRepository) SelectByID(ctx context.Context, id uuid.UUID) (*domain.Policy, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByID")
	defer cancel()
	defer span.End()

	span.SetAttributes(attribute.String("policy.id", id.String()))

	if !domain.IsUUIDV7(id) {
		errorType := &domain.InvalidPolicyIDError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	query := `
        SELECT
            pol.id,
            pol.name,
            pol.description,
            pol.allowed_action,
            pol.allowed_resource,
            pol.system,
            pol.created_at,
            pol.updated_at,
            array_agg(DISTINCT(ARRAY[COALESCE(res.id::varchar, '00000000-0000-0000-0000-000000000000'), COALESCE(res.name::varchar,'')])) AS resource
        FROM policies AS pol
            -- resources
            LEFT JOIN resources AS res ON pol.resources_id = res.id
        WHERE pol.id = $1
        GROUP BY pol.id, res.id;
    `

	cslog.Trace(ctx, "repository.Policies.SelectByID", "query", prettyPrint(query))

	row := ref.db.QueryRow(ctx, query, id)

	var element domain.Policy
	var resources []string

	if err := row.Scan(
		&element.ID,
		&element.Name,
		&element.Description,
		&element.AllowedAction,
		&element.AllowedResource,
		&element.System,
		&element.CreatedAt,
		&element.UpdatedAt,
		&resources,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errorType := &domain.PolicyNotFoundError{Message: "policy not found"}
			return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
		}

		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, id), ref.metrics, attrs)
	}

	var err error
	if len(resources) > 0 {
		element.Resource.ID, err = uuid.Parse(resources[0])
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		element.Resource.Name = resources[1]
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "policies selected successfully",
		attribute.String("policy.id", id.String()),
	)

	return &element, nil
}

func (ref *PoliciesRepository) Select(ctx context.Context, input *domain.SelectPoliciesInput) (*domain.SelectPoliciesOutput, error) {
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
	sqlFieldsPrefix := "pol."
	fieldsArray := []string{
		"id",
		"name",
		"description",
		"allowed_action",
		"allowed_resource",
		"system",
		"created_at",
		"updated_at",
		"serial_id",
		"array_agg(DISTINCT(ARRAY[COALESCE(res.id::varchar, '00000000-0000-0000-0000-000000000000'), COALESCE(res.name::varchar,'')])) AS resource",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.PoliciesFilterFields)
		filterQuery = fmt.Sprintf("WHERE (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "pol.serial_id DESC, pol.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.PoliciesSortFields)
	}

	// query template
	queryTemplate := `
        WITH pol AS (
            SELECT
                {{.QueryColumns}}
            FROM policies AS pol
                -- resources
                LEFT JOIN resources AS res ON pol.resources_id = res.id
            {{ .QueryWhere }}
            GROUP BY pol.id, res.id
            ORDER BY {{.QueryInternalSort}}
            LIMIT {{.QueryLimit}}
        ) SELECT * FROM pol ORDER BY {{.QueryExternalSort}}
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
	queryValues.QueryInternalSort = "pol.serial_id DESC, pol.id DESC"
	queryValues.QueryExternalSort = sortQuery

	tokenDirection, id, serial, err := domain.GetPaginatorDirection(input.Paginator.NextToken, input.Paginator.PrevToken)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("pol", tokenDirection, id, serial, filterQuery, false)

	// render the template on query variable
	var tpl bytes.Buffer
	t := template.Must(template.New("query").Parse(queryTemplate))
	err = t.Execute(&tpl, queryValues)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.Policies.Select", "query", prettyPrint(query))

	// execute the query
	rows, err := ref.db.Query(ctx, query)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.Policy
	for rows.Next() {
		var item domain.Policy
		var resources []string

		scanFields, err := ref.buildScanFields(&item, &resources, input.Fields)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		if err := rows.Scan(scanFields...); err != nil {
			return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
		}

		if len(resources) > 0 {
			item.Resource.ID, err = uuid.Parse(resources[0])
			if err != nil {
				return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			}

			item.Resource.Name = resources[1]
		}

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
		slog.Warn("repository.Policies.Select", "what", "no policies found")
		return &domain.SelectPoliciesOutput{
			Items:     make([]domain.Policy, 0),
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

	ret := &domain.SelectPoliciesOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "policies selected successfully")

	return ret, nil
}

// SelectByRoleID returns the policies with the specified role ID.
func (ref *PoliciesRepository) SelectByRoleID(ctx context.Context, roleID uuid.UUID, input *domain.SelectPoliciesInput) (*domain.SelectPoliciesOutput, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByRoleID")
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
	sqlFieldsPrefix := "pol."
	fieldsArray := []string{
		"id",
		"name",
		"description",
		"allowed_action",
		"allowed_resource",
		"system",
		"created_at",
		"updated_at",
		"serial_id",
		"array_agg(DISTINCT(ARRAY[COALESCE(res.id::varchar, '00000000-0000-0000-0000-000000000000'), COALESCE(res.name::varchar,'')])) AS resource",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.PoliciesFilterFields)
		filterQuery = fmt.Sprintf("AND (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "pol.serial_id DESC, pol.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.PoliciesSortFields)
	}

	// query template
	queryTemplate := `
        WITH pol AS (
            SELECT
                {{.QueryColumns}}
            FROM policies AS pol
                -- resources
                LEFT JOIN resources AS res ON pol.resources_id = res.id
                -- roles
                LEFT JOIN roles_policies AS rp ON pol.id = rp.policies_id
            WHERE rp.roles_id = $1
            {{ .QueryWhere }}
            GROUP BY pol.id, res.id
            ORDER BY {{.QueryInternalSort}}
            LIMIT {{.QueryLimit}}
        ) SELECT * FROM pol ORDER BY {{.QueryExternalSort}}
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
	queryValues.QueryInternalSort = "pol.serial_id DESC, pol.id DESC"
	queryValues.QueryExternalSort = sortQuery

	tokenDirection, id, serial, err := domain.GetPaginatorDirection(input.Paginator.NextToken, input.Paginator.PrevToken)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("pol", tokenDirection, id, serial, filterQuery, true)

	// render the template on query variable
	var tpl bytes.Buffer
	t := template.Must(template.New("query").Parse(queryTemplate))
	err = t.Execute(&tpl, queryValues)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.Policies.SelectByRoleID", "query", prettyPrint(query))

	// execute the query
	rows, err := ref.db.Query(ctx, query, roleID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.Policy

	for rows.Next() {
		var item domain.Policy
		var resources []string

		scanFields, err := ref.buildScanFields(&item, &resources, input.Fields)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		if err := rows.Scan(scanFields...); err != nil {
			return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
		}

		if len(resources) > 0 {
			item.Resource.ID, err = uuid.Parse(resources[0])
			if err != nil {
				return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			}

			item.Resource.Name = resources[1]
		}

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
		slog.Warn("repository.Policies.SelectByRoleID", "what", "no policies found")
		return &domain.SelectPoliciesOutput{
			Items:     make([]domain.Policy, 0),
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

	ret := &domain.SelectPoliciesOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "policies selected successfully")

	return ret, nil
}

// LinkRoles links roles to a policies.
func (ref *PoliciesRepository) LinkRoles(ctx context.Context, input *domain.LinkRolesToPolicyInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "LinkRoles")
	defer cancel()
	defer span.End()

	if input == nil {
		errorType := &domain.InvalidPolicyIDError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("policy_id", input.PolicyID.String()))

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	var policiesRoles bytes.Buffer
	for i, roleID := range input.RoleIDs {
		_, err := fmt.Fprintf(&policiesRoles, "('%s', '%s')", input.PolicyID.String(), roleID)
		if err != nil {
			return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		if i < len(input.RoleIDs)-1 {
			_, err := policiesRoles.WriteString(", ")
			if err != nil {
				return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			}
		}
	}

	query := fmt.Sprintf(`
        -- Insert roles to policies
        INSERT INTO roles_policies (policies_id, roles_id)
        VALUES %s
        ON CONFLICT (policies_id, roles_id)
        DO UPDATE SET updated_at = NOW();
    `,
		policiesRoles.String(),
	)

	cslog.Trace(ctx, "repository.Policies.LinkRoles", "query", prettyPrint(query))

	_, err := ref.db.Exec(ctx, query)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "roles linked to policies")

	return nil
}

// UnlinkRoles unlinks roles from a policies.
func (ref *PoliciesRepository) UnlinkRoles(ctx context.Context, input *domain.UnlinkRolesFromPolicyInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "UnlinkRoles")
	defer cancel()
	defer span.End()

	if input == nil {
		errorType := &domain.InvalidPolicyIDError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Mirrors UsersRepository.UnlinkRoles, which already does this correctly:
	// the role ids travel as one array parameter, expanded with unnest.
	//
	// This previously built `('<policy>', '<role>')` per role and substituted the
	// joined result into `roles_id IN %s` — the tuple shape copied from LinkRoles,
	// where it is correct for `INSERT ... VALUES`. In an `IN` list it produced:
	//
	//   two+ roles  →  ... roles_id IN ('P','R1'), ('P','R2')
	//                  ERROR: syntax error at or near "," — unlinking more than
	//                  one role at a time failed outright.
	//
	//   one role    →  ... roles_id IN ('P','R1')
	//                  Valid, but the policy id is matched as though it were a
	//                  role id. Correct only because a policy id never collides
	//                  with a role id — not because the query says what it means.
	//
	// The values were uuid.UUID throughout, so this was never injectable; it was
	// wrong.
	roleIDs := make([]string, len(input.RoleIDs))
	for i, roleID := range input.RoleIDs {
		roleIDs[i] = roleID.String()
	}

	query := `
        -- Delete roles from policies
        DELETE FROM roles_policies
        WHERE policies_id = $1 AND roles_id IN (SELECT unnest($2::uuid[]));
    `

	cslog.Trace(ctx, "repository.Policies.UnlinkRoles", "query",
		prettyPrint(query, input.PolicyID.String(), roleIDs))

	_, err := ref.db.Exec(ctx, query, input.PolicyID.String(), roleIDs)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "roles unlinked from policies")

	return nil
}

// handlePgError maps PostgreSQL errors to domain-specific errors.
// Returns the appropriate domain error or the original error if no mapping exists.
func (ref *PoliciesRepository) handlePgError(err error, input any) error {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case "23505": // Unique violation
			if strings.Contains(pgErr.Message, "_pkey") {
				switch v := input.(type) {
				case *domain.CreatePolicyInput:
					return &domain.PolicyAlreadyExistsError{ID: v.ID}
				case *domain.UpdatePolicyInput:
					return &domain.PolicyAlreadyExistsError{ID: v.ID}
				case uuid.UUID:
					return &domain.PolicyAlreadyExistsError{ID: v}
				}
			}
			if strings.Contains(pgErr.Message, "_name") {
				switch v := input.(type) {
				case *domain.CreatePolicyInput:
					return &domain.PolicyAlreadyExistsError{Name: v.Name}
				case *domain.UpdatePolicyInput:
					if v.Name != nil {
						return &domain.PolicyAlreadyExistsError{Name: *v.Name}
					}
				}
			}
		case "23503": // Foreign key violation
			if strings.Contains(pgErr.Message, "resources_id_fkey") {
				switch v := input.(type) {
				case *domain.CreatePolicyInput:
					id := v.ResourceID.String()
					return &domain.ResourceNotFoundError{ID: id}
				}
			}
		case "P0001": // Raised exception
			if strings.Contains(pgErr.Message, "updated") || strings.Contains(pgErr.Message, "deleted") {
				switch v := input.(type) {
				case *domain.UpdatePolicyInput:
					return &domain.SystemPolicyError{PolicyID: v.ID}
				case *domain.DeletePolicyInput:
					return &domain.SystemPolicyError{PolicyID: v.ID}
				case uuid.UUID:
					return &domain.SystemPolicyError{PolicyID: v}
				}
			}
		case "22021": // Invalid byte sequence for encoding
			return &domain.InvalidByteSequenceError{Message: pgErr.Message}
		case "08P01": // Invalid message format
			return &domain.InvalidMessageFormatError{Message: pgErr.Message}
		case "42703": // Undefined column
			return &domain.UndefinedColumnError{Message: pgErr.Message}
		case "42804": // Datatype mismatch
			return &domain.DatatypeMismatchError{Message: pgErr.Message}
		}
	}

	return err
}

// buildScanFields creates the scan targets for the result rows based on the requested fields.
func (ref *PoliciesRepository) buildScanFields(item *domain.Policy, resources *[]string, requestedFields string) ([]any, error) {
	scanFields := make([]any, 0)

	if requestedFields == "" {
		// All fields were requested
		return []any{
			&item.ID,
			&item.Name,
			&item.Description,
			&item.AllowedAction,
			&item.AllowedResource,
			&item.System,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.SerialID,
			resources,
		}, nil
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
		case "allowed_action":
			scanFields = append(scanFields, &item.AllowedAction)
		case "allowed_resource":
			scanFields = append(scanFields, &item.AllowedResource)
		case "system":
			scanFields = append(scanFields, &item.System)
		case "created_at":
			scanFields = append(scanFields, &item.CreatedAt)
		case "updated_at":
			scanFields = append(scanFields, &item.UpdatedAt)
		case "resource":
			scanFields = append(scanFields, resources)

		default:
			slog.Warn("repository.Policies.buildScanFields", "what", "field not found", "field", field)
		}
	}

	// always select id and serial_id for pagination
	// if id is not selected, it will be added to the scanFields
	if !idFound {
		scanFields = append(scanFields, &item.ID)
	}

	scanFields = append(scanFields, &item.SerialID)

	return scanFields, nil
}
