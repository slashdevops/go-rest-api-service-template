package app

import (
	"context"
	"log/slog"
	"time"
)

// startRevokedTokensSweeper deletes denylist rows once the token they name has
// expired on its own.
//
// # Why this exists now and did not before
//
// The denylist began as a record of logouts, which is a slow trickle and grows
// with something a person does rather than with traffic. Refresh rotation
// changed that: every refresh spends a token and writes a row, so the table
// now takes a row per refresh — with a five-minute access token against a
// twenty-four-hour session, on the order of 288 rows per session per day. A
// table nothing prunes would grow for as long as the service runs.
//
// # Why sweeping cannot change an answer
//
// A row is dead the instant the token it names would have expired anyway: past
// that point the token is refused for being expired, not for being revoked, so
// deleting the row removes a reason that is no longer the reason. Every lookup
// already carries the same `expires_at > NOW()` predicate, which is what lets
// this run on its own schedule — late, early, or not at all — without any
// request seeing a different outcome.
//
// # Replicas
//
// Every replica sweeps. The delete is idempotent and bounded by an index, so
// concurrent sweeps race only to do the same work, and the loser deletes
// nothing. That is cheaper than electing one replica to do it.
func (a *App) startRevokedTokensSweeper(ctx context.Context) {
	if a.repositories.RevokedTokens == nil {
		return
	}

	interval := a.configs.Authn.RevokedTokensSweepInterval.Value
	if interval <= 0 {
		slog.Warn("revoked token sweep disabled; the denylist will grow without bound while refresh rotation is on",
			"setting", a.configs.Authn.RevokedTokensSweepInterval.FlagName)

		return
	}

	go func() {
		// Once at startup, because a replica that restarts more often than the
		// interval would otherwise never reach the first tick.
		a.sweepRevokedTokens(ctx)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Debug("stopping the revoked token sweeper", "cause", context.Cause(ctx))

				return
			case <-ticker.C:
				a.sweepRevokedTokens(ctx)
			}
		}
	}()
}

// sweepRevokedTokens runs one sweep. A failure is logged and nothing else: the
// rows it could not delete are dead weight, not a correctness problem, and the
// next tick will try again.
func (a *App) sweepRevokedTokens(ctx context.Context) {
	start := time.Now()

	removed, err := a.repositories.RevokedTokens.DeleteExpired(ctx)
	if err != nil {
		slog.Error("could not sweep expired revoked tokens; they will be retried on the next tick", "error", err)

		return
	}

	if removed == 0 {
		slog.Debug("swept expired revoked tokens, none had expired", "duration", time.Since(start))

		return
	}

	slog.Info("swept expired revoked tokens", "removed", removed, "duration", time.Since(start))
}
