package usecase

import (
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/cache"
)

// authzCacheKey is the one key a user's effective permissions live under.
// Eight call sites used to spell it out by hand, which is exactly the drift
// a single helper exists to prevent; the AuthzServiceCache interface that was
// meant to own it was declared and implemented by nothing.
func authzCacheKey(userID uuid.UUID) cache.Identifier {
	return cache.Identifier{Type: "authz", ID: userID.String()}
}
