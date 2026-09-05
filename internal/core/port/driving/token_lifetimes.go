package driving

import (
	"context"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../mocks/handler/token_lifetimes.go -source=token_lifetimes.go TokenLifetimes

// TokenLifetimes is the driving port consumed by the HTTP token-lifetimes
// handler: read the singleton, replace the singleton.
//
// No List, Create or Delete. The row is seeded by migration and a service with
// no lifetimes cannot issue a token, so the shape is deliberately GET + PUT and
// nothing else -- "reset to defaults" is a PUT of the defaults GET returns.
type TokenLifetimes interface {
	// Get returns the stored row -- the source of truth, not this replica's
	// mirror, so it never disagrees with what a PUT just wrote.
	Get(ctx context.Context) (*domain.TokenLifetimes, error)

	// Update validates, writes, applies the change to this replica's mirror,
	// and tells the other replicas. Only the write can fail the call.
	Update(ctx context.Context, input *domain.UpdateTokenLifetimesInput) (*domain.TokenLifetimes, error)
}
