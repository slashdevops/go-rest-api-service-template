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

	metrics, err := o11y.NewLayerMetrics(ref.ot.Metrics.Meter, ref.metricsPrefix)
	if err != nil {
		return nil, err
	}

	ref.metrics = metrics

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

func (ref *UsersRepository) Insert(ctx context.Context, input *domain.InsertUserInput) (err error) {
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

	tx, err := ref.db.Begin(ctx)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}
	defer endTx(ctx, tx, &err, func(e error) error {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(e, input), ref.metrics, attrs)
	})

	// insert the user
	query1 := `
        INSERT INTO users (id, first_name, last_name, email, password_hash, disabled, verified, local_account, disabled_by_admin)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
    `

	cslog.Trace(ctx, "sql", "query",
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

	_, err = tx.Exec(ctx, query1,
		input.ID,
		input.FirstName,
		input.LastName,
		input.Email,
		input.PasswordHash,
		disabled,
		input.Verified != nil && *input.Verified,
		input.LocalAccount != nil && *input.LocalAccount,
		// Only a row that is stored disabled is disabled by anybody.
		disabled && input.DisabledByAdmin,
	)
	if err != nil {
		err = ref.handlePgError(err, input)
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
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

	cslog.Trace(ctx, "sql", "query", prettyPrint(query2, input.ID.String()))
	_, err = tx.Exec(ctx, query2, input.ID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// An account provisioned from a provider identity is stored with it.
	// They were two writes, the identity's after the account's commit, and
	// an identity that could not be stored left an account nobody could
	// sign in to, holding its address. One transaction: both, or neither.
	if identity := input.Identity; identity != nil {
		span.SetAttributes(attribute.String("idp.id", identity.IDPID.String()))

		cslog.Trace(ctx, "sql", "query", prettyPrint(queryLinkIdentity, identity.UserID.String(), identity.IDPID.String()))

		linked, linkErr := tx.Exec(ctx, queryLinkIdentity, identity.UserID, identity.IDPID, identity.Subject, identity.Email, identity.IssuerURL, identity.IDPTypeID)
		if linkErr != nil {
			return o11y.RecordError(ctx, span, start, identityPgError(linkErr), ref.metrics, attrs)
		}

		// The provider is not as the sign-in used it: the account is not
		// stored either, the transaction is rolled back with it.
		if linked.RowsAffected() == 0 {
			return o11y.RecordError(ctx, span, start, linkIdentityRefused(), ref.metrics, attrs)
		}
	}

	cslog.Trace(ctx, "repository.Users.Insert", "user.id", input.ID)
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

	if input.DisabledByAdmin != nil {
		args = append(args, *input.DisabledByAdmin)
	} else {
		args = append(args, nil)
	}

	// $9, the `admin` flag, when the body carries it: TRUE or FALSE. The
	// column is NOT NULL; this statement leaves the row's value as it is
	// unless it is told.
	if input.Admin != nil {
		args = append(args, *input.Admin)
	} else {
		args = append(args, nil)
	}

	// Two ways to the administrator's lock, both judged against the row as
	// it was before this statement.
	//
	// $8, the lock itself, when the body carries it. True locks: disabled,
	// and marked, whatever disabled already was. That is how a registration
	// whose link is still out is shut: it is disabled already, so
	// "disabled: true" for it is no change. False on a locked row unlocks:
	// the mark goes, and the account is enabled if its address is proven and
	// waits for its link if not (or is what $6 says, when the body says).
	//
	// Without $8, the mark follows a CHANGE of disabled ($6): a client sends
	// the whole form back, so "disabled: true" arrives with every rename of
	// a registration whose link has not been followed, and that is nobody
	// disabling it.
	//
	// This statement is the only one that sets disabled for somebody else;
	// the verification has one of its own (Verify).
	query := `
        UPDATE users SET
            first_name    = COALESCE(NULLIF($2, first_name), first_name),
            last_name     = COALESCE(NULLIF($3, last_name), last_name),
            email         = COALESCE(NULLIF($4, email), email),
            password_hash = COALESCE(NULLIF($5, password_hash), password_hash),
            disabled = CASE
                WHEN $8::boolean IS TRUE THEN TRUE
                WHEN $8::boolean IS FALSE AND disabled_by_admin THEN COALESCE($6::boolean, NOT verified)
                ELSE COALESCE($6::boolean, disabled)
            END,
            disabled_by_admin = CASE
                WHEN $8::boolean IS NOT NULL THEN $8::boolean
                WHEN $6::boolean IS NULL OR $6::boolean = disabled THEN disabled_by_admin
                ELSE $6::boolean
            END,
            local_account = COALESCE($7, local_account),
            admin         = COALESCE($9::boolean, admin),
            updated_at    = CURRENT_TIMESTAMP
        WHERE id = $1;
    `

	cslog.Trace(ctx, "sql", "query", prettyPrint(query, args...))

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

// Verify records that address is proven to be the account's, and opens the
// account when the unfollowed link was the only thing keeping it shut.
//
// One statement, so that what it reads and what it writes cannot be split:
// an account an administrator disabled stays disabled (disabled_by_admin),
// and so does one with the `admin` flag, which a mailed link never opens.
// Read and written in two steps, an account disabled in between would have
// been enabled by a value read before.
//
// It writes only a row that is not verified yet and whose address is the
// one the link was mailed to: a link proves the address it went to, and an
// account whose address was changed since has not proven the new one. No
// such row is a NotFoundError; which of the three it is (gone, verified
// meanwhile, another address) is the use-case's to tell.
func (ref *UsersRepository) Verify(ctx context.Context, id uuid.UUID, address string) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "Verify")
	defer cancel()
	defer span.End()

	if id == uuid.Nil() {
		errorValue := &domain.InvalidUserIDError{Message: "user ID cannot be empty"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.id", id.String()))

	query := `
        UPDATE users SET
            verified   = TRUE,
            disabled   = CASE WHEN disabled_by_admin OR admin IS TRUE THEN disabled ELSE FALSE END,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $1
          AND email = $2
          AND verified IS FALSE;
    `

	cslog.Trace(ctx, "sql", "query", prettyPrint(query, id.String(), address))

	result, err := ref.db.Exec(ctx, query, id, address)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	if result.RowsAffected() == 0 {
		errorType := &domain.UserNotFoundError{ID: id}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user verified successfully", attribute.String("user.id", id.String()))

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

	cslog.Trace(ctx, "sql", "query", prettyPrint(query, input.ID.String()))

	result, err := ref.db.Exec(ctx, query, input.ID)
	if err != nil {
		// Through the repository's own mapping: the seeded administrator's
		// delete is refused by a trigger, and that refusal is a 403.
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	if result.RowsAffected() == 0 {
		// Nothing was deleted, and the caller is told so. This returned nil,
		// "for security", and the use-case went on as after a real delete:
		// it gave a usage slot back, so deleting ids that do not exist
		// lowered the counter the limit is checked against. The handler has
		// always had the 404 arm.
		errorType := &domain.UserNotFoundError{ID: input.ID}

		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user deleted successfully", attribute.String("user.id", input.ID.String()))

	return nil
}

// queryResetPassword writes a reset link's password on the account it was
// mailed to, as the account is now ($1 the id, $2 the address the link went
// to, $3 the hash).
const queryResetPassword = `
        UPDATE users SET
            password_hash = $3,
            updated_at    = CURRENT_TIMESTAMP
        WHERE id = $1
          AND email = $2
          AND local_account IS TRUE
          AND disabled IS FALSE;
    `

// ResetPassword is the end of a reset link, and its only writer.
//
// A link is bound to the account and to the address it was mailed to: one
// mailed to an address the account no longer has proves nothing about the
// mailbox that holds it now, and it used to set the password until it
// expired. And it is held to the account as it is when the link is
// followed, in the statement that writes: a disabled account, which a link
// mailed before the disable could still give a new password, and one that
// is no longer a local account. Read and written in two steps, a change in
// between would have been written past. No such row is a UserNotFoundError;
// which it is (gone, another address, disabled, federated) is told to
// nobody.
func (ref *UsersRepository) ResetPassword(ctx context.Context, id uuid.UUID, address, passwordHash string) error {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "ResetPassword")
	defer cancel()
	defer span.End()

	if !domain.IsUUIDV7(id) {
		errorType := &domain.InvalidUserIDError{Message: "user ID cannot be empty"}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.id", id.String()))

	cslog.Trace(ctx, "sql", "query", prettyPrint(queryResetPassword, id, address, "[hash]"))

	result, err := ref.db.Exec(ctx, queryResetPassword, id, address, passwordHash)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	if result.RowsAffected() == 0 {
		errorType := &domain.UserNotFoundError{ID: id}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "password reset", attribute.String("user.id", id.String()))

	return nil
}

// `disabled` is NOT NULL; `IS TRUE` is kept so that the statement reads the
// same against a database created before it was.
const queryUserDisabled = `
        SELECT disabled IS TRUE FROM users WHERE id = $1::uuid;
    `

// SelectDisabled implements [repository.Users]: whether the account is
// disabled, or a UserNotFoundError when there is no such account.
//
// It is read on every authenticated request (UsersService.Standing), so it
// reads that column and nothing else: SelectByID reads the whole row, the
// password hash included, for one boolean.
func (ref *UsersRepository) SelectDisabled(ctx context.Context, id uuid.UUID) (bool, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectDisabled")
	defer cancel()
	defer span.End()

	if !domain.IsUUIDV7(id) {
		errorType := &domain.InvalidUserIDError{Message: "user ID cannot be empty"}
		return false, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.id", id.String()))

	var disabled bool
	if err := ref.db.QueryRow(ctx, queryUserDisabled, id).Scan(&disabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errorType := &domain.UserNotFoundError{ID: id}
			return false, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
		}

		return false, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user's standing read")

	return disabled, nil
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
            disabled_by_admin,
            verified,
            admin,
            local_account,
            created_at,
            updated_at
        FROM users
        WHERE id = $1;
    `

	cslog.Trace(ctx, "sql", "query", prettyPrint(query, id.String()))

	row := ref.db.QueryRow(ctx, query, id)

	var item domain.User
	if err := row.Scan(
		&item.ID,
		&item.FirstName,
		&item.LastName,
		&item.Email,
		&item.PasswordHash,
		&item.Disabled,
		&item.DisabledByAdmin,
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

// enabledAdministrator is the predicate of an account with the `admin` flag
// that may still sign in. That is not "is not disabled": an account that is
// no longer a local one signs in through a provider, one with no identity at
// a provider has no way in at all, and an identity at a provider that is
// disabled is not a way in either. `admin` and `disabled` are NOT NULL; the
// comparisons stay `IS TRUE`/`IS FALSE`, which read the same against a
// database created before they were.
//
// $1 is a provider to count without, or NULL.
//
// One predicate for the questions asked of it (how many, is this account one
// of them), so that "the last one" and "this one" cannot come to mean two
// things.
const enabledAdministrator = `
          admin IS TRUE
          AND disabled IS FALSE
          AND (local_account OR EXISTS (
                SELECT 1 FROM users_identities
                JOIN idps ON idps.id = users_identities.idps_id
                WHERE users_identities.users_id = users.id
                  AND idps.enabled
                  AND ($1::uuid IS NULL OR idps.id <> $1::uuid)
          ))`

// queryEnabledAdministrators counts them.
const queryEnabledAdministrators = `
        SELECT count(*) FROM users
        WHERE` + enabledAdministrator + `;
    `

// queryIsEnabledAdministrator asks whether one account ($2) is among them.
const queryIsEnabledAdministrator = `
        SELECT EXISTS (
            SELECT 1 FROM users
            WHERE id = $2::uuid AND` + enabledAdministrator + `
        );
    `

// CountEnabledAdministrators implements [repository.Users]: how many accounts
// with the `admin` flag may still sign in (queryEnabledAdministrators).
func (ref *UsersRepository) CountEnabledAdministrators(ctx context.Context) (int, error) {
	return ref.countEnabledAdministrators(ctx, "CountEnabledAdministrators", nil)
}

// CountEnabledAdministratorsWithout implements [repository.Users]: the same
// count as if the given identity provider were not there. A provider's
// disable and its delete ask it, so that neither leaves no administrator
// account that can sign in.
func (ref *UsersRepository) CountEnabledAdministratorsWithout(ctx context.Context, idpID uuid.UUID) (int, error) {
	if idpID == uuid.Nil() {
		// Never "without nothing": that is the other count, and whoever
		// asks this one means a provider.
		return 0, &domain.InvalidInputError{Message: "the administrators were counted without a provider that was not named"}
	}

	without := idpID.String()

	return ref.countEnabledAdministrators(ctx, "CountEnabledAdministratorsWithout", &without)
}

func (ref *UsersRepository) countEnabledAdministrators(ctx context.Context, action string, without *string) (int, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, action)
	defer cancel()
	defer span.End()

	cslog.Trace(ctx, "sql", "query", prettyPrint(queryEnabledAdministrators, without))

	var enabled int
	if err := ref.db.QueryRow(ctx, queryEnabledAdministrators, without).Scan(&enabled); err != nil {
		return 0, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "enabled administrator accounts counted")

	return enabled, nil
}

// IsEnabledAdministrator implements [repository.Users]: whether the account
// is one of those CountEnabledAdministrators counts. The guard of the
// administrator account asks it before it asks for the count: taking a
// sign-in away from an account that has none takes nothing away.
func (ref *UsersRepository) IsEnabledAdministrator(ctx context.Context, id uuid.UUID) (bool, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "IsEnabledAdministrator")
	defer cancel()
	defer span.End()

	if id == uuid.Nil() {
		return false, o11y.RecordError(ctx, span, start, &domain.InvalidInputError{Message: "an account was asked about that was not named"}, ref.metrics, attrs)
	}

	var without *string

	cslog.Trace(ctx, "sql", "query", prettyPrint(queryIsEnabledAdministrator, without, id.String()))

	var among bool
	if err := ref.db.QueryRow(ctx, queryIsEnabledAdministrator, without, id.String()).Scan(&among); err != nil {
		return false, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "enabled administrator account asked about")

	return among, nil
}

// queryAdministratorsAmong answers which of a set of accounts have the
// `admin` flag.
const queryAdministratorsAmong = `
        SELECT id FROM users WHERE id = ANY($1::uuid[]) AND admin IS TRUE ORDER BY id;
    `

// queryAdministratorsHolding answers which accounts with the `admin` flag
// hold a role ($1), if its delete would take it from them. A system role's
// delete takes it from nobody: its trigger refuses the delete whoever asks,
// and that refusal says why.
const queryAdministratorsHolding = `
        SELECT users.id FROM users
        JOIN users_roles ON users_roles.users_id = users.id
        JOIN roles ON roles.id = users_roles.roles_id
        WHERE users_roles.roles_id = $1 AND users.admin IS TRUE AND NOT roles.system
        ORDER BY users.id;
    `

// SelectAdministratorsHolding implements [repository.Users]: which accounts
// with the `admin` flag the role's delete would strip of it
// (queryAdministratorsHolding).
func (ref *UsersRepository) SelectAdministratorsHolding(ctx context.Context, roleID uuid.UUID) ([]uuid.UUID, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectAdministratorsHolding")
	defer cancel()
	defer span.End()

	cslog.Trace(ctx, "sql", "query", prettyPrint(queryAdministratorsHolding, roleID))

	administrators, err := ref.selectAccountIDs(ctx, queryAdministratorsHolding, roleID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "administrator accounts selected",
		attribute.String("role.id", roleID.String()))

	return administrators, nil
}

// selectAccountIDs runs a statement that answers a column of account ids.
// The caller's span records how it ended.
func (ref *UsersRepository) selectAccountIDs(ctx context.Context, query string, args ...any) ([]uuid.UUID, error) {
	rows, err := ref.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID

	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}

		ids = append(ids, id)
	}

	return ids, rows.Err()
}

// SelectAdministratorsAmong implements [repository.Users]: which of ids are
// accounts with the `admin` flag.
func (ref *UsersRepository) SelectAdministratorsAmong(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectAdministratorsAmong")
	defer cancel()
	defer span.End()

	asked := make([]string, len(ids))
	for i, id := range ids {
		asked[i] = id.String()
	}

	cslog.Trace(ctx, "sql", "query", prettyPrint(queryAdministratorsAmong, asked))

	rows, err := ref.db.Query(ctx, queryAdministratorsAmong, asked)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}
	defer rows.Close()

	var administrators []uuid.UUID

	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
		}

		administrators = append(administrators, id)
	}

	if err := rows.Err(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "administrator accounts selected")

	return administrators, nil
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
            disabled_by_admin,
            verified,
            local_account,
            created_at,
            updated_at
        FROM users
        WHERE email = $1;
    `

	cslog.Trace(ctx, "sql", "query", prettyPrint(query, email))

	row := ref.db.QueryRow(ctx, query, email)

	var item domain.User
	if err := row.Scan(
		&item.ID,
		&item.FirstName,
		&item.LastName,
		&item.Email,
		&item.PasswordHash,
		&item.Disabled,
		&item.DisabledByAdmin,
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
		"disabled_by_admin",
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
	cslog.Trace(ctx, "sql", "query", prettyPrint(query, roleID.String()))

	// execute the query
	rows, err := ref.db.Query(ctx, query, roleID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(filterRefusal(filterQuery, query, err), nil), ref.metrics, attrs)
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
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(filterRefusal(filterQuery, query, err), nil), ref.metrics, attrs)
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

// idpAccountColumns follows a provider's accounts' user columns: when the
// account was linked, and whether the provider is its only way in, which is
// the provider's delete's own rule (queryDeleteIDP): no password, and no
// identity at another provider. The accounts it is true for are the ones a
// 409 on the delete is about.
const idpAccountColumns = `, ui.linked_at, (usrs.local_account IS NOT TRUE AND NOT EXISTS (
                    SELECT 1 FROM users_identities AS other
                    WHERE other.users_id = usrs.id AND other.idps_id <> ui.idps_id
                )) AS only_way_in`

// SelectByIDPID implements [repository.Users]: a page of the accounts linked
// to an identity provider, under the users' own sort, filter and fields. A
// provider that is not there is a 404, not an empty page: a read of what a
// parent holds says when there is no parent.
func (ref *UsersRepository) SelectByIDPID(ctx context.Context, idpID uuid.UUID, input *domain.ListUsersInput) (*domain.SelectIDPAccountsOutput, error) {
	start := time.Now()
	ctx, span, attrs, cancel := o11y.SetupTraceWithTimeout(ctx, ref.ot.Traces.Tracer, ref.maxQueryTimeout, ref.metricsMetadata, "SelectByIDPID")
	defer cancel()
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if idpID == uuid.Nil() {
		invalidErr := &domain.InvalidInputError{Field: "idp_id", Message: "invalid idp ID"}
		return nil, o11y.RecordError(ctx, span, start, invalidErr, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	var there bool

	cslog.Trace(ctx, "sql", "query", prettyPrint(queryIDPExists, idpID.String()))

	if err := ref.db.QueryRow(ctx, queryIDPExists, idpID).Scan(&there); err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
	}

	if !there {
		return nil, o11y.RecordError(ctx, span, start, &domain.IDPNotFoundError{ID: idpID}, ref.metrics, attrs)
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
		"disabled_by_admin",
		"verified",
		"local_account",
		"created_at",
		"updated_at",
		"serial_id",
	}

	fieldsStr := buildFieldSelection(sqlFieldsPrefix, fieldsArray, input.Fields) + idpAccountColumns

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
                JOIN users_identities AS ui ON ui.users_id = usrs.id
            WHERE ui.idps_id = $1
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
	cslog.Trace(ctx, "sql", "query", prettyPrint(query, idpID.String()))

	// execute the query
	rows, err := ref.db.Query(ctx, query, idpID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(filterRefusal(filterQuery, query, err), nil), ref.metrics, attrs)
	}
	defer rows.Close()

	var fetchedItems []domain.IDPAccount
	for rows.Next() {
		var item domain.IDPAccount

		scanFields := append(ref.buildScanFields(&item.User, input.Fields), &item.LinkedAt, &item.OnlyWayIn)

		if err := rows.Scan(scanFields...); err != nil {
			return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(err, nil), ref.metrics, attrs)
		}

		fetchedItems = append(fetchedItems, item)
	}

	if err := rows.Err(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(filterRefusal(filterQuery, query, err), nil), ref.metrics, attrs)
	}

	hasMore := len(fetchedItems) > input.Paginator.Limit
	displayItems := fetchedItems
	if hasMore {
		displayItems = fetchedItems[:input.Paginator.Limit]
	}

	outLen := len(displayItems)
	if outLen == 0 {
		return &domain.SelectIDPAccountsOutput{
			Items:     make([]domain.IDPAccount, 0),
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
	// regardless of external sorting order (e.g., "first_name ASC")
	minSerialItem := displayItems[0].User
	maxSerialItem := displayItems[0].User

	for _, item := range displayItems {
		if item.User.SerialID < minSerialItem.SerialID {
			minSerialItem = item.User
		}
		if item.User.SerialID > maxSerialItem.SerialID {
			maxSerialItem = item.User
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

	ret := &domain.SelectIDPAccountsOutput{
		Items: displayItems,
		Paginator: domain.Paginator{
			Size:      outLen,
			Limit:     input.Paginator.Limit,
			NextToken: nextToken,
			PrevToken: prevToken,
		},
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "a provider's accounts selected")

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
		"disabled_by_admin",
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
	cslog.Trace(ctx, "sql", "query", prettyPrint(query, projectID.String()))

	// execute the query
	rows, err := ref.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(filterRefusal(filterQuery, query, err), nil), ref.metrics, attrs)
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
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(filterRefusal(filterQuery, query, err), nil), ref.metrics, attrs)
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
		"disabled_by_admin",
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
	cslog.Trace(ctx, "sql", "query", prettyPrint(query))

	// execute the query
	rows, err := ref.db.Query(ctx, query)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(filterRefusal(filterQuery, query, err), nil), ref.metrics, attrs)
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
		return nil, o11y.RecordError(ctx, span, start, ref.handlePgError(filterRefusal(filterQuery, query, err), nil), ref.metrics, attrs)
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
        -- insert the new roles. DISTINCT: a body may name one twice, and
        -- ON CONFLICT DO UPDATE refuses to touch one row twice in one
        -- statement (SQLSTATE 21000), which was a 500.
        INSERT INTO users_roles (users_id, roles_id)
        SELECT DISTINCT * FROM UNNEST($1::uuid[], $2::uuid[])
        ON CONFLICT (users_id, roles_id)
        DO UPDATE SET updated_at = NOW();
    `

	cslog.Trace(ctx, "sql", "query", prettyPrint(query, userIDs, roleIDs))

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

	cslog.Trace(ctx, "sql", "query", prettyPrint(queryString, input.UserID.String(), roleIDs))

	// Pass input.UserID and the slice of roleIDs as parameters
	_, err := ref.db.Exec(ctx, queryString, input.UserID.String(), roleIDs)
	if err != nil {
		return o11y.RecordError(ctx, span, start, ref.handlePgError(err, input), ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "roles unlinked successfully")

	return nil
}

// queryProjectNotOfCaller answers one of the given projects that the caller
// is not linked to, if there is one. view_projects_users has every project
// for an account with the `admin` flag and an account's memberships for
// anyone else; an id that is no project at all is in nobody's.
const queryProjectNotOfCaller = `
        SELECT wanted.id
        FROM UNNEST($2::uuid[]) AS wanted(id)
        WHERE NOT EXISTS (
            SELECT 1 FROM view_projects_users AS callers
            WHERE callers.user_id = $1 AND callers.id = wanted.id
        )
        LIMIT 1;
    `

// rowQuerier is the one call projectsOfCaller makes of the pool, so that the
// check can be asked without a database.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// errLinksWithoutCaller is what a change to a user's project links answers
// when it does not know who asks: a route wired without its caller. It is no
// domain error, so the answer is the fixed 500.
var errLinksWithoutCaller = errors.New("a user's project links were asked to change with no caller")

// projectsOfCaller refuses a change to a user's project links that names a
// project the caller is not linked to.
//
// POST and DELETE /users/{user_id}/projects take the projects from the body.
// With no project in the path no membership check runs, so the ids are held
// to who asks here: a caller links or unlinks a user only on a project the
// caller is linked to, in the sense the projects list has. Another account's
// project and one that does not exist are refused in the same words.
func projectsOfCaller(ctx context.Context, db rowQuerier, caller uuid.UUID, projectIDs []uuid.UUID) error {
	if caller == uuid.Nil() {
		// Never "any project": a write that does not know who asks is refused.
		return errLinksWithoutCaller
	}

	ids := make([]string, len(projectIDs))
	for i, id := range projectIDs {
		ids[i] = id.String()
	}

	cslog.Trace(ctx, "sql", "query", prettyPrint(queryProjectNotOfCaller, caller.String(), ids))

	var foreign uuid.UUID

	err := db.QueryRow(ctx, queryProjectNotOfCaller, caller, ids).Scan(&foreign)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}

	if err != nil {
		return err
	}

	return &domain.ProjectNotOfCallerError{ProjectID: foreign}
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

	if err := projectsOfCaller(ctx, ref.db, input.CallerID, input.ProjectIDs); err != nil {
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
        -- insert the new projects. DISTINCT: a body may name a project twice,
        -- and ON CONFLICT DO UPDATE refuses to touch one row twice in one
        -- statement (SQLSTATE 21000), which was a 500.
        INSERT INTO projects_users (users_id, projects_id)
        SELECT DISTINCT * FROM UNNEST($1::uuid[], $2::uuid[])
        ON CONFLICT (users_id, projects_id)
        DO UPDATE SET updated_at = NOW();
    `

	cslog.Trace(ctx, "sql", "query", prettyPrint(query, userIDs, projectIDs))

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

	if err := projectsOfCaller(ctx, ref.db, input.CallerID, input.ProjectIDs); err != nil {
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

	cslog.Trace(ctx, "sql", "query", prettyPrint(query, input.UserID.String(), projectIDs))

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

	cslog.Trace(ctx, "sql", "query", prettyPrint(query, userID.String()))

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

// bootstrapUserLinkMessage is what fn_protect_bootstrap_links raises when a
// statement would take the seeded administrator out of the Administrator
// role, as raised in 00007_roles_policies_tables.sql.
const bootstrapUserLinkMessage = "The bootstrap administrator cannot be removed from the Administrator role."

// bootstrapAdministratorMessage is what fn_protect_bootstrap_administrator
// (migration 00002) raises when a statement would take the `admin` flag from
// the account the migrations seed.
const bootstrapAdministratorMessage = "The bootstrap administrator keeps the admin flag."

// bootstrapUserLinkRefusal answers that refusal as the 403 a protected
// administrator account answers, for the three statements that can raise
// it, and is nil for anything else.
//
// The seeded administrator keeps the Administrator role, and so is deleted
// by nobody: a delete takes its link to the role with it. The trigger that
// guards that link refuses the delete and both unlinks of it (from the
// user's side and from the role's), and the refusal reached the caller as it
// came, a 500. It is matched by the trigger's own words, since P0001 is every
// RAISE EXCEPTION; TestTheSeededAdministratorsLinkIsA403 keeps the two in
// step.
func bootstrapUserLinkRefusal(pgErr *pgconn.PgError, input any) error {
	if !strings.Contains(pgErr.Message, bootstrapUserLinkMessage) {
		return nil
	}

	switch input.(type) {
	case *domain.DeleteUserInput:
		return &domain.AdministratorProtectedError{Action: "deleted", Message: theBootstrapAdministrator}
	case *domain.UnlinkRolesFromUsersInput, *domain.UnlinkUsersFromRoleInput:
		return &domain.AdministratorProtectedError{Action: "stripped of a role", Message: theBootstrapAdministrator}
	}

	return nil
}

// theBootstrapAdministrator is the rule bootstrapUserLinkRefusal names.
const theBootstrapAdministrator = "the bootstrap administrator"

// constraintProjectsUsersProject is the foreign key from a link to its
// project, by the name migration 00005 declares.
const constraintProjectsUsersProject = "projects_users_projects_id_fkey"

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
			// A link names two rows. The project was the caller's at the
			// check (projectsOfCaller) and is gone at the insert: the same
			// refusal, without an id, since the statement does not say which
			// one it was. It comes before the branch below, which answered
			// "User not found" for either of the two.
			if pgErr.ConstraintName == constraintProjectsUsersProject {
				if _, ok := input.(*domain.LinkProjectsToUserInput); ok {
					return &domain.ProjectNotOfCallerError{}
				}
			}

			if strings.Contains(pgErr.Message, "_fkey") {
				switch v := input.(type) {
				case *domain.LinkProjectsToUserInput: // works too for domain.UnlinkProjectsFromUserInput because is an alias
					return &domain.UserNotFoundError{ID: v.UserID, Message: "User not found"}
				}
			}

		case "P0001": // Raised exception
			if refusal := bootstrapUserLinkRefusal(pgErr, input); refusal != nil {
				return refusal
			}

			// The seeded account keeps its `admin` flag: its row's trigger
			// refuses (fn_protect_bootstrap_administrator, migration 00002),
			// answered as the 403 the guard of the administrator account
			// gives. Matched by the trigger's own words;
			// TestTheSeededAdministratorKeepsItsFlag keeps the two in step.
			if _, updating := input.(*domain.UpdateUserInput); updating && strings.Contains(pgErr.Message, bootstrapAdministratorMessage) {
				return &domain.AdministratorProtectedError{Action: "stripped of the admin flag", Message: theBootstrapAdministrator}
			}
		case "22021": // invalid byte sequence for encoding
			return &domain.InvalidByteSequenceError{Message: pgInvalidByteSequence}
		case "08P01": // invalid message format
			return &domain.InvalidMessageFormatError{Message: pgInvalidMessageFormat}

		case "42703": // undefined column
			return &domain.UndefinedColumnError{Message: pgUndefinedColumn}
		case "42804": // datatype mismatch - operator class does not accept data type
			return &domain.DatatypeMismatchError{Message: pgDatatypeMismatch}
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
			&item.DisabledByAdmin,
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
		case "disabled_by_admin":
			scanFields = append(scanFields, &item.DisabledByAdmin)
		case "verified":
			scanFields = append(scanFields, &item.Verified)
		case "local_account":
			scanFields = append(scanFields, &item.LocalAccount)
		case "created_at":
			scanFields = append(scanFields, &item.CreatedAt)
		case "updated_at":
			scanFields = append(scanFields, &item.UpdatedAt)

		default:
			slog.Warn("field not found while building the scan list", "field", field)
		}
	}

	// Always include ID and SerialID fields for pagination
	if !idFound {
		scanFields = append(scanFields, &item.ID)
	}

	scanFields = append(scanFields, &item.SerialID)
	return scanFields
}
