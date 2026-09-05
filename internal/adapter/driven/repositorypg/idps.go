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

type IDPsRepositoryConfig struct {
	DB              *pgxpool.Pool
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
	MaxPingTimeout  time.Duration
	MaxQueryTimeout time.Duration
}

type IDPsRepository struct {
	db              *pgxpool.Pool
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
	maxPingTimeout  time.Duration
	maxQueryTimeout time.Duration
}

func NewIDPsRepository(conf IDPsRepositoryConfig) (*IDPsRepository, error) {
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

	ref := &IDPsRepository{
		db:              conf.DB,
		maxPingTimeout:  conf.MaxPingTimeout,
		maxQueryTimeout: conf.MaxQueryTimeout,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "IDPs",
			Action: "NewIDPsRepository",
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

func (ref *IDPsRepository) DriverName() string {
	return sql.Drivers()[0]
}

func (ref *IDPsRepository) PingContext(ctx context.Context) error {
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

func (ref *IDPsRepository) Insert(ctx context.Context, input *domain.InsertIDPInput) error {
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

	span.SetAttributes(attribute.String("input.id", input.ID.String()))

	query := `
        INSERT INTO idps (id, idp_types, name, description, callback_url, login_redirect_url, register_redirect_url, logo, client_id, client_secret)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);
    `

	_, err := ref.db.Exec(ctx, query,
		input.ID,
		input.IDPTypeID,
		input.Name,
		input.Description,
		input.CallbackURL,
		input.LoginRedirectURL,
		input.RegisterRedirectURL,
		input.Logo,
		input.ClientID,
		input.ClientSecret,
	)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	return nil
}

func (ref *IDPsRepository) UpdateByID(ctx context.Context, input *domain.UpdateIDPInput) error {
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

	span.SetAttributes(attribute.String("input.id", input.ID.String()))

	args := []any{input.ID}

	if input.IDPTypeID != nil && *input.IDPTypeID != uuid.Nil() {
		args = append(args, *input.IDPTypeID)
	} else {
		args = append(args, nil)
	}

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

	if input.CallbackURL != nil && *input.CallbackURL != "" {
		args = append(args, *input.CallbackURL)
	} else {
		args = append(args, nil)
	}

	if input.LoginRedirectURL != nil && *input.LoginRedirectURL != "" {
		args = append(args, *input.LoginRedirectURL)
	} else {
		args = append(args, nil)
	}

	if input.RegisterRedirectURL != nil && *input.RegisterRedirectURL != "" {
		args = append(args, *input.RegisterRedirectURL)
	} else {
		args = append(args, nil)
	}

	if input.Logo != nil && *input.Logo != "" {
		args = append(args, *input.Logo)
	} else {
		args = append(args, nil)
	}

	if input.ClientID != nil && *input.ClientID != "" {
		args = append(args, *input.ClientID)
	} else {
		args = append(args, nil)
	}

	if input.ClientSecret != nil && *input.ClientSecret != "" {
		args = append(args, *input.ClientSecret)
	} else {
		args = append(args, nil)
	}

	query := `
        UPDATE idps SET
            idp_types             = COALESCE(NULLIF($2::uuid, idp_types), idp_types),
            name                  = COALESCE(NULLIF($3, name), name),
            description           = COALESCE(NULLIF($4, description), description),
            callback_url          = COALESCE(NULLIF($5, callback_url), callback_url),
            login_redirect_url    = COALESCE(NULLIF($6, login_redirect_url), login_redirect_url),
            register_redirect_url = COALESCE(NULLIF($7, register_redirect_url), register_redirect_url),
            logo                  = COALESCE(NULLIF($8, logo), logo),
            client_id             = COALESCE(NULLIF($9, client_id), client_id),
            client_secret         = COALESCE(NULLIF($10, client_secret), client_secret),
            updated_at            = CURRENT_TIMESTAMP
        WHERE id = $1;
    `

	cslog.Trace(ctx, "repository.IDPs.UpdateByID", "query", prettyPrint(query))

	result, err := ref.db.Exec(ctx, query, args...)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}
	if result.RowsAffected() == 0 {
		err := &domain.IDPNotFoundError{Message: fmt.Sprintf("idp with id '%s' not found", input.ID.String())}
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "idp updated successfully", attribute.String("idp.id", input.ID.String()))

	return nil
}

func (ref *IDPsRepository) DeleteByID(ctx context.Context, input *domain.DeleteIDPInput) error {
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

	span.SetAttributes(attribute.String("idp.id", input.ID.String()))

	query := `
        DELETE FROM idps WHERE id=$1;
    `

	cslog.Trace(ctx, "repository.IDPs.DeleteByID", "query", prettyPrint(query, input.ID))

	result, err := ref.db.Exec(ctx, query, input.ID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	if result.RowsAffected() == 0 {
		// grateful return user was deleted, security reason, but log and record error
		errorType := &domain.IDPNotFoundError{ID: input.ID}
		e := o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
		if e != nil {
			slog.Error("repository.IDPs.DeleteByID", "error", e)
		}

		return nil
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "idp deleted successfully", attribute.String("idp.id", input.ID.String()))

	return nil
}

func (ref *IDPsRepository) SelectByID(ctx context.Context, id uuid.UUID) (*domain.IDP, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByID")
	defer cancel()
	defer span.End()

	if !domain.IsUUIDV7(id) {
		errorType := &domain.InvalidIDPIDError{Message: "idp ID cannot be empty"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("idp.id", id.String()))

	query := `
        SELECT
            idp.id,
            idp.name,
            idp.description,
            idp.callback_url,
            idp.login_redirect_url,
            idp.register_redirect_url,
            idp.logo,
            idp.client_id,
            idp.client_secret,
            idp.created_at,
            idp.updated_at,
            ARRAY[COALESCE(idpt.id::varchar, '00000000-0000-0000-0000-000000000000'), COALESCE(idpt.name::varchar, ''), COALESCE(idpt.scopes::text, '{}'), COALESCE(idpt.user_info_api_url::varchar, '')] AS idp_type
        FROM idps AS idp
            LEFT JOIN idp_types AS idpt ON idp.idp_types = idpt.id
        WHERE idp.id=$1;
    `

	cslog.Trace(ctx, "repository.IDPs.SelectByID", "query", prettyPrint(query, id))

	row := ref.db.QueryRow(ctx, query, id)

	var idp domain.IDP
	var idpType []string
	err := row.Scan(
		&idp.ID,
		&idp.Name,
		&idp.Description,
		&idp.CallbackURL,
		&idp.LoginRedirectURL,
		&idp.RegisterRedirectURL,
		&idp.Logo,
		&idp.ClientID,
		&idp.ClientSecret,
		&idp.CreatedAt,
		&idp.UpdatedAt,
		&idpType,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errorType := &domain.IDPNotFoundError{ID: id}
			return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, nil)
		}

		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	// Parse the IDP type array
	if len(idpType) >= 4 {
		if id, err := uuid.Parse(idpType[0]); err == nil {
			idp.IDPType.ID = id
		}

		idp.IDPType.Name = idpType[1]

		// Parse scopes from text array format like "{scope1,scope2}"
		if idpType[2] != "{}" {
			scopesStr := strings.Trim(idpType[2], "{}")
			if scopesStr != "" {
				idp.IDPType.Scopes = strings.Split(scopesStr, ",")
				// Trim whitespace from each scope
				for i, scope := range idp.IDPType.Scopes {
					idp.IDPType.Scopes[i] = strings.TrimSpace(scope)
				}
			}
		}

		idp.IDPType.UserInfoAPIURL = idpType[3]
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "idp retrieved successfully", attribute.String("idp.id", id.String()))
	return &idp, nil
}

func (ref *IDPsRepository) SelectByName(ctx context.Context, name string) (*domain.IDP, error) {
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
            idp.id,
            idp.name,
            idp.description,
            idp.callback_url,
            idp.login_redirect_url,
            idp.register_redirect_url,
            idp.logo,
            idp.client_id,
            idp.client_secret,
            idp.created_at,
            idp.updated_at,
            ARRAY[COALESCE(idpt.id::varchar, '00000000-0000-0000-0000-000000000000'), COALESCE(idpt.name::varchar, ''), COALESCE(idpt.scopes::text, '{}'), COALESCE(idpt.user_info_api_url::varchar, '')] AS idp_type
        FROM idps AS idp
            LEFT JOIN idp_types AS idpt ON idp.idp_types = idpt.id
        WHERE idp.name=$1;
    `

	cslog.Trace(ctx, "repository.IDPs.SelectByName", "query", prettyPrint(query, name))

	row := ref.db.QueryRow(ctx, query, name)

	var idp domain.IDP
	var idpType []string
	err := row.Scan(
		&idp.ID,
		&idp.Name,
		&idp.Description,
		&idp.CallbackURL,
		&idp.LoginRedirectURL,
		&idp.RegisterRedirectURL,
		&idp.Logo,
		&idp.ClientID,
		&idp.ClientSecret,
		&idp.CreatedAt,
		&idp.UpdatedAt,
		&idpType,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errorType := &domain.IDPNotFoundError{ID: idp.ID}
			return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, nil)
		}

		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	// Parse the IDP type array
	if len(idpType) >= 4 {
		if id, err := uuid.Parse(idpType[0]); err == nil {
			idp.IDPType.ID = id
		}

		idp.IDPType.Name = idpType[1]

		// Parse scopes from text array format like "{scope1,scope2}"
		if idpType[2] != "{}" {
			scopesStr := strings.Trim(idpType[2], "{}")
			if scopesStr != "" {
				idp.IDPType.Scopes = strings.Split(scopesStr, ",")
				// Trim whitespace from each scope
				for i, scope := range idp.IDPType.Scopes {
					idp.IDPType.Scopes[i] = strings.TrimSpace(scope)
				}
			}
		}

		idp.IDPType.UserInfoAPIURL = idpType[3]
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "idp retrieved successfully", attribute.String("idp.name", name))
	return &idp, nil
}

func (ref *IDPsRepository) Select(ctx context.Context, input *domain.SelectIDPsInput) (*domain.ListIDPsOutput, error) {
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
	sqlFieldsPrefix := "idps."
	fieldsArray := []string{
		"id",
		"name",
		"description",
		"callback_url",
		"login_redirect_url",
		"register_redirect_url",
		"logo",
		"client_id",
		"client_secret",
		"created_at",
		"updated_at",
		"serial_id",
		"ARRAY[COALESCE(idpt.id::varchar, '00000000-0000-0000-0000-000000000000'), COALESCE(idpt.name::varchar, ''), COALESCE(idpt.scopes::text, '{}'), COALESCE(idpt.user_info_api_url::varchar, '')] AS idp_type",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields)

	var filterQuery string
	if input.Filter != "" {
		filterSentence := injectPrefixToFields(sqlFieldsPrefix, input.Filter, domain.IDPsFilterFields)
		filterQuery = fmt.Sprintf("WHERE (%s)", filterSentence)
	}

	var sortQuery string
	if input.Sort == "" {
		sortQuery = "idps.serial_id DESC, idps.id DESC"
	} else {
		sortQuery = injectPrefixToSortFields(sqlFieldsPrefix, input.Sort, domain.IDPsSortFields)
	}

	// query template
	queryTemplate := `
        WITH idps AS (
            SELECT
                {{.QueryColumns}}
            FROM idps AS idps
                LEFT JOIN idp_types AS idpt ON idps.idp_types = idpt.id
            {{ .QueryWhere }}
            ORDER BY {{.QueryInternalSort}}
            LIMIT {{.QueryLimit}}
        ) SELECT * FROM idps ORDER BY {{.QueryExternalSort}}
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
	queryValues.QueryInternalSort = "idps.serial_id DESC, idps.id DESC"
	queryValues.QueryExternalSort = sortQuery

	tokenDirection, id, serial, err := domain.GetPaginatorDirection(input.Paginator.NextToken, input.Paginator.PrevToken)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	queryValues.QueryWhere, queryValues.QueryInternalSort = buildPaginationCriteria("idps", tokenDirection, id, serial, filterQuery, false)

	// render the template on query variable
	var tpl bytes.Buffer
	t := template.Must(template.New("query").Parse(queryTemplate))
	err = t.Execute(&tpl, queryValues)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	query := tpl.String()
	cslog.Trace(ctx, "repository.IDPs.Select", "query", prettyPrint(query))

	// execute the query
	rows, err := ref.db.Query(ctx, query)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.IDP
	for rows.Next() {
		var item domain.IDP
		var idpType []string

		scanFields := ref.buildScanFields(&item, &idpType, input.Fields)

		if err := rows.Scan(scanFields...); err != nil {
			return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
		}

		// Parse the IDP type array
		if len(idpType) >= 4 {
			if id, err := uuid.Parse(idpType[0]); err == nil {
				item.IDPType.ID = id
			}

			item.IDPType.Name = idpType[1]

			// Parse scopes from text array format like "{scope1,scope2}"
			if idpType[2] != "{}" {
				scopesStr := strings.Trim(idpType[2], "{}")
				if scopesStr != "" {
					item.IDPType.Scopes = strings.Split(scopesStr, ",")
					// Trim whitespace from each scope
					for i, scope := range item.IDPType.Scopes {
						item.IDPType.Scopes[i] = strings.TrimSpace(scope)
					}
				}
			}

			item.IDPType.UserInfoAPIURL = idpType[3]
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
		return &domain.SelectIDPsOutput{
			Items:     make([]domain.IDP, 0),
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

	ret := &domain.SelectIDPsOutput{
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

// handlePgError maps PostgreSQL errors to domain-specific errors.
// Returns the appropriate domain error or the original error if no mapping exists.
func (ref *IDPsRepository) handlePgError(err error, input any) error {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case "23505": // Unique violation
			if strings.Contains(pgErr.Message, "_pkey") {
				switch v := input.(type) {
				case *domain.InsertIDPInput:
					return &domain.IDPAlreadyExistsError{ID: v.ID}
				case *domain.UpdateIDPInput:
					return &domain.IDPAlreadyExistsError{ID: v.ID}
				case uuid.UUID:
					return &domain.IDPAlreadyExistsError{ID: v}
				}
			}

			if strings.Contains(pgErr.Message, "_name") {
				switch v := input.(type) {
				case *domain.InsertIDPInput:
					return &domain.IDPAlreadyExistsError{Name: v.Name}
				case *domain.UpdateIDPInput:
					if v.Name != nil {
						return &domain.IDPAlreadyExistsError{Name: *v.Name}
					}
				}
			}
		case "23503": // Foreign key violation
			if strings.Contains(pgErr.Message, "_fkey") {
				switch v := input.(type) {
				case *domain.InsertIDPInput:
					return &domain.IDPTypesNotFoundError{ID: v.IDPTypeID, Message: "IDP type you are trying to link does not exist"}
				case *domain.UpdateIDPInput:
					if v.IDPTypeID != nil {
						return &domain.IDPTypesNotFoundError{ID: *v.IDPTypeID, Message: "IDP type you are trying to link does not exist"}
					}
				}
			}

		case "22021": // invalid byte sequence for encoding
			return &domain.InvalidByteSequenceError{Message: pgErr.Message}
		case "08P01": // invalid message format
			return &domain.InvalidMessageFormatError{Message: pgErr.Message}
		case "42703": // Undefined column
			return &domain.UndefinedColumnError{Message: pgErr.Message}
		case "42804": // Datatype mismatch - operator class mismatch
			return &domain.DatatypeMismatchError{Message: pgErr.Message}
		}
	}

	return err
}

// buildScanFields creates the scan targets for the result rows based on the requested fields.
func (ref *IDPsRepository) buildScanFields(item *domain.IDP, idpType *[]string, requestedFields string) []any {
	scanFields := make([]any, 0)

	if requestedFields == "" {
		// All fields were requested
		return []any{
			&item.ID,
			&item.Name,
			&item.Description,
			&item.CallbackURL,
			&item.LoginRedirectURL,
			&item.RegisterRedirectURL,
			&item.Logo,
			&item.ClientID,
			&item.ClientSecret,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.SerialID,
			idpType,
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
		case "callback_url":
			scanFields = append(scanFields, &item.CallbackURL)
		case "login_redirect_url":
			scanFields = append(scanFields, &item.LoginRedirectURL)
		case "register_redirect_url":
			scanFields = append(scanFields, &item.RegisterRedirectURL)
		case "logo":
			scanFields = append(scanFields, &item.Logo)
		case "client_id":
			scanFields = append(scanFields, &item.ClientID)
		case "client_secret":
			scanFields = append(scanFields, &item.ClientSecret)
		case "created_at":
			scanFields = append(scanFields, &item.CreatedAt)
		case "updated_at":
			scanFields = append(scanFields, &item.UpdatedAt)
		case "idp_type":
			scanFields = append(scanFields, idpType)

		default:
			slog.Warn("repository.IDPs.buildScanFields", "what", "field not found", "field", field)
		}
	}

	// Always include ID and SerialID fields for pagination
	if !idFound {
		scanFields = append(scanFields, &item.ID)
	}

	scanFields = append(scanFields, &item.SerialID)
	return scanFields
}
