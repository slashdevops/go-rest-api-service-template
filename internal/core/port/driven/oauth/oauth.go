// Package oauth defines the driven port that use-cases consume to drive an
// identity-provider sign-in. The implementation lives in
// internal/adapter/driven/oauthidp: OpenID Connect with discovery, PKCE, a
// nonce and a verified ID token for the oidc kind, plain OAuth2 against fixed
// endpoints for the github kind.
//
// Use-cases stay free of HTTP, JSON and provider shapes. They mint the state,
// the nonce and the PKCE verifier, ask for the authorization URL, and hand the
// callback's code back with the verifier and nonce; what comes back is one
// [domain.UserInfo] whose Subject is the identity.
package oauth

import (
	"context"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/oauth.go -source=oauth.go Provider

// AuthRequest is what the use case decided before sending the browser away.
//
// State is the signed, single-use token the provider echoes back. Nonce goes
// into the ID token and is checked on return, so a token minted for another
// request cannot complete this one. CodeVerifier is the PKCE secret the use
// case keeps (encrypted, inside the state) and presents at the exchange; the
// adapter derives the S256 challenge from it.
type AuthRequest struct {
	State        string
	Nonce        string
	CodeVerifier string
}

// Provider is the driven port consumed by use-cases.
type Provider interface {
	// AuthCodeURL builds the provider's authorization URL for req.
	AuthCodeURL(ctx context.Context, idp *domain.IDP, req AuthRequest) (string, error)

	// Exchange trades the callback code for the person's identity: the token
	// exchange with PKCE, the ID token verified against the discovered keys
	// and the nonce for the oidc kind, the user and emails endpoints for the
	// github kind. Every failure is one of this service's own errors; the
	// provider's wording goes to the log, never to the caller.
	Exchange(ctx context.Context, idp *domain.IDP, code string, req AuthRequest) (*domain.UserInfo, error)
}
