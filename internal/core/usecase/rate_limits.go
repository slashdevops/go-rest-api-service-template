package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"uuid"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/ratelimit"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// rateLimitCatalogueLimit bounds how many endpoint rows are read to validate a
// target. The catalogue is one row per API endpoint -- 136 today -- so this is a
// ceiling with headroom, not a page size an operator would ever notice.
const rateLimitCatalogueLimit = 1000

type RateLimitsServiceConf struct {
	Repository          repository.RateLimits
	ResourcesRepository repository.Resources

	// RuleSet is the in-memory mirror. nil when rules are not enforced, and the
	// service still works then -- a rule can be written before enforcement is
	// switched on, which is the sane order to roll this out in.
	RuleSet RateLimitRuleSet

	// Notifier tells the other replicas a write happened. nil is supported and
	// means ticker-only propagation, which is what cache.enabled=false gets.
	Notifier ratelimit.ChangeNotifier

	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// RateLimitsService is the CRUD use-case for rate-limit rules.
//
// Two things it does that a plain CRUD service does not:
//
//  1. It VALIDATES THE TARGET against the endpoint catalogue. A rule for a route
//     this service does not serve is not inert -- it looks correct in a listing,
//     reports no error, and silently protects nothing. The failure mode is a
//     limit an operator believes is in place.
//  2. It APPLIES the write to the serving replica's mirror before returning, so
//     the operator who saves a rule and immediately tests it sees it. Without
//     that they see the old behaviour for up to one reload interval and conclude
//     the feature is broken.
type RateLimitsService struct {
	repository          repository.RateLimits
	resourcesRepository repository.Resources
	ruleSet             RateLimitRuleSet
	notifier            ratelimit.ChangeNotifier
	ot                  *o11y.OpenTelemetry
	metrics             *o11y.LayerMetrics
	metricsMetadata     o11y.Metadata
	metricsPrefix       string
}

func NewRateLimitsService(conf RateLimitsServiceConf) (*RateLimitsService, error) {
	if conf.Repository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "Repository is nil, but it is required for RateLimitsService"}
	}

	if conf.ResourcesRepository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "ResourcesRepository is nil, but it is required for RateLimitsService"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for RateLimitsService"}
	}

	ref := &RateLimitsService{
		repository:          conf.Repository,
		resourcesRepository: conf.ResourcesRepository,
		ruleSet:             conf.RuleSet,
		notifier:            conf.Notifier,
		ot:                  conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "RateLimits",
			Action: "NewRateLimitsService",
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
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	ref.metrics = &o11y.LayerMetrics{Counter: callsCounter, Histogram: callsDuration}

	return ref, nil
}

// Create stores a rule after checking it targets a route this service serves.
func (ref *RateLimitsService) Create(ctx context.Context, input *domain.CreateRateLimitInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "Create")
	defer span.End()

	if input == nil {
		return o11y.RecordError(ctx, span, start, &domain.InvalidInputError{Message: "input is nil"}, ref.metrics, attrs)
	}

	var err error

	input.ID, err = domain.EnsureUUIDV7(input.ID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	// Validate BEFORE the catalogue check: a malformed target should be
	// reported as malformed, not as "no route matches", which sends the
	// operator looking for a routing problem that does not exist.
	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.validateTarget(ctx, input.TargetKind, input.Target, input.Methods); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.Insert(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	ref.applyLocally(ctx)

	o11y.RecordSuccess(
		ctx, span, start, ref.metrics, attrs, "Rate limit created",
		attribute.String("rate_limit.id", input.ID.String()),
		attribute.String("rate_limit.name", input.Name),
		attribute.String("rate_limit.strategy", string(input.Strategy)),
	)

	return nil
}

// UpdateByID replaces a rule and its window set.
func (ref *RateLimitsService) UpdateByID(ctx context.Context, input *domain.UpdateRateLimitInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "UpdateByID")
	defer span.End()

	if input == nil {
		return o11y.RecordError(ctx, span, start, &domain.InvalidInputError{Message: "input is nil"}, ref.metrics, attrs)
	}

	if err := input.Validate(); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.validateTarget(ctx, input.TargetKind, input.Target, input.Methods); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if err := ref.repository.UpdateByID(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	ref.applyLocally(ctx)

	o11y.RecordSuccess(
		ctx, span, start, ref.metrics, attrs, "Rate limit updated",
		attribute.String("rate_limit.id", input.ID.String()),
		attribute.String("rate_limit.strategy", string(input.Strategy)),
	)

	return nil
}

// DeleteByID removes a rule; its windows go with it through ON DELETE CASCADE.
func (ref *RateLimitsService) DeleteByID(ctx context.Context, input *domain.DeleteRateLimitInput) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "DeleteByID")
	defer span.End()

	if input == nil {
		return o11y.RecordError(ctx, span, start, &domain.InvalidInputError{Message: "input is nil"}, ref.metrics, attrs)
	}

	if err := ref.repository.DeleteByID(ctx, input); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	ref.applyLocally(ctx)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Rate limit deleted", attribute.String("rate_limit.id", input.ID.String()))

	return nil
}

// GetByID returns one rule with its windows.
func (ref *RateLimitsService) GetByID(ctx context.Context, id uuid.UUID) (*domain.RateLimit, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetByID")
	defer span.End()

	out, err := ref.repository.SelectByID(ctx, id)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Rate limit found", attribute.String("rate_limit.id", id.String()))

	return out, nil
}

// List returns a page of rules.
func (ref *RateLimitsService) List(ctx context.Context, input *domain.SelectRateLimitsInput) (*domain.ListRateLimitsOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "List")
	defer span.End()

	out, err := ref.repository.Select(ctx, input)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Rate limits listed", attribute.Int("rate_limits.count", len(out.Items)))

	return out, nil
}

// Enforcing reports whether rate limiting is switched on at all.
//
// It is not the same question as "are there rules". With ratelimit.enabled=false
// there is no mirror, so the rules in the database are real, listable, editable
// -- and enforcing nothing. A caller that shows a rule without showing this
// tells an operator a limit is in place that is not.
func (ref *RateLimitsService) Enforcing() bool { return ref.ruleSet != nil }

// Effective answers which rules apply to a (method, endpoint) pair.
//
// This is the endpoint that makes a precedence ladder usable by someone who has
// not read the docs. It resolves against the SAME pure function the middleware
// uses, and against the SAME mirrored set -- answering from a fresh query would
// let it disagree with what is actually being enforced, which is worse than not
// having it.
//
// That second half used to be a comment describing what the code did not do:
// it queried the repository on every call. Three ways that disagreed with
// enforcement, all of them silent -- a rule the mirror had dropped as
// unenforceable was reported as the winner; a rule written on another replica
// was reported before this one had reloaded it; and a stale mirror looked
// current. Every one of those makes the endpoint confidently wrong in exactly
// the situation an operator opens it to diagnose.
//
// The query remains as the fallback for the two cases where there is no
// mirrored answer at all -- no mirror wired (ratelimit.enabled=false), and
// a mirror that has never loaded -- filtered the same way the mirror filters, so
// the fallback cannot report a rule the mirror would have dropped either. The
// span records which of the two answered.
func (ref *RateLimitsService) Effective(ctx context.Context, req domain.RateLimitRequest) ([]domain.RateLimitMatch, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "Effective")
	defer span.End()

	if ref.ruleSet != nil {
		if matches, known := ref.ruleSet.Resolve(req); known {
			o11y.RecordSuccess(
				ctx, span, start, ref.metrics, attrs, "Effective rate limits resolved",
				attribute.String("method", req.Method),
				attribute.String("endpoint", req.Pattern),
				attribute.Int("matches", len(matches)),
				attribute.String("source", "mirror"),
			)

			return matches, nil
		}
	}

	// No mirror at all means ratelimit.enabled=false, and then the honest answer
	// is that NOTHING applies -- not "here are the rules that would apply".
	// Returning them would repeat the mistake this endpoint was already fixed
	// for once: describing rules that are not being enforced as the effective
	// ones. The caller distinguishes the two through Enforcing().
	if ref.ruleSet == nil {
		o11y.RecordSuccess(
			ctx, span, start, ref.metrics, attrs, "Effective rate limits resolved",
			attribute.String("method", req.Method),
			attribute.String("endpoint", req.Pattern),
			attribute.Int("matches", 0),
			attribute.String("source", "disabled"),
		)

		return nil, nil
	}

	rules, err := ref.repository.SelectAll(ctx)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	matches := domain.ResolveRateLimits(EnforceableRateLimits(rules), req)

	o11y.RecordSuccess(
		ctx, span, start, ref.metrics, attrs, "Effective rate limits resolved",
		attribute.String("method", req.Method),
		attribute.String("endpoint", req.Pattern),
		attribute.Int("matches", len(matches)),
		attribute.String("source", "repository"),
	)

	return matches, nil
}

// applyLocally refreshes the serving replica's mirror after a write.
//
// It reloads rather than mutating the cached set, because the write may have
// changed anything -- and a reload is one query against a table of tens of rows.
// A failure is logged, not returned: the write SUCCEEDED, and reporting an error
// would tell the caller their rule was not saved when it was. The ticker picks
// it up within the reload interval either way.
func (ref *RateLimitsService) applyLocally(ctx context.Context) {
	if ref.ruleSet == nil {
		return
	}

	if err := ref.ruleSet.Reload(ctx); err != nil {
		slog.Warn(
			"rate-limit rule written but the local mirror could not be refreshed",
			"error", err,
			"consequence", "this replica keeps enforcing the previous set until the next scheduled reload",
		)
	}

	// Tell the other replicas AFTER applying it here, so this replica is never
	// the last to know about its own write.
	//
	// A failure is logged, not returned: the write already succeeded, and
	// reporting an error would tell the caller their rule was not saved when it
	// was. The reload ticker carries the change either way -- which is the
	// whole reason this is allowed to fail.
	if ref.notifier == nil {
		return
	}

	if err := ref.notifier.Notify(ctx); err != nil {
		slog.Warn(
			"rate-limit rule written but other replicas could not be notified",
			"error", err,
			"consequence", "they apply it within ratelimit.reload.interval instead of at once",
		)
	}
}

// validateTarget checks the rule names a route this service actually serves.
//
// Catching it at write time is the whole point. A rule for a path that does not
// exist reports no error, appears in every listing, and protects nothing -- so
// the operator believes a limit is in place that is not.
func (ref *RateLimitsService) validateTarget(ctx context.Context, kind domain.RateLimitTargetKind, target string, methods []string) error {
	if kind == domain.RateLimitTargetKindGlobal {
		// A global rule matches every route by construction; there is nothing
		// to check it against.
		return nil
	}

	catalogue, err := ref.endpointCatalogue(ctx)
	if err != nil {
		return err
	}

	if kind == domain.RateLimitTargetKindPrefix {
		for path := range catalogue {
			if strings.HasPrefix(path, target) {
				return nil
			}
		}

		return &domain.InvalidRateLimitTargetError{
			Target: target,
			Reason: "no route starts with this prefix",
		}
	}

	verbs, ok := catalogue[target]
	if !ok {
		return &domain.InvalidRateLimitTargetError{
			Target: target,
			Reason: "no route matches this path. It must be the route template as registered, for example /projects/{project_id}/products",
		}
	}

	if slices.Contains(methods, domain.RateLimitAnyMethod) {
		return nil
	}

	for _, m := range methods {
		if slices.Contains(verbs, m) {
			continue
		}

		// HEAD is served by the GET route, so a rule naming HEAD is valid
		// wherever a GET row exists. Without this a legitimate rule is refused
		// for a verb the mux does serve.
		if m == "HEAD" && slices.Contains(verbs, "GET") {
			continue
		}

		return &domain.InvalidRateLimitTargetError{
			Target: target,
			Method: m,
			Reason: "this path is registered for " + strings.Join(verbs, ", ") + " but not " + m,
		}
	}

	return nil
}

// endpointCatalogue reads the API endpoint rows as path -> verbs.
//
// The resources table is one row per endpoint, generated from swagger, which
// makes it the closest thing this service has to a route table that
// internal/core may read. The '*' rows are authz wildcards, not endpoints, and
// are skipped -- including them would make every target validate.
func (ref *RateLimitsService) endpointCatalogue(ctx context.Context) (map[string][]string, error) {
	out, err := ref.resourcesRepository.Select(ctx, &domain.SelectResourcesInput{
		Paginator: domain.Paginator{Limit: rateLimitCatalogueLimit},
	})
	if err != nil {
		return nil, err
	}

	catalogue := make(map[string][]string, len(out.Items))

	for _, r := range out.Items {
		if r.Resource == "" || r.Resource == "*" || r.Action == "*" {
			continue
		}

		catalogue[r.Resource] = append(catalogue[r.Resource], r.Action)
	}

	if len(catalogue) == 0 {
		// Every target would be refused, which reads to an operator as "my path
		// is wrong" when the truth is that the catalogue did not load. Say which
		// it is.
		return nil, &domain.InvalidRateLimitTargetError{
			Reason: "the endpoint catalogue is empty, so no target can be validated; this is a server-side problem, not a problem with the rule",
		}
	}

	return catalogue, nil
}
