package driving

import (
	"context"

	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../mocks/handler/authn_idp.go -source=authn_idp.go AuthnIDPs

// AuthnIDPs is the driving port consumed by the HTTP authn-IDP handler.
//
// Three events start at a provider: login, register, and link. The first two
// are anonymous and end with a session; link is started by a signed-in user
// and ends with a users_identities row and no new session.
type AuthnIDPs interface {
	// GetLoginURL signs the state and builds the provider's authorization
	// URL. userID is the account a link is for and uuid.Nil() for the other
	// two events.
	GetLoginURL(ctx context.Context, idpID uuid.UUID, eventType domain.IDPEventType, userID uuid.UUID) (string, error)

	// Callback spends the state, exchanges the code, resolves the identity and
	// performs the event the state was signed for. Every refusal is an error
	// the handler maps; there is no half-success.
	Callback(ctx context.Context, idpID uuid.UUID, state, code string) (*domain.IDPCallbackOutput, error)

	// ListIdentities answers the profile page: which providers are linked to
	// this account.
	ListIdentities(ctx context.Context, userID uuid.UUID) ([]domain.UserIdentity, error)

	// UnlinkIdentity removes one provider from the account. Refused when it
	// would leave an account with no password and no identity -- nothing could
	// sign in to it afterwards.
	UnlinkIdentity(ctx context.Context, userID, idpID uuid.UUID) error
}
