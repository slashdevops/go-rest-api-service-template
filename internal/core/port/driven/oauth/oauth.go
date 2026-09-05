// Package oauth defines the driven port that use-cases consume to
// drive an OAuth2-based identity provider login flow. The
// implementation lives in internal/adapter/driven/oauthidp which
// wraps golang.org/x/oauth2 and the per-IDP UserInfo HTTP endpoints.
//
// Use-cases stay free of HTTP, JSON, and IDP-specific shapes; they
// describe what they want ("build a login URL", "exchange this
// callback code for a UserInfo"), and the adapter handles the
// transport and parsing.
package oauth

import (
	"context"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/oauth.go -source=oauth.go Provider

// Provider is the driven port consumed by use-cases.
type Provider interface {
	// LoginURL builds the IDP authorization URL the user should be
	// redirected to. state is a CSRF/correlation token the IDP will
	// echo back on callback.
	LoginURL(ctx context.Context, idp *domain.IDP, state string) (string, error)

	// Authenticate exchanges a callback `code` for the user's
	// canonical identity at the IDP. The adapter handles the
	// OAuth token exchange, the UserInfo HTTP fetch, and the
	// per-IDP response parsing.
	Authenticate(ctx context.Context, idp *domain.IDP, code string) (*domain.UserInfo, error)
}
