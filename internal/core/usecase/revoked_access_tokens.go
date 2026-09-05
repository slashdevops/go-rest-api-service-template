package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"sync/atomic"
	"time"
	"uuid"

	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// clockSkewMargin widens the reload window past the access-token lifetime.
//
// expires_at is written from this process's clock and compared against the
// database's NOW(), so the two only agree to within whatever skew exists
// between them. Erring wide costs a few extra rows in the set; erring narrow
// would silently omit a revocation, which is the one failure this whole
// mechanism exists to prevent.
const clockSkewMargin = time.Minute

// RevokedAccessTokensConfig configures [RevokedAccessTokens].
type RevokedAccessTokensConfig struct {
	Repository repository.RevokedTokens
	OT         *o11y.OpenTelemetry

	MetricsPrefix string

	// AccessTokenDuration bounds the reload window. See [clockSkewMargin] and
	// [repository.RevokedTokens.SelectUnexpiredJTIs] for why this, and not the
	// refresh lifetime, is the right horizon.
	AccessTokenDuration time.Duration

	// ReloadInterval is how often the set is rebuilt from the store, and
	// therefore how stale a revocation made on another replica may be.
	ReloadInterval time.Duration
}

// RevokedAccessTokens answers "is this access token revoked?" without touching
// the database.
//
// # Why a mirror and not a lookup
//
// Revocation has to fail closed, and the store is Postgres, so checking it on
// every authenticated request would put a database round trip on the hot path —
// to close a window that the access token's own lifetime already bounds. That
// trade is why access tokens were not denylisted at all until now.
//
// A mirror changes the trade because the set is small by construction. It holds
// only revoked-and-unexpired access tokens, so it is bounded by *logouts in the
// last access-token lifetime* — five minutes by default — not by traffic, not
// by sessions, and not by the refresh rotation that writes a row per refresh.
//
// # What it costs, stated plainly
//
//   - Revocation is not instant across a cluster. A token revoked on replica A
//     is honoured by replica B for up to ReloadInterval. With a 10s interval
//     against a 5-minute token that is a 3% residue on an already-small window.
//     Pretending otherwise would be the mistake; this is written down rather
//     than hidden.
//   - A revocation made by THIS replica is effective immediately, before the
//     response to the logout is written.
//
// # The two sets, and why local additions are never dropped
//
// A reload replaces the mirror wholesale. If it replaced local additions too,
// this sequence would lose a revocation: the reload query runs, a logout
// commits and calls [RevokedAccessTokens.Add], the reload result lands and
// overwrites the set — the token is live again until the next tick, on the very
// replica that revoked it.
//
// So there are two maps. `remote` is the last good reload and is replaced
// wholesale. `local` holds what this replica revoked, is only ever added to,
// and is pruned by expiry during a reload. A lookup consults both. Both are
// copy-on-write behind an atomic pointer, so the hot path takes no lock at all
// and writers — which are logouts, and rare — pay the copy.
//
// # Failure is loud, never silent
//
// A failed reload keeps the last good copy and logs an error. It must never
// clear the set: an empty set means "nothing is revoked", which is the
// fail-open answer this exists to avoid. The initial load is fatal at startup
// for the same reason — serving with a set that was never built is serving with
// the check switched off.
type RevokedAccessTokens struct {
	repository repository.RevokedTokens
	ot         *o11y.OpenTelemetry

	// remote is the last successful reload. Replaced wholesale, never mutated.
	remote atomic.Pointer[map[uuid.UUID]struct{}]

	// local is what this replica revoked, jti to the moment it expires.
	// Replaced wholesale, never mutated.
	local atomic.Pointer[map[uuid.UUID]time.Time]

	// writeMu serialises the copy-on-write of local. It is never taken on the
	// read path.
	writeMu sync.Mutex

	// lastReload is the unix-nano instant of the last SUCCESSFUL reload. Zero
	// until the first one lands.
	lastReload atomic.Int64

	reloadFailures atomic.Int64

	horizon        time.Duration
	reloadInterval time.Duration
}

// NewRevokedAccessTokens builds the mirror. It does not load anything; call
// [RevokedAccessTokens.Reload] before serving traffic.
func NewRevokedAccessTokens(conf RevokedAccessTokensConfig) (*RevokedAccessTokens, error) {
	if conf.Repository == nil {
		return nil, &domain.InvalidRepositoryError{Message: "RevokedTokens repository is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	if conf.AccessTokenDuration <= 0 {
		return nil, &domain.InvalidInputError{Message: "AccessTokenDuration must be positive"}
	}

	if conf.ReloadInterval <= 0 {
		return nil, &domain.InvalidInputError{Message: "ReloadInterval must be positive"}
	}

	ref := &RevokedAccessTokens{
		repository:     conf.Repository,
		ot:             conf.OT,
		horizon:        conf.AccessTokenDuration + clockSkewMargin,
		reloadInterval: conf.ReloadInterval,
	}

	ref.remote.Store(&map[uuid.UUID]struct{}{})
	ref.local.Store(&map[uuid.UUID]time.Time{})

	if err := ref.registerMetrics(conf.MetricsPrefix); err != nil {
		return nil, err
	}

	return ref, nil
}

// registerMetrics publishes the two numbers an operator needs to know whether
// this is working: how many tokens are refused, and how long ago the set was
// last rebuilt from the truth.
//
// The second is the one that matters. A mirror that stops reloading keeps
// answering, confidently, from a snapshot that gets older every second — and
// every symptom of that is invisible. Alert on staleness, not on failures.
func (ref *RevokedAccessTokens) registerMetrics(prefix string) error {
	name := func(suffix string) string {
		if prefix == "" {
			return suffix
		}

		return fmt.Sprintf("%s_%s", prefix, suffix)
	}

	if _, err := ref.ot.Metrics.Meter.Int64ObservableGauge(
		name("revoked_access_tokens_size"),
		metric.WithDescription("Number of revoked, unexpired access tokens this replica refuses"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(int64(ref.Size()))

			return nil
		}),
	); err != nil {
		return err
	}

	if _, err := ref.ot.Metrics.Meter.Float64ObservableGauge(
		name("revoked_access_tokens_staleness_seconds"),
		metric.WithDescription("Seconds since the revocation set was last rebuilt from the store; grows without bound if reloads are failing"),
		metric.WithUnit("s"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			o.Observe(ref.Staleness().Seconds())

			return nil
		}),
	); err != nil {
		return err
	}

	if _, err := ref.ot.Metrics.Meter.Int64ObservableCounter(
		name("revoked_access_tokens_reload_failures"),
		metric.WithDescription("Reloads of the revocation set that failed; the previous set is kept"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(ref.reloadFailures.Load())

			return nil
		}),
	); err != nil {
		return err
	}

	return nil
}

// Contains reports whether jti names a revoked access token.
//
// O(1), no I/O, no lock. This runs on every authenticated request.
func (ref *RevokedAccessTokens) Contains(jti uuid.UUID) bool {
	if remote := ref.remote.Load(); remote != nil {
		if _, found := (*remote)[jti]; found {
			return true
		}
	}

	if local := ref.local.Load(); local != nil {
		if _, found := (*local)[jti]; found {
			return true
		}
	}

	return false
}

// Add refuses jti from this instant, on this replica, without waiting for a
// reload. expiresAt is the token's own exp; past it the entry is dead weight
// and the next reload drops it.
//
// Callers must still record the revocation in the store — this is the local
// half of a revocation, not the revocation itself. A replica that only called
// Add would forget on restart and would never tell the other replicas.
func (ref *RevokedAccessTokens) Add(jti uuid.UUID, expiresAt time.Time) {
	if jti == uuid.Nil() {
		return
	}

	ref.writeMu.Lock()
	defer ref.writeMu.Unlock()

	current := ref.local.Load()

	next := make(map[uuid.UUID]time.Time, len(*current)+1)
	maps.Copy(next, *current)

	next[jti] = expiresAt

	ref.local.Store(&next)
}

// Reload rebuilds the set from the store.
//
// On failure the previous set is kept and the error is returned: a mirror that
// emptied itself when the database blinked would re-validate every token
// anybody had logged out of.
func (ref *RevokedAccessTokens) Reload(ctx context.Context) error {
	jtis, err := ref.repository.SelectUnexpiredJTIs(ctx, ref.horizon)
	if err != nil {
		ref.reloadFailures.Add(1)

		return err
	}

	next := make(map[uuid.UUID]struct{}, len(jtis))
	for _, jti := range jtis {
		next[jti] = struct{}{}
	}

	ref.remote.Store(&next)
	ref.lastReload.Store(time.Now().UnixNano())

	ref.pruneLocal()

	return nil
}

// pruneLocal drops local entries whose token has expired on its own. It is the
// only thing that removes from local, and it is safe: an expired token is
// refused for being expired, so forgetting that it was also revoked changes no
// answer.
func (ref *RevokedAccessTokens) pruneLocal() {
	ref.writeMu.Lock()
	defer ref.writeMu.Unlock()

	current := ref.local.Load()

	now := time.Now()

	live := 0

	for _, expiresAt := range *current {
		if now.Before(expiresAt) {
			live++
		}
	}

	if live == len(*current) {
		return
	}

	next := make(map[uuid.UUID]time.Time, live)

	for jti, expiresAt := range *current {
		if now.Before(expiresAt) {
			next[jti] = expiresAt
		}
	}

	ref.local.Store(&next)
}

// Size is how many tokens this replica currently refuses.
func (ref *RevokedAccessTokens) Size() int {
	size := 0

	if remote := ref.remote.Load(); remote != nil {
		size += len(*remote)
	}

	if local := ref.local.Load(); local != nil {
		size += len(*local)
	}

	return size
}

// Staleness is how long ago the set was last rebuilt from the store, and is the
// number to alert on. Before the first successful reload it reports the largest
// possible staleness rather than zero, so a mirror that has never loaded does
// not look freshly loaded.
func (ref *RevokedAccessTokens) Staleness() time.Duration {
	last := ref.lastReload.Load()
	if last == 0 {
		return time.Duration(1<<63 - 1)
	}

	return time.Since(time.Unix(0, last))
}

// Run reloads on a ticker until ctx is done. A failed reload is logged and the
// previous set is kept; the next tick tries again.
func (ref *RevokedAccessTokens) Run(ctx context.Context) {
	ticker := time.NewTicker(ref.reloadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Debug("stopping the revoked access token mirror", "cause", context.Cause(ctx))

			return
		case <-ticker.C:
			if err := ref.Reload(ctx); err != nil {
				// Loud, because every symptom of this is invisible: the mirror
				// keeps answering from a set that is quietly getting older, and
				// a revocation made on another replica is never seen here.
				slog.Error(
					"could not reload the revoked access token set; serving from the previous copy",
					"error", err,
					"staleness", ref.Staleness(),
					"size", ref.Size(),
				)
			}
		}
	}
}

// RevokedAccessTokenSet is the half of [RevokedAccessTokens] a revoking caller
// needs: add this token, now, on this replica.
//
// An interface so the authn service depends on the operation rather than on the
// mirror, and so "the check is switched off" is expressible as a nil.
type RevokedAccessTokenSet interface {
	Add(jti uuid.UUID, expiresAt time.Time)
}

var _ RevokedAccessTokenSet = (*RevokedAccessTokens)(nil)
