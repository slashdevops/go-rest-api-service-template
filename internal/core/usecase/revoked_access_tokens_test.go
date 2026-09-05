package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// fakeRevokedTokens is a repository.RevokedTokens that only implements the one
// method the mirror uses. Every other method panics, which is the point: if the
// mirror ever starts reaching for the database on the read path, these tests
// stop compiling quietly and start failing loudly.
type fakeRevokedTokens struct {
	mu     sync.Mutex
	jtis   []uuid.UUID
	err    error
	calls  int
	lastAt time.Duration
}

func (f *fakeRevokedTokens) SelectUnexpiredJTIs(_ context.Context, horizon time.Duration) ([]uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	f.lastAt = horizon

	if f.err != nil {
		return nil, f.err
	}

	return append([]uuid.UUID(nil), f.jtis...), nil
}

func (f *fakeRevokedTokens) set(jtis ...uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.jtis = jtis
	f.err = nil
}

func (f *fakeRevokedTokens) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.err = err
}

func (f *fakeRevokedTokens) Revoke(context.Context, uuid.UUID, uuid.UUID, time.Time) error {
	panic("the mirror must not write")
}

func (f *fakeRevokedTokens) Rotate(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error {
	panic("the mirror must not write")
}

func (f *fakeRevokedTokens) Consume(context.Context, uuid.UUID, uuid.UUID, time.Time) (bool, error) {
	panic("the mirror must not write")
}

func (f *fakeRevokedTokens) Get(context.Context, uuid.UUID) (*domain.TokenRevocation, error) {
	panic("the mirror must not read per token; that is the lookup it exists to avoid")
}

func (f *fakeRevokedTokens) RevokeChain(context.Context, uuid.UUID, uuid.UUID, time.Time) (uuid.UUID, error) {
	panic("the mirror must not write")
}

func (f *fakeRevokedTokens) DeleteExpired(context.Context) (int64, error) {
	panic("the mirror must not sweep")
}

func newTestMirror(t *testing.T, repo *fakeRevokedTokens) *RevokedAccessTokens {
	t.Helper()

	ctx := t.Context()

	ot := &o11y.OpenTelemetry{
		Traces:  o11y.NewOpenTelemetryTracer(ctx, &o11y.OpenTelemetryTracerConfig{Name: "test"}),
		Metrics: o11y.NewOpenTelemetryMeter(ctx, &o11y.OpenTelemetryMeterConfig{Name: "test"}),
	}

	mirror, err := NewRevokedAccessTokens(RevokedAccessTokensConfig{
		Repository:          repo,
		OT:                  ot,
		AccessTokenDuration: 5 * time.Minute,
		ReloadInterval:      10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRevokedAccessTokens: %v", err)
	}

	return mirror
}

// TestALocalRevocationSurvivesAReload is the reason there are two maps rather
// than one.
//
// A reload replaces the set wholesale. If it replaced local additions too, this
// interleaving loses a revocation entirely: the reload query runs and returns
// rows that predate the logout, the logout commits and calls Add, and then the
// reload result lands and overwrites the set. The token is live again — on the
// very replica that just revoked it — until the next tick.
//
// The window is real, not theoretical: it is exactly the duration of the reload
// query, and logouts are not rare.
func TestALocalRevocationSurvivesAReload(t *testing.T) {
	t.Parallel()

	repo := &fakeRevokedTokens{}
	mirror := newTestMirror(t, repo)

	revoked := uuid.NewV7()
	mirror.Add(revoked, time.Now().Add(5*time.Minute))

	if !mirror.Contains(revoked) {
		t.Fatal("Add must take effect immediately, before any reload")
	}

	// The reload returns a set that does NOT contain it — the row was committed
	// after this query read the table.
	repo.set(uuid.NewV7(), uuid.NewV7())

	if err := mirror.Reload(t.Context()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if !mirror.Contains(revoked) {
		t.Error("a reload dropped a revocation this replica had made; the token is live again on the replica that revoked it")
	}
}

// TestAFailedReloadKeepsTheLastGoodSet pins the fail-closed half.
//
// An empty set means "nothing is revoked". Clearing the mirror when the
// database blinks would re-validate every token anybody had logged out of, on
// every replica at once, and nothing about the service would look wrong.
func TestAFailedReloadKeepsTheLastGoodSet(t *testing.T) {
	t.Parallel()

	repo := &fakeRevokedTokens{}
	mirror := newTestMirror(t, repo)

	revoked := uuid.NewV7()
	repo.set(revoked)

	if err := mirror.Reload(t.Context()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if !mirror.Contains(revoked) {
		t.Fatal("the first reload did not take effect")
	}

	repo.fail(errors.New("connection refused"))

	if err := mirror.Reload(t.Context()); err == nil {
		t.Error("a failed reload must return the error, not swallow it")
	}

	if !mirror.Contains(revoked) {
		t.Error("a failed reload emptied the set; every logged-out token is valid again")
	}
}

// TestStalenessBeforeTheFirstLoadIsNotZero matters because staleness is the
// number an operator alerts on. A mirror that has never loaded reporting
// "0 seconds since last reload" would look healthier than one that reloaded a
// second ago.
func TestStalenessBeforeTheFirstLoadIsNotZero(t *testing.T) {
	t.Parallel()

	mirror := newTestMirror(t, &fakeRevokedTokens{})

	if got := mirror.Staleness(); got < 24*time.Hour {
		t.Errorf("Staleness() before any load = %v, want something enormous", got)
	}

	if err := mirror.Reload(t.Context()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if got := mirror.Staleness(); got > time.Minute {
		t.Errorf("Staleness() straight after a load = %v, want ~0", got)
	}
}

// TestExpiredLocalEntriesArePruned keeps the local half bounded. Dropping an
// expired entry changes no answer — the token is refused for being expired —
// but keeping it forever would make this map grow for the life of the process.
func TestExpiredLocalEntriesArePruned(t *testing.T) {
	t.Parallel()

	repo := &fakeRevokedTokens{}
	mirror := newTestMirror(t, repo)

	stale := uuid.NewV7()
	live := uuid.NewV7()

	mirror.Add(stale, time.Now().Add(-time.Second))
	mirror.Add(live, time.Now().Add(5*time.Minute))

	if got := mirror.Size(); got != 2 {
		t.Fatalf("Size() = %d, want 2", got)
	}

	if err := mirror.Reload(t.Context()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if mirror.Contains(stale) {
		t.Error("an expired local entry was kept")
	}

	if !mirror.Contains(live) {
		t.Error("prune dropped a live entry")
	}
}

// TestTheReloadWindowCoversTheAccessTokenLifetime pins the horizon.
//
// The window is what keeps the set small, and it is only safe because an access
// token cannot outlive its own lifetime. A horizon SHORTER than that lifetime
// would silently omit live revocations — the mirror would look fine, report a
// healthy size and a fresh staleness, and let revoked tokens through.
func TestTheReloadWindowCoversTheAccessTokenLifetime(t *testing.T) {
	t.Parallel()

	repo := &fakeRevokedTokens{}
	mirror := newTestMirror(t, repo)

	if err := mirror.Reload(t.Context()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	repo.mu.Lock()
	horizon := repo.lastAt
	repo.mu.Unlock()

	if horizon <= 5*time.Minute {
		t.Errorf("reload horizon = %v, want strictly more than the 5m access-token lifetime; equal is not enough because expires_at is written from this process's clock and compared against the database's", horizon)
	}
}

// TestContainsIsSafeUnderConcurrentAdds is not about correctness of the answer
// but about the read path taking no lock: Contains runs on every authenticated
// request, and Add runs on every logout.
func TestContainsIsSafeUnderConcurrentAdds(t *testing.T) {
	t.Parallel()

	mirror := newTestMirror(t, &fakeRevokedTokens{})

	var wg sync.WaitGroup

	for range 8 {
		wg.Go(func() {
			for range 100 {
				mirror.Add(uuid.NewV7(), time.Now().Add(time.Minute))
			}
		})
	}

	for range 8 {
		wg.Go(func() {
			for range 100 {
				_ = mirror.Contains(uuid.NewV7())
			}
		})
	}

	wg.Wait()

	if got := mirror.Size(); got != 800 {
		t.Errorf("Size() = %d, want 800; a lost update means the copy-on-write is racy", got)
	}
}
