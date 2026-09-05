package usecase

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"uuid"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

//go:generate go tool mockgen -package=mocks -destination=../../../mocks/service/resources_limits_consumer.go -source=resources_limits.go ResourcesLimitsServiceConsumer

// ResourcesLimitsServiceConsumer is the interface for consumers would implement limits
type ResourcesLimitsServiceConsumer interface {
	// ReserveUsage claims a slot before the resource is created. Prefer this
	// over CheckUsage + IncrementUsage on any write path: the pair is not
	// atomic and concurrent callers can walk straight through a hard limit.
	ReserveUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType) error

	IncrementUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType) error
	DecrementUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType) error

	// CheckUsage reports the current status. It is a read: use it for status
	// endpoints, never to gate a creation.
	CheckUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType) (*domain.ResourcesLimitsStatus, error)

	// Generic method to ensure limits exist for any scope
	// EnsureLimits(ctx context.Context, scope domain.ResourcesLimitsScope) error
}

type ResourcesLimitsServiceConf struct {
	Repository    repository.ResourcesLimits
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
	PrivateKey    []byte
	PublicKey     []byte
}

type ResourcesLimitsService struct {
	repository      repository.ResourcesLimits
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	driftCounter    metric.Int64Counter
	tamperCounter   metric.Int64Counter
	signingKey      *ecdsa.PrivateKey
	verificationKey *ecdsa.PublicKey
	metricsMetadata o11y.Metadata
	metricsPrefix   string
	privateKey      []byte
	publicKey       []byte
}

// NewResourcesLimitsService creates a new ResourcesLimitsService.
func NewResourcesLimitsService(conf ResourcesLimitsServiceConf) (*ResourcesLimitsService, error) {
	if conf.Repository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "Repository is nil, but it is required for ResourcesLimitsService"}
	}

	// Emptiness only. The length bound that used to be here -- between 3 and
	// 255 bytes -- was a bound on a file PATH, borrowed and applied to the PEM
	// bytes of a key, and it was both redundant and dangerous: decodeECDSAKey
	// below is the real gate, and a P-256 PEM is already 227 bytes, so 28 bytes
	// of slack stood between a valid key and a rejection that had nothing to do
	// with the key. A pre-check that can refuse something valid while admitting
	// garbage is worse than no pre-check.
	if len(conf.PrivateKey) == 0 {
		return nil, &domain.InvalidPrivateKeyError{
			Message: "PrivateKey is empty, but it is required for ResourcesLimitsService",
		}
	}

	// Emptiness only, for the same reason as the private key above.
	if len(conf.PublicKey) == 0 {
		return nil, &domain.InvalidPublicKeyError{
			Message: "PublicKey is empty, but it is required for ResourcesLimitsService",
		}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for ResourcesLimitsService"}
	}

	// Parse the PEM once. These used to be decoded on every sign and every
	// verify, which meant two parses per resource creation on a path that
	// already holds a database row lock.
	signingKey, err := decodeECDSAPrivateKey(conf.PrivateKey)
	if err != nil {
		return nil, err
	}

	verificationKey, err := decodeECDSAPublicKey(conf.PublicKey)
	if err != nil {
		return nil, err
	}

	ref := &ResourcesLimitsService{
		repository:      conf.Repository,
		privateKey:      conf.PrivateKey,
		publicKey:       conf.PublicKey,
		signingKey:      signingKey,
		verificationKey: verificationKey,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "ResourcesLimits",
			Action: "NewResourcesLimitsService",
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

	// Drift is worth its own instrument rather than a log line. A counter that
	// is repeatedly wrong points at a code path mutating resources without going
	// through the service — a bug to find, not a number to keep fixing.
	driftCounter, err := ref.ot.Metrics.Meter.Int64Counter(
		fmt.Sprintf("%sresources_limits_drift_corrected", ref.metricsPrefix),
		metric.WithDescription("Total units of resource-usage drift corrected by reconciliation. Positive means the stored counter was too high."),
	)
	if err != nil {
		return nil, err
	}

	ref.driftCounter = driftCounter

	tamperCounter, err := ref.ot.Metrics.Meter.Int64Counter(
		fmt.Sprintf("%sresources_limits_signature_invalid", ref.metricsPrefix),
		metric.WithDescription("Counters whose stored signature did not verify. Any value above zero warrants investigation."),
	)
	if err != nil {
		return nil, err
	}

	ref.tamperCounter = tamperCounter

	return ref, nil
}

func (ref *ResourcesLimitsService) List(ctx context.Context, input *domain.ListResourcesLimitsInput) (*domain.ListResourcesLimitsOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "List")
	defer span.End()

	if input == nil {
		errorValue := &domain.InvalidInputError{Message: "input is required"}
		o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
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
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "resources limits listed successfully",
		attribute.Int("count", len(out.Items)))

	return out, nil
}

// CheckUsage checks the current usage against the limits for a given scope and resource type.
func (ref *ResourcesLimitsService) CheckUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType) (*domain.ResourcesLimitsStatus, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "CheckUsage")
	defer span.End()

	if scope.Type == "" {
		errorType := &domain.InvalidScopeTypeError{Message: "invalid scope type. It must not be empty"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	if resourceType == "" {
		errorType := &domain.InvalidResourceTypeError{Message: "invalid resource type. It must not be empty"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	// Check the usage in the repository
	check, err := ref.repository.CheckUsage(ctx, scope, resourceType)
	if err != nil {
		return nil, err
	}

	slog.Debug("service.ResourcesLimits.CheckUsage", "scopeType", scope.Type, "scopeID", scope.ID, "resourceType", resourceType, "usage", check.Usage, "softLimit", check.SoftLimit, "hardLimit", check.HardLimit)

	// Verify whenever a counter exists, not merely when it is above zero. The
	// old `Usage > 0` condition skipped verification for exactly the value an
	// attacker would write.
	//
	// A failure does **not** fail this call. CheckUsage is a read, and a tenant
	// must not lose sight of their own data because a row is bad; creation is
	// refused independently inside ReserveUsage, which verifies under the row
	// lock. Reporting it as a flag keeps that distinction — and hard-failing
	// here is what turned one racy write into a tenant-wide outage before.
	var tampered bool
	if check.HasUsageRow {
		if err := ref.verifySignature(check.Signature, scope, resourceType, check.Usage); err != nil {
			tampered = true

			scopeID := uuid.Nil()
			if scope.ID != nil {
				scopeID = *scope.ID
			}

			slog.Error(
				"resource usage counter failed signature verification; writes for this scope will be refused until it is reconciled",
				"scope_type", scope.Type,
				"scope_id", scopeID,
				"resource_type", resourceType,
				"usage", check.Usage,
			)

			ref.tamperCounter.Add(ctx, 1, metric.WithAttributes(
				attribute.String("scope_type", scope.Type.String()),
				attribute.String("resource_type", resourceType.String()),
			))
		}
	}

	var status *domain.ResourcesLimitsStatus
	if check.HardLimit == domain.ResourcesLimitsUnlimited || check.SoftLimit == domain.ResourcesLimitsUnlimited || check.Usage == domain.ResourcesLimitsUnlimited {
		// No limit is configured for this scope.
		//
		// The sentinel is passed through as-is rather than dressed up as
		// 1,000,000. Inventing a number told every caller — including the
		// frontend — that a ceiling existed at a value nothing in the system
		// had ever agreed to, and reporting CurrentUsage as 0 discarded the
		// tenant's real count. "No limit configured" is a distinct answer from
		// "your limit is one million", and only the caller can decide how to
		// show it.
		status = &domain.ResourcesLimitsStatus{
			CanCreate:        true,
			SoftLimitReached: false,
			SoftLimit:        domain.ResourcesLimitsUnlimited,
			HardLimit:        domain.ResourcesLimitsUnlimited,
			CurrentUsage:     max(check.Usage, 0),
		}
	} else {
		status = &domain.ResourcesLimitsStatus{
			CanCreate:        check.Usage < check.HardLimit,
			SoftLimitReached: check.Usage >= check.SoftLimit && check.Usage < check.HardLimit,
			SoftLimit:        check.SoftLimit,
			HardLimit:        check.HardLimit,
			CurrentUsage:     check.Usage,
		}
	}

	// A tampered counter cannot be trusted to say anything about capacity, so
	// report it as unable to create regardless of the numbers. The reservation
	// refuses independently; this keeps a status endpoint from telling a caller
	// they have room when the write will be rejected.
	status.TamperDetected = tampered
	if tampered {
		status.CanCreate = false
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "service.ResourcesLimits.CheckUsage")

	return status, nil
}

// StatusByScope reports every limit that applies to a scope, together with what
// the scope has consumed against it.
//
// The resource types come from the scope type rather than the caller, so a
// caller cannot ask a user scope about a project's models, and cannot be told
// about a resource that scope does not own.
//
// It is a read. A counter whose signature does not verify is reported with
// TamperDetected set rather than failing the call — see [ResourcesLimitsService.CheckUsage].
func (ref *ResourcesLimitsService) StatusByScope(ctx context.Context, scope domain.ResourcesLimitsScope) (*domain.ResourcesLimitsScopeStatus, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "StatusByScope")
	defer span.End()

	resourceTypes := domain.ResourceTypesForScope(scope.Type)
	if len(resourceTypes) == 0 {
		errorType := &domain.InvalidScopeTypeError{
			Message: fmt.Sprintf("unknown scope type %q", scope.Type),
		}

		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	out := &domain.ResourcesLimitsScopeStatus{
		ScopeType: scope.Type,
		Resources: make([]domain.ResourcesLimitsResourceStatus, 0, len(resourceTypes)),
	}

	if scope.ID != nil {
		out.ScopeID = *scope.ID
	}

	for _, resourceType := range resourceTypes {
		status, err := ref.CheckUsage(ctx, scope, resourceType)
		if err != nil {
			return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		out.Resources = append(out.Resources, domain.ResourcesLimitsResourceStatus{
			ResourceType: resourceType,
			Status:       *status,
		})
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "service.ResourcesLimits.StatusByScope",
		attribute.String("scope_type", scope.Type.String()),
		attribute.Int("resources", len(out.Resources)))

	return out, nil
}

// ReserveUsage claims one unit of a resource before it is created, or returns
// [domain.ResourcesLimitsHardLimitReachedError] when the hard limit is reached.
//
// This replaces the old check-then-create-then-increment sequence. Those were
// three separate statements with no lock between them, so concurrent callers
// could all pass the same check and overshoot the limit. Everything now happens
// under one row lock inside the repository.
//
// Release the reservation with [ResourcesLimitsService.DecrementUsage] if the
// creation that follows fails.
func (ref *ResourcesLimitsService) ReserveUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ReserveUsage")
	defer span.End()

	if scope.Type == "" {
		errorType := &domain.InvalidScopeTypeError{Message: "invalid scope type. It must not be empty"}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	if resourceType == "" {
		errorType := &domain.InvalidResourceTypeError{Message: "invalid resource type. It must not be empty"}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	newUsage, err := ref.repository.ReserveUsage(
		ctx, scope, resourceType,
		func(usage int, signature []byte) error {
			return ref.verifySignature(signature, scope, resourceType, usage)
		},
		func(newUsage int) ([]byte, error) {
			return ref.createSignature(scope, resourceType, newUsage)
		},
	)
	if err != nil {
		return err
	}

	slog.Debug("service.ResourcesLimits.ReserveUsage",
		"scopeType", scope.Type, "scopeID", scope.ID, "resourceType", resourceType, "usage", newUsage)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "service.ResourcesLimits.ReserveUsage")

	return nil
}

// RecountUsage repairs one counter by recomputing it from the resources it is
// supposed to be counting, and re-signing the result.
//
// This is the only recovery path from drift. The counter is a second source of
// truth and drifts one way — upward, toward refusing creations the tenant is
// entitled to — whenever a resource is removed outside the service's delete
// path. Because the value is signed, it cannot be corrected by hand: a
// hand-written counter fails verification and leaves the scope worse off than
// before.
func (ref *ResourcesLimitsService) RecountUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType) (*domain.ResourcesLimitsRecountOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "RecountUsage")
	defer span.End()

	if scope.Type == "" {
		errorType := &domain.InvalidScopeTypeError{Message: "invalid scope type. It must not be empty"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	if resourceType == "" {
		errorType := &domain.InvalidResourceTypeError{Message: "invalid resource type. It must not be empty"}
		return nil, o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	out, err := ref.repository.RecountUsage(ctx, scope, resourceType, func(newUsage int) ([]byte, error) {
		return ref.createSignature(scope, resourceType, newUsage)
	})
	if err != nil {
		return nil, err
	}

	if out.Corrected() {
		scopeID := uuid.Nil()
		if scope.ID != nil {
			scopeID = *scope.ID
		}

		// Warn, not info: a corrected counter means something changed resources
		// without telling the limits subsystem, and the size of the drift is the
		// clue to which path did it.
		slog.Warn(
			"resource usage counter corrected by reconciliation",
			"scope_type", scope.Type,
			"scope_id", scopeID,
			"resource_type", resourceType,
			"previous", out.Previous,
			"actual", out.Actual,
			"drift", out.Drift(),
		)

		ref.driftCounter.Add(ctx, int64(out.Drift()), metric.WithAttributes(
			attribute.String("scope_type", scope.Type.String()),
			attribute.String("resource_type", resourceType.String()),
		))
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "service.ResourcesLimits.RecountUsage",
		attribute.Int("drift", out.Drift()))

	return out, nil
}

// ReconcileAll recounts every scope that currently has a counter and reports
// how many were wrong.
//
// It walks `resources_usage` rather than the tenant list, so it needs no
// knowledge of users or projects and repairs exactly the counters that exist.
// A scope whose recount fails is logged and skipped: one broken scope must not
// stop the rest being repaired, and this typically runs at startup.
func (ref *ResourcesLimitsService) ReconcileAll(ctx context.Context) (corrected int, err error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ReconcileAll")
	defer span.End()

	scopes, err := ref.repository.SelectTrackedScopes(ctx)
	if err != nil {
		return 0, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	for _, tracked := range scopes {
		scope := domain.ResourcesLimitsScope{Type: tracked.ScopeType}
		if tracked.ScopeID != uuid.Nil() {
			scope.ID = &tracked.ScopeID
		}

		out, recountErr := ref.RecountUsage(ctx, scope, tracked.ResourceType)
		if recountErr != nil {
			slog.Error(
				"reconciliation skipped a scope",
				"scope_type", tracked.ScopeType,
				"scope_id", tracked.ScopeID,
				"resource_type", tracked.ResourceType,
				"error", recountErr,
			)

			continue
		}

		if out.Corrected() {
			corrected++
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "service.ResourcesLimits.ReconcileAll",
		attribute.Int("scopes", len(scopes)), attribute.Int("corrected", corrected))

	return corrected, nil
}

// IncrementUsage increments the resource usage count after successful creation.
func (ref *ResourcesLimitsService) IncrementUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "IncrementUsage")
	defer span.End()

	if resourceType == "" {
		errorType := &domain.InvalidResourceTypeError{Message: "invalid resource type. It must not be empty"}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	// The counter and its signature are written in one transaction: the
	// repository calls back into createSignature with the value it just wrote,
	// while still holding the row lock.
	if _, err := ref.repository.IncrementUsage(ctx, scope, resourceType, func(newUsage int) ([]byte, error) {
		return ref.createSignature(scope, resourceType, newUsage)
	}); err != nil {
		return err
	}

	// Record the usage in the metrics
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "service.ResourcesLimits.IncrementUsage")

	return nil
}

// DecrementUsage decrements the resource usage count after successful deletion.
func (ref *ResourcesLimitsService) DecrementUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "DecrementUsage")
	defer span.End()

	if resourceType == "" {
		errorType := &domain.InvalidResourceTypeError{Message: "invalid resource type. It must not be empty"}
		return o11y.RecordError(ctx, span, start, errorType, ref.metrics, attrs)
	}

	// One transaction, as with IncrementUsage. Re-reading the counter afterwards
	// to sign it — which is what this used to do — races with any concurrent
	// writer and can store a signature for a value the row no longer holds.
	if _, err := ref.repository.DecrementUsage(ctx, scope, resourceType, func(newUsage int) ([]byte, error) {
		return ref.createSignature(scope, resourceType, newUsage)
	}); err != nil {
		return err
	}

	// Record the usage in the metrics
	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "service.ResourcesLimits.DecrementUsage")

	return nil
}

// createSignature creates a digital signature for the given scope, resource type, and count using ECDSA.
func (ref *ResourcesLimitsService) createSignature(scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType, count int) ([]byte, error) {
	// Create the signature payload including scope ID
	var scopeIDStr string
	if scope.ID != nil {
		scopeIDStr = scope.ID.String()
	}

	payload := fmt.Sprintf("scope_type=%s&scope_id=%s&resource_type=%s&count=%d", scope.Type, scopeIDStr, resourceType, count)

	// Create a SHA256 hash of the payload
	hasher := sha256.New()
	hasher.Write([]byte(payload))
	signhash := hasher.Sum(nil)

	// The key was parsed once at construction; this path runs while the
	// repository holds a row lock, so it must not re-parse PEM.
	signature, err := ecdsa.SignASN1(rand.Reader, ref.signingKey, signhash)
	if err != nil {
		return nil, err
	}

	return signature, nil
}

// verifySignature verifies the digital signature for the given scope, resource type, and count using ECDSA.
func (ref *ResourcesLimitsService) verifySignature(signature []byte, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType, count int) error {
	// Create the signature payload including scope ID
	var scopeIDStr string
	if scope.ID != nil {
		scopeIDStr = scope.ID.String()
	}

	payload := fmt.Sprintf("scope_type=%s&scope_id=%s&resource_type=%s&count=%d", scope.Type, scopeIDStr, resourceType, count)

	// Create a SHA256 hash of the payload
	hasher := sha256.New()
	hasher.Write([]byte(payload))
	signhash := hasher.Sum(nil)

	isValid := ecdsa.VerifyASN1(ref.verificationKey, signhash, signature)
	if !isValid {
		return &domain.ResourcesLimitsInvalidSignatureError{Message: "invalid signature, possible tampering detected"}
	}

	return nil
}

// decodeECDSAPrivateKey decodes a PEM encoded ECDSA private key.
func decodeECDSAPrivateKey(privKey []byte) (privateKey *ecdsa.PrivateKey, err error) {
	blockPriv, _ := pem.Decode(privKey)
	if blockPriv == nil {
		return nil, &domain.InvalidPrivateKeyPEMError{Message: "failed to decode PEM block for private key"}
	}

	x509EncodedPriv := blockPriv.Bytes
	privateKey, err = x509.ParseECPrivateKey(x509EncodedPriv)
	if err != nil {
		return nil, &domain.InvalidPrivateKeyParseError{Message: "failed to parse EC private key", Err: err}
	}

	return
}

// decodeECDSAPublicKey decodes a PEM encoded ECDSA public key.
func decodeECDSAPublicKey(pubKey []byte) (publicKey *ecdsa.PublicKey, err error) {
	blockPub, _ := pem.Decode(pubKey)
	if blockPub == nil {
		return nil, &domain.InvalidPublicKeyPEMError{Message: "failed to decode PEM block for public key"}
	}

	x509EncodedPub := blockPub.Bytes

	genericPublicKey, err := x509.ParsePKIXPublicKey(x509EncodedPub)
	if err != nil {
		return nil, &domain.InvalidPublicKeyParseError{Message: "failed to parse PKIX public key", Err: err}
	}

	publicKey, ok := genericPublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, &domain.InvalidPublicKeyTypeError{Message: "public key is not an ECDSA key"}
	}

	return
}
