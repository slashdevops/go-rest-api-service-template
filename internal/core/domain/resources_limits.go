package domain

import (
	"time"

	"uuid"
)

var (
	// ResourcesLimitsFilterFields is a list of valid fields for filtering resource limits.
	//
	// FieldUsage belongs here: the repository has always been able to select and
	// scan it, but it was missing from these lists, so `?fields=usage` — the one
	// column a caller is most likely to ask for on its own — was rejected by the
	// parser.
	ResourcesLimitsFilterFields = []string{FieldID, FieldScopeType, FieldScopeID, FieldResourceType, FieldUsage, FieldSoftLimit, FieldHardLimit, FieldCreatedAt, FieldUpdatedAt}

	// ResourcesLimitsSortFields is a list of valid fields for sorting resource limits.
	ResourcesLimitsSortFields = []string{FieldID, FieldScopeType, FieldScopeID, FieldResourceType, FieldUsage, FieldSoftLimit, FieldHardLimit, FieldCreatedAt, FieldUpdatedAt}

	// ResourcesLimitsPartialFields is a list of valid fields for partial responses.
	ResourcesLimitsPartialFields = []string{
		FieldID,
		FieldScopeType,
		FieldScopeID,
		FieldResourceType,
		FieldUsage,
		FieldSoftLimit,
		FieldHardLimit,
		FieldCreatedAt,
		FieldUpdatedAt,
	}
)

type ResourcesLimits struct {
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ScopeType    string
	ResourceType string
	Usage        int
	SoftLimit    int
	HardLimit    int
	SerialID     int64
	ID           uuid.UUID
	ScopeID      uuid.UUID
}

// ResourcesLimitsUnlimited is the value a limit takes when no policy is
// configured for a scope at all.
//
// It lives in the domain because both the repository (which produces it from
// the resolution query) and the use-case (which interprets it) have to agree on
// it, and a sentinel each layer defines for itself is a sentinel they will
// eventually disagree about.
//
// It is passed through to callers rather than translated into a large number:
// "no limit configured" and "your limit is one million" are different answers,
// and only the caller can decide how to present the first.
//
// This is deliberately fail-open and is scheduled to change — under licensing,
// no policy must resolve to a free tier, not to no ceiling.
const ResourcesLimitsUnlimited = -1

// ResourcesLimitsScopeType represents the type of scope for resource limits.
type ResourcesLimitsScopeType string

const (
	// ResourcesLimitsScopeTypeSystem used for system-wide resource limits.
	ResourcesLimitsScopeTypeSystem ResourcesLimitsScopeType = "system"

	// ResourcesLimitsScopeTypeUser used for user-specific resource limits.
	ResourcesLimitsScopeTypeUser ResourcesLimitsScopeType = "user"

	// ResourcesLimitsScopeTypeProject used for project-specific resource limits.
	ResourcesLimitsScopeTypeProject ResourcesLimitsScopeType = "project"
)

func (ref ResourcesLimitsScopeType) String() string {
	return string(ref)
}

type ResourcesLimitsResourceType string

const (
	ResourcesLimitsResourceTypeUsers    ResourcesLimitsResourceType = "users"
	ResourcesLimitsResourceTypeProjects ResourcesLimitsResourceType = "projects"
	ResourcesLimitsResourceTypeProducts ResourcesLimitsResourceType = "products"
	ResourcesLimitsResourceTypeIDPs     ResourcesLimitsResourceType = "idps"
)

func (ref ResourcesLimitsResourceType) String() string {
	return string(ref)
}

// ResourceTypesForScope returns the resource types a scope type governs.
//
// The mapping is one-directional on purpose: a scope type implies its resources,
// so a status endpoint can answer "what are my limits" without the caller
// naming each resource — and without a caller being able to ask for a resource
// that scope does not own.
//
// It is also the single place this association is written down. Leaving it
// implicit across the creation paths is how a resource ends up checking one
// scope's limit while decrementing another's.
func ResourceTypesForScope(scopeType ResourcesLimitsScopeType) []ResourcesLimitsResourceType {
	switch scopeType {
	case ResourcesLimitsScopeTypeSystem:
		return []ResourcesLimitsResourceType{
			ResourcesLimitsResourceTypeUsers,
			ResourcesLimitsResourceTypeIDPs,
		}
	case ResourcesLimitsScopeTypeUser:
		return []ResourcesLimitsResourceType{
			ResourcesLimitsResourceTypeProjects,
		}
	case ResourcesLimitsScopeTypeProject:
		return []ResourcesLimitsResourceType{
			ResourcesLimitsResourceTypeProducts,
		}
	default:
		return nil
	}
}

// ResourcesLimitsResourceStatus is one resource's limit and consumption within a
// scope, as reported by the status endpoints.
type ResourcesLimitsResourceStatus struct {
	ResourceType ResourcesLimitsResourceType
	Status       ResourcesLimitsStatus
}

// ResourcesLimitsScopeStatus is everything a caller can be told about one
// scope's limits.
type ResourcesLimitsScopeStatus struct {
	ScopeType ResourcesLimitsScopeType
	Resources []ResourcesLimitsResourceStatus
	ScopeID   uuid.UUID
}

// ResourcesLimitsScope represents a scope for resource limits with type and optional ID.
type ResourcesLimitsScope struct {
	ID   *uuid.UUID // nil for system-level or default templates
	Type ResourcesLimitsScopeType
}

// ResourcesLimitsCheckUsageOutput represents the output of checking resource limits.
type ResourcesLimitsCheckUsageOutput struct {
	Signature []byte
	SoftLimit int
	HardLimit int
	Usage     int

	// HasUsageRow reports whether a counter exists for this scope at all.
	//
	// It is what separates "never used" from "counter is zero", which the
	// integrity check needs: a scope with no row has no signature to verify, and
	// treating that as tampering would refuse every tenant's first creation.
	// Checking `Usage > 0` instead — as this used to — skips verification
	// entirely whenever the counter reads zero, which is the cheapest tamper
	// available.
	HasUsageRow bool
}

// ResourcesLimitsTrackedScope identifies one counter that exists in
// resources_usage, so a reconciliation pass can walk every tracked scope
// without being told the tenant list.
type ResourcesLimitsTrackedScope struct {
	ScopeType    ResourcesLimitsScopeType
	ResourceType ResourcesLimitsResourceType
	ScopeID      uuid.UUID
}

// ResourcesLimitsRecountOutput reports what a reconciliation found.
//
// Both numbers are kept rather than just the corrected one, because the
// interesting signal is the size and direction of the drift: a counter that is
// repeatedly wrong points at a code path that mutates resources without going
// through the service, which is a bug to find rather than a number to fix.
type ResourcesLimitsRecountOutput struct {
	// Previous is the counter as stored before the recount.
	Previous int

	// Actual is what the resource table says, and what the counter now holds.
	Actual int

	// HadUsageRow reports whether a counter existed at all. A scope with
	// resources but no row has never been counted.
	HadUsageRow bool
}

// Drift is how far the stored counter had strayed. Positive means the counter
// was too high — the direction that refuses creation the tenant is entitled to.
func (ref ResourcesLimitsRecountOutput) Drift() int {
	return ref.Previous - ref.Actual
}

// Corrected reports whether the recount actually changed anything.
func (ref ResourcesLimitsRecountOutput) Corrected() bool {
	return ref.Previous != ref.Actual
}

// ResourcesLimitsStatus represents the status of resource limits.
type ResourcesLimitsStatus struct {
	SoftLimit    int
	HardLimit    int
	CurrentUsage int

	CanCreate        bool
	SoftLimitReached bool

	// TamperDetected reports that the stored counter did not match its
	// signature.
	//
	// It is a flag rather than an error because reads must keep working: a bad
	// row means a tenant cannot be trusted to *create* anything, not that they
	// should be locked out of seeing their own data. Creation is refused
	// independently, inside the reservation, which verifies under the row lock.
	TamperDetected bool
}

type SelectResourcesLimitsInput struct {
	Sort      string
	Filter    string
	Fields    string
	Paginator Paginator
}

func (ref *SelectResourcesLimitsInput) Validate() error {
	var errs ValidationErrors

	// Validate paginator
	errs.Add(ref.Paginator.Validate())

	// Validate sort expression
	errs.Add(ValidateSortExpression(ref.Sort, FieldSort))

	// Validate filter expression
	errs.Add(ValidateFilterExpression(ref.Filter, FieldFilter))

	// Validate fields expression
	errs.Add(ValidateFieldsExpression(ref.Fields, FieldFields))

	// Additional business logic validation for sort fields
	if ref.Sort != "" {
		_, err := ResourcesLimitsSortParser.Parse(ref.Sort)
		if err != nil {
			errs.AddError(FieldSort, err.Error(), "INVALID_SORT_FIELD")
		}
	}

	// Additional business logic validation for filter fields
	if ref.Filter != "" {
		_, err := ResourcesLimitsFilterParser.Parse(ref.Filter)
		if err != nil {
			errs.AddError(FieldFilter, err.Error(), "INVALID_FILTER_FIELD")
		}
	}

	// Additional business logic validation for fields
	if ref.Fields != "" {
		_, err := ResourcesLimitsFieldsParser.Parse(ref.Fields)
		if err != nil {
			errs.AddError(FieldFields, err.Error(), "INVALID_FIELDS_FIELD")
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

type SelectResourcesLimitsOutput struct {
	Items     []ResourcesLimits
	Paginator Paginator
}

type ListResourcesLimitsInput = SelectResourcesLimitsInput

type ListResourcesLimitsOutput = SelectResourcesLimitsOutput
