package repositorypg

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/mail"
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

type UsersRepositoryConfig struct {
	DB              *pgxpool.Pool
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
	MaxPingTimeout  time.Duration
	MaxQueryTimeout time.Duration
}

type UsersRepository struct {
	db              *pgxpool.Pool
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
	maxPingTimeout  time.Duration
	maxQueryTimeout time.Duration
}

func NewUsersRepository(conf UsersRepositoryConfig) (*UsersRepository, error) {
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

	ref := &UsersRepository{
		db:              conf.DB,
		maxPingTimeout:  conf.MaxPingTimeout,
		maxQueryTimeout: conf.MaxQueryTimeout,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Users",
			Action: "NewUsersRepository",
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

func (ref *UsersRepository) DriverName() string {
	return sql.Drivers()[0]
}

func (ref *UsersRepository) PingContext(ctx context.Context) error {
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

func (ref *UsersRepository) Insert(ctx context.Context, input *domain.InsertUserInput) error {
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

	span.SetAttributes(attribute.String("user.id", input.ID.String()))

	tx, txErr := ref.db.Begin(ctx)
	if txErr != nil {
		return o11y.RecordError(ctx, span, start, txErr, ref.metrics, attrs)
	}

	defer func() {
		if txErr != nil {
			if err := tx.Rollback(ctx); err != nil {
				e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
				slog.Error("repository.Users.Insert", "error", e)
			}
		} else {
			if err := tx.Commit(ctx); err != nil {
				e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
				slog.Error("repository.Users.Insert", "error", e)
			}
		}
	}()

	// insert the user
	query1 := `
        INSERT INTO users (id, first_name, last_name, email, password_hash, disabled, verified, local_account)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
    `

	slog.Debug("repository.Users.Insert", "query",
		prettyPrint(query1,
			input.ID,
			input.FirstName,
			input.LastName,
			input.Email,
			input.PasswordHash,
			input.Disabled,
			input.Verified,
			input.LocalAccount,
		))

	var disabled bool
	if input.Disabled != nil {
		disabled = *input.Disabled
	}

	_, txErr = tx.Exec(ctx, query1,
		input.ID,
		input.FirstName,
		input.LastName,
		input.Email,
		input.PasswordHash,
		disabled,
		input.Verified != nil && *input.Verified,
		input.LocalAccount != nil && *input.LocalAccount,
	)
	if txErr != nil {
		txErr = ref.handlePgError(txErr, input)
		return o11y.RecordError(ctx, span, start, txErr, ref.metrics, attrs)
	}

	// select from roles where default is true and link to the new user
	query2 := `
        WITH
            default_roles AS (
                SELECT id FROM roles WHERE auto_assign = true
            )

        INSERT INTO users_roles (users_id, roles_id)
        SELECT $1, id FROM default_roles
        ON CONFLICT (users_id, roles_id) DO NOTHING;
    `

	cslog.Trace(ctx, "repository.Users.Insert", "query", prettyPrint(query2, input.ID.String()))
	_, txErr = tx.Exec(ctx, query2, input.ID)
	if txErr != nil {
		return o11y.RecordError(ctx, span, start, txErr, ref.metrics, attrs)
	}

	slog.Debug("repository.Users.Insert", "user.id", input.ID)
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user inserted successfully", attribute.String("user.id", input.ID.String()))

	return nil
}

func (ref *UsersRepository) UpdateByID(ctx context.Context, input *domain.UpdateUserInput) error {
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

	span.SetAttributes(attribute.String("user.id", input.ID.String()))

	args := []any{input.ID}

	if input.FirstName != nil && *input.FirstName != "" {
		args = append(args, *input.FirstName)
	} else {
		args = append(args, nil)
	}

	if input.LastName != nil && *input.LastName != "" {
		args = append(args, *input.LastName)
	} else {
		args = append(args, nil)
	}

	if input.Email != nil && *input.Email != "" {
		args = append(args, *input.Email)
	} else {
		args = append(args, nil)
	}

	if input.PasswordHash != nil && *input.PasswordHash != "" {
		args = append(args, *input.PasswordHash)
	} else {
		args = append(args, nil)
	}

	if input.Disabled != nil {
		args = append(args, *input.Disabled)
	} else {
		args = append(args, nil)
	}

	if input.LocalAccount != nil {
		args = append(args, *input.LocalAccount)
	} else {
		args = append(args, nil)
	}

	if input.Verified != nil {
		args = append(args, *input.Verified)
	} else {
		args = append(args, nil)
	}

	query := `
        UPDATE users SET
            first_name    = COALESCE(NULLIF($2, first_name), first_name),
            last_name     = COALESCE(NULLIF($3, last_name), last_name),
            email         = COALESCE(NULLIF($4, email), email),
            password_hash = COALESCE(NULLIF($5, password_hash), password_hash),
            disabled      = COALESCE($6, disabled),
            local_account = COALESCE($7, local_account),
            verified      = COALESCE($8, verified),
            updated_at    = CURRENT_TIMESTAMP
        WHERE id = $1;
    `

	cslog.Trace(ctx, "repository.Users.UpdateByID", "query", prettyPrint(query, args...))

	result, err := ref.db.Exec(ctx, query, args...)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	if result.RowsAffected() == 0 {
		errorType := &domain.UserNotFoundError{ID: input.ID}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user updated successfully", attribute.String("user.id", input.ID.String()))

	return nil
}

func (ref *UsersRepository) DeleteByID(ctx context.Context, input *domain.DeleteUserInput) error {
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

	span.SetAttributes(attribute.String("user.id", input.ID.String()))

	query := `
        DELETE FROM users WHERE id = $1
    `

	cslog.Trace(ctx, "repository.Users.DeleteByID", "query", prettyPrint(query, input.ID.String()))

	result, err := ref.db.Exec(ctx, query, input.ID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if result.RowsAffected() == 0 {
		// grateful return user was deleted, security reason, but log and record error
		errorType := &domain.UserNotFoundError{ID: input.ID}
		e := o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
		if e != nil {
			slog.Error("repository.Users.DeleteByID", "error", e)
		}

		return nil
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user deleted successfully", attribute.String("user.id", input.ID.String()))

	return nil
}

func (ref *UsersRepository) SelectByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByID")
	defer cancel()
	defer span.End()

	if !domain.IsUUIDV7(id) {
		errorType := &domain.InvalidUserIDError{Message: "user ID cannot be empty"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.id", id.String()))

	query := `
        SELECT
            id,
            first_name,
            last_name,
            email,
            password_hash,
            disabled,
            verified,
            admin,
            local_account,
            created_at,
            updated_at
        FROM users
        WHERE id = $1;
    `

	cslog.Trace(ctx, "repository.Users.SelectByID", "query", prettyPrint(query, id.String()))

	row := ref.db.QueryRow(ctx, query, id)

	var item domain.User
	if err := row.Scan(
		&item.ID,
		&item.FirstName,
		&item.LastName,
		&item.Email,
		&item.PasswordHash,
		&item.Disabled,
		&item.Verified,
		&item.Admin,
		&item.LocalAccount,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errorType := &domain.UserNotFoundError{ID: id}
			return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
		}

		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user selected successfully", attribute.String("user.id", id.String()))
	return &item, nil
}

func (ref *UsersRepository) SelectByEmail(ctx context.Context, email string) (*domain.User, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByEmail")
	defer cancel()
	defer span.End()

	span.SetAttributes(attribute.String("user.email", email))

	if email == "" {
		errorType := &domain.InvalidEmailError{Email: email}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	if len(email) < domain.ValidUserEmailMinLength || len(email) > domain.ValidUserEmailMaxLength {
		errorType := &domain.InvalidEmailError{Email: email}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	_, err := mail.ParseAddress(email)
	if err != nil {
		errorType := &domain.InvalidEmailError{Email: email}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	query := `
        SELECT
            id,
            first_name,
            last_name,
            email,
            password_hash,
            disabled,
            verified,
            local_account,
            created_at,
            updated_at
        FROM users
        WHERE email = $1;
    `

	cslog.Trace(ctx, "repository.Users.SelectByEmail", "query", prettyPrint(query, email))

	row := ref.db.QueryRow(ctx, query, email)

	var item domain.User
	if err := row.Scan(
		&item.ID,
		&item.FirstName,
		&item.LastName,
		&item.Email,
		&item.PasswordHash,
		&item.Disabled,
		&item.Verified,
		&item.LocalAccount,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.UserNotFoundError{ID: uuid.Nil(), Message: fmt.Sprintf("User not found with email: %s", email)}
		}

		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user selected successfully", attribute.String("user.email", email))

	return &item, nil
}

func (ref *UsersRepository) SelectByRoleID(ctx context.Context, roleID uuid.UUID, input *domain.ListUsersInput) (*domain.SelectUsersOutput, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByRoleID")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if roleID == uuid.Nil() {
		invalidErr := &domain.InvalidRoleIDError{Message: "invalid role ID"}
		return nil, o11y.RecordError(ctx, span, start, invalidErr, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// if no fields are provided, select all fields
	sqlFieldsPrefix := "usrs."
	fieldsArray := []string{
		"id",
		"first_name",
		"last_name",
		"email",
		"password_hash",
		"disabled",
		"verified",
		"local_account",
		"created_at",
		"updated_at",
		"serial_id",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.UsersFilterFields)
		filterQuery = fmt.Sprintf("AND (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "usrs.serial_id DESC, usrs.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.UsersSortFields)
	}

	// query template
	queryTemplate := `
        WITH usrs AS (
            SELECT
                {{.QueryColumns}}
            FROM users AS usrs
                -- roles
                JOIN users_roles AS ur ON usrs.id = ur.users_id
                JOIN roles AS rls ON ur.roles_id = rls.id
            WHERE rls.id = $1
            {{ .QueryWhere }}
            ORDER BY {{.QueryInternalSort}}
            LIMIT {{.QueryLimit}}
        ) SELECT * FROM usrs ORDER BY {{.QueryExternalSort}}
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
	queryValues.QueryInternalSort = "usrs.serial_id DESC, usrs.id DESC"
	queryValues.QueryExternalSort = sortQuery

	tokenDirection, id, serial, err := domain.GetPaginatorDirection(input.Paginator.NextToken, input.Paginator.PrevToken)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("usrs", tokenDirection, id, serial, filterQuery, true)

	// render the template on query variable
	var tpl bytes.Buffer
	t := template.Must(template.New("query").Parse(queryTemplate))
	err = t.Execute(&tpl, queryValues)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.Users.SelectByRoleID", "query", prettyPrint(query, roleID.String()))

	// execute the query
	rows, err := ref.db.Query(ctx, query, roleID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.User
	for rows.Next() {
		var item domain.User

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
		return &domain.SelectUsersOutput{
			Items:     make([]domain.User, 0),
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

	ret := &domain.SelectUsersOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "users selected successfully")

	return ret, nil
}

func (ref *UsersRepository) SelectByProjectID(ctx context.Context, projectID uuid.UUID, input *domain.SelectUsersInput) (*domain.SelectUsersOutput, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByProjectID")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if projectID == uuid.Nil() {
		invalidErr := &domain.InvalidProjectIDError{Message: "invalid project ID"}
		return nil, o11y.RecordError(ctx, span, start, invalidErr, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// if no fields are provided, select all fields
	sqlFieldsPrefix := "usrs."
	fieldsArray := []string{
		"id",
		"first_name",
		"last_name",
		"email",
		"password_hash",
		"disabled",
		"verified",
		"local_account",
		"created_at",
		"updated_at",
		"serial_id",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.UsersFilterFields)
		filterQuery = fmt.Sprintf("AND (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "usrs.serial_id DESC, usrs.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.UsersSortFields)
	}

	// query template
	queryTemplate := `
        WITH usrs AS (
            SELECT
                {{.QueryColumns}}
            FROM users AS usrs
            JOIN projects_users AS pu ON usrs.id = pu.users_id
            WHERE pu.projects_id = $1
            {{ .QueryWhere }}
            ORDER BY {{.QueryInternalSort}}
            LIMIT {{.QueryLimit}}
        ) SELECT * FROM usrs ORDER BY {{.QueryExternalSort}}
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
	queryValues.QueryInternalSort = "usrs.serial_id DESC, usrs.id DESC"
	queryValues.QueryExternalSort = sortQuery

	tokenDirection, id, serial, err := domain.GetPaginatorDirection(input.Paginator.NextToken, input.Paginator.PrevToken)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("usrs", tokenDirection, id, serial, filterQuery, true)

	// render the template on query variable
	var tpl bytes.Buffer
	t := template.Must(template.New("query").Parse(queryTemplate))
	err = t.Execute(&tpl, queryValues)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.Users.SelectByProjectID", "query", prettyPrint(query, projectID.String()))

	// execute the query
	rows, err := ref.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.User
	for rows.Next() {
		var item domain.User

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
		return &domain.SelectUsersOutput{
			Items:     make([]domain.User, 0),
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

	ret := &domain.SelectUsersOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "users selected successfully")

	return ret, nil
}

func (ref *UsersRepository) Select(ctx context.Context, input *domain.SelectUsersInput) (*domain.SelectUsersOutput, error) {
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
	sqlFieldsPrefix := "usrs."
	fieldsArray := []string{
		"id",
		"first_name",
		"last_name",
		"email",
		"password_hash",
		"disabled",
		"verified",
		"local_account",
		"created_at",
		"updated_at",
		"serial_id",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.UsersFilterFields)
		filterQuery = fmt.Sprintf("WHERE (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "usrs.serial_id DESC, usrs.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.UsersSortFields)
	}

	// query template
	queryTemplate := `
        WITH usrs AS (
            SELECT
                {{.QueryColumns}}
            FROM users AS usrs
            {{ .QueryWhere }}
            ORDER BY {{.QueryInternalSort}}
            LIMIT {{.QueryLimit}}
        ) SELECT * FROM usrs ORDER BY {{.QueryExternalSort}}
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
	queryValues.QueryInternalSort = "usrs.serial_id DESC, usrs.id DESC"
	queryValues.QueryExternalSort = sortQuery

	tokenDirection, id, serial, err := domain.GetPaginatorDirection(input.Paginator.NextToken, input.Paginator.PrevToken)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("usrs", tokenDirection, id, serial, filterQuery, false)

	// render the template on query variable
	var tpl bytes.Buffer
	t := template.Must(template.New("query").Parse(queryTemplate))
	err = t.Execute(&tpl, queryValues)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.Users.Select", "query", prettyPrint(query))

	// execute the query
	rows, err := ref.db.Query(ctx, query)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.User
	for rows.Next() {
		var item domain.User

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
		return &domain.SelectUsersOutput{
			Items:     make([]domain.User, 0),
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

	ret := &domain.SelectUsersOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "users selected successfully")

	return ret, nil
}

func (ref *UsersRepository) LinkRoles(ctx context.Context, input *domain.LinkRolesToUserInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "LinkRoles")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.id", input.UserID.String()))

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Prepare arrays for UNNEST
	userIDs := make([]string, len(input.RoleIDs))
	roleIDs := make([]string, len(input.RoleIDs))
	for i, roleID := range input.RoleIDs {
		userIDs[i] = input.UserID.String()
		roleIDs[i] = roleID.String() // Assuming roleID is a UUID or similar that needs String()
	}

	query := `
        -- insert the new roles
        INSERT INTO users_roles (users_id, roles_id)
        SELECT * FROM UNNEST($1::uuid[], $2::uuid[]) -- Use appropriate type casting for your UUIDs
        ON CONFLICT (users_id, roles_id)
        DO UPDATE SET updated_at = NOW();
    `

	cslog.Trace(ctx, "repository.Users.LinkRoles", "query", prettyPrint(query, userIDs, roleIDs))

	_, err := ref.db.Exec(ctx, query, userIDs, roleIDs)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "roles linked successfully")

	return nil
}

func (ref *UsersRepository) UnlinkRoles(ctx context.Context, input *domain.UnlinkRolesFromUsersInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "UnlinkRoles")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.id", input.UserID.String()))

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Convert input.RoleIDs to a slice of strings (or UUIDs if that's their underlying type)
	// to pass as a single parameter to the IN clause.
	roleIDs := make([]string, len(input.RoleIDs))
	for i, roleID := range input.RoleIDs {
		roleIDs[i] = roleID.String()
	}

	queryString := `
        DELETE FROM users_roles
        WHERE users_id = $1 AND roles_id IN (SELECT unnest($2::uuid[]));
    `

	cslog.Trace(ctx, "repository.Users.UnlinkRoles", "query", prettyPrint(queryString, input.UserID.String(), roleIDs))

	// Pass input.UserID and the slice of roleIDs as parameters
	_, err := ref.db.Exec(ctx, queryString, input.UserID.String(), roleIDs)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "roles unlinked successfully")

	return nil
}

func (ref *UsersRepository) LinkProjects(ctx context.Context, input *domain.LinkProjectsToUserInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "LinkProjects")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.id", input.UserID.String()))

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Prepare arrays for UNNEST
	userIDs := make([]string, len(input.ProjectIDs))
	projectIDs := make([]string, len(input.ProjectIDs))
	for i, projectID := range input.ProjectIDs {
		userIDs[i] = input.UserID.String()
		projectIDs[i] = projectID.String()
	}

	query := `
        -- insert the new projects
        INSERT INTO projects_users (users_id, projects_id)
        SELECT * FROM UNNEST($1::uuid[], $2::uuid[]) -- Use appropriate type casting for your UUIDs
        ON CONFLICT (users_id, projects_id)
        DO UPDATE SET updated_at = NOW();
    `

	cslog.Trace(ctx, "repository.Users.LinkProjects", "query", prettyPrint(query, userIDs, projectIDs))

	_, err := ref.db.Exec(ctx, query, userIDs, projectIDs)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "projects linked successfully")

	return nil
}

func (ref *UsersRepository) UnlinkProjects(ctx context.Context, input *domain.UnlinkProjectsFromUserInput) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "UnlinkProjects")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.id", input.UserID.String()))

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Prepare arrays for UNNEST
	projectIDs := make([]string, len(input.ProjectIDs))
	for i, projectID := range input.ProjectIDs {
		projectIDs[i] = projectID.String()
	}

	query := `
        -- delete the projects_users
        DELETE FROM projects_users
        WHERE users_id = $1 AND projects_id IN (SELECT unnest($2::uuid[]));
    `

	cslog.Trace(ctx, "repository.Users.UnlinkProjects", "query", prettyPrint(query, input.UserID.String(), projectIDs))

	_, err := ref.db.Exec(ctx, query, input.UserID.String(), projectIDs)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "projects unlinked successfully")

	return nil
}

func (ref *UsersRepository) SelectAuthz(ctx context.Context, userID uuid.UUID) (*domain.SelectAuthzOutput, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectAuthz")
	defer cancel()
	defer span.End()

	span.SetAttributes(attribute.String("user.id", userID.String()))

	if userID == uuid.Nil() {
		errorType := &domain.InvalidUserIDError{Message: "user id is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	query := `
        WITH raw_access_data AS (
        -- 1. Gather all raw data (User -> Role -> Policy -> Resource -> Action)
        SELECT
            u.id AS user_id,
            r.id AS role_id,
            pol.id AS policy_id,
            pol.allowed_resource,
            pol.allowed_action
        FROM
            users AS u
            JOIN users_roles AS ur ON u.id = ur.users_id
            JOIN roles AS r ON ur.roles_id = r.id
            JOIN roles_policies AS rpol ON r.id = rpol.roles_id
            JOIN policies AS pol ON rpol.policies_id = pol.id
        WHERE
            u.id = $1
        ),
        grouped_actions AS (
        -- 2. Flatten actions by resource (Ignoring which policy/role provided them)
        -- This ensures "/users" appears only once per user, with an array of actions.
        SELECT
            user_id,
            allowed_resource,
            array_agg(DISTINCT allowed_action) AS actions
        FROM
            raw_access_data
        GROUP BY
            user_id,
            allowed_resource
        )
        SELECT
        -- 3. Aggregate Roles and Policies from the raw data
        (
            SELECT
            array_agg(
                DISTINCT COALESCE(
                role_id :: varchar, '00000000-0000-0000-0000-000000000000'
                )
            )
            FROM
            raw_access_data
        ) AS roles,
        (
            SELECT
            array_agg(
                DISTINCT COALESCE(
                policy_id :: varchar, '00000000-0000-0000-0000-000000000000'
                )
            )
            FROM
            raw_access_data
        ) AS policies,
        -- 4. Build the JSON object from the clean grouped_actions
        json_build_object(
            'permissions',
            json_build_object(
            'users',
            json_build_object(
                user_id,
                json_object_agg(allowed_resource, actions)
            )
            )
        ) AS permissions
        FROM
        grouped_actions
        GROUP BY
        user_id;
    `

	cslog.Trace(ctx, "repository.Users.SelectAuthz", "query", prettyPrint(query, userID.String()))

	rows, err := ref.db.Query(ctx, query, userID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}
	defer rows.Close()

	var roles []string
	var policies []string
	var permissionsMap map[string]any

	for rows.Next() {
		if err := rows.Scan(&roles, &policies, &permissionsMap); err != nil {
			return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(rows.Err(), nil), ref.metrics, attrs)
	}

	out := &domain.SelectAuthzOutput{
		Roles:       make([]string, 0, len(roles)),
		Policies:    make([]string, 0, len(policies)),
		Permissions: permissionsMap,
	}

	out.Roles = append(out.Roles, roles...)
	out.Policies = append(out.Policies, policies...)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user roles selected successfully")

	return out, nil
}

// handlePgError maps PostgreSQL errors to domain-specific errors.
// Returns the appropriate domain error or the original error if no mapping exists.
func (ref *UsersRepository) handlePgError(err error, input any) error {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case "23505": // Unique violation
			if strings.Contains(pgErr.Message, "_pkey") {
				switch v := input.(type) {
				case *domain.InsertUserInput:
					return &domain.UserAlreadyExistsError{ID: v.ID}
				case *domain.UpdateUserInput:
					return &domain.UserAlreadyExistsError{ID: v.ID}
				case *domain.LinkProjectsToUserInput: // works too for domain.UnlinkProjectsFromUserInput because is an alias
					return &domain.UserAlreadyExistsError{ID: v.UserID}
				case uuid.UUID:
					return &domain.UserAlreadyExistsError{ID: v}
				}
			}

			if strings.Contains(pgErr.Message, "_email") {
				switch v := input.(type) {
				case *domain.InsertUserInput:
					return &domain.UserAlreadyExistsError{Email: v.Email}
				case *domain.UpdateUserInput:
					if v.Email != nil {
						return &domain.UserAlreadyExistsError{Email: *v.Email}
					}
				}
			}
		case "23503": // Foreign key violation
			if strings.Contains(pgErr.Message, "_fkey") {
				switch v := input.(type) {
				case *domain.LinkProjectsToUserInput: // works too for domain.UnlinkProjectsFromUserInput because is an alias
					return &domain.UserNotFoundError{ID: v.UserID, Message: "User not found"}
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

// buildScanFields creates the scan targets for the result rows based on the requested fields.
func (ref *UsersRepository) buildScanFields(item *domain.User, requestedFields string) []any {
	scanFields := make([]any, 0)

	if requestedFields == "" {
		// All fields were requested
		return []any{
			&item.ID,
			&item.FirstName,
			&item.LastName,
			&item.Email,
			&item.PasswordHash,
			&item.Disabled,
			&item.Verified,
			&item.LocalAccount,
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
		case "first_name":
			scanFields = append(scanFields, &item.FirstName)
		case "last_name":
			scanFields = append(scanFields, &item.LastName)
		case "email":
			scanFields = append(scanFields, &item.Email)
		case "password_hash":
			scanFields = append(scanFields, &item.PasswordHash)
		case "disabled":
			scanFields = append(scanFields, &item.Disabled)
		case "verified":
			scanFields = append(scanFields, &item.Verified)
		case "local_account":
			scanFields = append(scanFields, &item.LocalAccount)
		case "created_at":
			scanFields = append(scanFields, &item.CreatedAt)
		case "updated_at":
			scanFields = append(scanFields, &item.UpdatedAt)

		default:
			slog.Warn("repository.Users.buildScanFields", "what", "field not found", "field", field)
		}
	}

	// Always include ID and SerialID fields for pagination
	if !idFound {
		scanFields = append(scanFields, &item.ID)
	}

	scanFields = append(scanFields, &item.SerialID)
	return scanFields
}
