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

type ProjectsRepositoryConfig struct {
	DB              *pgxpool.Pool
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
	MaxPingTimeout  time.Duration
	MaxQueryTimeout time.Duration
}

// ProjectsRepository is a PostgreSQL store.
type ProjectsRepository struct {
	db              *pgxpool.Pool
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
	maxPingTimeout  time.Duration
	maxQueryTimeout time.Duration
}

// NewProjectsRepository creates a new ProjectsRepository.
func NewProjectsRepository(conf ProjectsRepositoryConfig) (*ProjectsRepository, error) {
	if conf.DB == nil {
		return nil, &domain.InvalidDBConfigurationError{Message: "invalid database configuration. It is nil"}
	}

	if conf.MaxPingTimeout < domain.ValidDatabaseMinPingTimeout || conf.MaxPingTimeout > domain.ValidDatabaseMaxPingTimeout {
		return nil, &domain.InvalidDBMaxPingTimeoutError{
			Message: fmt.Sprintf(
				"invalid max ping timeout. It must be between %d and %d",
				domain.ValidDatabaseMinPingTimeout, domain.ValidDatabaseMaxPingTimeout,
			),
		}
	}

	if conf.MaxQueryTimeout < domain.ValidDatabaseMinQueryTimeout || conf.MaxQueryTimeout > domain.ValidDatabaseMaxQueryTimeout {
		return nil, &domain.InvalidDBMaxQueryTimeoutError{
			Message: fmt.Sprintf(
				"invalid max query timeout. It must be between %d and %d",
				domain.ValidDatabaseMinQueryTimeout, domain.ValidDatabaseMaxQueryTimeout,
			),
		}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "invalid OpenTelemetry configuration. It is nil"}
	}

	ref := &ProjectsRepository{
		db:              conf.DB,
		maxPingTimeout:  conf.MaxPingTimeout,
		maxQueryTimeout: conf.MaxQueryTimeout,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Projects",
			Action: "NewProjectsRepository",
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
func (ref *ProjectsRepository) DriverName() string {
	return sql.Drivers()[0]
}

// PingContext verifies a connection to the repository is still alive, establishing a connection if necessary.
func (ref *ProjectsRepository) PingContext(ctx context.Context) error {
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

// Insert a new project into the database.
func (ref *ProjectsRepository) Insert(ctx context.Context, input *domain.InsertProjectInput) error {
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

	span.SetAttributes(attribute.String("project.id", input.ID.String()))

	tx, txErr := ref.db.Begin(ctx)
	if txErr != nil {
		return o11y.RecordError(ctx, span, start, txErr, ref.metrics, attrs)
	}
	defer func() {
		if txErr != nil {
			if err := tx.Rollback(ctx); err != nil {
				e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
				slog.Error("repository.Projects.Insert", "error", e)
			}
		} else {
			if err := tx.Commit(ctx); err != nil {
				e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
				if e != nil {
					slog.Error("repository.Projects.Insert", "error", e)
				}
			}
		}
	}()

	query1 := `
        INSERT INTO projects (id, name, description, disabled)
        VALUES ($1, $2, $3, $4);
    `

	cslog.Trace(ctx, "repository.Projects.Insert",
		"query", prettyPrint(
			query1,
			input.ID.String(),
			input.Name,
			input.Description,
			input.Disabled,
		),
	)

	_, txErr = tx.Exec(ctx, query1,
		input.ID,
		input.Name,
		input.Description,
		input.Disabled,
	)
	if txErr != nil {
		txErr = o11y.RecordError(ctx, span, start, txErr, ref.metrics, attrs)
		return ref.handlePgError(txErr, input)
	}

	query2 := `
        INSERT INTO projects_users (projects_id, users_id)
        VALUES ($1, $2);
    `

	cslog.Trace(ctx, "repository.Projects.Insert",
		"query", prettyPrint(
			query2,
			input.ID.String(),
			input.UserID.String(),
		),
	)

	_, txErr = tx.Exec(ctx, query2, input.ID, input.UserID)
	if txErr != nil {
		txErr = o11y.RecordError(ctx, span, start, txErr, ref.metrics, attrs)
		return ref.handlePgError(txErr, input)
	}

	slog.Debug("repository.Projects.Insert", "project.id", input.ID)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "project inserted successfully", attribute.String("project.id", input.ID.String()))

	return nil
}

func (ref *ProjectsRepository) UpdateByID(ctx context.Context, input *domain.UpdateProjectInput) error {
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

	span.SetAttributes(attribute.String("project.id", input.ID.String()))

	args := []any{input.ID, input.UserID}

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

	if input.Disabled != nil {
		args = append(args, *input.Disabled)
	} else {
		args = append(args, nil)
	}

	query := `
        UPDATE projects SET
            name        = COALESCE(NULLIF($3, ''), name),
            description = COALESCE(NULLIF($4, ''), description),
            disabled    = COALESCE($5, disabled),
            updated_at  = CURRENT_TIMESTAMP
        WHERE id = $1
        AND (
            -- 2. Add a security check that must pass
            -- Condition A: The user is an admin
            (SELECT admin FROM users WHERE id = $2) = TRUE
            OR
            -- Condition B: The user is assigned to this specific project
            EXISTS (
                SELECT 1
                FROM projects_users
                WHERE projects_id = $1
                AND users_id = $2
            )
        );
    `

	cslog.Trace(ctx, "repository.Projects.UpdateByID", "query", prettyPrint(query, args...))

	result, err := ref.db.Exec(ctx, query, args...)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	if result.RowsAffected() == 0 {
		errorType := &domain.ProjectNotFoundError{ID: input.ID}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "project updated successfully", attribute.String("project.id", input.ID.String()))

	return nil
}

// DeleteByID deletes the project with the specified ID.
func (ref *ProjectsRepository) DeleteByID(ctx context.Context, input *domain.DeleteProjectInput) error {
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

	span.SetAttributes(attribute.String("project.id", input.ID.String()))

	query := `
        DELETE FROM projects
        WHERE id = $1
        AND (
            -- 2. Add a security check that must pass
            -- Condition A: The user is an admin
            (SELECT admin FROM users WHERE id = $2) = TRUE
            OR
            -- Condition B: The user is assigned to this specific project
            EXISTS (
                SELECT 1
                FROM projects_users
                WHERE projects_id = $1
                AND users_id = $2
            )
        );
    `

	result, err := ref.db.Exec(ctx, query, input.ID, input.UserID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	// Nothing matched: either no such project, or the caller may not delete it.
	// Reporting that matters beyond the status code — the use-case decrements
	// the owner's resource counter after a successful delete, so silently
	// returning nil here let a caller lower their own usage by deleting ids that
	// do not exist, and then create past their limit indefinitely.
	//
	// The handler already maps this error to the 404 its annotations declare;
	// until now that branch was unreachable.
	if result.RowsAffected() == 0 {
		errorType := &domain.ProjectNotFoundError{Message: fmt.Sprintf("project with ID %s not found", input.ID.String())}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "project deleted successfully", attribute.String("project.id", input.ID.String()))

	return nil
}

// SelectByIDByUserID returns the project with the specified ID.
func (ref *ProjectsRepository) SelectByIDByUserID(ctx context.Context, id, userID uuid.UUID) (*domain.Project, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByIDByUserID")
	defer cancel()
	defer span.End()

	span.SetAttributes(attribute.String("project.id", id.String()))

	if !domain.IsUUIDV7(id) {
		errorType := &domain.InvalidProjectIDError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	if userID == uuid.Nil() {
		errorType := &domain.InvalidUserIDError{Message: "userID is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	if id == uuid.Nil() && userID == uuid.Nil() {
		errorType := &domain.InvalidInputError{Message: "both project ID and user ID are nil"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	query := `
        SELECT
            vp.id,
            vp.name,
            vp.description,
            vp.disabled,
            vp.system,
            vp.created_at,
            vp.updated_at
        FROM view_projects_users AS vp
        WHERE vp.id = $1 AND vp.user_id = $2;
    `

	cslog.Trace(ctx, "repository.Projects.SelectByIDByUserID",
		"query", prettyPrint(
			query,
			id.String(),
			userID.String(),
		),
	)

	row := ref.db.QueryRow(ctx, query, id.String(), userID.String())

	var item domain.Project

	if err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Description,
		&item.Disabled,
		&item.System,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, id), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "project selected successfully", attribute.String("project.id", id.String()))
	return &item, nil
}

func (ref *ProjectsRepository) SelectByUserID(ctx context.Context, input *domain.SelectProjectsInput) (*domain.SelectProjectsOutput, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByUserID")
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
	sqlFieldsPrefix := "vp."
	fieldsArray := []string{
		"id",
		"name",
		"description",
		"disabled",
		"system",
		"created_at",
		"updated_at",
		"serial_id",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.ProjectFilterFields)
		filterQuery = fmt.Sprintf("AND (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "vp.serial_id DESC, vp.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.ProjectSortFields)
	}

	// query template
	queryTemplate := `
        WITH vp AS (
            SELECT
                {{.QueryColumns}}
            FROM view_projects_users AS vp
            WHERE vp.user_id = $1
                {{ .QueryWhere }}
            ORDER BY {{.QueryInternalSort}}
            LIMIT {{.QueryLimit}}
        ) SELECT * FROM vp ORDER BY {{.QueryExternalSort}}
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
	queryValues.QueryInternalSort = "vp.serial_id DESC, vp.id DESC"
	queryValues.QueryExternalSort = sortQuery

	tokenDirection, id, serial, err := domain.GetPaginatorDirection(input.Paginator.NextToken, input.Paginator.PrevToken)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("vp", tokenDirection, id, serial, filterQuery, true)

	// render the template on query variable
	var tpl bytes.Buffer
	t := template.Must(template.New("query").Parse(queryTemplate))
	err = t.Execute(&tpl, queryValues)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.Projects.Select", "query", prettyPrint(query))

	// execute the query
	rows, err := ref.db.Query(ctx, query, input.UserID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.Project
	for rows.Next() {
		var item domain.Project

		scanFields := ref.buildScanFields(&item, input.Fields)

		if err := rows.Scan(scanFields...); err != nil {
			return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
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
		slog.Warn("repository.Projects.Select", "what", "no projects found")
		return &domain.SelectProjectsOutput{
			Items:     make([]domain.Project, 0),
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
	ret := &domain.SelectProjectsOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "projects selected successfully")

	return ret, nil
}

func (ref *ProjectsRepository) Select(ctx context.Context, input *domain.SelectProjectsInput) (*domain.SelectProjectsOutput, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "Select")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if error := input.Validate(); error != nil {
		return nil, o11y.RecordError(ctx, span, start, error, ref.metrics, attrs)
	}

	// if no fields are provided, select all fields
	sqlFieldsPrefix := "vp."
	fieldsArray := []string{
		"id",
		"name",
		"description",
		"disabled",
		"system",
		"created_at",
		"updated_at",
		"serial_id",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.ProjectFilterFields)
		filterQuery = fmt.Sprintf("AND (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "vp.serial_id DESC, vp.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.ProjectSortFields)
	}

	// query template
	queryTemplate := `
        WITH vp AS (
            SELECT
                {{.QueryColumns}}
            FROM view_projects_users AS vp
            WHERE vp.user_id = $1
                {{ .QueryWhere }}
            ORDER BY {{.QueryInternalSort}}
            LIMIT {{.QueryLimit}}
        ) SELECT * FROM vp ORDER BY {{.QueryExternalSort}}
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
	queryValues.QueryInternalSort = "vp.serial_id DESC, vp.id DESC"
	queryValues.QueryExternalSort = sortQuery

	tokenDirection, id, serial, err := domain.GetPaginatorDirection(input.Paginator.NextToken, input.Paginator.PrevToken)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("vp", tokenDirection, id, serial, filterQuery, true)

	// render the template on query variable
	var tpl bytes.Buffer
	t := template.Must(template.New("query").Parse(queryTemplate))
	err = t.Execute(&tpl, queryValues)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.Projects.Select",
		"query", prettyPrint(
			query, input.UserID.String(),
		),
	)

	// execute the query
	rows, err := ref.db.Query(ctx, query, input.UserID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.Project
	for rows.Next() {
		var item domain.Project

		scanFields := ref.buildScanFields(&item, input.Fields)

		if err := rows.Scan(scanFields...); err != nil {
			return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
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
		slog.Warn("repository.Projects.Select", "what", "no projects found")
		return &domain.SelectProjectsOutput{
			Items:     make([]domain.Project, 0),
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
	ret := &domain.SelectProjectsOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "projects selected successfully")

	return ret, nil
}

func (ref *ProjectsRepository) LinkUsers(ctx context.Context, input *domain.LinkUsersToProjectInput) error {
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
	projectIDs := make([]string, len(input.UserIDs))
	usersIDs := make([]string, len(input.UserIDs))
	for i, userID := range input.UserIDs {
		projectIDs[i] = input.ProjectID.String()
		usersIDs[i] = userID.String()
	}

	query := `
        INSERT INTO projects_users (projects_id, users_id)
        SELECT * FROM UNNEST($1::uuid[], $2::uuid[])
        ON CONFLICT (projects_id, users_id)
        DO UPDATE SET updated_at = NOW();
    `

	cslog.Trace(ctx, "repository.Projects.LinkUsers", "query", prettyPrint(query, projectIDs, usersIDs))

	_, err := ref.db.Exec(ctx, query, projectIDs, usersIDs)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "projects linked successfully")

	return nil
}

func (ref *ProjectsRepository) UnlinkUsers(ctx context.Context, input *domain.UnlinkUsersFromProjectInput) error {
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

	// Prepare arrays for UNNEST
	userIDs := make([]string, len(input.UserIDs))
	for i, userID := range input.UserIDs {
		userIDs[i] = userID.String()
	}

	query := `
      -- Unlink users from projects
      DELETE FROM projects_users
      WHERE projects_id = $1 AND users_id IN (SELECT unnest($2::uuid[]));
    `

	cslog.Trace(ctx, "repository.Projects.UnlinkUsers", "query", prettyPrint(query, input.ProjectID.String(), userIDs))

	_, err := ref.db.Exec(ctx, query, input.ProjectID.String(), userIDs)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "projects unlinked successfully")

	return nil
}

// handlePgError maps PostgreSQL errors to domain-specific errors.
// Returns the appropriate domain error or the original error if no mapping exists.
func (ref *ProjectsRepository) handlePgError(err error, input any) error {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case "23505": // Unique violation
			if strings.Contains(pgErr.Message, "_pkey") {
				switch v := input.(type) {
				case *domain.InsertProjectInput:
					return &domain.ProjectAlreadyExistsError{ID: v.ID}
				case *domain.UpdateProjectInput:
					return &domain.ProjectAlreadyExistsError{ID: v.ID}
				case *domain.LinkUsersToProjectInput: //
					return &domain.ProjectAlreadyExistsError{ID: v.ProjectID, Message: "Project already exists"}
				case uuid.UUID:
					return &domain.ProjectAlreadyExistsError{ID: v}
				}
			}

			if strings.Contains(pgErr.Message, "projects_name") || strings.Contains(pgErr.Message, "name") {
				switch v := input.(type) {
				case *domain.InsertProjectInput:
					return &domain.ProjectAlreadyExistsError{Name: v.Name}
				case *domain.UpdateProjectInput:
					if v.Name != nil {
						return &domain.ProjectAlreadyExistsError{Name: *v.Name}
					}
				}
			}

			if strings.Contains(pgErr.Message, "_users_id_fkey") {
				switch v := input.(type) {
				case *domain.UpdateProjectInput:
					return &domain.UserNotFoundError{ID: v.ID}
				case uuid.UUID:
					return &domain.UserNotFoundError{ID: v}
				}
			}

			if strings.Contains(pgErr.Message, "_projects_id_fkey") {
				switch v := input.(type) {
				case *domain.UpdateProjectInput:
					return &domain.ProjectNotFoundError{ID: v.ID}
				case uuid.UUID:
					return &domain.ProjectNotFoundError{ID: v}
				}
			}
		case "23503": // Foreign key violation
			if strings.Contains(pgErr.Message, "_fkey") {
				switch v := input.(type) {
				case *domain.LinkUsersToProjectInput: // works too for model.Unlink
					return &domain.ProjectNotFoundError{ID: v.ProjectID, Message: "Project not found"}
				}
			}
		case "P0001": // Raised exception
			if strings.Contains(pgErr.Message, "updated") {
				switch v := input.(type) {
				case *domain.UpdateProjectInput:
					return &domain.SystemProjectError{ID: v.ID}
				case uuid.UUID:
					return &domain.SystemProjectError{ID: v}
				}
			}

			if strings.Contains(pgErr.Message, "deleted") {
				switch v := input.(type) {
				case *domain.DeleteProjectInput:
					return &domain.SystemProjectError{ID: v.ID}
				case uuid.UUID:
					return &domain.SystemProjectError{ID: v}
				}
			}
		case "22021": // invalid byte sequence for encoding
			return &domain.InvalidByteSequenceError{Message: pgErr.Message}
		case "08P01": // invalid message format
			return &domain.InvalidMessageFormatError{Message: pgErr.Message}
		case "42703": // undefined column
			return &domain.UndefinedColumnError{Message: pgErr.Message}
		case "42804": // datatype mismatch
			return &domain.DatatypeMismatchError{Message: pgErr.Message}
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return &domain.ProjectNotFoundError{}
	}

	return err
}

func (ref *ProjectsRepository) buildScanFields(item *domain.Project, requestedFields string) []any {
	scanFields := make([]any, 0)

	if requestedFields == "" {
		// All fields were requested
		return []any{
			&item.ID,
			&item.Name,
			&item.Description,
			&item.Disabled,
			&item.System,
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
		case "disabled":
			scanFields = append(scanFields, &item.Disabled)
		case "system":
			scanFields = append(scanFields, &item.System)
		case "created_at":
			scanFields = append(scanFields, &item.CreatedAt)
		case "updated_at":
			scanFields = append(scanFields, &item.UpdatedAt)

		default:
			slog.Warn("repository.Projects.buildScanFields", "what", "field not found", "field", field)
		}
	}

	// Always include ID and SerialID fields for pagination
	if !idFound {
		scanFields = append(scanFields, &item.ID)
	}

	scanFields = append(scanFields, &item.SerialID)
	return scanFields
}
