package repository

import (
	"context"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/resources_limits.go -source=resources_limits.go ResourcesLimits

// ResourcesLimitsSigner produces the integrity signature for a usage counter
// that has just moved to newUsage.
//
// It is passed into the repository rather than applied afterwards because the
// counter and its signature must be written in the same transaction. When they
// were two calls, two concurrent increments interleaved as
//
//	T1 increment -> 5
//	T2 increment -> 6
//	T2 write signature for 6
//	T1 write signature for 5   (last writer wins)
//
// leaving usage 6 stored against a signature for 5. Every later read of that
// scope then failed verification and the tenant could not create anything, with
// no way back: only the service can mint a valid signature, so no SQL repair is
// possible. Holding the row lock from the increment until the signature is
// written makes concurrent updates serialise instead.
//
// Implementations must be pure — the repository calls this while holding a
// database transaction open.
type ResourcesLimitsSigner func(newUsage int) ([]byte, error)

// ResourcesLimitsVerifier checks a stored counter against its stored signature
// and reports whether the pair is intact.
//
// Like [ResourcesLimitsSigner] it is passed into the repository so the check can
// happen inside the transaction that is about to mutate the row, under the same
// lock. Verifying beforehand in the use-case would leave a window in which the
// row changes between the check and the write.
//
// It is called only when a usage row actually exists. A scope that has never
// been used has no signature, and treating that as tampering would refuse every
// tenant's first creation.
type ResourcesLimitsVerifier func(usage int, signature []byte) error

// ResourcesLimits is the driven persistence port for resource-limit
// usage tracking.
type ResourcesLimits interface {
	Select(ctx context.Context, input *domain.SelectResourcesLimitsInput) (*domain.SelectResourcesLimitsOutput, error)

	// ReserveUsage claims one unit of a resource for a scope, or returns
	// [domain.ResourcesLimitsHardLimitReachedError] when the hard limit is
	// already reached. Resolution, signature verification, the limit test and
	// the increment all happen under one row lock, which is what makes
	// enforcement safe against concurrent callers.
	//
	// Callers reserve before creating the resource and release with
	// DecrementUsage if creation then fails.
	ReserveUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType, verify ResourcesLimitsVerifier, sign ResourcesLimitsSigner) (int, error)

	// IncrementUsage raises the counter by one and stores the signature for the
	// resulting value atomically. It returns the new usage.
	IncrementUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType, sign ResourcesLimitsSigner) (int, error)

	// DecrementUsage lowers the counter by one, never below zero, and stores the
	// signature for the resulting value atomically. It returns the new usage.
	DecrementUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType, sign ResourcesLimitsSigner) (int, error)

	CheckUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType) (*domain.ResourcesLimitsCheckUsageOutput, error)

	// RecountUsage recomputes the counter from the resource table it tracks and
	// stores the corrected value with a fresh signature.
	//
	// This is the only way back from drift. The counter is a second source of
	// truth, so anything that removes a resource without going through the
	// service's delete path — direct SQL, an operator cleanup, an ON DELETE
	// CASCADE — leaves it high, and it only ever drifts upward, toward refusing
	// creation of resources the tenant is entitled to. Observed in development:
	// an idps counter pinned at its ceiling while the table was empty.
	//
	// It cannot be a SQL script, because a hand-written counter fails signature
	// verification. Repair has to happen here, where the signing key is reachable.
	RecountUsage(ctx context.Context, scope domain.ResourcesLimitsScope, resourceType domain.ResourcesLimitsResourceType, sign ResourcesLimitsSigner) (*domain.ResourcesLimitsRecountOutput, error)

	// SelectTrackedScopes returns every scope that currently has a usage row, so
	// a reconciliation pass can walk them without knowing the tenant list.
	SelectTrackedScopes(ctx context.Context) ([]domain.ResourcesLimitsTrackedScope, error)
}
