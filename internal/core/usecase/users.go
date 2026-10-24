package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"uuid"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/cache"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

type UsersServiceConf struct {
	// Guard refuses a grant the caller does not hold. Required: a service
	// that can widen permissions without it is the escalation path.
	Guard           *GrantGuard
	Repository      repository.Users
	CacheService    cache.Cache
	ResourcesLimits ResourcesLimitsServiceConsumer
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
}

type UsersService struct {
	guard           *GrantGuard
	repository      repository.Users
	cacheService    cache.Cache
	resourcesLimits ResourcesLimitsServiceConsumer
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

func NewUsersService(conf UsersServiceConf) (*UsersService, error) {
	if conf.Guard == nil {
		return nil, &domain.InvalidInputError{Message: "Guard is nil, but it is required for UsersService"}
	}

	if conf.Repository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "Repository is nil, but it is required for UsersService"}
	}

	if conf.ResourcesLimits == nil {
		return nil, &domain.InvalidResourcesLimitsError{Message: "ResourcesLimits is nil, but it is required for UsersService"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for UsersService"}
	}

	ref := &UsersService{
		guard:           conf.Guard,
		repository:      conf.Repository,
		cacheService:    conf.CacheService,
		resourcesLimits: conf.ResourcesLimits,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Users",
			Action: "NewUsersService",
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

func (ref *UsersService) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetByID")
	defer span.End()

	if !domain.IsUUIDV7(id) {
		errorType := &domain.InvalidUserIDError{Message: "user ID cannot be empty"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.id", id.String()))

	var out *domain.User
	var err error

	userFetcher := func(ctx context.Context) (*domain.User, []cache.Identifier, error) {
		out, err := ref.repository.SelectByID(ctx, id)
		if err != nil {
			return nil, nil, err
		}

		return withoutCredentials(out), nil, nil
	}

	if ref.cacheService == nil {
		slog.DebugContext(ctx, "cache lookup", "cache", "disabled")

		// Through the same fetcher, so the value a caller receives does not
		// depend on whether caching happens to be enabled.
		out, _, err = userFetcher(ctx)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	} else {

		cacheKey := cache.Identifier{
			Type: "user",
			ID:   id.String(),
		}

		slog.DebugContext(ctx, "cache lookup", "cache", "enabled", "user_id", id)
		out, err = cache.GetTyped[*domain.User](ctx, ref.cacheService, cacheKey, userFetcher)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user found", attribute.String("user.email", out.Email))

	return out, nil
}

// withoutCredentials returns a copy of u with the credential fields cleared.
//
// [UsersService.GetByEmail] and [UsersService.GetByID] are cached, and their
// results are written to Valkey — a store outside the database, with a twelve
// hour hard TTL by default and, unless cache.tls.enabled is set, reached over a
// cleartext connection. A bcrypt hash does not belong there.
//
// It returns a copy rather than clearing in place: the value belongs to the
// caller of the fetcher, and mutating it would surprise anyone who still holds
// a reference to what the repository returned.
//
// The one caller that genuinely needs a hash uses
// [UsersService.GetByEmailForAuth], which does not cache. Everything else gets
// a user with no credentials on it, so no future caller can start depending on
// one being there.
func withoutCredentials(u *domain.User) *domain.User {
	if u == nil {
		return nil
	}

	stripped := *u
	stripped.Password = ""
	stripped.PasswordHash = ""

	return &stripped
}

// GetByEmailForAuth returns the user for email with the password hash intact,
// reading straight from the repository.
//
// This is the only path in the service that returns credentials, and it is
// deliberately uncached. Caching it would put bcrypt hashes in Valkey, and
// caching it *without* the hash would be worse still: the compare in
// [AuthnService.LoginUser] would run against an empty string.
//
// The cost is one indexed lookup per login. Logins are rare next to
// authenticated requests — the per-request authorization data is cached
// separately under authz:<userID> and is unaffected — so there is nothing to
// save here worth the exposure.
func (ref *UsersService) GetByEmailForAuth(ctx context.Context, email string) (*domain.User, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetByEmailForAuth")
	defer span.End()

	span.SetAttributes(attribute.String("user.email", email))

	if email == "" {
		errorType := &domain.InvalidEmailError{Email: email}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	out, err := ref.repository.SelectByEmail(ctx, email)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user found", attribute.String("user.email", out.Email))

	return out, nil
}

func (ref *UsersService) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetByEmail")
	defer span.End()

	span.SetAttributes(attribute.String("user.email", email))

	if email == "" {
		errorType := &domain.InvalidEmailError{Email: email}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	var out *domain.User
	var err error

	userFetcher := func(ctx context.Context) (*domain.User, []cache.Identifier, error) {
		out, err := ref.repository.SelectByEmail(ctx, email)
		if err != nil {
			return nil, nil, err
		}

		// since this is fetching by email, we can also add the user ID cache dependency
		dependencies := []cache.Identifier{
			{
				Type: "user",
				ID:   out.ID.String(),
			},
		}

		return withoutCredentials(out), dependencies, nil
	}

	if ref.cacheService == nil {
		slog.DebugContext(ctx, "cache lookup", "cache", "disabled")

		// Through the same fetcher, so the value a caller receives does not
		// depend on whether caching happens to be enabled.
		out, _, err = userFetcher(ctx)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	} else {
		cacheKey := cache.Identifier{
			Type: "user",
			ID:   email,
		}

		slog.DebugContext(ctx, "cache lookup", "cache", "enabled")
		out, err = cache.GetTyped[*domain.User](ctx, ref.cacheService, cacheKey, userFetcher)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user found", attribute.String("user.email", out.Email))

	return out, nil
}

func (ref *UsersService) Create(ctx context.Context, input *domain.CreateUserInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "Create")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.email", input.Email))

	var err error
	input.ID, err = domain.EnsureUUIDV7(input.ID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Check resource limits
	rlScope := domain.ResourcesLimitsScope{
		Type: domain.ResourcesLimitsScopeTypeSystem,
	}

	resourceLimitParams := ResourceLimitCheckParams{
		Ctx:             ctx,
		ResourcesLimits: ref.resourcesLimits,
		Scope:           rlScope,
		ResourceType:    domain.ResourcesLimitsResourceTypeUsers,
	}

	// Reserve the slot before creating, and give it back if anything after this
	// point fails.
	if err := ReserveResourceSlot(resourceLimitParams); err != nil {
		return err
	}

	hashPwd, err := HashAndSaltPassword(input.Password)
	if err != nil {
		ReleaseResourceSlot(resourceLimitParams)
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	input.PasswordHash = hashPwd

	if err := ref.repository.Insert(ctx, input); err != nil {
		ReleaseResourceSlot(resourceLimitParams)
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	WarnOnSoftLimit(resourceLimitParams)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user created successfully",
		attribute.String("user.email", input.Email),
		attribute.String("user.id", input.ID.String()))

	return nil
}

// Standing reports whether the account may act on a request it is
// authenticated for.
//
// It reads the repository and not the cache. The cached account lives twelve
// hours and an invalidation is best effort: a failed one would leave an
// account that was disabled working for half a day, on the one read that
// decides whether "disabled" means anything. It is one read by primary key
// for every authenticated request, of the one column it needs
// (SelectDisabled): it used to read the whole row, the password hash
// included.
func (ref *UsersService) Standing(ctx context.Context, id uuid.UUID) (domain.AccountStanding, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "Standing")
	defer span.End()

	span.SetAttributes(attribute.String("user.id", id.String()))

	disabled, err := ref.repository.SelectDisabled(ctx, id)
	if _, gone := errors.AsType[*domain.UserNotFoundError](err); gone {
		o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "the account is not there")

		return domain.AccountMissing, nil
	}

	if err != nil {
		return domain.AccountUnknown, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if disabled {
		o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "the account is disabled")

		return domain.AccountDisabled, nil
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "the account may act")

	return domain.AccountActive, nil
}

// Verify records that the account's address is proven: the end of the
// verification link, and its only writer.
//
// Following the link opens a registration, which is disabled for no other
// reason than that. An account an administrator disabled has the same two
// values (verified false, disabled true), so the row says which it is: the
// repository's statement enables only an account no administrator disabled,
// and never one with the `admin` flag; for those it proves the address and
// opens nothing, and the answer is the same.
//
// It is not UpdateByID with two fields: the decision is taken in the
// statement that writes, from the row as it is then. That goes for the
// address too: the link proves the address it was mailed to, and the
// statement writes only a row that still has it.
//
// A statement that wrote nothing says why, from the row and not from a
// cached copy, in the words the flow uses when its own copy says the same:
// the account is gone (UserNotFoundError), its address is proven already
// (UserAlreadyVerifiedError), or the address is not the account's any more
// (ErrVerifiedAddressIsNotTheAccounts).
func (ref *UsersService) Verify(ctx context.Context, id uuid.UUID, address string) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "Verify")
	defer span.End()

	span.SetAttributes(attribute.String("user.id", id.String()))

	if err := ref.repository.Verify(ctx, id, address); err != nil {
		if _, none := errors.AsType[*domain.UserNotFoundError](err); none {
			err = ref.whyNotVerified(ctx, id, err)
		}

		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// The cached account says "unverified, disabled" for twelve hours
	// otherwise, and the flows that read it would believe it.
	if ref.cacheService != nil {
		if err := ref.cacheService.Invalidate(ctx, cache.Identifier{Type: "user", ID: id.String()}); err != nil {
			slog.WarnContext(ctx, "failed to invalidate cache", slog.Any("error", err), "user_id", id)
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user verified successfully",
		attribute.String("user.id", id.String()))

	return nil
}

// whyNotVerified says why the verification's statement found no row to
// write: notFound, as the statement answered, when the account is gone.
func (ref *UsersService) whyNotVerified(ctx context.Context, id uuid.UUID, notFound error) error {
	stored, err := ref.repository.SelectByID(ctx, id)
	if err != nil {
		if _, gone := errors.AsType[*domain.UserNotFoundError](err); gone {
			return notFound
		}

		return err
	}

	if stored.Verified != nil && *stored.Verified {
		return &domain.UserAlreadyVerifiedError{UserID: id.String()}
	}

	return ErrVerifiedAddressIsNotTheAccounts
}

// ErrVerifiedAddressIsNotTheAccounts is what a verification answers when the
// address its link was mailed to is not the account's any more: the account
// is there, is not verified, and has another address. A link proves the
// address it went to and no other.
var ErrVerifiedAddressIsNotTheAccounts = errors.New("the address the link was mailed to is not the account's")

// ResetPassword writes the password a reset link was followed for, on the
// account the link was mailed to as it is now (the repository's statement:
// that id, that address, a local account, not disabled). No such account is
// a UserNotFoundError, whichever condition failed.
func (ref *UsersService) ResetPassword(ctx context.Context, id uuid.UUID, address, passwordHash string) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ResetPassword")
	defer span.End()

	span.SetAttributes(attribute.String("user.id", id.String()))

	if err := ref.repository.ResetPassword(ctx, id, address, passwordHash); err != nil {
		if _, none := errors.AsType[*domain.UserNotFoundError](err); none {
			ref.whyResetRefused(ctx, span, id, address)
		}

		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		if err := ref.cacheService.Invalidate(ctx, cache.Identifier{Type: "user", ID: id.String()}); err != nil {
			slog.WarnContext(ctx, "failed to invalidate cache", slog.Any("error", err), "user_id", id.String())
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "password reset",
		attribute.String("user.id", id.String()))

	return nil
}

// whyResetRefused says, on the span and at DEBUG, which condition of the
// reset's statement failed: the holder hears one 401 whatever it was, and an
// operator asked why a link does not work should not need the database. A
// read that fails says so and nothing else; the refusal stands either way.
func (ref *UsersService) whyResetRefused(ctx context.Context, span trace.Span, id uuid.UUID, address string) {
	reason := "the account is not there"

	stored, err := ref.repository.SelectByID(ctx, id)

	_, gone := errors.AsType[*domain.UserNotFoundError](err)

	switch {
	case err != nil && !gone:
		reason = "the account could not be read"
	case err != nil:
	case stored.Email != address:
		reason = "the account's address is not the one the link was mailed to"
	case stored.LocalAccount == nil || !*stored.LocalAccount:
		reason = "the account is not a local account"
	case stored.Disabled == nil || *stored.Disabled:
		reason = "the account is disabled"
	default:
		reason = "the account changed between the statement and this read"
	}

	span.SetAttributes(attribute.String("authn.reset.refused_reason", reason))
	slog.DebugContext(ctx, "reset link refused", "reason", reason)
}

// StoredAccount returns the account as the repository has it, without its
// credentials and never from the cache. GetByID is cached for twelve hours
// and its invalidation is best effort: where a reset link is mailed is not
// decided from it, or a failed invalidation after an address was corrected
// mails the previous one.
func (ref *UsersService) StoredAccount(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "StoredAccount")
	defer span.End()

	span.SetAttributes(attribute.String("user.id", id.String()))

	stored, err := ref.repository.SelectByID(ctx, id)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user found", attribute.String("user.id", id.String()))

	return withoutCredentials(stored), nil
}

func (ref *UsersService) UpdateByID(ctx context.Context, input *domain.UpdateUserInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "UpdateByID")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.id", input.ID.String()))

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// An address is not every caller's to change. It is judged against the
	// stored row, read only when the body carries one: the profile, the
	// verification and a password reset carry none.
	if input.Email != nil {
		if err := ref.addressMayChange(ctx, input); err != nil {
			return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	// Nor is the `admin` flag, nor an account's sign-in (disabled, the lock,
	// local_account) when the account has the flag. Judged against the
	// stored row too, read only when the body carries one of them.
	if input.Admin != nil || touchesSignIn(input) {
		stored, err := ref.repository.SelectByID(ctx, input.ID)
		if err != nil {
			return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		twoDirections := lockMayChange(input, stored)

		if err := ref.flagMayChange(ctx, input, stored); err != nil {
			return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		if err := ref.administratorSignInMayChange(ctx, input, stored); err != nil {
			return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		// Last: an account that may not change the sign-in at all hears
		// that, and nothing about what the row holds.
		if twoDirections != nil {
			return o11y.RecordError(ctx, span, start, twoDirections, ref.metrics, attrs)
		}
	}

	if input.Password != nil {
		if len(*input.Password) < domain.ValidUserPasswordMinLength || len(*input.Password) > domain.ValidUserPasswordMaxLength {
			errorValue := &domain.InvalidUserPasswordError{Message: fmt.Sprintf("password must be between %d and %d characters", domain.ValidUserPasswordMinLength, domain.ValidUserPasswordMaxLength)}
			return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
		}

		hashPwd, err := HashAndSaltPassword(*input.Password)
		if err != nil {
			return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		input.PasswordHash = &hashPwd
	}

	if err := ref.repository.UpdateByID(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.DebugContext(ctx, "invalidate cache", "user_id", input.ID)

		cacheKey := cache.Identifier{
			Type: "user",
			ID:   input.ID.String(),
		}

		if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
			slog.WarnContext(ctx, "failed to invalidate cache", slog.Any("error", err), "user_id", input.ID)
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user updated successfully",
		attribute.String("user.id", input.ID.String()))

	return nil
}

// errAddressChangeWithoutCaller is what a change of address answers when it
// does not know who asks: a route wired without its caller. It is no domain
// error, so the answer is the fixed 500.
var errAddressChangeWithoutCaller = errors.New("an account's e-mail address was asked to change with no caller")

// addressMayChange refuses a change of an account's e-mail address by anyone
// but the administrator account.
//
// An address is where a password reset is sent, so it is part of how the
// account is held. Only the administrator account, the one with the `admin`
// flag, changes an account's address. The
// flag, not the Administrator role: the role is a set of grants and can be
// given; the flag is given only by an account that has it (flagMayChange).
//
// An address that is the stored one is no change, and is let through: a
// client sends the whole form back. "The stored one" is the same bytes, or
// the stored address in the spelling the service gives every address
// (ValidateEmail, which the request has been through), for a row older than
// that spelling. Nothing looser: a comparison that folds case folds more
// than case, and the statement compares bytes.
//
// And an address that is no change is taken out of the input: the statement
// is never handed one by anybody but the administrator account. The check
// and the write are two statements, and what was "the stored address" at
// the check would be written back over a correction made in between.
//
// Both rows are read from the repository and not through the cache: the
// answer decides who may have the account.
//
// The administrator account is the exception to the spelling: it may write a
// row's address in the service's spelling, which is no change to anybody
// else. Such a row cannot sign in, since a sign-in looks the address up as
// the service spells it, and the account that may change an address was the
// one account that could not correct it: its form sends the address back
// spelled, and that was "the stored one". For that account only the same
// bytes are no change.
func (ref *UsersService) addressMayChange(ctx context.Context, input *domain.UpdateUserInput) error {
	stored, err := ref.repository.SelectByID(ctx, input.ID)
	if err != nil {
		return err
	}

	if stored.Email == *input.Email {
		input.Email = nil

		return nil
	}

	if sameAddress(stored.Email, *input.Email) {
		respells := false

		if input.CallerID != uuid.Nil() {
			asked, err := ref.callerIsAdministrator(ctx, input.CallerID)
			if err != nil {
				return err
			}

			respells = asked
		}

		if !respells {
			input.Email = nil
		}

		return nil
	}

	if input.CallerID == uuid.Nil() {
		return errAddressChangeWithoutCaller
	}

	asked, err := ref.callerIsAdministrator(ctx, input.CallerID)
	if err != nil {
		return err
	}

	if asked {
		return nil
	}

	return &domain.AdministratorOnlyError{Entity: "user", Field: domain.FieldEmail}
}

// callerIsAdministrator reports whether the caller is the administrator
// account: an account that is there, with the flag, and not disabled. An
// account that is not there, a row read without its flag, and an account
// that is disabled are not the administrator's: a disabled account does not sign
// in, and a token it still holds is not a reason to let it write an address.
func (ref *UsersService) callerIsAdministrator(ctx context.Context, callerID uuid.UUID) (bool, error) {
	caller, err := ref.repository.SelectByID(ctx, callerID)
	if _, gone := errors.AsType[*domain.UserNotFoundError](err); gone {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return hasAdminFlag(caller) && !isDisabled(caller), nil
}

// sameAddress reports whether sent is the stored address: byte for byte, or
// once the stored one is spelled the way the service spells every address.
func sameAddress(stored, sent string) bool {
	if stored == sent {
		return true
	}

	spelled, err := domain.ValidateEmail(stored, domain.FieldEmail)

	return err == nil && spelled == sent
}

// touchesSignIn reports whether the update carries a field that decides
// whether, or how, an account signs in. Whether it is a change is judged
// against the stored row (administratorSignInMayChange).
func touchesSignIn(input *domain.UpdateUserInput) bool {
	return input.Disabled != nil || input.DisabledByAdmin != nil || input.LocalAccount != nil
}

// flagMayChange refuses a change of the `admin` flag by anyone but an account
// that has it, and the change that would leave no such account able to sign
// in.
//
// The flag is the administrator account: every project, an account's
// address, another administrator account's sign-in. No route set it, so the
// account the migrations seed was the only one a deployment could have, and
// a deployment whose one administrator lost its mailbox or its password had
// no way back but the database.
//
// The flag is granted and taken back through PUT /users/{user_id}, by an
// account that has it and by nobody else. It is not a grant of the policy
// engine: whoever holds the route's grant, as the seeded UserManager role
// does, would otherwise make itself the administrator account with one
// request.
//
// What is refused is a change, not the field: a form sends back what it
// read, and `admin: false` for an account that never had the flag is no
// change, whoever sends it. It is taken out of the update, so that the
// statement is handed only what an administrator account changed.
//
// Taking the flag is the guard's (guardAdministrator): it takes the account
// out of the set that can do these things, as disabling it does, so the last
// one that can still sign in keeps it. The seeded account keeps it whoever
// asks: its row's trigger refuses, and the repository answers the same 403
// (fn_protect_bootstrap_administrator).
//
// The target's own standing is not asked about when the flag is granted. A
// disabled account has the flag and cannot use it, and is not counted as one
// that can sign in. A registration still waiting for its link is such an
// account, and with the flag its link proves the address and opens nothing
// (UsersRepository.Verify): an administrator account enables it, with
// `disabled: false`.
func (ref *UsersService) flagMayChange(ctx context.Context, input *domain.UpdateUserInput, stored *domain.User) error {
	if input.Admin == nil {
		return nil
	}

	if *input.Admin == hasAdminFlag(stored) {
		input.Admin = nil

		return nil
	}

	asked, err := isAdministratorAccount(ctx, ref.repository, input.CallerID)
	if err != nil {
		return err
	}

	if !asked {
		return &domain.AdministratorOnlyError{Entity: "user", Field: domain.FieldAdmin}
	}

	if *input.Admin {
		return nil
	}

	return guardAdministrator(ctx, ref.repository, stored, input.CallerID, administratorDemoted, true)
}

// lockMayChange takes out of the update what is no change of the lock or of
// `disabled`, and says whether the body pulls the account in two directions.
//
// `disabled` alone cannot say "an administrator shut this account" for a
// registration whose link is still out: the account is disabled already,
// the value is no change, and a form sends it back with every rename. So
// the lock is a field of its own (`disabled_by_admin`): true locks whatever
// `disabled` is, false unlocks.
//
// A client may send both. A value equal to the stored one is no change and
// is dropped, for either field: what is left is what the client changed.
//
// When the body carries the lock, a CHANGE of `disabled` beside it has to
// agree with the lock the body states, whether or not that lock is itself a
// change. A locked account that is told `disabled: false` with
// `disabled_by_admin: true` is told to be enabled and to stay locked, and
// cannot be both: with the lock dropped as "no change" first, the `disabled`
// that was left would enable the account and take the lock off, and the same
// body sent again would lock it, and so on. The mirror, `disabled: true`
// with `disabled_by_admin: false` on an enabled account, is "disabled, and
// not by an administrator", which no account an administrator disables is.
// Both are refused whole.
//
// The refusal is returned, not acted on: the caller asks who may change the
// account first (administratorSignInMayChange), so that an account that may
// not change it at all hears that, and nothing about what the row holds.
func lockMayChange(input *domain.UpdateUserInput, stored *domain.User) (twoDirections error) {
	if input.DisabledByAdmin == nil {
		return nil
	}

	lock := *input.DisabledByAdmin

	if input.Disabled != nil {
		if *input.Disabled == isDisabled(stored) {
			input.Disabled = nil
		} else if *input.Disabled != lock {
			twoDirections = &domain.InvalidInputError{
				Field:   domain.FieldDisabledByAdmin,
				Message: "disabled and disabled_by_admin change the account in two directions",
			}
		}
	}

	if lock == (stored.DisabledByAdmin != nil && *stored.DisabledByAdmin) {
		input.DisabledByAdmin = nil
	}

	return twoDirections
}

// administratorSignInMayChange refuses a change of `disabled`, of the lock
// (`disabled_by_admin`) or of `local_account` on an account with the `admin`
// flag, unless an account with the flag asks (guardAdministrator). In either
// direction: disabling or locking it, or making it a federated account,
// takes its sign-in away, and is refused for everybody on the last such
// account that can still sign in; enabling or unlocking it, or making it a
// local account again, undoes what an administrator did.
//
// What is refused is a change, not the field: a client sends the whole form
// back, `disabled: false` and the rest, with every edit, and a user manager
// renaming the administrator account sends what it read.
//
// And a value that is no change is taken out of the update, on an account
// with the flag: the check and the write are two statements, and "as it
// already is" from an account without the flag would undo what an
// administrator changed in between.
func (ref *UsersService) administratorSignInMayChange(ctx context.Context, input *domain.UpdateUserInput, stored *domain.User) error {
	if !hasAdminFlag(stored) {
		return nil
	}

	wasDisabled := isDisabled(stored)
	// An account read without its `local_account` has no password login:
	// the sign-in refuses it as it refuses a federated one.
	wasLocal := stored.LocalAccount != nil && *stored.LocalAccount

	if input.Disabled != nil {
		switch {
		case *input.Disabled == wasDisabled:
			input.Disabled = nil
		case *input.Disabled:
			if err := guardAdministrator(ctx, ref.repository, stored, input.CallerID, administratorDisabled, true); err != nil {
				return err
			}
		default:
			if err := guardAdministrator(ctx, ref.repository, stored, input.CallerID, administratorEnabled, false); err != nil {
				return err
			}
		}
	}

	// The lock, when it is here, is a change (lockMayChange): locking takes
	// the sign-in away as disabling does, and unlocking gives it back.
	if input.DisabledByAdmin != nil {
		action, locks := administratorEnabled, false
		if *input.DisabledByAdmin {
			action, locks = administratorDisabled, true
		}

		if err := guardAdministrator(ctx, ref.repository, stored, input.CallerID, action, locks); err != nil {
			return err
		}
	}

	if input.LocalAccount != nil {
		switch {
		case *input.LocalAccount == wasLocal:
			input.LocalAccount = nil
		case !*input.LocalAccount:
			if err := guardAdministrator(ctx, ref.repository, stored, input.CallerID, administratorFederated, true); err != nil {
				return err
			}
		default:
			if err := guardAdministrator(ctx, ref.repository, stored, input.CallerID, administratorMadeLocal, false); err != nil {
				return err
			}
		}
	}

	return nil
}

func hasAdminFlag(account *domain.User) bool {
	return account != nil && account.Admin != nil && *account.Admin
}

func isDisabled(account *domain.User) bool {
	return account != nil && account.Disabled != nil && *account.Disabled
}

func (ref *UsersService) DeleteByID(ctx context.Context, input *domain.DeleteUserInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "DeleteByID")
	defer span.End()

	span.SetAttributes(attribute.String("user.id", input.ID.String()))

	if input.ID == uuid.Nil() {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// An account with the `admin` flag is deleted only by one that has it,
	// and the last that can still sign in by nobody. An account that is not
	// there is the repository's to answer, as before.
	stored, err := ref.repository.SelectByID(ctx, input.ID)
	if _, missing := errors.AsType[*domain.UserNotFoundError](err); err != nil && !missing {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := guardAdministrator(ctx, ref.repository, stored, input.CallerID, administratorDeleted, true); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.DeleteByID(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// What is cached of it goes before anything that can still fail: the
	// row is gone, and a usage decrement that failed returned from here with
	// it still in the cache.
	if ref.cacheService != nil {
		slog.DebugContext(ctx, "invalidate cache", "user_id", input.ID)

		cacheKey := cache.Identifier{
			Type: "user",
			ID:   input.ID.String(),
		}

		if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
			slog.WarnContext(ctx, "failed to invalidate cache", slog.Any("error", err), "user_id", input.ID)
		}
	}

	// decrement resource usage
	systemScope := domain.ResourcesLimitsScope{
		Type: domain.ResourcesLimitsScopeTypeSystem,
	}

	if err := ref.resourcesLimits.DecrementUsage(ctx, systemScope, domain.ResourcesLimitsResourceTypeUsers); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user deleted successfully", attribute.String("user.id", input.ID.String()))

	return nil
}

func (ref *UsersService) List(ctx context.Context, input *domain.ListUsersInput) (*domain.ListUsersOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "List")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is required"}
		_ = o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
		return nil, errorValue
	}

	// Validated before anything of it reaches the span: an expression over
	// its length is refused there, and a span is exported.
	if err := input.Validate(); err != nil {
		_ = o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		return nil, err
	}

	span.SetAttributes(
		attribute.String("sort", input.Sort),
		attribute.String("fields", input.Fields),
		attribute.String("filter", input.Filter),
		attribute.Int("limit", input.Paginator.Limit),
	)

	out, err := ref.repository.Select(ctx, input)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "users listed successfully",
		attribute.Int("count", len(out.Items)))

	return out, nil
}

func (ref *UsersService) LinkRoles(ctx context.Context, input *domain.LinkRolesToUserInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "LinkRoles")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.id", input.UserID.String()))

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.guard.CheckRoles(ctx, input.CallerID, input.RoleIDs); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.LinkRoles(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.DebugContext(ctx, "invalidate cache", "user_id", input.UserID.String())

		cacheKeys := []cache.Identifier{
			{
				Type: "user",
				ID:   input.UserID.String(),
			},
			authzCacheKey(input.UserID),
		}

		for _, roleID := range input.RoleIDs {
			cacheKey := cache.Identifier{
				Type: "role",
				ID:   roleID.String(),
			}

			cacheKeys = append(cacheKeys, cacheKey)
		}

		for _, cacheKey := range cacheKeys {
			slog.DebugContext(ctx, "invalidate cache", "cache_type", cacheKey.Type, "cache_key", cacheKey.ID)

			if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
				slog.WarnContext(ctx, "failed to invalidate cache", slog.Any("error", err), "cache_type", cacheKey.Type, "cache_key", cacheKey.ID)
			}
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "roles linked successfully",
		attribute.String("user.id", input.UserID.String()))

	return nil
}

func (ref *UsersService) UnlinkRoles(ctx context.Context, input *domain.UnlinkRolesFromUsersInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "UnlinkRoles")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.id", input.UserID.String()))

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// An account with the `admin` flag is stripped of a role only by one
	// that has the flag. An account that is not there is the repository's
	// to answer, as before.
	stored, err := ref.repository.SelectByID(ctx, input.UserID)
	if _, missing := errors.AsType[*domain.UserNotFoundError](err); err != nil && !missing {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := guardAdministrator(ctx, ref.repository, stored, input.CallerID, administratorStripped, false); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.UnlinkRoles(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.DebugContext(ctx, "invalidate cache", "user_id", input.UserID.String())

		cacheKeys := []cache.Identifier{
			{
				Type: "user",
				ID:   input.UserID.String(),
			},
			authzCacheKey(input.UserID),
		}

		for _, roleID := range input.RoleIDs {
			cacheKey := cache.Identifier{
				Type: "role",
				ID:   roleID.String(),
			}

			cacheKeys = append(cacheKeys, cacheKey)
		}

		for _, cacheKey := range cacheKeys {
			slog.DebugContext(ctx, "invalidate cache", "cache_type", cacheKey.Type, "cache_key", cacheKey.ID)

			if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
				slog.WarnContext(ctx, "failed to invalidate cache", slog.Any("error", err), "cache_type", cacheKey.Type, "cache_key", cacheKey.ID)
			}
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "roles unlinked successfully",
		attribute.String("user.id", input.UserID.String()))

	return nil
}

func (ref *UsersService) LinkProjects(ctx context.Context, input *domain.LinkProjectsToUserInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "LinkProjects")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.id", input.UserID.String()))

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.LinkProjects(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.DebugContext(ctx, "invalidate cache", "user_id", input.UserID.String())

		cacheKeys := []cache.Identifier{
			{
				Type: "user",
				ID:   input.UserID.String(),
			},
			authzCacheKey(input.UserID),
		}

		for _, cacheKey := range cacheKeys {
			slog.DebugContext(ctx, "invalidate cache", "cache_type", cacheKey.Type, "cache_key", cacheKey.ID)

			if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
				slog.WarnContext(ctx, "failed to invalidate cache", slog.Any("error", err), "cache_type", cacheKey.Type, "cache_key", cacheKey.ID)
			}
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "projects linked successfully",
		attribute.String("user.id", input.UserID.String()))

	return nil
}

func (ref *UsersService) UnlinkProjects(ctx context.Context, input *domain.UnlinkProjectsFromUserInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "UnlinkProjects")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("user.id", input.UserID.String()))

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.UnlinkProjects(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.DebugContext(ctx, "invalidate cache", "user_id", input.UserID.String())

		cacheKeys := []cache.Identifier{
			{
				Type: "user",
				ID:   input.UserID.String(),
			},
			authzCacheKey(input.UserID),
		}

		for _, cacheKey := range cacheKeys {
			slog.DebugContext(ctx, "invalidate cache", "cache_type", cacheKey.Type, "cache_key", cacheKey.ID)

			if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
				slog.WarnContext(ctx, "failed to invalidate cache", slog.Any("error", err), "cache_type", cacheKey.Type, "cache_key", cacheKey.ID)
			}
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "projects unlinked successfully",
		attribute.String("user.id", input.UserID.String()))

	return nil
}

// ListByRoleID returns a list of users by role ID
func (ref *UsersService) ListByRoleID(ctx context.Context, roleID uuid.UUID, input *domain.ListUsersInput) (*domain.ListUsersOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ListByRoleID")
	defer span.End()

	span.SetAttributes(attribute.String("role.id", roleID.String()))

	if roleID == uuid.Nil() {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	out, err := ref.repository.SelectByRoleID(ctx, roleID, input)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "users found by role ID",
		attribute.String("role.id", roleID.String()),
		attribute.Int("count", len(out.Items)))

	return out, nil
}

// ListByIDPID returns a page of the accounts linked to an identity provider,
// each with whether the provider is its only way in: the accounts a 409 on
// the provider's delete is about.
func (ref *UsersService) ListByIDPID(ctx context.Context, idpID uuid.UUID, input *domain.ListUsersInput) (*domain.SelectIDPAccountsOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ListByIDPID")
	defer span.End()

	span.SetAttributes(attribute.String("idp.id", idpID.String()))

	if idpID == uuid.Nil() {
		return nil, o11y.RecordError(ctx, span, start, &domain.InvalidInputError{Field: "idp_id", Message: "invalid idp ID"}, ref.metrics, attrs)
	}

	if input == nil {
		return nil, o11y.RecordError(ctx, span, start, &domain.InvalidInputError{Message: "input is nil"}, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	out, err := ref.repository.SelectByIDPID(ctx, idpID, input)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "a provider's accounts listed",
		attribute.String("idp.id", idpID.String()),
		attribute.Int("count", len(out.Items)))

	return out, nil
}

// ListByProjectID returns a list of users by project ID
func (ref *UsersService) ListByProjectID(ctx context.Context, projectID uuid.UUID, input *domain.ListUsersInput) (*domain.ListUsersOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ListByProjectID")
	defer span.End()

	span.SetAttributes(attribute.String("project.id", projectID.String()))

	if projectID == uuid.Nil() {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	out, err := ref.repository.SelectByProjectID(ctx, projectID, input)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "users found by project ID",
		attribute.String("project.id", projectID.String()),
		attribute.Int("count", len(out.Items)))

	return out, nil
}

// SelectAuthz retrieves the authorization data for a user.
func (ref *UsersService) SelectAuthz(ctx context.Context, userID uuid.UUID) (map[string]any, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "SelectAuthz")
	defer span.End()

	span.SetAttributes(attribute.String("user.id", userID.String()))

	if userID == uuid.Nil() {
		errorType := &domain.InvalidUserIDError{Message: "user ID cannot be empty"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	var userAuthPermissions map[string]any
	var err error

	// userAuthFetcher is a function that fetches the user authorization data from the repository.
	userAuthFetcher := func(ctx context.Context) (map[string]any, []cache.Identifier, error) {
		authz, err := ref.repository.SelectAuthz(ctx, userID)
		if err != nil {
			return nil, nil, err
		}

		dependencies := make([]cache.Identifier, 0, len(authz.Roles)+len(authz.Policies)+1)

		// add user dependency
		dependencies = append(dependencies, cache.Identifier{Type: "user", ID: userID.String()})

		// add role dependencies
		for _, roleID := range authz.Roles {
			dependencies = append(dependencies, cache.Identifier{Type: "role", ID: roleID})
		}

		// add policy dependencies
		for _, policyID := range authz.Policies {
			dependencies = append(dependencies, cache.Identifier{Type: "policy", ID: policyID})
		}

		return authz.Permissions, dependencies, nil
	}

	// Get the user Authz from the database, cache is disabled
	if ref.cacheService == nil {
		slog.DebugContext(ctx, "cache lookup", "cache", "disabled")

		userAuthPermissions, _, err = userAuthFetcher(ctx)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	} else {

		cacheKey := authzCacheKey(userID)

		slog.DebugContext(ctx, "cache lookup", "cache", "enabled", "key", cacheKey)

		userAuthPermissions, err = cache.GetTyped[map[string]any](ctx, ref.cacheService, cacheKey, userAuthFetcher)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user authz found successfully",
		attribute.String("user.id", userID.String()))

	return userAuthPermissions, nil
}
