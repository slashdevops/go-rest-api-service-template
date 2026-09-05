package driving

import (
	"context"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// AuthnIDPs is the driving port consumed by the HTTP authn-IDP handler.
type AuthnIDPs interface {
	GetLoginURL(ctx context.Context, idpID uuid.UUID, eventType domain.IDPEventType) (string, error)
	Callback(ctx context.Context, idpID uuid.UUID, state, code string) domain.IDPCallbackResult
}
