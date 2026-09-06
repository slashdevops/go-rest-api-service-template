//go:build unit

package usecase

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/oauth"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// The fakes below stand in for the ports around the IdP use case. They are
// deliberately dumb: the rules under test live in the use case, and a fake
// that re-implemented them would prove nothing.

type fakeAuthn struct {
	loggedIn   []uuid.UUID
	registered []*domain.RegisterUserInput
}

func (f *fakeAuthn) LoginUserByID(_ context.Context, id uuid.UUID, _ domain.LoginMethod) (*domain.LoginUserOutput, error) {
	f.loggedIn = append(f.loggedIn, id)

	return &domain.LoginUserOutput{UserID: id, AccessToken: "at", RefreshToken: "rt"}, nil
}

func (f *fakeAuthn) RegisterUser(_ context.Context, in *domain.RegisterUserInput) error {
	f.registered = append(f.registered, in)

	return nil
}

type fakeIDPs struct{ idp *domain.IDP }

func (f *fakeIDPs) GetByID(context.Context, uuid.UUID) (*domain.IDP, error) { return f.idp, nil }

type fakeUsers struct {
	byEmail map[string]*domain.User
	byID    map[uuid.UUID]*domain.User
}

func (f *fakeUsers) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}

	return nil, &domain.UserNotFoundError{}
}

func (f *fakeUsers) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}

	return nil, &domain.UserNotFoundError{}
}

type identityKey struct {
	idp     uuid.UUID
	subject string
}

type fakeIdentities struct {
	mu    sync.Mutex
	links map[identityKey]domain.UserIdentity
}

func newFakeIdentities() *fakeIdentities {
	return &fakeIdentities{links: map[identityKey]domain.UserIdentity{}}
}

func (f *fakeIdentities) Link(_ context.Context, in *domain.LinkUserIdentityInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	k := identityKey{in.IDPID, in.Subject}
	if _, exists := f.links[k]; exists {
		return &domain.UserIdentityAlreadyLinkedError{}
	}

	for _, l := range f.links {
		if l.UserID == in.UserID && l.IDPID == in.IDPID {
			return &domain.UserIdentityAlreadyLinkedError{}
		}
	}

	f.links[k] = domain.UserIdentity{UserID: in.UserID, IDPID: in.IDPID, Subject: in.Subject, Email: in.Email}

	return nil
}

func (f *fakeIdentities) Unlink(_ context.Context, in *domain.UnlinkUserIdentityInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	for k, l := range f.links {
		if l.UserID == in.UserID && l.IDPID == in.IDPID {
			delete(f.links, k)

			return nil
		}
	}

	return &domain.UserIdentityNotFoundError{}
}

func (f *fakeIdentities) SelectBySubject(_ context.Context, idp uuid.UUID, subject string) (*domain.UserIdentity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if l, ok := f.links[identityKey{idp, subject}]; ok {
		return &l, nil
	}

	return nil, &domain.UserIdentityNotFoundError{}
}

func (f *fakeIdentities) SelectByUserID(_ context.Context, user uuid.UUID) ([]domain.UserIdentity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []domain.UserIdentity

	for _, l := range f.links {
		if l.UserID == user {
			out = append(out, l)
		}
	}

	return out, nil
}

// fakeOAuth answers every exchange with one UserInfo and records what it saw.
type fakeOAuth struct {
	info    *domain.UserInfo
	lastReq oauth.AuthRequest
}

func (f *fakeOAuth) AuthCodeURL(_ context.Context, _ *domain.IDP, req oauth.AuthRequest) (string, error) {
	f.lastReq = req

	return "https://idp.example/authorize?state=" + req.State, nil
}

func (f *fakeOAuth) Exchange(_ context.Context, _ *domain.IDP, _ string, req oauth.AuthRequest) (*domain.UserInfo, error) {
	f.lastReq = req

	return f.info, nil
}

// fakeSigner signs by concatenation and verifies by splitting: enough to carry
// claims through, and a "signature" nobody could mistake for one.
type fakeSigner struct{}

func (fakeSigner) Sign(_ context.Context, c domain.JWTClaims) (string, error) {
	jti := uuid.NewV7().String()
	exp := time.Now().Add(c.TokenDuration).Unix()

	return strings.Join([]string{"tok", c.IDP, c.Subject, string(c.TokenType), c.Data, jti, base64.RawURLEncoding.EncodeToString([]byte(time.Unix(exp, 0).Format(time.RFC3339)))}, "|"), nil
}

func (fakeSigner) Verify(_ context.Context, tok string) (map[string]any, error) {
	p := strings.Split(tok, "|")
	if len(p) != 7 || p[0] != "tok" {
		return nil, errors.New("bad token")
	}

	expRaw, _ := base64.RawURLEncoding.DecodeString(p[6])
	exp, _ := time.Parse(time.RFC3339, string(expRaw))

	return map[string]any{"idp": p[1], "sub": p[2], "token_type": p[3], "data": p[4], "jti": p[5], "exp": float64(exp.Unix())}, nil
}

func (fakeSigner) SigningKeyID() string   { return "k" }
func (fakeSigner) VerifyKeyIDs() []string { return []string{"k"} }

// fakeCipher is reversible and obviously not a cipher.
type fakeCipher struct{}

func (fakeCipher) EncryptString(b []byte) (string, error) {
	return "enc:" + base64.StdEncoding.EncodeToString(b), nil
}
func (fakeCipher) DecryptString(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimPrefix(s, "enc:"))
}

// fakeRevoked spends jtis once.
type fakeRevoked struct {
	mu    sync.Mutex
	spent map[uuid.UUID]bool
}

func (f *fakeRevoked) Consume(_ context.Context, jti, _ uuid.UUID, _ domain.TokenType, _ time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.spent == nil {
		f.spent = map[uuid.UUID]bool{}
	}

	if f.spent[jti] {
		return false, nil
	}

	f.spent[jti] = true

	return true, nil
}

func (f *fakeRevoked) Revoke(context.Context, uuid.UUID, uuid.UUID, domain.TokenType, time.Time) error {
	panic("not used")
}
func (f *fakeRevoked) Rotate(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error {
	panic("not used")
}
func (f *fakeRevoked) Get(context.Context, uuid.UUID) (*domain.TokenRevocation, error) {
	panic("not used")
}
func (f *fakeRevoked) RevokeChain(context.Context, uuid.UUID, uuid.UUID, time.Time) (uuid.UUID, error) {
	panic("not used")
}
func (f *fakeRevoked) DeleteExpired(context.Context) (int64, error) { panic("not used") }
func (f *fakeRevoked) SelectUnexpiredJTIs(context.Context, domain.TokenType) ([]uuid.UUID, error) {
	panic("not used")
}

type idpFixture struct {
	svc        *AuthnIDPsService
	authn      *fakeAuthn
	users      *fakeUsers
	identities *fakeIdentities
	oauth      *fakeOAuth
	idp        *domain.IDP
}

func newIDPFixture(t *testing.T, info *domain.UserInfo) *idpFixture {
	t.Helper()

	ctx := t.Context()
	ot := &o11y.OpenTelemetry{
		Traces:  o11y.NewOpenTelemetryTracer(ctx, &o11y.OpenTelemetryTracerConfig{Name: "test"}),
		Metrics: o11y.NewOpenTelemetryMeter(ctx, &o11y.OpenTelemetryMeterConfig{Name: "test"}),
	}

	f := &idpFixture{
		authn:      &fakeAuthn{},
		users:      &fakeUsers{byEmail: map[string]*domain.User{}, byID: map[uuid.UUID]*domain.User{}},
		identities: newFakeIdentities(),
		oauth:      &fakeOAuth{info: info},
		idp: &domain.IDP{
			ID: uuid.NewV7(), Name: "Test", Enabled: true, AutoProvision: true, ClientID: "c", ClientSecret: "s",
			CallbackURL: "https://app.example/cb", IssuerURL: "https://issuer.example",
			IDPType: domain.IDPTypes{Name: "Okta", Kind: domain.IDPTypeKindOIDC},
		},
	}

	svc, err := NewAuthnIDPsService(AuthnIDPsServiceConf{
		AuthnService: f.authn, IDPsService: &fakeIDPs{idp: f.idp}, UserService: f.users, UsersIdentities: f.identities,
		TokenSigner: fakeSigner{}, OAuth: f.oauth, RevokedTokens: &fakeRevoked{}, Cipher: fakeCipher{},
		Issuer: "https://api.example", OT: ot,
	})
	if err != nil {
		t.Fatalf("NewAuthnIDPsService: %v", err)
	}

	f.svc = svc

	return f
}

// stateFor starts a flow and pulls the state out of the URL the provider
// would receive.
func (f *idpFixture) stateFor(t *testing.T, event domain.IDPEventType, userID uuid.UUID) string {
	t.Helper()

	u, err := f.svc.GetLoginURL(t.Context(), f.idp.ID, event, userID)
	if err != nil {
		t.Fatalf("GetLoginURL: %v", err)
	}

	return strings.TrimPrefix(u, "https://idp.example/authorize?state=")
}

func verified(subject, email string) *domain.UserInfo {
	return &domain.UserInfo{Subject: subject, Email: email, EmailVerified: true, FirstName: "Jane", LastName: "Doe"}
}

// A known identity signs in as the account it is linked to, whatever the
// provider says the email is now.
func TestIDPCallbackAKnownIdentitySignsIn(t *testing.T) {
	t.Parallel()

	f := newIDPFixture(t, verified("sub-1", "renamed@example.com"))
	userID := uuid.NewV7()
	f.users.byID[userID] = &domain.User{ID: userID}
	_ = f.identities.Link(t.Context(), &domain.LinkUserIdentityInput{UserID: userID, IDPID: f.idp.ID, Subject: "sub-1", Email: "old@example.com"})

	out, err := f.svc.Callback(t.Context(), f.idp.ID, f.stateFor(t, domain.IDPEventTypeLogin, uuid.Nil()), "code")
	if err != nil {
		t.Fatalf("Callback: %v", err)
	}

	if out.Login == nil || out.Login.UserID != userID {
		t.Fatalf("expected a session for %s, got %+v", userID, out)
	}

	if len(f.authn.registered) != 0 {
		t.Fatal("a known identity must not register anything")
	}
}

// The takeover that used to exist: an unknown identity whose email matches an
// existing account. Refused, and the account untouched.
func TestIDPCallbackRefusesAnUnknownIdentityWhoseEmailHasAnAccount(t *testing.T) {
	t.Parallel()

	f := newIDPFixture(t, verified("attacker-sub", "victim@example.com"))
	victim := uuid.NewV7()
	f.users.byEmail["victim@example.com"] = &domain.User{ID: victim, LocalAccount: new(true)}

	_, err := f.svc.Callback(t.Context(), f.idp.ID, f.stateFor(t, domain.IDPEventTypeLogin, uuid.Nil()), "code")

	if _, ok := errors.AsType[*domain.IDPIdentityNotLinkedError](err); !ok {
		t.Fatalf("expected IDPIdentityNotLinkedError, got %v", err)
	}

	if len(f.authn.loggedIn) != 0 || len(f.authn.registered) != 0 {
		t.Fatal("nothing may be signed in or created")
	}
}

// Provisioning needs a verified email and an IdP that allows it; the refusal
// wording is the same either way.
func TestIDPCallbackProvisioningGates(t *testing.T) {
	t.Parallel()

	t.Run("unverified_email", func(t *testing.T) {
		t.Parallel()

		info := verified("s", "new@example.com")
		info.EmailVerified = false
		f := newIDPFixture(t, info)

		_, err := f.svc.Callback(t.Context(), f.idp.ID, f.stateFor(t, domain.IDPEventTypeRegister, uuid.Nil()), "code")
		if _, ok := errors.AsType[*domain.IDPIdentityNotLinkedError](err); !ok {
			t.Fatalf("expected a refusal, got %v", err)
		}
	})

	t.Run("auto_provision_off", func(t *testing.T) {
		t.Parallel()

		f := newIDPFixture(t, verified("s", "new@example.com"))
		f.idp.AutoProvision = false

		_, err := f.svc.Callback(t.Context(), f.idp.ID, f.stateFor(t, domain.IDPEventTypeLogin, uuid.Nil()), "code")
		if _, ok := errors.AsType[*domain.IDPIdentityNotLinkedError](err); !ok {
			t.Fatalf("expected a refusal, got %v", err)
		}
	})
}

// A new identity with a verified email and no account behind it is
// provisioned, linked, and signed in -- in that order.
func TestIDPCallbackProvisionsAndLinks(t *testing.T) {
	t.Parallel()

	f := newIDPFixture(t, verified("sub-new", "new@example.com"))

	out, err := f.svc.Callback(t.Context(), f.idp.ID, f.stateFor(t, domain.IDPEventTypeLogin, uuid.Nil()), "code")
	if err != nil {
		t.Fatalf("Callback: %v", err)
	}

	if len(f.authn.registered) != 1 || f.authn.registered[0].RegisterMethod != domain.RegisterMethodOAuth {
		t.Fatalf("expected one OAuth registration, got %+v", f.authn.registered)
	}

	link, err := f.identities.SelectBySubject(t.Context(), f.idp.ID, "sub-new")
	if err != nil || link.UserID != out.Login.UserID {
		t.Fatalf("the identity must be linked to the new account: %v %+v", err, link)
	}
}

// A link attaches the identity to the account that STARTED it, which is in the
// state, not in anything the callback carries.
func TestIDPCallbackLinkAttachesToTheStartingAccount(t *testing.T) {
	t.Parallel()

	f := newIDPFixture(t, verified("sub-l", "jane@example.com"))
	me := uuid.NewV7()
	f.users.byID[me] = &domain.User{ID: me, LocalAccount: new(true)}

	out, err := f.svc.Callback(t.Context(), f.idp.ID, f.stateFor(t, domain.IDPEventTypeLink, me), "code")
	if err != nil {
		t.Fatalf("Callback: %v", err)
	}

	if out.EventType != domain.IDPEventTypeLink || out.Linked != me || out.Login != nil {
		t.Fatalf("expected a link to %s with no session, got %+v", me, out)
	}

	// Now the identity is known and signs in as me.
	out, err = f.svc.Callback(t.Context(), f.idp.ID, f.stateFor(t, domain.IDPEventTypeLogin, uuid.Nil()), "code")
	if err != nil || out.Login.UserID != me {
		t.Fatalf("the linked identity must sign in as %s: %v %+v", me, err, out)
	}
}

// A link cannot be started for an account by an anonymous flow, and cannot
// steal an identity already linked elsewhere.
func TestIDPLinkRefusals(t *testing.T) {
	t.Parallel()

	f := newIDPFixture(t, verified("sub-x", "x@example.com"))

	if _, err := f.svc.GetLoginURL(t.Context(), f.idp.ID, domain.IDPEventTypeLink, uuid.Nil()); err == nil {
		t.Fatal("a link needs the signed-in account")
	}

	owner, other := uuid.NewV7(), uuid.NewV7()
	f.users.byID[owner] = &domain.User{ID: owner}
	f.users.byID[other] = &domain.User{ID: other}
	_ = f.identities.Link(t.Context(), &domain.LinkUserIdentityInput{UserID: owner, IDPID: f.idp.ID, Subject: "sub-x", Email: "x@example.com"})

	_, err := f.svc.Callback(t.Context(), f.idp.ID, f.stateFor(t, domain.IDPEventTypeLink, other), "code")
	if _, ok := errors.AsType[*domain.UserIdentityAlreadyLinkedError](err); !ok {
		t.Fatalf("an identity linked to another account must not be re-linked: %v", err)
	}
}

// The state carries the PKCE verifier and the nonce sealed, and the exchange
// gets them back. A state is spent on first use.
func TestIDPStateCarriesPKCEAndIsSingleUse(t *testing.T) {
	t.Parallel()

	f := newIDPFixture(t, verified("s", "a@example.com"))
	state := f.stateFor(t, domain.IDPEventTypeLogin, uuid.Nil())

	minted := f.oauth.lastReq
	if minted.CodeVerifier == "" || minted.Nonce == "" {
		t.Fatal("the flow must mint a verifier and a nonce")
	}

	if strings.Contains(state, minted.CodeVerifier) {
		t.Fatal("the verifier must not be readable in the state")
	}

	if _, err := f.svc.Callback(t.Context(), f.idp.ID, state, "code"); err != nil {
		t.Fatalf("Callback: %v", err)
	}

	if f.oauth.lastReq.CodeVerifier != minted.CodeVerifier || f.oauth.lastReq.Nonce != minted.Nonce {
		t.Fatal("the exchange must present the verifier and nonce the flow was minted with")
	}

	if _, err := f.svc.Callback(t.Context(), f.idp.ID, state, "code"); err == nil {
		t.Fatal("a state presented twice must be refused")
	}
}

// A disabled provider is refused before a state is minted and at the callback.
func TestIDPDisabledProviderIsRefused(t *testing.T) {
	t.Parallel()

	f := newIDPFixture(t, verified("s", "a@example.com"))
	state := f.stateFor(t, domain.IDPEventTypeLogin, uuid.Nil())
	f.idp.Enabled = false

	if _, err := f.svc.GetLoginURL(t.Context(), f.idp.ID, domain.IDPEventTypeLogin, uuid.Nil()); err == nil {
		t.Fatal("a disabled provider must not start a flow")
	}

	if _, err := f.svc.Callback(t.Context(), f.idp.ID, state, "code"); err == nil {
		t.Fatal("a disabled provider must not complete a flow")
	}
}

// Unlinking the only way into a password-less account is refused.
func TestIDPUnlinkNeverStrandsAnAccount(t *testing.T) {
	t.Parallel()

	f := newIDPFixture(t, nil)
	me := uuid.NewV7()
	f.users.byID[me] = &domain.User{ID: me, LocalAccount: new(false)}
	_ = f.identities.Link(t.Context(), &domain.LinkUserIdentityInput{UserID: me, IDPID: f.idp.ID, Subject: "only", Email: "me@example.com"})

	if err := f.svc.UnlinkIdentity(t.Context(), me, f.idp.ID); err == nil {
		t.Fatal("the only identity of a password-less account must not be removed")
	}

	f.users.byID[me].LocalAccount = new(true)

	if err := f.svc.UnlinkIdentity(t.Context(), me, f.idp.ID); err != nil {
		t.Fatalf("with a password the identity may go: %v", err)
	}
}
