package domain

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"uuid"
)

const (
	RateLimitNameMinLength        = 2
	RateLimitNameMaxLength        = 100
	RateLimitDescriptionMinLength = 2
	RateLimitDescriptionMaxLength = 1024

	// RateLimitMinPeriod and RateLimitMaxPeriod bound a window.
	//
	// The floor is one second because the counter backing a shared window has
	// whole-second granularity; a sub-second window would be enforced by
	// rounding, which is a different limit from the one the operator wrote.
	//
	// The ceiling is 24h and matches the CHECK constraint on
	// rate_limit_windows.period_seconds. Beyond a day a budget stops behaving
	// like a rate limit and starts behaving like a quota, which is what the
	// resource-limits feature is for.
	RateLimitMinPeriod = 1 * time.Second
	RateLimitMaxPeriod = 24 * time.Hour

	// RateLimitMaxWindows bounds how many windows one rule may carry. Every
	// window is evaluated on every request, so this is a latency bound, not a
	// storage one.
	RateLimitMaxWindows = 5

	RateLimitsRateLimitCreatedSuccessfully = "Rate limit created successfully"
	RateLimitsRateLimitUpdatedSuccessfully = "Rate limit updated successfully"
	RateLimitsRateLimitDeletedSuccessfully = "Rate limit deleted successfully"

	// ValidRateLimitTargetRegex accepts what a rate-limit rule targets, which is
	// NOT what an authz policy targets.
	//
	// A policy's resource is a concrete path that has already had its ids
	// substituted; a rule's target is the ROUTE TEMPLATE as registered on the
	// mux -- "/projects/{project_id}/generate", literally, braces included. The
	// two vocabularies are deliberately separate: reusing ValidResourceRegex
	// here would reject every parameterised route, and applying the frontend's
	// toAllowedResource transform to a target would silently produce a rule
	// that matches nothing.
	//
	// Accepted: a leading "/", lowercase segments and {param} placeholders, an
	// optional trailing "/" (which is what makes a prefix target readable), or
	// the single character "*" for the global rule.
	ValidRateLimitTargetRegex = `^(\*|\/([a-z_]{1,50}|\{[a-z_]{1,50}\})(\/([a-z_]{1,50}|\{[a-z_]{1,50}\})){0,7}\/?)$`
)

// RateLimitStrategy selects how a window is enforced.
//
// The two admit IDENTICALLY at equal parameters -- they are duals, and this was
// measured, not assumed. What differs is the PARAMETERISATION each invites: a
// token bucket is written as "N per period with a burst of B", a leaky bucket as
// "one every interval with a tolerance of B". So this field records which
// question the operator was asking, and the UI must present it as budget vs
// pace, never as "bursty vs smooth" -- that would sell a difference that does
// not exist.
//
// The string values are the wire format, the database CHECK constraint, and the
// values ratelimiter.ParseStrategy accepts. TestRateLimitStrategiesMatchLibrary
// fails if the library ever renames one.
type RateLimitStrategy string

const (
	// RateLimitStrategyTokenBucket is the default, and the default matters: it
	// is what keeps every existing rule and every seeded row behaving as it
	// does today.
	RateLimitStrategyTokenBucket RateLimitStrategy = "token_bucket"
	RateLimitStrategyLeakyBucket RateLimitStrategy = "leaky_bucket"
)

// RateLimitScope is what the bucket is keyed on.
type RateLimitScope string

const (
	RateLimitScopeIP      RateLimitScope = "ip"
	RateLimitScopeUser    RateLimitScope = "user"
	RateLimitScopeToken   RateLimitScope = "token"
	RateLimitScopeProject RateLimitScope = "project"
	RateLimitScopeGlobal  RateLimitScope = "global"
)

// RateLimitAudience narrows a rule to authenticated or unauthenticated callers.
// It is orthogonal to scope: an ip-scoped rule can apply to guests only.
type RateLimitAudience string

const (
	RateLimitAudienceAny   RateLimitAudience = "any"
	RateLimitAudienceGuest RateLimitAudience = "guest"
	RateLimitAudienceAuth  RateLimitAudience = "auth"
)

// RateLimitTargetKind is how Target is matched against a request.
type RateLimitTargetKind string

const (
	RateLimitTargetKindEndpoint RateLimitTargetKind = "endpoint"
	RateLimitTargetKindPrefix   RateLimitTargetKind = "prefix"
	RateLimitTargetKindGlobal   RateLimitTargetKind = "global"
)

// RateLimitAnyMethod is the wildcard verb. A rule carrying it shares ONE budget
// across every verb -- the bucket key does not expand by method. That is a
// deliberate choice and the form has to say so, because "any verb" reads like
// "a budget per verb" to most people.
const RateLimitAnyMethod = "*"

// RateLimitGlobalTarget is the only target a global rule may carry, enforced
// both here and by chk_rate_limits_global_target.
const RateLimitGlobalTarget = "*"

var (
	// RateLimitsFilterFields is a list of valid fields for filtering models.
	RateLimitsFilterFields = []string{
		FieldID, FieldName, FieldTargetKind, FieldTarget, FieldScope,
		FieldAudience, FieldStrategy, FieldEnabled, FieldSystem, FieldCreatedAt, FieldUpdatedAt,
	}

	// RateLimitsSortFields is a list of valid fields for sorting models.
	RateLimitsSortFields = []string{
		FieldID, FieldName, FieldTargetKind, FieldTarget, FieldScope,
		FieldAudience, FieldStrategy, FieldEnabled, FieldSystem, FieldCreatedAt, FieldUpdatedAt,
	}

	// RateLimitsPartialFields is a list of valid fields for partial responses.
	//
	// This is a []string, so a column left out here fails at RUN time, not at
	// compile time -- which is exactly how `strategy` would go missing without
	// anything noticing. TestRateLimitsPartialFieldsCoverEveryColumn guards it.
	RateLimitsPartialFields = []string{
		FieldID,
		FieldName,
		FieldDescription,
		FieldTargetKind,
		FieldTarget,
		FieldMethods,
		FieldScope,
		FieldAudience,
		FieldStrategy,
		FieldEnabled,
		FieldWindows,
		FieldSystem,
		FieldCreatedAt,
		FieldUpdatedAt,
	}

	validRateLimitStrategies = []RateLimitStrategy{RateLimitStrategyTokenBucket, RateLimitStrategyLeakyBucket}
	validRateLimitScopes     = []RateLimitScope{
		RateLimitScopeIP, RateLimitScopeUser, RateLimitScopeToken,
		RateLimitScopeProject, RateLimitScopeGlobal,
	}
	validRateLimitAudiences   = []RateLimitAudience{RateLimitAudienceAny, RateLimitAudienceGuest, RateLimitAudienceAuth}
	validRateLimitTargetKinds = []RateLimitTargetKind{
		RateLimitTargetKindEndpoint, RateLimitTargetKindPrefix, RateLimitTargetKindGlobal,
	}

	rateLimitTargetRe = regexp.MustCompile(ValidRateLimitTargetRegex)
	rateLimitActionRe = regexp.MustCompile(ValidActionsRegex)
)

// RateLimitStrategies returns every accepted strategy, for an error message that
// tells the caller what to do instead of only what was wrong.
func RateLimitStrategies() []RateLimitStrategy { return slices.Clone(validRateLimitStrategies) }

// RateLimitScopes returns every accepted scope.
func RateLimitScopes() []RateLimitScope { return slices.Clone(validRateLimitScopes) }

// RateLimitAudiences returns every accepted audience.
func RateLimitAudiences() []RateLimitAudience { return slices.Clone(validRateLimitAudiences) }

// RateLimitTargetKinds returns every accepted target kind.
func RateLimitTargetKinds() []RateLimitTargetKind { return slices.Clone(validRateLimitTargetKinds) }

func joinStrategies() string {
	out := make([]string, 0, len(validRateLimitStrategies))
	for _, s := range validRateLimitStrategies {
		out = append(out, string(s))
	}

	return strings.Join(out, ", ")
}

// IsValidRateLimitStrategy reports whether s is one this service can build a
// limiter from.
//
// An unrecognised value must never fall back to the default. A rule that says
// leaky_bucket and silently enforces a token bucket is worse than a refusal:
// nothing in the response, the logs or the metrics would say the operator did
// not get what they asked for.
func IsValidRateLimitStrategy(s RateLimitStrategy) error {
	if s == "" {
		return fmt.Errorf("strategy cannot be empty, must be one of %s", joinStrategies())
	}

	if !slices.Contains(validRateLimitStrategies, s) {
		return fmt.Errorf("invalid strategy: %s, must be one of %s", s, joinStrategies())
	}

	return nil
}

// IsValidRateLimitTarget validates a rule target against the ROUTE TEMPLATE
// vocabulary. See ValidRateLimitTargetRegex for why this is not IsValidResource.
func IsValidRateLimitTarget(target string) error {
	if target == "" {
		return fmt.Errorf("target cannot be empty")
	}

	if !rateLimitTargetRe.MatchString(target) {
		return fmt.Errorf("invalid target: %s, must be a route template such as /projects/{project_id}/generate, a prefix such as /projects/, or * for the global rule", target)
	}

	return nil
}

// RateLimitWindow is one budget: Requests over Period, with a bucket capacity of
// Burst.
type RateLimitWindow struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	System    *bool
	Period    time.Duration
	Requests  int
	Burst     int
	SerialID  int64
	ID        uuid.UUID
	RateLimit uuid.UUID
}

// EffectiveBurst is the capacity the limiter is built with.
//
// Zero means "same as Requests", which is the sensible default for a token
// bucket. It is stored as 0 rather than being expanded on write so that a rule
// whose Requests is later edited keeps its burst tracking it, instead of
// silently freezing at the old value.
func (ref *RateLimitWindow) EffectiveBurst() int {
	if ref.Burst <= 0 {
		return ref.Requests
	}

	return ref.Burst
}

// RateLimit is one rule: where it applies, who it applies to, what it buckets
// on, and how it is enforced. The budget lives in Windows.
type RateLimit struct {
	CreatedAt   time.Time
	UpdatedAt   time.Time
	System      *bool
	Enabled     *bool
	Name        string
	Description string
	Target      string
	TargetKind  RateLimitTargetKind
	Scope       RateLimitScope
	Audience    RateLimitAudience
	Strategy    RateLimitStrategy
	Methods     []string
	Windows     []RateLimitWindow
	SerialID    int64
	ID          uuid.UUID
}

// MatchesMethod reports whether this rule covers the given verb.
//
// HEAD is served by a GET route, so a rule naming GET covers HEAD too. Without
// this a HEAD request slips past every rule that was written for the endpoint it
// hits -- a hole that is invisible until someone probes with HEAD.
func (ref *RateLimit) MatchesMethod(method string) bool {
	if ref.AnyMethod() {
		return true
	}

	if slices.Contains(ref.Methods, method) {
		return true
	}

	return method == "HEAD" && slices.Contains(ref.Methods, "GET")
}

// AnyMethod reports whether the rule was written with the wildcard verb.
func (ref *RateLimit) AnyMethod() bool {
	return len(ref.Methods) == 1 && ref.Methods[0] == RateLimitAnyMethod
}

// MatchesAudience reports whether this rule applies to an authenticated or
// unauthenticated caller.
func (ref *RateLimit) MatchesAudience(authenticated bool) bool {
	switch ref.Audience {
	case RateLimitAudienceAny:
		return true
	case RateLimitAudienceAuth:
		return authenticated
	case RateLimitAudienceGuest:
		return !authenticated
	default:
		// An audience that is not one of the three cannot have come through
		// Validate or the CHECK constraint. Matching nothing is the safe
		// reading: it disables the rule rather than applying it to everyone.
		return false
	}
}

// IsEnabled reports whether the rule should be applied. A nil Enabled means the
// database default, which is true.
func (ref *RateLimit) IsEnabled() bool {
	return ref.Enabled == nil || *ref.Enabled
}

// IsSystem reports whether this row is protected by the system trigger.
func (ref *RateLimit) IsSystem() bool {
	return ref.System != nil && *ref.System
}

// SortedWindows returns the windows SHORTEST PERIOD FIRST.
//
// Evaluation order is load-bearing, not cosmetic. A rule carrying 10/s and
// 300/min must consult the second window first: spending the minute budget on
// requests the second window would have refused makes the long window drain at
// a rate nobody asked for, and the boundary that is eventually reported is the
// wrong one.
func (ref *RateLimit) SortedWindows() []RateLimitWindow {
	out := slices.Clone(ref.Windows)
	slices.SortFunc(out, func(a, b RateLimitWindow) int {
		switch {
		case a.Period < b.Period:
			return -1
		case a.Period > b.Period:
			return 1
		default:
			return 0
		}
	})

	return out
}

// CreateRateLimitInput is the create payload, after transport decoding.
type CreateRateLimitInput struct {
	Enabled     *bool
	Name        string
	Description string
	Target      string
	TargetKind  RateLimitTargetKind
	Scope       RateLimitScope
	Audience    RateLimitAudience
	Strategy    RateLimitStrategy
	Methods     []string
	Windows     []RateLimitWindow
	ID          uuid.UUID
}

func (ref *CreateRateLimitInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ValidateUUID(ref.ID, 7, FieldID))
	validateRateLimitCommon(&errs, ref.Name, ref.Description, ref.TargetKind, ref.Target, ref.Methods, ref.Scope, ref.Audience, ref.Strategy, ref.Windows)

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// UpdateRateLimitInput is the update payload.
//
// Windows are replaced WHOLESALE, never merged. A partial update of a set is the
// kind of API that leaves two windows on one period, or a rule whose budget
// changed in a way nobody wrote down.
type UpdateRateLimitInput struct {
	Enabled     *bool
	Name        string
	Description string
	Target      string
	TargetKind  RateLimitTargetKind
	Scope       RateLimitScope
	Audience    RateLimitAudience
	Strategy    RateLimitStrategy
	Methods     []string
	Windows     []RateLimitWindow
	ID          uuid.UUID
}

func (ref *UpdateRateLimitInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ValidateUUID(ref.ID, 7, FieldID))
	validateRateLimitCommon(&errs, ref.Name, ref.Description, ref.TargetKind, ref.Target, ref.Methods, ref.Scope, ref.Audience, ref.Strategy, ref.Windows)

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

//nolint:gocognit // one rule has ten independent fields; splitting this only moves the checks somewhere less obvious.
func validateRateLimitCommon(
	errs *ValidationErrors,
	name, description string,
	kind RateLimitTargetKind,
	target string,
	methods []string,
	scope RateLimitScope,
	audience RateLimitAudience,
	strategy RateLimitStrategy,
	windows []RateLimitWindow,
) {
	nameOptions := StringValidationOptions{
		MinLength: RateLimitNameMinLength, MaxLength: RateLimitNameMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldName,
	}
	if _, err := ValidateString(name, nameOptions); err != nil {
		errs.Add(err)
	}

	descriptionOptions := StringValidationOptions{
		MinLength: RateLimitDescriptionMinLength, MaxLength: RateLimitDescriptionMaxLength,
		TrimWhitespace: true, AllowEmpty: false, NoControlChars: true, NoHTMLTags: true,
		NoScriptTags: true, NoNullBytes: true, NormalizeUnicode: true, FieldName: FieldDescription,
	}
	if _, err := ValidateString(description, descriptionOptions); err != nil {
		errs.Add(err)
	}

	if !slices.Contains(validRateLimitTargetKinds, kind) {
		errs.AddError(FieldTargetKind, "must be one of endpoint, prefix, global", "INVALID_VALUE")
	}

	if err := IsValidRateLimitTarget(target); err != nil {
		errs.AddError(FieldTarget, err.Error(), "INVALID_FORMAT")
	}

	// The two halves of chk_rate_limits_global_target, checked here so the
	// caller gets a sentence instead of a constraint-violation 500.
	switch {
	case kind == RateLimitTargetKindGlobal && target != RateLimitGlobalTarget:
		errs.AddError(FieldTarget, "a global rule must target *", "INVALID_VALUE")
	case kind != RateLimitTargetKindGlobal && target == RateLimitGlobalTarget:
		errs.AddError(FieldTarget, "* is only valid for a global rule", "INVALID_VALUE")
	}

	if kind == RateLimitTargetKindPrefix && !strings.HasSuffix(target, "/") {
		errs.AddError(FieldTarget, "a prefix target must end with /, so that /projects/ cannot also match /projects_archive", "INVALID_FORMAT")
	}

	validateRateLimitMethods(errs, methods)

	if !slices.Contains(validRateLimitScopes, scope) {
		errs.AddError(FieldScope, "must be one of ip, user, token, project, global", "INVALID_VALUE")
	}

	if !slices.Contains(validRateLimitAudiences, audience) {
		errs.AddError(FieldAudience, "must be one of any, guest, auth", "INVALID_VALUE")
	}

	// A guest has no user and no token, so a rule keyed on one would bucket
	// every anonymous caller together -- one global budget wearing a per-user
	// label. Refuse it rather than enforce something the operator did not write.
	if audience == RateLimitAudienceGuest && (scope == RateLimitScopeUser || scope == RateLimitScopeToken) {
		errs.AddError(FieldScope, "a guest rule cannot be scoped to user or token; guests have neither, so every caller would share one bucket", "INVALID_COMBINATION")
	}

	if err := IsValidRateLimitStrategy(strategy); err != nil {
		errs.AddError(FieldStrategy, err.Error(), "INVALID_VALUE")
	}

	validateRateLimitWindows(errs, windows)
}

func validateRateLimitMethods(errs *ValidationErrors, methods []string) {
	if len(methods) == 0 {
		errs.AddError(FieldMethods, "must name at least one method, or * for any", "REQUIRED")
		return
	}

	// chk_rate_limits_methods: either exactly {*} or a list without it.
	// A mixed list makes the precedence ladder ambiguous at the same tier --
	// is such a rule "any verb" or "named verb"?
	if slices.Contains(methods, RateLimitAnyMethod) && len(methods) > 1 {
		errs.AddError(FieldMethods, "* cannot be combined with named methods; use * alone for any verb", "INVALID_COMBINATION")
		return
	}

	seen := make(map[string]struct{}, len(methods))

	for _, m := range methods {
		if _, dup := seen[m]; dup {
			errs.AddError(FieldMethods, "duplicate method: "+m, "DUPLICATE")
			continue
		}

		seen[m] = struct{}{}

		if !rateLimitActionRe.MatchString(m) {
			errs.AddError(FieldMethods, "invalid method: "+m+", must be one of "+GetValidActions()+" in Uppercase", "INVALID_FORMAT")
		}
	}
}

func validateRateLimitWindows(errs *ValidationErrors, windows []RateLimitWindow) {
	// A rule with no window allows nothing, so it cannot be saved. The loader
	// SKIPS such a rule rather than refusing traffic, but that is a safety net
	// for a row that got in some other way -- it is not a supported state.
	if len(windows) == 0 {
		errs.AddError(FieldWindows, "must define at least one window; a rule with no window has no budget", "REQUIRED")
		return
	}

	if len(windows) > RateLimitMaxWindows {
		errs.AddError(FieldWindows, fmt.Sprintf("at most %d windows per rule", RateLimitMaxWindows), "TOO_MANY")
		return
	}

	seen := make(map[time.Duration]struct{}, len(windows))

	for i, w := range windows {
		field := fmt.Sprintf("%s[%d]", FieldWindows, i)

		if _, dup := seen[w.Period]; dup {
			// unique_rate_limit_window_period, checked here so the caller gets
			// a sentence rather than a 23505 that reads like a duplicate name.
			errs.AddError(field, "duplicate period: "+w.Period.String(), "DUPLICATE")
		}

		seen[w.Period] = struct{}{}

		if w.Requests < 1 {
			errs.AddError(field, "requests must be at least 1", "OUT_OF_RANGE")
		}

		if w.Period < RateLimitMinPeriod || w.Period > RateLimitMaxPeriod {
			errs.AddError(field, fmt.Sprintf("period must be between %s and %s", RateLimitMinPeriod, RateLimitMaxPeriod), "OUT_OF_RANGE")
		}

		if w.Burst < 0 {
			errs.AddError(field, "burst cannot be negative; use 0 to mean the same as requests", "OUT_OF_RANGE")
		}
	}
}

// SelectRateLimitsInput is the list query.
type SelectRateLimitsInput struct {
	Sort      string
	Filter    string
	Fields    string
	Paginator Paginator
}

func (ref *SelectRateLimitsInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ref.Paginator.Validate())

	if ref.Sort != "" {
		if _, err := RateLimitsSortParser.Parse(ref.Sort); err != nil {
			errs.Add(&ValidationError{Field: FieldSort, Message: err.Error(), Code: "INVALID_SORT_FIELD"})
		}
	}

	if ref.Filter != "" {
		if _, err := RateLimitsFilterParser.Parse(ref.Filter); err != nil {
			errs.Add(&ValidationError{Field: FieldFilter, Message: err.Error(), Code: "INVALID_FILTER_FIELD"})
		}
	}

	if ref.Fields != "" {
		if _, err := RateLimitsFieldsParser.Parse(ref.Fields); err != nil {
			errs.Add(&ValidationError{Field: FieldFields, Message: err.Error(), Code: "INVALID_FIELDS_FIELD"})
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// SelectRateLimitsOutput is one page of rules.
type SelectRateLimitsOutput struct {
	Items     []RateLimit
	Paginator Paginator
}

type ListRateLimitsOutput = SelectRateLimitsOutput

// DeleteRateLimitInput identifies the rule to remove.
type DeleteRateLimitInput struct {
	ID uuid.UUID
}

func (ref *DeleteRateLimitInput) Validate() error {
	var errs ValidationErrors

	errs.Add(ValidateUUID(ref.ID, 7, FieldID))

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// EnforceabilityProblem reports why this rule cannot be enforced, or "" when it
// can.
//
// # Why this exists at all
//
// Every field here is validated before a rule is written, so a rule that
// reaches enforcement should be enforceable by construction. It is not: goose
// does not checksum, the API is not the only writer, and a partially applied
// change or a hand-edited row lands a rule the validator never saw. The
// question "can this rule actually be enforced?" therefore has to be asked
// again at load time, where the answer can be logged and the rule dropped.
//
// # Why dropping is right, and refusing is not
//
// A broken rule is a MISCONFIGURATION, not an availability incident, and the
// two want opposite responses. Refusing traffic because a row is malformed
// converts an operator's typo into an outage of the endpoint they were trying
// to protect -- and it is the endpoint they cared about most, which is why they
// wrote a rule for it. Dropping falls through to the next tier, and to the flag
// floor if nothing else matches, so the traffic stays bounded by something.
//
// The cost of dropping is that a limit silently does not apply, which is why
// the caller MUST log the returned reason. Both are bad; only one of them is
// recoverable without a deploy.
//
// The returned string is a reason for a log line, not an error for a caller to
// match on -- there is nothing a caller can do differently per reason.
func (ref *RateLimit) EnforceabilityProblem() string {
	if len(ref.Windows) == 0 {
		return "the rule has no window, so it carries no budget"
	}

	if err := IsValidRateLimitStrategy(ref.Strategy); err != nil {
		return "the rule names a strategy no limiter can be built from: " + err.Error()
	}

	return ""
}

// DecidedPreAuth reports whether this scope's bucket key is knowable before the
// authentication chain has run.
//
// ip and global are: both are derived from the connection. user, token and
// project are not -- they come from claims that do not exist until the token has
// been verified, which is why the limiter runs in two stages at all.
//
// This is the SINGLE definition of that split. It lives in the domain rather
// than in the middleware because the stage filter and the audience rule have to
// agree about it, and a disagreement is invisible from the outside: the rule is
// accepted, is listed, and simply never fires.
func (ref RateLimitScope) DecidedPreAuth() bool {
	return ref == RateLimitScopeIP || ref == RateLimitScopeGlobal
}
