package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
	"go.opentelemetry.io/otel/metric"
)

// GrantsReader reads a caller's effective grants. It is the repository, not
// the cached service: a guard must see the grants the caller holds now, and
// a stale allowance is the one thing it must never serve.
type GrantsReader interface {
	SelectAuthz(ctx context.Context, userID uuid.UUID) (*domain.SelectAuthzOutput, error)
}

// PoliciesReader resolves what a policy, or every policy of a role, grants.
type PoliciesReader interface {
	SelectByID(ctx context.Context, id uuid.UUID) (*domain.Policy, error)
	SelectByRoleID(ctx context.Context, roleID uuid.UUID, input *domain.SelectPoliciesInput) (*domain.SelectPoliciesOutput, error)
}

type GrantGuardConf struct {
	Grants        GrantsReader
	Policies      PoliciesReader
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// GrantGuard answers one question before anything is granted: does the
// caller already hold what they are about to hand out?
//
// Every path that widens someone's permissions goes through it -- creating
// or editing a policy, linking policies to a role, linking roles to a user
// -- because each is one step of the same escalation: a caller holding
// POST /policies, POST /roles, POST /roles/*/policies and POST /users/*/roles
// minted an allow-all policy, a role, attached both to itself and became an
// administrator. Measured 2026-09-06. The authorization policy could not
// have caught it: each request was, on its own, one the caller was allowed.
//
// "Holds" is deliberately literal: an administrator (every method on "*")
// holds everything; otherwise the caller must hold the same action on the
// same pattern, or on "*". A grant on /projects/* does not let its holder
// hand out /projects/<one id>, though it would be sound: narrowing patterns
// is work the policy engine does, and doing it twice, differently, is how
// the two would drift.
type GrantGuard struct {
	grants          GrantsReader
	policies        PoliciesReader
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

func NewGrantGuard(conf GrantGuardConf) (*GrantGuard, error) {
	if conf.Grants == nil || conf.Policies == nil {
		return nil, &domain.InvalidInputError{Message: "GrantGuard needs a grants reader and a policies reader"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry cannot be nil"}
	}

	ref := &GrantGuard{
		grants:   conf.Grants,
		policies: conf.Policies,
		ot:       conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "GrantGuard",
			Action: "NewGrantGuard",
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
		metric.WithDescription(fmt.Sprintf("Duration of %s service calls", AppLayer)),
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

// CheckGrants refuses unless the caller holds every grant.
func (ref *GrantGuard) CheckGrants(ctx context.Context, callerID uuid.UUID, grants []domain.Grant) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "CheckGrants")
	defer span.End()

	if callerID == uuid.Nil() {
		return o11y.RecordError(ctx, span, start, &domain.InvalidUserIDError{Message: "caller is required"}, ref.metrics, attrs)
	}

	held, err := ref.grants.SelectAuthz(ctx, callerID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	mine := grantsOf(held, callerID)

	for _, g := range grants {
		if !covers(mine, g) {
			return o11y.RecordError(ctx, span, start, &domain.GrantNotHeldError{Grant: g}, ref.metrics, attrs)
		}
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "grants held")

	return nil
}

// CheckPolicies refuses unless the caller holds what every policy grants.
func (ref *GrantGuard) CheckPolicies(ctx context.Context, callerID uuid.UUID, policyIDs []uuid.UUID) error {
	grants := make([]domain.Grant, 0, len(policyIDs))

	for _, id := range policyIDs {
		p, err := ref.policies.SelectByID(ctx, id)
		if err != nil {
			return err
		}

		grants = append(grants, domain.Grant{Action: p.AllowedAction, Resource: p.AllowedResource})
	}

	return ref.CheckGrants(ctx, callerID, grants)
}

// CheckRoles refuses unless the caller holds what every policy of every
// role grants.
func (ref *GrantGuard) CheckRoles(ctx context.Context, callerID uuid.UUID, roleIDs []uuid.UUID) error {
	var grants []domain.Grant

	for _, roleID := range roleIDs {
		input := &domain.SelectPoliciesInput{Paginator: domain.Paginator{Limit: domain.PaginatorMaxLimit}}

		for {
			out, err := ref.policies.SelectByRoleID(ctx, roleID, input)
			if err != nil {
				return err
			}

			for _, p := range out.Items {
				grants = append(grants, domain.Grant{Action: p.AllowedAction, Resource: p.AllowedResource})
			}

			if out.Paginator.NextToken == "" || len(out.Items) == 0 {
				break
			}

			input.Paginator.NextToken = out.Paginator.NextToken
		}
	}

	return ref.CheckGrants(ctx, callerID, grants)
}

// grantsOf lifts the repository's map into pattern -> actions for one user.
func grantsOf(out *domain.SelectAuthzOutput, userID uuid.UUID) map[string][]string {
	result := map[string][]string{}
	if out == nil {
		return result
	}

	perms, _ := out.Permissions["permissions"].(map[string]any)
	users, _ := perms["users"].(map[string]any)
	byUser, _ := users[userID.String()].(map[string]any)

	for pattern, raw := range byUser {
		switch actions := raw.(type) {
		case []any:
			for _, a := range actions {
				if s, ok := a.(string); ok {
					result[pattern] = append(result[pattern], s)
				}
			}
		case []string:
			result[pattern] = append(result[pattern], actions...)
		}
	}

	return result
}

// covers mirrors the policy's own rules at the pattern level: every method
// on "*" holds everything; otherwise the action (or "*") on the same pattern
// or on "*".
func covers(mine map[string][]string, g domain.Grant) bool {
	for _, pattern := range []string{"*", g.Resource} {
		for _, a := range mine[pattern] {
			if a == "*" || a == g.Action {
				return true
			}
		}
	}

	return false
}
