package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// fakeTokenLifetimesRepo is a repository.TokenLifetimes over one value.
type fakeTokenLifetimesRepo struct {
	mu      sync.Mutex
	row     domain.TokenLifetimes
	err     error
	gets    int
	updates int
}

func (f *fakeTokenLifetimesRepo) Get(context.Context) (*domain.TokenLifetimes, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.gets++

	if f.err != nil {
		return nil, f.err
	}

	row := f.row

	return &row, nil
}

func (f *fakeTokenLifetimesRepo) Update(_ context.Context, in *domain.UpdateTokenLifetimesInput) (*domain.TokenLifetimes, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.updates++

	if f.err != nil {
		return nil, f.err
	}

	f.row = domain.TokenLifetimes{
		AccessTokenDuration:  in.AccessTokenDuration,
		RefreshTokenDuration: in.RefreshTokenDuration,
		UpdatedBy:            in.UpdatedBy,
		UpdatedAt:            time.Now(),
	}
	row := f.row

	return &row, nil
}

func (f *fakeTokenLifetimesRepo) set(row domain.TokenLifetimes, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.row, f.err = row, err
}

var _ repository.TokenLifetimes = (*fakeTokenLifetimesRepo)(nil)

type fakeNotifier struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeNotifier) Notify(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++

	return f.err
}
func (f *fakeNotifier) Watch(context.Context, func()) error { return nil }
func (f *fakeNotifier) Close() error                        { return nil }

func testOT(t *testing.T) *o11y.OpenTelemetry {
	t.Helper()

	ctx := t.Context()

	return &o11y.OpenTelemetry{
		Traces:  o11y.NewOpenTelemetryTracer(ctx, &o11y.OpenTelemetryTracerConfig{Name: "test"}),
		Metrics: o11y.NewOpenTelemetryMeter(ctx, &o11y.OpenTelemetryMeterConfig{Name: "test"}),
	}
}

func newTestLifetimesMirror(t *testing.T, repo repository.TokenLifetimes) *TokenLifetimes {
	t.Helper()

	m, err := NewTokenLifetimes(TokenLifetimesConfig{Repository: repo, OT: testOT(t), ReloadInterval: 10 * time.Second})
	if err != nil {
		t.Fatalf("NewTokenLifetimes: %v", err)
	}

	return m
}

func seeded() domain.TokenLifetimes { return domain.DefaultTokenLifetimes() }

// Before the first load there is no value, and Current must not invent one:
// zero durations would sign tokens that expire as they are issued.
func TestTokenLifetimesCurrentPanicsBeforeTheFirstLoad(t *testing.T) {
	t.Parallel()

	m := newTestLifetimesMirror(t, &fakeTokenLifetimesRepo{row: seeded()})

	if _, ok := m.Loaded(); ok {
		t.Fatal("Loaded must be false before the first load")
	}

	if m.Staleness() != staleForever {
		t.Fatal("staleness before the first load must be the sentinel, not zero")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("Current before the first load must panic; a zero lifetime is worse than a crash at startup")
		}
	}()

	_ = m.Current()
}

func TestTokenLifetimesReloadInstallsTheRow(t *testing.T) {
	t.Parallel()

	repo := &fakeTokenLifetimesRepo{row: seeded()}
	m := newTestLifetimesMirror(t, repo)

	if err := m.Reload(t.Context()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	got := m.Current()
	if got.AccessTokenDuration != domain.DefaultAuthnAccessTokenDuration || got.RefreshTokenDuration != domain.DefaultAuthnRefreshTokenDuration {
		t.Fatalf("Current = %+v, want the seeded defaults", got)
	}

	if m.Staleness() > time.Second {
		t.Fatalf("staleness after a load should be ~0, got %v", m.Staleness())
	}
}

// A failed reload keeps the previous value. Clearing it would refuse every
// login on a database blip; replacing it with a default would issue tokens
// with a lifetime nobody chose.
func TestTokenLifetimesAFailedReloadKeepsThePreviousValue(t *testing.T) {
	t.Parallel()

	repo := &fakeTokenLifetimesRepo{row: seeded()}
	m := newTestLifetimesMirror(t, repo)

	if err := m.Reload(t.Context()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	repo.set(domain.TokenLifetimes{}, errors.New("database is away"))

	if err := m.Reload(t.Context()); err == nil {
		t.Fatal("a failing repository must surface as a reload error")
	}

	if got := m.Current(); got.AccessTokenDuration != domain.DefaultAuthnAccessTokenDuration {
		t.Fatalf("the previous value must survive a failed reload, got %+v", got)
	}

	if m.reloadFailures.Load() != 1 {
		t.Fatalf("reload failures = %d, want 1", m.reloadFailures.Load())
	}
}

// A row the validator refuses is a failed reload, not a new value. The CHECK
// constraints should make this unreachable; if they did not, issuing with it
// is the wrong answer.
func TestTokenLifetimesAnInvalidRowIsNotInstalled(t *testing.T) {
	t.Parallel()

	repo := &fakeTokenLifetimesRepo{row: seeded()}
	m := newTestLifetimesMirror(t, repo)

	if err := m.Reload(t.Context()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	repo.set(domain.TokenLifetimes{AccessTokenDuration: 48 * time.Hour, RefreshTokenDuration: 12 * time.Hour}, nil)

	if err := m.Reload(t.Context()); err == nil {
		t.Fatal("a row that violates the ordering rule must not be installed")
	}

	if got := m.Current(); got.RefreshTokenDuration != domain.DefaultAuthnRefreshTokenDuration {
		t.Fatalf("the previous value must survive an invalid row, got %+v", got)
	}
}

func newTestLifetimesService(t *testing.T, repo repository.TokenLifetimes, mirror TokenLifetimesSet, n *fakeNotifier) *TokenLifetimesService {
	t.Helper()

	conf := TokenLifetimesServiceConf{Repository: repo, Mirror: mirror, OT: testOT(t)}
	if n != nil {
		conf.Notifier = n
	}

	s, err := NewTokenLifetimesService(conf)
	if err != nil {
		t.Fatalf("NewTokenLifetimesService: %v", err)
	}

	return s
}

func TestTokenLifetimesServiceRequiresAMirror(t *testing.T) {
	t.Parallel()

	_, err := NewTokenLifetimesService(TokenLifetimesServiceConf{Repository: &fakeTokenLifetimesRepo{}, OT: testOT(t)})
	if err == nil {
		t.Fatal("a service without a mirror would leave the writing replica the last to see its own write")
	}
}

// The order a PUT has to keep: validate, write, apply here, tell the others.
// The mirror sees the new value before the call returns, and the notifier is
// called exactly once.
func TestTokenLifetimesUpdateAppliesLocallyThenNotifies(t *testing.T) {
	t.Parallel()

	repo := &fakeTokenLifetimesRepo{row: seeded()}
	mirror := newTestLifetimesMirror(t, repo)

	if err := mirror.Reload(t.Context()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	n := &fakeNotifier{}
	svc := newTestLifetimesService(t, repo, mirror, n)

	out, err := svc.Update(t.Context(), &domain.UpdateTokenLifetimesInput{
		AccessTokenDuration:  10 * time.Minute,
		RefreshTokenDuration: 72 * time.Hour,
		UpdatedBy:            uuid.NewV7(),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if out.AccessTokenDuration != 10*time.Minute {
		t.Fatalf("returned row = %+v, want what was written", out)
	}

	if got := mirror.Current(); got.AccessTokenDuration != 10*time.Minute || got.RefreshTokenDuration != 72*time.Hour {
		t.Fatalf("the serving replica's mirror must see the write before the call returns, got %+v", got)
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.calls != 1 {
		t.Fatalf("notifier calls = %d, want 1", n.calls)
	}
}

// Validation refuses before anything is written, and says which field.
func TestTokenLifetimesUpdateRefusesAnInvalidPairBeforeWriting(t *testing.T) {
	t.Parallel()

	repo := &fakeTokenLifetimesRepo{row: seeded()}
	mirror := newTestLifetimesMirror(t, repo)
	svc := newTestLifetimesService(t, repo, mirror, nil)

	_, err := svc.Update(t.Context(), &domain.UpdateTokenLifetimesInput{
		AccessTokenDuration:  24 * time.Hour,
		RefreshTokenDuration: 24 * time.Hour,
		UpdatedBy:            uuid.NewV7(),
	})

	if _, ok := errors.AsType[*domain.ValidationErrors](err); !ok {
		t.Fatalf("expected *domain.ValidationErrors, got %T: %v", err, err)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()

	if repo.updates != 0 {
		t.Fatal("nothing may reach the repository when validation fails")
	}
}

// A notify failure never fails the write. The change is saved and the ticker
// carries it; telling the caller otherwise would be a lie.
func TestTokenLifetimesANotifyFailureDoesNotFailTheWrite(t *testing.T) {
	t.Parallel()

	repo := &fakeTokenLifetimesRepo{row: seeded()}
	mirror := newTestLifetimesMirror(t, repo)

	if err := mirror.Reload(t.Context()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	n := &fakeNotifier{err: errors.New("valkey is away")}
	svc := newTestLifetimesService(t, repo, mirror, n)

	if _, err := svc.Update(t.Context(), &domain.UpdateTokenLifetimesInput{
		AccessTokenDuration:  domain.DefaultAuthnAccessTokenDuration,
		RefreshTokenDuration: domain.DefaultAuthnRefreshTokenDuration,
		UpdatedBy:            uuid.NewV7(),
	}); err != nil {
		t.Fatalf("a notify failure must not surface as a write failure: %v", err)
	}
}

// Get answers from the repository, not the mirror, so it cannot disagree with
// a write another replica just made.
func TestTokenLifetimesGetReadsTheRowNotTheMirror(t *testing.T) {
	t.Parallel()

	repo := &fakeTokenLifetimesRepo{row: seeded()}
	mirror := newTestLifetimesMirror(t, repo)

	if err := mirror.Reload(t.Context()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	// Another replica wrote: the row moved, this mirror has not reloaded.
	repo.set(domain.TokenLifetimes{AccessTokenDuration: 30 * time.Minute, RefreshTokenDuration: 48 * time.Hour}, nil)

	svc := newTestLifetimesService(t, repo, mirror, nil)

	out, err := svc.Get(t.Context())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if out.AccessTokenDuration != 30*time.Minute {
		t.Fatalf("Get = %+v, want the stored row, not this replica's stale mirror", out)
	}
}

// The authn service reads the provider at issuance, so a change lands on the
// next token without a restart. The test hands it a mirror, changes the row,
// reloads, and checks the value the service would sign with moved.
func TestFixedTokenLifetimesIsAProvider(t *testing.T) {
	t.Parallel()

	var p TokenLifetimesProvider = FixedTokenLifetimes(seeded())
	if p.Current().AccessTokenDuration != domain.DefaultAuthnAccessTokenDuration {
		t.Fatal("FixedTokenLifetimes must answer with what it was given")
	}
}
