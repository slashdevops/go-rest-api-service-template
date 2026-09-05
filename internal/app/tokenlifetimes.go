package app

import (
	"context"
	"fmt"
	"log/slog"

	"uuid"

	"github.com/valkey-io/valkey-go"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/changenotifyvalkey"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/changenotify"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/usecase"
)

// initTokenLifetimes builds the token-lifetimes mirror, its change notifier and
// the CRUD service.
//
// It does not load the row -- startTokenLifetimesMirror does, synchronously
// and fatally, before the HTTP server accepts a request. Until then the mirror
// has no value and the authn service, which reads it at issuance, must not be
// asked for one. Nothing is: the service is constructed here and first called
// from a handler.
//
// This runs BEFORE the authn service is built, because the authn service takes
// the mirror as its lifetimes provider. There is no fallback: the two startup
// flags that used to carry the lifetimes are gone, and a replica that cannot
// read the row refuses to start rather than sign tokens with a lifetime nobody
// chose.
func (a *App) initTokenLifetimes() error {
	if a.repositories.TokenLifetimes == nil {
		return fmt.Errorf("there is no token lifetimes repository")
	}

	mirror, err := usecase.NewTokenLifetimes(usecase.TokenLifetimesConfig{
		Repository:     a.repositories.TokenLifetimes,
		OT:             a.telemetry,
		ReloadInterval: a.configs.Authn.TokenLifetimesReloadInterval.Value,
	})
	if err != nil {
		return fmt.Errorf("error creating the token lifetimes mirror: %w", err)
	}

	a.services.TokenLifetimesMirror = mirror

	// The change signal, when there is a cache to carry it. Without one the
	// reload ticker is the whole mechanism, which is a supported deployment:
	// a change then takes up to authn.token.lifetimes.reload.interval to reach
	// the other replicas.
	if a.cacheClient != nil {
		subClient, err := a.changeNotifyClient()
		if err != nil {
			return fmt.Errorf("error creating the token lifetimes notification client: %w", err)
		}

		if subClient != nil {
			notifier, err := changenotifyvalkey.NewNotifier(changenotifyvalkey.NotifierConfig{
				Client:     subClient,
				Channel:    changenotifyvalkey.TokenLifetimesChannel,
				Subject:    "token lifetimes",
				InstanceID: a.instanceID(),
			})
			if err != nil {
				return fmt.Errorf("error creating the token lifetimes notifier: %w", err)
			}

			a.tokenLifetimesNotifier = notifier
		}
	}

	a.services.TokenLifetimes, err = usecase.NewTokenLifetimesService(usecase.TokenLifetimesServiceConf{
		Repository: a.repositories.TokenLifetimes,
		Mirror:     mirror,
		Notifier:   a.tokenLifetimesNotifierOrNil(),
		OT:         a.telemetry,
	})
	if err != nil {
		return fmt.Errorf("error creating the token lifetimes service: %w", err)
	}

	return nil
}

// startTokenLifetimesMirror loads the row and keeps it loaded.
//
// The FIRST load is synchronous and fatal, for the same reason the rate-limit
// rules' is: there is no fallback value, so a replica that started serving
// before the row was read would have nothing to sign a token with. Postgres is
// already a hard startup dependency and the migration that seeds the row has
// run by here, so a failure at this point is genuinely exceptional -- and it
// buys the invariant issuance relies on: if the service is serving, it has
// lifetimes.
func (a *App) startTokenLifetimesMirror(ctx context.Context) error {
	if a.services == nil || a.services.TokenLifetimesMirror == nil {
		return nil
	}

	if err := a.services.TokenLifetimesMirror.Reload(ctx); err != nil {
		return fmt.Errorf("could not load the token lifetimes: %w", err)
	}

	current := a.services.TokenLifetimesMirror.Current()

	slog.Info("token lifetimes loaded",
		"access_token", current.AccessTokenDuration,
		"refresh_token", current.RefreshTokenDuration,
		"reload_interval", a.configs.Authn.TokenLifetimesReloadInterval.Value,
		"change_signal", a.tokenLifetimesNotifier != nil,
		"edit", "PUT /auth/token_lifetimes",
	)

	go a.services.TokenLifetimesMirror.Run(ctx)

	if a.tokenLifetimesNotifier != nil {
		go a.watchTokenLifetimesChanges(ctx)
	}

	return nil
}

// watchTokenLifetimesChanges reloads the mirror when another replica writes.
// It reloads rather than applying anything from the message: the payload is a
// signal, not a value.
func (a *App) watchTokenLifetimesChanges(ctx context.Context) {
	err := a.tokenLifetimesNotifier.Watch(ctx, func() {
		if err := a.services.TokenLifetimesMirror.Reload(ctx); err != nil {
			slog.Warn("notified of a token lifetimes change but the reload failed",
				"error", err,
				"consequence", "this replica keeps issuing with the previous lifetimes until the next scheduled reload",
			)

			return
		}

		slog.Debug("token lifetimes reloaded after a change on another replica")
	})
	if err != nil {
		slog.Error("the token lifetimes change watcher stopped", "error", err)
	}
}

// tokenLifetimesNotifierOrNil returns the notifier, or a nil INTERFACE when
// there is none -- the concrete nil pointer would be a non-nil interface, and
// `!= nil` would then be true right up to the first call.
func (a *App) tokenLifetimesNotifierOrNil() changenotify.Notifier {
	if a.tokenLifetimesNotifier == nil {
		return nil
	}

	return a.tokenLifetimesNotifier
}

// changeNotifyClient returns the Valkey client every change notifier subscribes
// through, creating it on first use.
//
// A SECOND client, deliberately, and shared by the notifiers only. valkey-go
// puts a connection into subscribe mode, where it accepts nothing but further
// subscription commands, so the cache's client must not be used; but one
// subscriber client serves any number of channels, because each Watch takes a
// dedicated connection from it.
func (a *App) changeNotifyClient() (valkey.Client, error) {
	if a.changeNotifyValkey != nil {
		return a.changeNotifyValkey, nil
	}

	client, err := a.initCacheClient()
	if err != nil {
		return nil, err
	}

	a.changeNotifyValkey = client

	return client, nil
}

// instanceID identifies this replica in the change messages it publishes, so it
// can recognise the echo of its own write. One per process, shared by every
// notifier: two ids would make a replica reload on its own rate-limit write
// while ignoring its own lifetimes write, or the reverse.
func (a *App) instanceID() string {
	if a.replicaID == "" {
		a.replicaID = uuid.NewV7().String()
	}

	return a.replicaID
}
