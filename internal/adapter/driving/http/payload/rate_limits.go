package payload

import (
	"strconv"
	"time"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// RateLimitWindowResponse is one budget within a rule.
//
//	@Description	One window of a rate-limit rule: a request budget over a period
type RateLimitWindowResponse struct {
	// Period is rendered as a Go duration string ("1m0s"), not as seconds.
	// Seconds are what the column holds; a duration is what an operator reads
	// and what every other duration in this API uses.
	Period   string    `json:"period" example:"1m0s" format:"string"`                                              // Window the budget applies to
	Requests int       `json:"requests" example:"300"`                                                             // Requests allowed within the period
	Burst    int       `json:"burst,omitempty" example:"300"`                                                      // Capacity available at once. 0 means the same as requests
	ID       uuid.UUID `json:"id,omitempty,omitzero" example:"019b4b0d-a682-7e34-a20c-c71a7147d7e7" format:"uuid"` // Unique identifier for the window
}

// RateLimitResponse represents a rate-limit rule.
//
//	@Description	A rate-limit rule: where it applies, who to, what it buckets on, and how it is enforced
type RateLimitResponse struct {
	CreatedAt   time.Time                 `json:"created_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`                      // When the rule was created
	UpdatedAt   time.Time                 `json:"updated_at,omitzero" example:"2021-01-01T00:00:00Z" format:"date-time"`                      // When the rule was last updated
	System      *bool                     `json:"system,omitempty,omitzero" example:"false"`                                                  // System-managed rules cannot be modified or deleted
	Enabled     *bool                     `json:"enabled,omitempty,omitzero" example:"true"`                                                  // Whether the rule is applied
	Name        string                    `json:"name,omitempty" example:"Generate per project" format:"string"`                              // Rule display name
	Description string                    `json:"description,omitempty" example:"One expensive call, seconds of wall clock" format:"string"`  // What the rule is for
	TargetKind  string                    `json:"target_kind,omitempty" example:"endpoint" format:"string" enum:"endpoint,prefix,global"`     // How target is matched
	Target      string                    `json:"target,omitempty" example:"/projects/{project_id}/generate" format:"string"`                 // Route template, prefix, or * for the global rule
	Scope       string                    `json:"scope,omitempty" example:"project" format:"string" enum:"ip,user,token,project,global"`      // What the bucket is keyed on
	Audience    string                    `json:"audience,omitempty" example:"auth" format:"string" enum:"any,guest,auth"`                    // Which callers the rule applies to
	Strategy    string                    `json:"strategy,omitempty" example:"leaky_bucket" format:"string" enum:"token_bucket,leaky_bucket"` // How the budget is enforced
	Methods     []string                  `json:"methods,omitempty" example:"POST" format:"array"`                                            // HTTP verbs the rule covers, or ["*"] for any
	Windows     []RateLimitWindowResponse `json:"windows,omitempty"`                                                                          // Budgets, all of which apply
	ID          uuid.UUID                 `json:"id,omitempty,omitzero" example:"019b4b0d-a682-7e34-a20c-c71a7147d7e7" format:"uuid"`         // Unique identifier for the rule
}

// ListRateLimitsResponse is a page of rules.
//
//	@Description	Paginated list of rate-limit rules
type ListRateLimitsResponse struct {
	Items     []RateLimitResponse `json:"items"`     // Rate-limit rules
	Paginator domain.Paginator    `json:"paginator"` // Pagination information

	// Enforcing is false when ratelimit.enabled=false. The rules above are then
	// real, listable and editable, and applying to nothing.
	//
	// It belongs on the LIST because the list is where an operator looks at
	// rules. Reading it from /rate_limits/effective instead would mean inventing
	// a method and an endpoint to ask about, purely to learn a property of the
	// deployment.
	Enforcing bool `json:"enforcing" example:"true"`
}

// RateLimitWindowRequest is one budget on a create or update.
//
//	@Description	One window of a rate-limit rule
type RateLimitWindowRequest struct {
	Period   string `json:"period" example:"1m0s" format:"string" validate:"required"` // Window as a duration string, between 1s and 24h
	Requests int    `json:"requests" example:"300" validate:"required" minimum:"1"`    // Requests allowed within the period
	Burst    int    `json:"burst,omitempty" example:"300" validate:"optional"`         // Capacity at once. Omit or 0 to mean the same as requests
}

// CreateRateLimitRequest creates a rule.
//
//	@Description	Request payload for creating a rate-limit rule
type CreateRateLimitRequest struct {
	Enabled     *bool                    `json:"enabled,omitempty,omitzero" example:"true" validate:"optional"`                                                                                  // Defaults to true
	Name        string                   `json:"name" example:"Generate per project" format:"string" validate:"required" minLength:"2" maxLength:"100"`                                          // Rule display name
	Description string                   `json:"description" example:"One expensive call, seconds of wall clock, real money" format:"string" validate:"required" minLength:"2" maxLength:"1024"` // What the rule is for
	TargetKind  string                   `json:"target_kind" example:"endpoint" format:"string" validate:"required" enum:"endpoint,prefix,global"`                                               // How target is matched
	Target      string                   `json:"target" example:"/projects/{project_id}/generate" format:"string" validate:"required"`                                                           // Route template as registered. A prefix must end with /; a global rule must use *
	Scope       string                   `json:"scope" example:"project" format:"string" validate:"required" enum:"ip,user,token,project,global"`                                                // What the bucket is keyed on
	Audience    string                   `json:"audience,omitempty" example:"auth" format:"string" validate:"optional" enum:"any,guest,auth"`                                                    // Defaults to any
	Strategy    string                   `json:"strategy,omitempty" example:"leaky_bucket" format:"string" validate:"optional" enum:"token_bucket,leaky_bucket"`                                 // Defaults to token_bucket
	Methods     []string                 `json:"methods" example:"POST" format:"array" validate:"required"`                                                                                      // Verbs the rule covers. ["*"] means any verb and ONE shared budget across them
	Windows     []RateLimitWindowRequest `json:"windows" validate:"required"`                                                                                                                    // At least one. All apply; the shortest period is evaluated first
	ID          uuid.UUID                `json:"id,omitempty,omitzero" example:"019b4b0d-a682-7e38-b235-3dfcb59f4d9e" format:"uuid" validate:"optional"`                                         // Optional custom ID
}

// UpdateRateLimitRequest replaces a rule.
//
// Every field is required, and the window set is replaced WHOLESALE. A partial
// update of a set is the shape that leaves two windows on one period, and the
// caller would then see a duplicate-period error that reads like a bug in the
// API rather than in the request.
//
//	@Description	Request payload for replacing a rate-limit rule. The window set is replaced in full
type UpdateRateLimitRequest struct {
	Enabled     *bool                    `json:"enabled,omitempty,omitzero" example:"true" validate:"optional"`
	Name        string                   `json:"name" example:"Generate per project" format:"string" validate:"required" minLength:"2" maxLength:"100"`
	Description string                   `json:"description" example:"One expensive call, seconds of wall clock, real money" format:"string" validate:"required" minLength:"2" maxLength:"1024"`
	TargetKind  string                   `json:"target_kind" example:"endpoint" format:"string" validate:"required" enum:"endpoint,prefix,global"`
	Target      string                   `json:"target" example:"/projects/{project_id}/generate" format:"string" validate:"required"`
	Scope       string                   `json:"scope" example:"project" format:"string" validate:"required" enum:"ip,user,token,project,global"`
	Audience    string                   `json:"audience,omitempty" example:"auth" format:"string" validate:"optional" enum:"any,guest,auth"`
	Strategy    string                   `json:"strategy,omitempty" example:"leaky_bucket" format:"string" validate:"optional" enum:"token_bucket,leaky_bucket"`
	Methods     []string                 `json:"methods" example:"POST" format:"array" validate:"required"`
	Windows     []RateLimitWindowRequest `json:"windows" validate:"required"`
}

// EffectiveRateLimitEntry is one rule that applies, and why.
//
//	@Description	A rule that applies to a request, with the reason it won its scope
type EffectiveRateLimitEntry struct {
	// Why is prose on purpose. The ladder is what operators get wrong, and a
	// tier number answers "which rung" when the question being asked is "why
	// not the other rule".
	Why      string                    `json:"why" example:"exact endpoint and a named verb — the most specific match there is" format:"string"`
	Scope    string                    `json:"scope" example:"project" format:"string"` // What this rule buckets on
	Name     string                    `json:"name" example:"Generate per project" format:"string"`
	Strategy string                    `json:"strategy" example:"leaky_bucket" format:"string"`
	Windows  []RateLimitWindowResponse `json:"windows"`
	RuleID   uuid.UUID                 `json:"rule_id" example:"019b4b0d-a682-7e34-a20c-c71a7147d7e7" format:"uuid"`
}

// EffectiveRateLimitsResponse answers which rules apply to one request.
//
//	@Description	The rules that apply to a method and endpoint, one per scope, most specific first
type EffectiveRateLimitsResponse struct {
	Method   string `json:"method" example:"POST" format:"string"`
	Endpoint string `json:"endpoint" example:"/projects/{project_id}/generate" format:"string"`

	// Enforcing is false when ratelimit.enabled=false: the rules below are real
	// and editable, and nothing is applying them. A client that renders the
	// rules without rendering this tells an operator a limit is in place that
	// is not.
	Enforcing bool `json:"enforcing" example:"true"`

	// BucketKey states what a budget is keyed on, because "10 per minute" alone
	// does not say per what. A rule's verbs share one budget; each of its windows
	// gets its own.
	BucketKey string `json:"bucket_key" example:"(rule_id, window_id, scope_key) — one budget per window, shared across the rule's verbs" format:"string"`

	// Effective carries ONE entry per scope. That is the shape of the design: an
	// IP rule and a project rule both apply, and neither substitutes for the
	// other.
	Effective []EffectiveRateLimitEntry `json:"effective"`
}

// ToRateLimitResponse maps a domain rule onto the wire shape.
func ToRateLimitResponse(in *domain.RateLimit) RateLimitResponse {
	return RateLimitResponse{
		ID:          in.ID,
		Name:        in.Name,
		Description: in.Description,
		TargetKind:  string(in.TargetKind),
		Target:      in.Target,
		Methods:     in.Methods,
		Scope:       string(in.Scope),
		Audience:    string(in.Audience),
		Strategy:    string(in.Strategy),
		Enabled:     in.Enabled,
		System:      in.System,
		Windows:     toRateLimitWindowResponses(in.SortedWindows()),
		CreatedAt:   in.CreatedAt,
		UpdatedAt:   in.UpdatedAt,
	}
}

// toRateLimitWindowResponses renders windows shortest period first, matching
// the order they are evaluated in. A listing that showed them in insertion
// order would disagree with which boundary a caller actually hits first.
func toRateLimitWindowResponses(in []domain.RateLimitWindow) []RateLimitWindowResponse {
	out := make([]RateLimitWindowResponse, 0, len(in))

	for _, w := range in {
		out = append(out, RateLimitWindowResponse{
			ID:       w.ID,
			Requests: w.Requests,
			Period:   w.Period.String(),
			Burst:    w.Burst,
		})
	}

	return out
}

// ToRateLimitWindows parses the wire windows into domain windows.
//
// A malformed duration is returned as a validation error naming the index, not
// swallowed into a zero period -- a zero period is a budget of "N per instant",
// which the CHECK constraint would then reject with a message about seconds.
func ToRateLimitWindows(in []RateLimitWindowRequest) ([]domain.RateLimitWindow, error) {
	var errs domain.ValidationErrors

	out := make([]domain.RateLimitWindow, 0, len(in))

	for i, w := range in {
		period, err := time.ParseDuration(w.Period)
		if err != nil {
			errs.AddError(
				domain.FieldWindows+"["+strconv.Itoa(i)+"]",
				"period must be a duration such as 1s, 1m0s or 1h0m0s: "+w.Period+" is not one",
				"INVALID_FORMAT",
			)

			continue
		}

		out = append(out, domain.RateLimitWindow{
			Requests: w.Requests,
			Period:   period,
			Burst:    w.Burst,
		})
	}

	if errs.HasErrors() {
		return nil, &errs
	}

	return out, nil
}
