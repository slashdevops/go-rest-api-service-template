//go:build unit

package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/notifier"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// verifiedFixture exercises the paths that read the verified flag. Disabled
// and verified used to be one column, and recovery on a fresh registration
// -- disabled until its link is followed -- was refused silently, which
// looked like a broken mailer from the outside.
type verifiedFixture struct {
	svc      *AuthnService
	users    *fakeUserStore
	notifier *recordingNotifier
	signer   claimsSigner
}

// fakeUserStore is a UserServiceConsumer over two maps. It records every
// UpdateByID so a test can see exactly which flags a flow set.
type fakeUserStore struct {
	mu      sync.Mutex
	byEmail map[string]*domain.User
	byID    map[uuid.UUID]*domain.User
	updates []*domain.UpdateUserInput
}

func (f *fakeUserStore) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}

	return nil, &domain.UserNotFoundError{}
}

func (f *fakeUserStore) GetByEmailForAuth(ctx context.Context, email string) (*domain.User, error) {
	return f.GetByEmail(ctx, email)
}

func (f *fakeUserStore) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}

	return nil, &domain.UserNotFoundError{}
}

func (f *fakeUserStore) SelectAuthz(context.Context, uuid.UUID) (map[string]any, error) {
	return map[string]any{}, nil
}

func (f *fakeUserStore) UpdateByID(_ context.Context, input *domain.UpdateUserInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, input)

	return nil
}

func (f *fakeUserStore) Create(context.Context, *domain.CreateUserInput) error { return nil }

func (f *fakeUserStore) add(u *domain.User) {
	f.byEmail[u.Email] = u
	f.byID[u.ID] = u
}

// recordingNotifier counts what was sent, by kind.
type recordingNotifier struct {
	mu            sync.Mutex
	verifications []string
	resets        []string
	exists        []string
}

func (n *recordingNotifier) SendAccountVerification(_ context.Context, to notifier.Recipient, _ string, _ string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.verifications = append(n.verifications, to.Email)

	return nil
}

func (n *recordingNotifier) SendPasswordReset(_ context.Context, to notifier.Recipient, _ string, _ string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.resets = append(n.resets, to.Email)

	return nil
}

func (n *recordingNotifier) SendAccountExists(_ context.Context, to notifier.Recipient) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.exists = append(n.exists, to.Email)

	return nil
}

// claimsSigner carries every claim VerifyUser reads: the idp fake's signer
// drops the email claim, which that path refuses.
type claimsSigner struct{}

func (claimsSigner) Sign(_ context.Context, c domain.JWTClaims) (string, error) {
	claims := map[string]any{
		"sub": c.Subject, "email": c.Email, "token_type": string(c.TokenType), "iss": c.Issuer,
		"jti": uuid.NewV7().String(), "exp": float64(time.Now().Add(c.TokenDuration).Unix()),
	}
	raw, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (claimsSigner) Verify(_ context.Context, tok string) (map[string]any, error) {
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return nil, errors.New("bad token")
	}

	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, errors.New("bad token")
	}

	return claims, nil
}

func (claimsSigner) SigningKeyID() string   { return "k" }
func (claimsSigner) VerifyKeyIDs() []string { return []string{"k"} }

type fixedLifetimes struct{}

func (fixedLifetimes) Current() domain.TokenLifetimes {
	return domain.TokenLifetimes{AccessTokenDuration: 5 * time.Minute, RefreshTokenDuration: time.Hour}
}

func newVerifiedFixture(t *testing.T) *verifiedFixture {
	t.Helper()

	ctx := t.Context()
	ot := &o11y.OpenTelemetry{
		Traces:  o11y.NewOpenTelemetryTracer(ctx, &o11y.OpenTelemetryTracerConfig{Name: "test"}),
		Metrics: o11y.NewOpenTelemetryMeter(ctx, &o11y.OpenTelemetryMeterConfig{Name: "test"}),
	}

	f := &verifiedFixture{
		users:    &fakeUserStore{byEmail: map[string]*domain.User{}, byID: map[uuid.UUID]*domain.User{}},
		notifier: &recordingNotifier{},
	}

	svc, err := NewAuthnService(AuthnServiceConf{
		UserService: f.users, Notifier: f.notifier, TokenSigner: f.signer, RevokedTokens: &fakeRevoked{},
		TokenLifetimes: fixedLifetimes{}, Issuer: "https://api.example", OT: ot,
		UserVerificationTokenTTL: 24 * time.Hour, UserResetPasswordTokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewAuthnService: %v", err)
	}

	f.svc = svc

	return f
}

// account builds a local account in the given state. A fresh registration is
// unverified and disabled; a verified account is enabled unless disabled says
// otherwise.
func account(t *testing.T, email string, verified, disabled bool) *domain.User {
	t.Helper()

	hash, err := HashAndSaltPassword("Meadow7Lark!")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	return &domain.User{
		ID: uuid.NewV7(), Email: email, FirstName: "Jane", LastName: "Doe", PasswordHash: hash,
		LocalAccount: new(true), Verified: new(verified), Disabled: new(disabled),
	}
}

func TestRecoverPasswordOnAnUnverifiedAccountSendsTheVerificationMail(t *testing.T) {
	t.Parallel()

	f := newVerifiedFixture(t)
	f.users.add(account(t, "fresh@example.com", false, true))

	if err := f.svc.RecoverPassword(t.Context(), &domain.RecoverPasswordInput{Email: "fresh@example.com"}); err != nil {
		t.Fatalf("RecoverPassword: %v", err)
	}

	if got := f.notifier.verifications; len(got) != 1 || got[0] != "fresh@example.com" {
		t.Fatalf("expected one verification mail to the account, got %v", got)
	}
	if len(f.notifier.resets) != 0 {
		t.Fatal("an unverified account has no password to reset yet; no reset mail")
	}
}

func TestRecoverPasswordOnADisabledVerifiedAccountSendsNothing(t *testing.T) {
	t.Parallel()

	f := newVerifiedFixture(t)
	f.users.add(account(t, "off@example.com", true, true))

	// The answer is the same 200 as every other outcome.
	if err := f.svc.RecoverPassword(t.Context(), &domain.RecoverPasswordInput{Email: "off@example.com"}); err != nil {
		t.Fatalf("RecoverPassword: %v", err)
	}

	if len(f.notifier.verifications)+len(f.notifier.resets) != 0 {
		t.Fatalf("a disabled account must get no mail, got %+v", f.notifier)
	}
}

func TestRecoverPasswordOnAVerifiedAccountSendsTheResetMail(t *testing.T) {
	t.Parallel()

	f := newVerifiedFixture(t)
	f.users.add(account(t, "ok@example.com", true, false))

	if err := f.svc.RecoverPassword(t.Context(), &domain.RecoverPasswordInput{Email: "ok@example.com"}); err != nil {
		t.Fatalf("RecoverPassword: %v", err)
	}

	if got := f.notifier.resets; len(got) != 1 || got[0] != "ok@example.com" {
		t.Fatalf("expected one reset mail, got %v", got)
	}
	if len(f.notifier.verifications) != 0 {
		t.Fatal("a verified account does not get the verification mail again")
	}
}

func TestVerifyUserProvesTheAddressAndOpensTheAccount(t *testing.T) {
	t.Parallel()

	f := newVerifiedFixture(t)
	u := account(t, "fresh@example.com", false, true)
	f.users.add(u)

	tok, err := f.signer.Sign(t.Context(), domain.JWTClaims{
		Email: u.Email, Subject: u.ID.String(), Issuer: "https://api.example",
		TokenType: domain.TokenTypeEmailVerification, TokenDuration: time.Hour,
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	if err := f.svc.VerifyUser(t.Context(), tok); err != nil {
		t.Fatalf("VerifyUser: %v", err)
	}

	if len(f.users.updates) != 1 {
		t.Fatalf("expected one update, got %d", len(f.users.updates))
	}
	up := f.users.updates[0]
	if up.Verified == nil || !*up.Verified {
		t.Fatal("verification must set verified")
	}
	if up.Disabled == nil || *up.Disabled {
		t.Fatal("verification must clear disabled: that is the only reason a registration is disabled")
	}
}

func TestVerifyUserRefusesAnAlreadyVerifiedAccount(t *testing.T) {
	t.Parallel()

	f := newVerifiedFixture(t)
	// Verified but disabled by an administrator: the link must not reopen it.
	u := account(t, "off@example.com", true, true)
	f.users.add(u)

	tok, _ := f.signer.Sign(t.Context(), domain.JWTClaims{
		Email: u.Email, Subject: u.ID.String(), Issuer: "https://api.example",
		TokenType: domain.TokenTypeEmailVerification, TokenDuration: time.Hour,
	})

	err := f.svc.VerifyUser(t.Context(), tok)
	if _, ok := errors.AsType[*domain.UserAlreadyVerifiedError](err); !ok {
		t.Fatalf("expected UserAlreadyVerifiedError, got %v", err)
	}
	if len(f.users.updates) != 0 {
		t.Fatal("a verified account must not be touched by a verification link")
	}
}

func TestReVerifyUserAnswersByTheVerifiedFlag(t *testing.T) {
	t.Parallel()

	f := newVerifiedFixture(t)
	f.users.add(account(t, "fresh@example.com", false, true))
	f.users.add(account(t, "done@example.com", true, false))

	if err := f.svc.ReVerifyUser(t.Context(), "fresh@example.com"); err != nil {
		t.Fatalf("ReVerifyUser: %v", err)
	}
	if err := f.svc.ReVerifyUser(t.Context(), "done@example.com"); err != nil {
		t.Fatalf("ReVerifyUser: %v", err)
	}

	if got := f.notifier.verifications; len(got) != 1 || got[0] != "fresh@example.com" {
		t.Fatalf("only the unverified account gets the mail, got %v", got)
	}
}

// A fresh registration is refused at login because it is DISABLED until its
// link is followed, not because it is unverified.
func TestLoginRefusesAFreshRegistrationWithTheRightPassword(t *testing.T) {
	t.Parallel()

	f := newVerifiedFixture(t)
	f.users.add(account(t, "fresh@example.com", false, true))

	_, err := f.svc.LoginUser(t.Context(), &domain.LoginUserInput{
		Email: "fresh@example.com", Password: "Meadow7Lark!", LoginMethod: domain.LoginMethodPassword,
	})
	if _, ok := errors.AsType[*domain.InvalidCredentialsError](err); !ok {
		t.Fatalf("expected the uniform credentials refusal, got %v", err)
	}
}

// A password that was fine when it was set must still sign in after the
// choosing rule tightens: login validates a login password, not a new one.
func TestLoginDoesNotApplyTheChoosingRule(t *testing.T) {
	t.Parallel()

	f := newVerifiedFixture(t)
	u := account(t, "legacy@example.com", true, false)
	hash, _ := HashAndSaltPassword("abc123")
	u.PasswordHash = hash
	f.users.add(u)

	_, err := f.svc.LoginUser(t.Context(), &domain.LoginUserInput{
		Email: "legacy@example.com", Password: "abc123", LoginMethod: domain.LoginMethodPassword,
	})
	if err != nil {
		t.Fatalf("a six-character legacy password must reach the compare and sign in, got %v", err)
	}
}

// An account an administrator created or enabled has never proven its
// address, and nothing will send it a verification link. disabled is what
// decides sign-in; verified must not.
func TestLoginAdmitsAnEnabledAccountThatNeverVerified(t *testing.T) {
	t.Parallel()

	f := newVerifiedFixture(t)
	f.users.add(account(t, "provisioned@example.com", false, false))

	_, err := f.svc.LoginUser(t.Context(), &domain.LoginUserInput{
		Email: "provisioned@example.com", Password: "Meadow7Lark!", LoginMethod: domain.LoginMethodPassword,
	})
	if err != nil {
		t.Fatalf("an enabled account must sign in whether or not it verified, got %v", err)
	}
}
