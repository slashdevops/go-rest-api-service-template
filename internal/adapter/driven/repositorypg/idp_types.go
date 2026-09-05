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

type IDPTypesRepositoryConfig struct {
	DB              *pgxpool.Pool
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
	MaxPingTimeout  time.Duration
	MaxQueryTimeout time.Duration
}

type IDPTypesRepository struct {
	db              *pgxpool.Pool
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
	maxPingTimeout  time.Duration
	maxQueryTimeout time.Duration
}

func NewIDPTypesRepository(conf IDPTypesRepositoryConfig) (*IDPTypesRepository, error) {
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

	ref := &IDPTypesRepository{
		db:              conf.DB,
		maxPingTimeout:  conf.MaxPingTimeout,
		maxQueryTimeout: conf.MaxQueryTimeout,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "IDPTypes",
			Action: "NewIDPTypesRepository",
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

func (ref *IDPTypesRepository) DriverName() string {
	return sql.Drivers()[0]
}

func (ref *IDPTypesRepository) PingContext(ctx context.Context) error {
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

func (ref *IDPTypesRepository) SelectByID(ctx context.Context, id uuid.UUID) (*domain.IDPTypes, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByID")
	defer cancel()
	defer span.End()

	if !domain.IsUUIDV7(id) {
		errorType := &domain.InvalidUserIDError{Message: "idp ID cannot be empty"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("idp.id", id.String()))

	query := `
        SELECT
            id,
            name,
            description,
            system,
            scopes,
            user_info_api_url,
            created_at,
            updated_at
        FROM idp_types
        WHERE id=$1;
    `

	cslog.Trace(ctx, "repository.IDPTypes.SelectByID", "query", prettyPrint(query, id))

	row := ref.db.QueryRow(ctx, query, id)

	var idp domain.IDPTypes
	err := row.Scan(
		&idp.ID,
		&idp.Name,
		&idp.Description,
		&idp.System,
		&idp.Scopes,
		&idp.UserInfoAPIURL,
		&idp.CreatedAt,
		&idp.UpdatedAt,
	)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "idp retrieved successfully", attribute.String("idp.id", id.String()))
	return &idp, nil
}

func (ref *IDPTypesRepository) SelectByName(ctx context.Context, name string) (*domain.IDPTypes, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByName")
	defer cancel()
	defer span.End()

	if name == "" {
		errorType := &domain.InvalidInputError{Message: "name is empty"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("idp.name", name))

	query := `
        SELECT
            id,
            name,
            description,
            system,
            scopes,
            user_info_api_url,
            created_at,
            updated_at
        FROM idp_types
        WHERE name=$1;
    `

	cslog.Trace(ctx, "repository.IDPTypes.SelectByName", "query", prettyPrint(query, name))

	row := ref.db.QueryRow(ctx, query, name)

	var idp domain.IDPTypes
	err := row.Scan(
		&idp.ID,
		&idp.Name,
		&idp.Description,
		&idp.System,
		&idp.Scopes,
		&idp.UserInfoAPIURL,
		&idp.CreatedAt,
		&idp.UpdatedAt,
	)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "idp retrieved successfully", attribute.String("idp.name", name))
	return &idp, nil
}

func (ref *IDPTypesRepository) Select(ctx context.Context, input *domain.SelectIDPTypesInput) (*domain.ListIDPTypesOutput, error) {
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
	sqlFieldsPrefix := "idpt."
	fieldsArray := []string{
		"id",
		"name",
		"description",
		"system",
		"scopes",
		"user_info_api_url",
		"created_at",
		"updated_at",
		"serial_id",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.IDPTypesFilterFields)
		filterQuery = fmt.Sprintf("WHERE (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "idpt.serial_id DESC, idpt.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.IDPTypesSortFields)
	}

	// query template
	queryTemplate := `
        WITH idpt AS (
            SELECT
                {{.QueryColumns}}
            FROM idp_types AS idpt
            {{ .QueryWhere }}
            ORDER BY {{.QueryInternalSort}}
            LIMIT {{.QueryLimit}}
        ) SELECT * FROM idpt ORDER BY {{.QueryExternalSort}}
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
	queryValues.QueryInternalSort = "idpt.serial_id DESC, idpt.id DESC"
	queryValues.QueryExternalSort = sortQuery

	tokenDirection, id, serial, err := domain.GetPaginatorDirection(input.Paginator.NextToken, input.Paginator.PrevToken)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("idpt", tokenDirection, id, serial, filterQuery, false)

	// render the template on query variable
	var tpl bytes.Buffer
	t := template.Must(template.New("query").Parse(queryTemplate))
	err = t.Execute(&tpl, queryValues)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.IDPTypes.Select", "query", prettyPrint(query))

	// execute the query
	rows, err := ref.db.Query(ctx, query)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.IDPTypes
	for rows.Next() {
		var item domain.IDPTypes

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
		return &domain.SelectIDPTypesOutput{
			Items:     make([]domain.IDPTypes, 0),
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

	ret := &domain.SelectIDPTypesOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "idps selected successfully")

	return ret, nil
}

// buildScanFields creates the scan targets for the result rows based on the requested fields.
func (ref *IDPTypesRepository) buildScanFields(item *domain.IDPTypes, requestedFields string) []any {
	scanFields := make([]any, 0)

	if requestedFields == "" {
		// All fields were requested
		return []any{
			&item.ID,
			&item.Name,
			&item.Description,
			&item.System,
			&item.Scopes,
			&item.UserInfoAPIURL,
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
		case "scopes":
			scanFields = append(scanFields, &item.Scopes)
		case "user_info_api_url":
			scanFields = append(scanFields, &item.UserInfoAPIURL)
		case "created_at":
			scanFields = append(scanFields, &item.CreatedAt)
		case "updated_at":
			scanFields = append(scanFields, &item.UpdatedAt)

		default:
			slog.Warn("repository.IDPTypes.buildScanFields", "what", "field not found", "field", field)
		}
	}

	// Always include ID and SerialID fields for pagination
	if !idFound {
		scanFields = append(scanFields, &item.ID)
	}

	scanFields = append(scanFields, &item.SerialID)
	return scanFields
}

// handlePgError handles PostgreSQL errors and returns appropriate domain-specific errors.
func (ref *IDPTypesRepository) handlePgError(err error, _ any) error {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case "22021": // Invalid byte sequence for encoding
			return &domain.InvalidByteSequenceError{Message: pgErr.Message}
		case "08P01": // Invalid message format
			return &domain.InvalidMessageFormatError{Message: pgErr.Message}
		case "42703": // Undefined column
			return &domain.UndefinedColumnError{Message: pgErr.Message}
		case "42804": // Datatype mismatch - operator class mismatch
			return &domain.DatatypeMismatchError{Message: pgErr.Message}
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return &domain.IDPTypesNotFoundError{Message: "IDP type not found"}
	}

	return err
}
