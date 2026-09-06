package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"uuid"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/cache"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/cipher"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

type IDPsServiceConf struct {
	Repository repository.IDPs

	// IDPTypes answers which KIND a provider is, which decides whether an
	// issuer is required. Without it an oidc row with no issuer is accepted and
	// fails at the first sign-in, when the operator is no longer looking.
	IDPTypes        repository.IDPTypes
	CacheService    cache.Cache
	Cipher          cipher.Cipher
	ResourcesLimits ResourcesLimitsServiceConsumer
	OT              *o11y.OpenTelemetry
	MetricsPrefix   string
}

type IDPsService struct {
	repository      repository.IDPs
	idpTypes        repository.IDPTypes
	cacheService    cache.Cache
	cipher          cipher.Cipher
	resourcesLimits ResourcesLimitsServiceConsumer
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

func NewIDPsService(conf IDPsServiceConf) (*IDPsService, error) {
	if conf.Repository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "Repository is nil, but it is required for IDPsService"}
	}

	if conf.IDPTypes == nil {
		return nil, &domain.InvalidRepositoryError{Message: "IDPTypes is nil, but it is required for IDPsService"}
	}

	if conf.Cipher == nil {
		return nil, &domain.InvalidCipherError{Message: "Cipher is nil, but it is required for IDPsService"}
	}

	if conf.ResourcesLimits == nil {
		return nil, &domain.InvalidResourcesLimitsError{Message: "ResourcesLimits is nil, but it is required for IDPsService"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for IDPsService"}
	}

	ref := &IDPsService{
		repository:      conf.Repository,
		idpTypes:        conf.IDPTypes,
		cacheService:    conf.CacheService,
		cipher:          conf.Cipher,
		resourcesLimits: conf.ResourcesLimits,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "IDPs",
			Action: "NewIDPsService",
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
		metric.WithDescription(fmt.Sprintf("Duration of %s handler calls", AppLayer)),
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

// idpCollection identifies the *set* of IdPs, as opposed to any individual one.
//
// GetAvailableIDPs caches the whole list under one key and derives that entry's
// cache dependencies from the IdPs the list happens to contain. That makes
// update and delete work — both invalidate idp:<id>, which is in the dependency
// set, so the cascade reaches the list — but it cannot express insertion. A
// newly created IdP is by definition absent from the cached list's
// dependencies, so nothing links the two and the new provider stays invisible
// on the login page until the entry's hard TTL expires, up to 12 hours later.
//
// Depending on this identifier as well gives every write one thing to
// invalidate, whether it added, changed or removed a member. Update and delete
// invalidate it too rather than relying on the member cascade, because a
// reverse-dependency set can expire before the entry that depends on it.
func idpCollection() cache.Identifier {
	return cache.Identifier{Type: "idp_collection", ID: "all"}
}

func (ref *IDPsService) GetByID(ctx context.Context, id uuid.UUID) (*domain.IDP, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetByID")
	defer span.End()

	if !domain.IsUUIDV7(id) {
		errorType := &domain.InvalidInputError{Message: "IDP ID cannot be empty"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("idp.id", id.String()))

	var out *domain.IDP
	var err error

	idpFetcher := func(ctx context.Context) (*domain.IDP, []cache.Identifier, error) {
		out, err := ref.repository.SelectByID(ctx, id)
		if err != nil {
			return nil, nil, err
		}

		dependencies := []cache.Identifier{
			{
				Type: "idp_type",
				ID:   out.IDPType.ID.String(),
			},
		}

		return out, dependencies, nil
	}

	if ref.cacheService == nil {
		slog.Debug("service.IDPs.GetByID", "cache", "disabled")
		out, err = ref.repository.SelectByID(ctx, id)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	} else {
		cacheKey := cache.Identifier{
			Type: "idp",
			ID:   id.String(),
		}

		slog.Debug("service.IDPs.GetByID", "cache", "enabled", "idp_id", id.String())
		out, err = cache.GetTyped[*domain.IDP](ctx, ref.cacheService, cacheKey, idpFetcher)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	bytesClientSecret, err := ref.cipher.DecryptString(out.ClientSecret)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}
	out.ClientSecret = string(bytesClientSecret)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "IDP found", attribute.String("idp.name", out.Name))

	return out, nil
}

func (ref *IDPsService) GetByName(ctx context.Context, name string) (*domain.IDP, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetByName")
	defer span.End()

	if name == "" {
		errorValue := &domain.InvalidInputError{Message: "name is empty"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("idp.name", name))

	var out *domain.IDP
	var err error

	idpFetcher := func(ctx context.Context) (*domain.IDP, []cache.Identifier, error) {
		out, err := ref.repository.SelectByName(ctx, name)
		if err != nil {
			return nil, nil, err
		}

		dependencies := []cache.Identifier{
			{
				Type: "idp_type",
				ID:   out.IDPType.ID.String(),
			},
			{
				Type: "idp",
				ID:   out.ID.String(),
			},
		}

		return out, dependencies, nil
	}

	if ref.cacheService == nil {
		slog.Debug("service.IDPs.GetByName", "cache", "disabled")
		out, err = ref.repository.SelectByName(ctx, name)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	} else {
		cacheKey := cache.Identifier{
			Type: "idp",
			ID:   name,
		}

		slog.Debug("service.IDPs.GetByName", "cache", "enabled", "idp_name", name)
		out, err = cache.GetTyped[*domain.IDP](ctx, ref.cacheService, cacheKey, idpFetcher)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	bytesClientSecret, err := ref.cipher.DecryptString(out.ClientSecret)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}
	out.ClientSecret = string(bytesClientSecret)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "IDP found", attribute.String("idp.name", out.Name))

	return out, nil
}

func (ref *IDPsService) Create(ctx context.Context, input *domain.CreateIDPInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "Create")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("idp.name", input.Name))

	var err error
	input.ID, err = domain.EnsureUUIDV7(input.ID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.requireIssuerForKind(ctx, input.IDPTypeID, input.IssuerURL); err != nil {
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
		ResourceType:    domain.ResourcesLimitsResourceTypeIDPs,
	}

	// Reserve the slot before creating, and give it back if anything after this
	// point fails.
	if err := ReserveResourceSlot(resourceLimitParams); err != nil {
		return err
	}

	cypherText, err := ref.cipher.EncryptString([]byte(input.ClientSecret))
	if err != nil {
		ReleaseResourceSlot(resourceLimitParams)
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}
	input.ClientSecret = cypherText

	if err := ref.repository.Insert(ctx, input); err != nil {
		ReleaseResourceSlot(resourceLimitParams)
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	WarnOnSoftLimit(resourceLimitParams)

	if ref.cacheService != nil {
		// Only the collection: this IdP has no cache entry of its own yet, and
		// nothing can be depending on an id that did not exist a moment ago.
		slog.Debug("service.IDPs.Create", "what", "invalidate cache", "id", idpCollection().String())

		if err := ref.cacheService.Invalidate(ctx, idpCollection()); err != nil {
			slog.Warn("service.IDPs.Create", "what", "failed to invalidate cache",
				slog.Any("error", err), "idp_id", input.ID.String())
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "IDP created successfully",
		attribute.String("idp.name", input.Name),
		attribute.String("idp.id", input.ID.String()))

	return nil
}

func (ref *IDPsService) UpdateByID(ctx context.Context, input *domain.UpdateIDPInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "UpdateByID")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("idp.id", input.ID.String()))

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// The kind and the issuer after this update, whichever of the two moved:
	// a row changing to the oidc kind needs an issuer, and an oidc row may not
	// clear its issuer.
	if input.IDPTypeID != nil || input.IssuerURL != nil {
		current, err := ref.repository.SelectByID(ctx, input.ID)
		if err != nil {
			return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		typeID, issuer := current.IDPType.ID, current.IssuerURL

		if input.IDPTypeID != nil {
			typeID = *input.IDPTypeID
		}

		if input.IssuerURL != nil {
			issuer = *input.IssuerURL
		}

		if err := ref.requireIssuerForKind(ctx, typeID, issuer); err != nil {
			return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	if input.ClientSecret != nil && *input.ClientSecret != "" {
		cypherText, err := ref.cipher.EncryptString([]byte(*input.ClientSecret))
		if err != nil {
			return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
		*input.ClientSecret = cypherText
	}

	if err := ref.repository.UpdateByID(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.Debug("service.IDPs.UpdateByID", "what", "invalidate cache", "idp_id", input.ID.String())

		cacheKeys := []cache.Identifier{
			{
				Type: "idp",
				ID:   input.ID.String(),
			},
			idpCollection(),
		}

		for _, cacheKey := range cacheKeys {
			if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
				slog.Warn("service.IDPs.UpdateByID", "what", "failed to invalidate cache",
					"key", cacheKey.String(), "idp_id", input.ID.String(), "error", err)
			}
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "IDP updated successfully",
		attribute.String("idp.id", input.ID.String()))

	return nil
}

func (ref *IDPsService) DeleteByID(ctx context.Context, input *domain.DeleteIDPInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "DeleteByID")
	defer span.End()

	span.SetAttributes(attribute.String("idp.id", input.ID.String()))

	if input.ID == uuid.Nil() {
		errorValue := &domain.InvalidInputError{Message: "input is nil"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if err := ref.repository.DeleteByID(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// decrement resource usage
	systemScope := domain.ResourcesLimitsScope{
		Type: domain.ResourcesLimitsScopeTypeSystem,
	}

	if err := ref.resourcesLimits.DecrementUsage(ctx, systemScope, domain.ResourcesLimitsResourceTypeIDPs); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if ref.cacheService != nil {
		slog.Debug("service.IDPs.DeleteByID", "what", "invalidate cache", "idp_id", input.ID.String())

		cacheKeys := []cache.Identifier{
			{
				Type: "idp",
				ID:   input.ID.String(),
			},
			idpCollection(),
		}

		for _, cacheKey := range cacheKeys {
			if err := ref.cacheService.Invalidate(ctx, cacheKey); err != nil {
				slog.Warn("service.IDPs.DeleteByID", "what", "failed to invalidate cache",
					"key", cacheKey.String(), "idp_id", input.ID.String(), "error", err)
			}
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "IDP deleted successfully", attribute.String("idp.id", input.ID.String()))

	return nil
}

func (ref *IDPsService) List(ctx context.Context, input *domain.ListIDPsInput) (*domain.ListIDPsOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "List")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is required"}
		_ = o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
		return nil, errorValue
	}

	span.SetAttributes(
		attribute.String("sort", input.Sort),
		attribute.String("fields", input.Fields),
		attribute.String("filter", input.Filter),
		attribute.Int("limit", input.Paginator.Limit),
	)

	out, err := ref.repository.Select(ctx, input)
	if err != nil {
		_ = o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		return nil, err
	}

	// The secret is never part of a listing, so it is not decrypted here: the
	// only reader of the clear text is the adapter, through GetByID. It used to
	// decrypt every row and the handler then dropped the field.
	for i := range out.Items {
		out.Items[i].ClientSecret = ""
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "IDPs listed successfully",
		attribute.Int("count", len(out.Items)))

	return out, nil
}

func (ref *IDPsService) GetAvailableIDPs(ctx context.Context) (*domain.SelectIDPAvailableOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetAvailableIDPs")
	defer span.End()

	input := &domain.SelectIDPsInput{
		Sort:   "name ASC",
		Filter: "",
		Fields: "",
		Paginator: domain.Paginator{
			Limit: 100,
		},
	}

	var out *domain.ListIDPsOutput
	var err error

	idpsFetcher := func(ctx context.Context) (*domain.ListIDPsOutput, []cache.Identifier, error) {
		out, err := ref.repository.Select(ctx, input)
		if err != nil {
			return nil, nil, err
		}

		// The collection identifier comes first and is always present, even
		// for an empty list — that is what lets a create invalidate this
		// entry. The per-member dependencies stay so a change to one IdP
		// still cascades here without the writer having to know about it.
		dependencies := make([]cache.Identifier, 0, len(out.Items)*2+1)
		dependencies = append(dependencies, idpCollection())

		for _, idp := range out.Items {
			dependencies = append(dependencies, cache.Identifier{
				Type: "idp",
				ID:   idp.ID.String(),
			})
			dependencies = append(dependencies, cache.Identifier{
				Type: "idp_type",
				ID:   idp.IDPType.ID.String(),
			})
		}

		return out, dependencies, nil
	}

	if ref.cacheService == nil {
		slog.Debug("service.IDPs.GetAvailableIDPs", "cache", "disabled")

		out, err = ref.repository.Select(ctx, input)
		if err != nil {
			_ = o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			return nil, err
		}
	} else {
		cacheKey := cache.Identifier{
			Type: "idps_available",
			ID:   "all",
		}
		slog.Debug("service.IDPs.GetAvailableIDPs", "cache", "enabled")
		out, err = cache.GetTyped[*domain.ListIDPsOutput](ctx, ref.cacheService, cacheKey, idpsFetcher)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}
	}

	idps := &domain.SelectIDPAvailableOutput{
		Items: make([]domain.IDPAvailable, 0, len(out.Items)),
	}

	for _, idp := range out.Items {
		// A disabled provider stays configured and listed to admins; it is not
		// offered on the login page.
		if !idp.Enabled {
			continue
		}

		idps.Items = append(idps.Items, domain.IDPAvailable{
			AutoProvision: idp.AutoProvision,
			ID:            idp.ID,
			Name:          idp.Name,
			Description:   idp.Description,
			Logo:          idp.Logo,
			IDPType: domain.IDPTypes{
				ID:          idp.IDPType.ID,
				Name:        idp.IDPType.Name,
				Description: idp.IDPType.Description,
			},
		})
	}

	return idps, nil
}

// requireIssuerForKind enforces the one rule that needs the type row: an oidc
// provider must name its issuer, because discovery, the token endpoint and the
// ID token's iss check all come from it. A github row carries none.
func (ref *IDPsService) requireIssuerForKind(ctx context.Context, typeID uuid.UUID, issuer string) error {
	idpType, err := ref.idpTypes.SelectByID(ctx, typeID)
	if err != nil {
		return err
	}

	var errs domain.ValidationErrors

	switch idpType.Kind {
	case domain.IDPTypeKindOIDC:
		if issuer == "" {
			errs.AddError(domain.FieldIssuerURL, "an "+string(idpType.Kind)+" provider needs its issuer URL; "+idpType.Name+" expects "+idpType.IssuerHint, "REQUIRED")
		}
	case domain.IDPTypeKindGithub:
		if issuer != "" {
			errs.AddError(domain.FieldIssuerURL, "a "+string(idpType.Kind)+" provider has no issuer; leave it empty", "NOT_APPLICABLE")
		}
	default:
		errs.AddError(domain.FieldIDPTypeID, "the provider type has an unknown kind "+string(idpType.Kind), "INVALID")
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}
