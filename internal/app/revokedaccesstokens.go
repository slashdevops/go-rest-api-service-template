package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/usecase"
)

// initRevokedAccessTokens builds the in-memory denylist the auth middleware
// consults. It runs before the authn service, which takes the mirror as a
// dependency so a logout can add to it the moment it revokes a token.
//
// It does not load the set — startRevokedAccessTokensMirror does that, and
// treats a failure as fatal. See there for why.
func (a *App) initRevokedAccessTokens() error {
	if !a.configs.Authn.AccessTokenRevocationEnabled.Value {
		// Explicit, because the absence of this is invisible from the outside:
		// every request still succeeds, and a logged-out access token keeps
		// working until it expires, which looks exactly like a session that has
		// not ended yet.
		slog.Warn(
			"access token revocation is off; a logged-out access token keeps working until it expires",
			"setting", a.configs.Authn.AccessTokenRevocationEnabled.FlagName,
			"window", "the access-token lifetime, a runtime setting: GET /auth/token_lifetimes",
		)

		return nil
	}

	if a.repositories.RevokedTokens == nil {
		return fmt.Errorf("access token revocation is enabled but there is no revoked tokens repository")
	}

	mirror, err := usecase.NewRevokedAccessTokens(usecase.RevokedAccessTokensConfig{
		Repository:     a.repositories.RevokedTokens,
		OT:             a.telemetry,
		ReloadInterval: a.configs.Authn.AccessTokenRevocationReloadInterval.Value,
	})
	if err != nil {
		return fmt.Errorf("error creating the revoked access token mirror: %w", err)
	}

	a.services.RevokedAccessTokens = mirror

	return nil
}

// startRevokedAccessTokensMirror loads the denylist and keeps it loaded.
//
// The FIRST load is fatal. A mirror that never loaded is an empty set, an empty
// set means "nothing is revoked", and that is the fail-open answer this whole
// mechanism exists to avoid — served silently, on every request, by a process
// that looks healthy. Refusing to start is the loud version of the same fact.
//
// Every reload after that is not fatal: the previous set is kept, the failure
// is logged, and the next tick tries again. The number to watch is the
// staleness gauge, not the failure count.
func (a *App) startRevokedAccessTokensMirror(ctx context.Context) error {
	if a.services == nil || a.services.RevokedAccessTokens == nil {
		return nil
	}

	if err := a.services.RevokedAccessTokens.Reload(ctx); err != nil {
		return fmt.Errorf("could not load the revoked access token denylist: %w", err)
	}

	slog.Info(
		"revoked access token denylist loaded",
		"size", a.services.RevokedAccessTokens.Size(),
		"reload_interval", a.configs.Authn.AccessTokenRevocationReloadInterval.Value,
		"cross_replica_staleness", a.configs.Authn.AccessTokenRevocationReloadInterval.Value,
	)

	go a.services.RevokedAccessTokens.Run(ctx)

	return nil
}

// revokedAccessTokensOrNil returns the mirror, or a nil interface when the
// check is off.
//
// Returning the concrete *usecase.RevokedAccessTokens directly would hand the
// authn service a non-nil interface wrapping a nil pointer, which is the
// classic Go trap: `!= nil` would be true and the first call would panic.
func (a *App) revokedAccessTokensOrNil() usecase.RevokedAccessTokenSet {
	if a.services.RevokedAccessTokens == nil {
		return nil
	}

	return a.services.RevokedAccessTokens
}

// revokedAccessTokensCheckerOrNil returns the mirror for the middleware, or a
// nil interface when the check is off. Same nil-interface trap as
// revokedAccessTokensOrNil.
func (a *App) revokedAccessTokensCheckerOrNil() middleware.RevokedAccessTokens {
	if a.services == nil || a.services.RevokedAccessTokens == nil {
		return nil
	}

	return a.services.RevokedAccessTokens
}
