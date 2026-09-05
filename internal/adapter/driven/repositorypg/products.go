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
	"go.opentelemetry.io/otel/trace"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
	"github.com/slashdevops/go-rest-api-service-template/pkg/cslog"
)

// projectMembershipPredicate is the tenant check every product query carries.
//
// OPA authorises the PATH -- a policy granting `/projects/*/products` matches
// every project -- so without this a caller with that policy could read and
// write the products of a project they were never added to. The check belongs
// in the same statement as the row it guards: done as a separate SELECT it is a
// TOCTOU window, and done in the use-case it would have to be repeated at six
// call sites and could be forgotten at the seventh.
//
// $1 is the project id and $2 the caller. Admins bypass membership, which is
// the same rule projects_users enforces for projects themselves.
const projectMembershipPredicate = `
        (
            (SELECT admin FROM users WHERE id = $2) = TRUE
            OR EXISTS (
                SELECT 1 FROM projects_users
                WHERE projects_id = $1 AND users_id = $2
            )
        )`

type ProductsRepositoryConfig struct {
	DB              *pgxpool.Pool
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
	MaxPingTimeout  time.Duration
	MaxQueryTimeout time.Duration
}

// ProductsRepository is a PostgreSQL store.
type ProductsRepository struct {
	db              *pgxpool.Pool
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
	maxPingTimeout  time.Duration
	maxQueryTimeout time.Duration
}

// NewProductsRepository creates a new ProductsRepository.
func NewProductsRepository(conf ProductsRepositoryConfig) (*ProductsRepository, error) {
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

	ref := &ProductsRepository{
		db:              conf.DB,
		maxPingTimeout:  conf.MaxPingTimeout,
		maxQueryTimeout: conf.MaxQueryTimeout,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Products",
			Action: "NewProductsRepository",
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
func (ref *ProductsRepository) DriverName() string {
	return sql.Drivers()[0]
}

// PingContext verifies a connection to the repository is still alive, establishing a connection if necessary.
func (ref *ProductsRepository) PingContext(ctx context.Context) error {
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

// InsertByProjectID inserts a product into a project the caller belongs to.
func (ref *ProductsRepository) InsertByProjectID(ctx context.Context, input *domain.InsertProductInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "InsertByProjectID")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(
		attribute.String("product.id", input.ID.String()),
		attribute.String("product.project_id", input.ProjectID.String()),
	)

	// INSERT ... SELECT rather than VALUES: the row is written only if the
	// membership predicate holds, in one statement. A zero row count then means
	// "not a member", which is reported as not-found rather than forbidden so
	// the endpoint does not confirm the project exists to someone outside it.
	query := `
        INSERT INTO products (id, projects_id, name, description)
        SELECT $3, $1, $4, $5
        WHERE ` + projectMembershipPredicate + `;
    `

	cslog.Trace(ctx, "repository.Products.InsertByProjectID",
		"query", prettyPrint(query,
			input.ProjectID.String(),
			input.UserID.String(),
			input.ID.String(),
			input.Name,
			input.Description,
		),
	)

	ct, err := ref.db.Exec(ctx, query,
		input.ProjectID,
		input.UserID,
		input.ID,
		input.Name,
		input.Description,
	)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	if ct.RowsAffected() == 0 {
		notFound := &domain.ProjectNotFoundError{
			ID:      input.ProjectID,
			Message: "project not found, or you do not have access to it",
		}

		return o11y.RecordError(ctx, span, start, notFound, ref.metrics, attrs)
	}

	slog.Debug("repository.Products.InsertByProjectID", "product.id", input.ID)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "product inserted successfully",
		attribute.String("product.id", input.ID.String()))

	return nil
}

// UpdateByIDByProjectID updates a product in a project the caller belongs to.
func (ref *ProductsRepository) UpdateByIDByProjectID(ctx context.Context, input *domain.UpdateProductInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "UpdateByIDByProjectID")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(
		attribute.String("product.id", input.ID.String()),
		attribute.String("product.project_id", input.ProjectID.String()),
	)

	var name any
	if input.Name != nil && *input.Name != "" {
		name = *input.Name
	}

	var description any
	if input.Description != nil && *input.Description != "" {
		description = *input.Description
	}

	query := `
        UPDATE products SET
            name        = COALESCE($4, name),
            description = COALESCE($5, description),
            updated_at  = CURRENT_TIMESTAMP
        WHERE id = $3 AND projects_id = $1
        AND ` + projectMembershipPredicate + `;
    `

	args := []any{input.ProjectID, input.UserID, input.ID, name, description}

	cslog.Trace(ctx, "repository.Products.UpdateByIDByProjectID", "query", prettyPrint(query, args...))

	ct, err := ref.db.Exec(ctx, query, args...)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	if ct.RowsAffected() == 0 {
		notFound := &domain.ProductNotFoundError{ID: input.ID, Message: "product not found in this project, or you do not have access to it"}
		return o11y.RecordError(ctx, span, start, notFound, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "product updated successfully",
		attribute.String("product.id", input.ID.String()))

	return nil
}

// DeleteByIDByProjectID removes a product from a project the caller belongs to.
func (ref *ProductsRepository) DeleteByIDByProjectID(ctx context.Context, input *domain.DeleteProductInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "DeleteByIDByProjectID")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(
		attribute.String("product.id", input.ID.String()),
		attribute.String("product.project_id", input.ProjectID.String()),
	)

	query := `
        DELETE FROM products
        WHERE id = $3 AND projects_id = $1
        AND ` + projectMembershipPredicate + `;
    `

	cslog.Trace(ctx, "repository.Products.DeleteByIDByProjectID",
		"query", prettyPrint(query, input.ProjectID.String(), input.UserID.String(), input.ID.String()))

	ct, err := ref.db.Exec(ctx, query, input.ProjectID, input.UserID, input.ID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	// A delete that matched nothing is reported, not swallowed. The use-case
	// decrements the resource counter on success, and a silent no-op here would
	// hand back a slot the row still occupies.
	if ct.RowsAffected() == 0 {
		notFound := &domain.ProductNotFoundError{ID: input.ID, Message: "product not found in this project, or you do not have access to it"}
		return o11y.RecordError(ctx, span, start, notFound, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "product deleted successfully",
		attribute.String("product.id", input.ID.String()))

	return nil
}

// SelectByIDByProjectID returns the product with the specified ID and project ID.
func (ref *ProductsRepository) SelectByIDByProjectID(ctx context.Context, id, projectID, userID uuid.UUID) (*domain.Product, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByIDByProjectID")
	defer cancel()
	defer span.End()

	if !domain.IsUUIDV7(id) {
		valueError := &domain.InvalidProductError{Message: "Product ID cannot be nil"}
		return nil, o11y.RecordError(ctx, span, start, valueError, ref.metrics, attrs)
	}

	if projectID == uuid.Nil() {
		valueError := &domain.InvalidProjectIDError{Message: "Project ID cannot be nil"}
		return nil, o11y.RecordError(ctx, span, start, valueError, ref.metrics, attrs)
	}

	if userID == uuid.Nil() {
		valueError := &domain.InvalidUserIDError{Message: "User ID cannot be nil"}
		return nil, o11y.RecordError(ctx, span, start, valueError, ref.metrics, attrs)
	}

	query := `
        SELECT
            prd.id,
            prd.name,
            prd.description,
            prd.created_at,
            prd.updated_at,
            ARRAY[COALESCE(prjs.id::varchar, '00000000-0000-0000-0000-000000000000'), COALESCE(prjs.name::varchar, '')] AS project
        FROM products AS prd
        LEFT JOIN projects AS prjs ON prd.projects_id = prjs.id
        WHERE prd.id = $3 AND prd.projects_id = $1
        AND ` + projectMembershipPredicate + `
        GROUP BY prd.id, prjs.id;
    `

	cslog.Trace(ctx, "repository.Products.SelectByIDByProjectID", "query", prettyPrint(query))

	row := ref.db.QueryRow(ctx, query, projectID, userID, id)

	var element domain.Product
	var project []string

	if err := row.Scan(
		&element.ID,
		&element.Name,
		&element.Description,
		&element.CreatedAt,
		&element.UpdatedAt,
		&project,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			notFoundError := &domain.ProductNotFoundError{ID: id, Message: "product not found for the given ID and project ID"}
			return nil, o11y.RecordError(ctx, span, start, notFoundError, ref.metrics, attrs)
		}

		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	var err error
	element.Project.ID, err = uuid.Parse(project[0])
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}
	element.Project.Name = project[1]

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "product selected successfully",
		attribute.String("product.id", id.String()))

	return &element, nil
}

// SelectByProjectID returns a page of products belonging to one project.
func (ref *ProductsRepository) SelectByProjectID(ctx context.Context, projectID, userID uuid.UUID, input *domain.SelectProductsInput) (*domain.SelectProductsOutput, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByProjectID")
	defer cancel()
	defer span.End()

	if projectID == uuid.Nil() {
		valueError := &domain.InvalidProjectIDError{Message: "Project ID cannot be nil"}
		return nil, o11y.RecordError(ctx, span, start, valueError, ref.metrics, attrs)
	}

	if userID == uuid.Nil() {
		valueError := &domain.InvalidUserIDError{Message: "User ID cannot be nil"}
		return nil, o11y.RecordError(ctx, span, start, valueError, ref.metrics, attrs)
	}

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	sqlFieldsPrefix := "prd."
	fieldsArray := []string{
		"id",
		"name",
		"description",
		"created_at",
		"updated_at",
		"serial_id",
		"ARRAY[COALESCE(prjs.id::varchar, '00000000-0000-0000-0000-000000000000'), COALESCE(prjs.name::varchar, '')] AS project",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.ProductsFilterFields)
		filterQuery = fmt.Sprintf("AND (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "prd.serial_id DESC, prd.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.ProductsSortFields)
	}

	queryTemplate := `
        WITH prd AS (
            SELECT
                {{.QueryColumns}}
            FROM products AS prd
                LEFT JOIN projects AS prjs ON prd.projects_id = prjs.id
            WHERE prd.projects_id = $1
            AND ` + projectMembershipPredicate + `
            {{ .QueryWhere }}
            GROUP BY prd.id, prjs.id
            ORDER BY {{.QueryInternalSort}}
            LIMIT {{.QueryLimit}}
        ) SELECT * FROM prd ORDER BY {{.QueryExternalSort}}
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
	queryValues.QueryLimit = input.Paginator.Limit + 1 // Fetch one extra item
	queryValues.QueryInternalSort = "prd.serial_id DESC, prd.id DESC"
	queryValues.QueryExternalSort = sortQuery

	tokenDirection, id, serial, err := domain.GetPaginatorDirection(input.Paginator.NextToken, input.Paginator.PrevToken)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("prd", tokenDirection, id, serial, filterQuery, true)

	var tpl bytes.Buffer
	t := template.Must(template.New("query").Parse(queryTemplate))

	if err := t.Execute(&tpl, queryValues); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.Products.SelectByProjectID", "query", prettyPrint(query))

	rows, err := ref.db.Query(ctx, query, projectID, userID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}
	defer rows.Close()

	return ref.collectPage(ctx, span, start, attrs, rows, input, tokenDirection, "SelectByProjectID")
}

// Select returns a page of products across every project.
//
// There is no membership predicate here on purpose: this backs `GET /products`,
// which is authorised as its own resource. A deployment that does not want it
// reachable removes the policy, not the endpoint.
func (ref *ProductsRepository) Select(ctx context.Context, input *domain.SelectProductsInput) (*domain.SelectProductsOutput, error) {
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

	sqlFieldsPrefix := "prd."
	fieldsArray := []string{
		"id",
		"name",
		"description",
		"created_at",
		"updated_at",
		"serial_id",
		"ARRAY[COALESCE(prjs.id::varchar, '00000000-0000-0000-0000-000000000000'), COALESCE(prjs.name::varchar, '')] AS project",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.ProductsFilterFields)
		filterQuery = fmt.Sprintf("AND (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "prd.serial_id DESC, prd.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.ProductsSortFields)
	}

	// `WHERE 1=1` so buildPaginationCriteria's `AND ...` has something to
	// attach to, the same shape the project-scoped query gets from its
	// membership predicate.
	queryTemplate := `
        WITH prd AS (
            SELECT
                {{.QueryColumns}}
            FROM products AS prd
                LEFT JOIN projects AS prjs ON prd.projects_id = prjs.id
            WHERE 1 = 1
            {{ .QueryWhere }}
            GROUP BY prd.id, prjs.id
            ORDER BY {{.QueryInternalSort}}
            LIMIT {{.QueryLimit}}
        ) SELECT * FROM prd ORDER BY {{.QueryExternalSort}}
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
	queryValues.QueryLimit = input.Paginator.Limit + 1 // Fetch one extra item
	queryValues.QueryInternalSort = "prd.serial_id DESC, prd.id DESC"
	queryValues.QueryExternalSort = sortQuery

	tokenDirection, id, serial, err := domain.GetPaginatorDirection(input.Paginator.NextToken, input.Paginator.PrevToken)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("prd", tokenDirection, id, serial, filterQuery, true)

	var tpl bytes.Buffer
	t := template.Must(template.New("query").Parse(queryTemplate))

	if err := t.Execute(&tpl, queryValues); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.Products.Select", "query", prettyPrint(query))

	rows, err := ref.db.Query(ctx, query)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}
	defer rows.Close()

	return ref.collectPage(ctx, span, start, attrs, rows, input, tokenDirection, "Select")
}

// collectPage scans a result set into a page and computes its tokens.
//
// Both list queries produce the same columns and the same pagination shape, so
// this is shared rather than written twice -- the two copies in the layered
// implementation had already drifted, and only one of them handled the
// "fetched one extra" boundary.
func (ref *ProductsRepository) collectPage(
	ctx context.Context,
	span trace.Span,
	start time.Time,
	attrs []attribute.KeyValue,
	rows pgx.Rows,
	input *domain.SelectProductsInput,
	tokenDirection domain.TokenDirection,
	action string,
) (*domain.SelectProductsOutput, error) {
	var fetchedItems []domain.Product

	for rows.Next() {
		var item domain.Product
		var project []string

		scanFields := ref.buildScanFields(&item, &project, input.Fields)

		if err := rows.Scan(scanFields...); err != nil {
			return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
		}

		if len(project) > 0 {
			id, err := uuid.Parse(project[0])
			if err != nil {
				return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			}

			item.Project.ID = id
			item.Project.Name = project[1]
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
		slog.Warn("repository.Products."+action, "what", "no products found")

		return &domain.SelectProductsOutput{
			Items:     make([]domain.Product, 0),
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
	}

	// Calculate min/max serial_id from the whole page so the tokens are right
	// regardless of the external sort order (e.g. "name ASC").
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
		maxSerialItem.ID, // Use item with MAX serial for prev token
		maxSerialItem.SerialID,
		minSerialItem.ID, // Use item with MIN serial for next token
		minSerialItem.SerialID,
		tokenDirection,
		repoFoundMoreForNextQuery,
		repoFoundMoreForPrevQuery,
	)

	ret := &domain.SelectProductsOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "products selected successfully")

	return ret, nil
}

func (ref *ProductsRepository) buildScanFields(item *domain.Product, project *[]string, requestedFields string) []any {
	if requestedFields == "" {
		// All fields were requested
		return []any{
			&item.ID,
			&item.Name,
			&item.Description,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.SerialID,
			project,
		}
	}

	scanFields := make([]any, 0)

	var idFound bool

	for field := range strings.SplitSeq(requestedFields, ",") {
		field = strings.TrimSpace(field)

		switch field {
		case "id":
			scanFields = append(scanFields, &item.ID)
			idFound = true
		case "name":
			scanFields = append(scanFields, &item.Name)
		case "description":
			scanFields = append(scanFields, &item.Description)
		case "created_at":
			scanFields = append(scanFields, &item.CreatedAt)
		case "updated_at":
			scanFields = append(scanFields, &item.UpdatedAt)
		case "project":
			scanFields = append(scanFields, project)
		default:
			slog.Warn("repository.Products.buildScanFields", "what", "field not found", "field", field)
		}
	}

	// always select id and serial_id for pagination
	if !idFound {
		scanFields = append(scanFields, &item.ID)
	}

	scanFields = append(scanFields, &item.SerialID)

	return scanFields
}

// handlePgError maps PostgreSQL errors to domain-specific errors.
// Returns the appropriate domain error or the original error if no mapping exists.
func (ref *ProductsRepository) handlePgError(err error, input any) error {
	_ = input // currently unused, but kept for future use if needed

	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case "23505": // Unique violation
			return &domain.ProductAlreadyExistsError{Message: pgErr.Message}
		case "23503": // Foreign key violation
			return &domain.ProjectNotFoundError{Message: pgErr.Message}
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

	if errors.Is(err, pgx.ErrNoRows) {
		return &domain.ProductNotFoundError{}
	}

	return err
}
