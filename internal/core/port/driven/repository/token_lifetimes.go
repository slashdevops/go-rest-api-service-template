package repository

import (
	"context"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/token_lifetimes.go -source=token_lifetimes.go TokenLifetimes

// TokenLifetimes is the driven persistence port for the one row of
// authn_token_lifetimes.
//
// Two operations, because the row is a singleton: it is seeded by migration,
// never created through the API, and never deleted. Get is what the mirror
// reloads from and what the GET endpoint answers with; Update is the whole of
// PUT.
//
// An error from Get must be treated as "unknown", never as "use the defaults":
// there is no fallback value anywhere in Go, and a replica that cannot read the
// row refuses to start rather than issue tokens with a lifetime nobody chose.
type TokenLifetimes interface {
	// Get returns the row. A missing row is a *domain.TokenLifetimesNotFoundError,
	// which callers treat as a fault, not as an empty configuration.
	Get(ctx context.Context) (*domain.TokenLifetimes, error)

	// Update replaces both durations and records who did it, returning the row
	// as stored. The database CHECK constraints are the last line; a violation
	// surfaces as a *domain.ValidationErrors so the caller sees a 400 and not
	// a 500, but the use case validates first and should never reach them.
	Update(ctx context.Context, input *domain.UpdateTokenLifetimesInput) (*domain.TokenLifetimes, error)
}
