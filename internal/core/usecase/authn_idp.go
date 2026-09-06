package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"uuid"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/cipher"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/oauth"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/token"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

const (
	idpLoginDuration        = 15 * time.Minute
	idpRegistrationDuration = 25 * time.Minute
	idpLinkDuration         = 15 * time.Minute
)

// AuthnServiceConsumer is the half of the authn service an IdP sign-in needs.
//
// LoginUserByID, never LoginUser: the identity has already been resolved to an
// account by (idp, subject) when this is called, and the old email-based login
// is exactly what let a provider-asserted email take an account over.
type AuthnServiceConsumer interface {
	LoginUserByID(ctx context.Context, userID uuid.UUID, method domain.LoginMethod) (*domain.LoginUserOutput, error)
	RegisterUser(ctx context.Context, input *domain.RegisterUserInput) error
}

type IDPsServiceConsumer interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.IDP, error)
}

// IDPUserServiceConsumer is what identity resolution needs from users: does an
// account with this email exist, and does this account have a password.
type IDPUserServiceConsumer interface {
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

type AuthnIDPsServiceConf struct {
	AuthnService    AuthnServiceConsumer
	IDPsService     IDPsServiceConsumer
	UserService     IDPUserServiceConsumer
	UsersIdentities repository.UsersIdentities
	TokenSigner     token.Signer
	OAuth           oauth.Provider
	RevokedTokens   repository.RevokedTokens

	// Cipher seals the PKCE verifier, the nonce and the linking account into
	// the state token. The state is signed, so it cannot be forged, but it is
	// readable by whoever sees the URL, and the verifier must not be.
	Cipher cipher.Cipher

	OT            *o11y.OpenTelemetry
	Issuer        string
	MetricsPrefix string
}

// AuthnIDPsService drives the three provider events: login, register, link.
//
// # How a callback becomes an account
//
// The provider's word is the subject; the email is a hint. A callback resolves
// (idp, subject) in users_identities FIRST:
//
//   - known identity: the linked account signs in, whatever the email says now;
//   - unknown identity, link event: the signed-in account that started the
//     link gets the identity, provided no account already owns it;
//   - unknown identity, login or register: an account may be CREATED when the
//     IdP allows auto-provisioning, the provider vouches for the email, and no
//     account has that email yet. Otherwise the sign-in is refused with one
//     wording for every reason, because the differences would tell whoever
//     controls the provider account which addresses have accounts here.
//
// An existing account is never linked by email. The holder links a provider
// from their profile while signed in, which is the only moment both sides of
// the link are proven.
type AuthnIDPsService struct {
	authnService    AuthnServiceConsumer
	idpsService     IDPsServiceConsumer
	userService     IDPUserServiceConsumer
	usersIdentities repository.UsersIdentities
	tokenSigner     token.Signer
	oauth           oauth.Provider
	revokedTokens   repository.RevokedTokens
	cipher          cipher.Cipher
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	issuer          string
	metricsPrefix   string
}

// NewAuthnIDPsService creates a new AuthnIDPsService.
func NewAuthnIDPsService(conf AuthnIDPsServiceConf) (*AuthnIDPsService, error) {
	if conf.AuthnService == nil {
		return nil, &domain.InvalidAuthnServiceError{Message: "AuthnService is nil, but it is required for AuthnIDPsService"}
	}

	if conf.IDPsService == nil {
		return nil, &domain.InvalidIDPsServiceError{Message: "IDPsService is nil, but it is required for AuthnIDPsService"}
	}

	if conf.UserService == nil {
		return nil, &domain.InvalidUserServiceError{Message: "UserService is nil, but it is required for AuthnIDPsService"}
	}

	if conf.UsersIdentities == nil {
		return nil, &domain.InvalidRepositoryError{Message: "UsersIdentities is nil, but it is required for AuthnIDPsService; without it an identity cannot be resolved to an account"}
	}

	if conf.TokenSigner == nil {
		return nil, &domain.InvalidTokenSignerError{Message: "TokenSigner is nil, but it is required for AuthnIDPsService"}
	}

	if conf.OAuth == nil {
		return nil, &domain.InvalidIdentityProvidersError{Message: "OAuth provider is nil, but it is required for AuthnIDPsService"}
	}

	if conf.Cipher == nil {
		return nil, &domain.InvalidCipherError{Message: "Cipher is nil, but it is required for AuthnIDPsService; the PKCE verifier travels sealed inside the state"}
	}

	if conf.RevokedTokens == nil {
		// The state used to be replayable when this was nil, with a warning.
		// A replayable state is a callback anyone who saw the URL can replay,
		// so it is required now.
		return nil, &domain.InvalidRepositoryError{Message: "RevokedTokens is nil, but it is required for AuthnIDPsService; the OAuth state must be single-use"}
	}

	if len(conf.Issuer) <= 2 || len(conf.Issuer) > 100 {
		return nil, &domain.InvalidIssuerError{Message: "Issuer is invalid, but it is required for AuthnIDPsService"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for AuthnIDPsService"}
	}

	ref := &AuthnIDPsService{
		authnService:    conf.AuthnService,
		idpsService:     conf.IDPsService,
		userService:     conf.UserService,
		usersIdentities: conf.UsersIdentities,
		tokenSigner:     conf.TokenSigner,
		oauth:           conf.OAuth,
		revokedTokens:   conf.RevokedTokens,
		cipher:          conf.Cipher,
		issuer:          conf.Issuer,
		ot:              conf.OT,
		metricsMetadata: o11y.Metadata{Layer: AppLayer, Domain: "AuthnIDPs", Action: "NewAuthnIDPsService"},
	}

	if conf.MetricsPrefix != "" {
		ref.metricsPrefix = strings.ReplaceAll(conf.MetricsPrefix, "-", "_") + "_"
	}

	callsCounter, err := ref.ot.Metrics.Meter.Int64Counter(
		fmt.Sprintf("%s%s", ref.metricsPrefix, MetricCallsCounterName),
		metric.WithDescription(fmt.Sprintf("Total number of %s calls", AppLayer)),
	)
	if err != nil {
		return nil, err
	}

	callsDuration, err := ref.ot.Metrics.Meter.Float64Histogram(
		fmt.Sprintf("%s%s", ref.metricsPrefix, MetricDurationHistogramName),
		metric.WithDescription(fmt.Sprintf("Duration of %s calls", AppLayer)),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	ref.metrics = &o11y.LayerMetrics{Counter: callsCounter, Histogram: callsDuration}

	return ref, nil
}

// stateData is what the state token carries sealed: the PKCE verifier the
// exchange needs, the nonce the ID token must echo, and -- for a link -- the
// account the signed-in user was when they started.
type stateData struct {
	Verifier string    `json:"v"`
	Nonce    string    `json:"n"`
	UserID   uuid.UUID `json:"u,omitzero"`
}

// GetLoginURL implements driving.AuthnIDPs.
func (ref *AuthnIDPsService) GetLoginURL(ctx context.Context, idpID uuid.UUID, eventType domain.IDPEventType, userID uuid.UUID) (string, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetLoginURL")
	defer span.End()

	if idpID == uuid.Nil() {
		return "", o11y.RecordError(ctx, span, start, &domain.InvalidIdentityProvidersError{Message: "idpID is empty"}, ref.metrics, attrs)
	}

	var tokenType domain.TokenType

	var duration time.Duration

	switch eventType {
	case domain.IDPEventTypeLogin:
		tokenType, duration = domain.TokenTypeIDPSignin, idpLoginDuration
	case domain.IDPEventTypeRegister:
		tokenType, duration = domain.TokenTypeIDPRegister, idpRegistrationDuration
	case domain.IDPEventTypeLink:
		if userID == uuid.Nil() {
			return "", o11y.RecordError(ctx, span, start, &domain.InvalidIdentityProvidersError{Message: "a link needs the signed-in account"}, ref.metrics, attrs)
		}

		tokenType, duration = domain.TokenTypeIDPLink, idpLinkDuration
	default:
		return "", o11y.RecordError(ctx, span, start, &domain.InvalidIdentityProvidersError{Message: fmt.Sprintf("invalid event type: %s", eventType)}, ref.metrics, attrs)
	}

	// The IdP first: a disabled or unknown provider is refused before a state
	// is minted for it.
	idp, err := ref.idpsService.GetByID(ctx, idpID)
	if err != nil {
		return "", o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if !idp.Enabled {
		return "", o11y.RecordError(ctx, span, start, &domain.IDPNotFoundError{Message: "this identity provider is not enabled"}, ref.metrics, attrs)
	}

	req := oauth.AuthRequest{Nonce: randomToken(), CodeVerifier: randomToken()}

	sealed, err := ref.seal(stateData{Verifier: req.CodeVerifier, Nonce: req.Nonce, UserID: userID})
	if err != nil {
		return "", o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	state, err := ref.tokenSigner.Sign(ctx, domain.JWTClaims{
		IDP:           idpID.String(),
		Subject:       eventType.String(),
		Issuer:        ref.issuer,
		TokenType:     tokenType,
		TokenDuration: duration,
		Data:          sealed,
	})
	if err != nil {
		slog.Error("service.AuthnIDPs.GetLoginURL: could not sign the state", "error", err)

		return "", o11y.RecordError(ctx, span, start, &domain.InvalidIdentityProvidersError{Message: "failed to create the sign-in state"}, ref.metrics, attrs)
	}

	req.State = state

	url, err := ref.oauth.AuthCodeURL(ctx, idp, req)
	if err != nil {
		return "", o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "authorization URL built",
		attribute.String("idp.id", idpID.String()), attribute.String("event", eventType.String()))

	return url, nil
}

// Callback implements driving.AuthnIDPs.
func (ref *AuthnIDPsService) Callback(ctx context.Context, idpID uuid.UUID, state, code string) (*domain.IDPCallbackOutput, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "Callback")
	defer span.End()

	if idpID == uuid.Nil() || state == "" || code == "" {
		return nil, o11y.RecordError(ctx, span, start, &domain.InvalidIdentityProvidersError{Message: "idp, state and code are required"}, ref.metrics, attrs)
	}

	claims, data, eventType, err := ref.spendState(ctx, state, idpID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	_ = claims

	idp, err := ref.idpsService.GetByID(ctx, idpID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if !idp.Enabled {
		return nil, o11y.RecordError(ctx, span, start, &domain.IDPNotFoundError{Message: "this identity provider is not enabled"}, ref.metrics, attrs)
	}

	info, err := ref.oauth.Exchange(ctx, idp, code, oauth.AuthRequest{State: state, Nonce: data.Nonce, CodeVerifier: data.Verifier})
	if err != nil {
		// The adapter already says this in words this service owns and has
		// logged the provider's own text.
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	span.SetAttributes(attribute.String("idp.id", idpID.String()), attribute.String("event", eventType.String()))

	var out *domain.IDPCallbackOutput

	switch eventType {
	case domain.IDPEventTypeLink:
		out, err = ref.link(ctx, idp, info, data.UserID)
	default:
		out, err = ref.signIn(ctx, idp, info, eventType)
	}

	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "callback completed", attribute.String("event", eventType.String()))

	return out, nil
}

// signIn resolves the identity and ends with a session, or creates the
// account first when every condition for that holds.
func (ref *AuthnIDPsService) signIn(ctx context.Context, idp *domain.IDP, info *domain.UserInfo, eventType domain.IDPEventType) (*domain.IDPCallbackOutput, error) {
	identity, err := ref.usersIdentities.SelectBySubject(ctx, idp.ID, info.Subject)

	switch {
	case err == nil:
		// A known identity signs in as the account it is linked to, whatever
		// email the provider reports today.
		login, err := ref.authnService.LoginUserByID(ctx, identity.UserID, domain.LoginMethodOAuth)
		if err != nil {
			return nil, err
		}

		return &domain.IDPCallbackOutput{EventType: eventType, Login: login}, nil

	case !isNotFound(err):
		return nil, err
	}

	// Unknown identity. Provisioning has three conditions, and each failure
	// is answered with the same wording -- see IDPIdentityNotLinkedError.
	if !idp.AutoProvision {
		slog.Info("service.AuthnIDPs: sign-in refused, auto-provisioning is off", "idp", idp.Name)

		return nil, &domain.IDPIdentityNotLinkedError{}
	}

	if !info.EmailVerified {
		slog.Info("service.AuthnIDPs: sign-in refused, the provider does not vouch for the email", "idp", idp.Name)

		return nil, &domain.IDPIdentityNotLinkedError{}
	}

	if existing, err := ref.userService.GetByEmail(ctx, info.Email); err == nil && existing != nil {
		// The one case the takeover lived in: an account with this email
		// exists and nothing proves this identity belongs to its holder.
		slog.Warn("service.AuthnIDPs: sign-in refused, an account with the provider's email exists and is not linked to this identity",
			"idp", idp.Name, "user.id", existing.ID.String())

		return nil, &domain.IDPIdentityNotLinkedError{}
	} else if err != nil && !isNotFound(err) {
		return nil, err
	}

	userID := uuid.NewV7()

	if err := ref.authnService.RegisterUser(ctx, &domain.RegisterUserInput{
		ID:             userID,
		Email:          info.Email,
		FirstName:      info.FirstName,
		LastName:       info.LastName,
		Password:       uuid.NewV7().String(), // random and never used; the account has no password
		Disabled:       new(false),
		RegisterMethod: domain.RegisterMethodOAuth,
	}); err != nil {
		return nil, err
	}

	if err := ref.usersIdentities.Link(ctx, &domain.LinkUserIdentityInput{
		UserID: userID, IDPID: idp.ID, Subject: info.Subject, Email: info.Email,
	}); err != nil {
		return nil, err
	}

	login, err := ref.authnService.LoginUserByID(ctx, userID, domain.LoginMethodOAuth)
	if err != nil {
		return nil, err
	}

	slog.Info("service.AuthnIDPs: account provisioned from a provider identity", "idp", idp.Name, "user.id", userID.String())

	return &domain.IDPCallbackOutput{EventType: eventType, Login: login}, nil
}

// link attaches the identity to the account that started the link.
func (ref *AuthnIDPsService) link(ctx context.Context, idp *domain.IDP, info *domain.UserInfo, userID uuid.UUID) (*domain.IDPCallbackOutput, error) {
	if userID == uuid.Nil() {
		return nil, &domain.InvalidJWTError{Message: "the link state names no account"}
	}

	// The account must still exist and be enabled: the state was minted up to
	// fifteen minutes ago.
	user, err := ref.userService.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if user.Disabled != nil && *user.Disabled {
		return nil, &domain.IDPIdentityNotLinkedError{}
	}

	if err := ref.usersIdentities.Link(ctx, &domain.LinkUserIdentityInput{
		UserID: userID, IDPID: idp.ID, Subject: info.Subject, Email: info.Email,
	}); err != nil {
		return nil, err
	}

	slog.Info("service.AuthnIDPs: provider identity linked", "idp", idp.Name, "user.id", userID.String())

	return &domain.IDPCallbackOutput{EventType: domain.IDPEventTypeLink, Linked: userID}, nil
}

// ListIdentities implements driving.AuthnIDPs.
func (ref *AuthnIDPsService) ListIdentities(ctx context.Context, userID uuid.UUID) ([]domain.UserIdentity, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "ListIdentities")
	defer span.End()

	items, err := ref.usersIdentities.SelectByUserID(ctx, userID)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "identities listed", attribute.Int("count", len(items)))

	return items, nil
}

// UnlinkIdentity implements driving.AuthnIDPs.
func (ref *AuthnIDPsService) UnlinkIdentity(ctx context.Context, userID, idpID uuid.UUID) error {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "UnlinkIdentity")
	defer span.End()

	// Never strand an account: one with no password (local_account false)
	// and a single identity would have nothing left to sign in with.
	user, err := ref.userService.GetByID(ctx, userID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if user.LocalAccount == nil || !*user.LocalAccount {
		items, err := ref.usersIdentities.SelectByUserID(ctx, userID)
		if err != nil {
			return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		}

		if len(items) <= 1 {
			return o11y.RecordError(ctx, span, start, &domain.UserIdentityAlreadyLinkedError{Message: "this is the only way into the account; set a password or link another provider first"}, ref.metrics, attrs)
		}
	}

	if err := ref.usersIdentities.Unlink(ctx, &domain.UnlinkUserIdentityInput{UserID: userID, IDPID: idpID}); err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "identity unlinked", attribute.String("idp.id", idpID.String()))

	return nil
}

// spendState verifies the state, checks it was minted for this IdP, spends it,
// and unseals what it carried.
//
// It is spent BEFORE the authorization code is exchanged: a state that
// survives a failed exchange is a state an attacker can retry. The cost is that
// a transient failure at the provider means starting the flow again.
func (ref *AuthnIDPsService) spendState(ctx context.Context, state string, idpID uuid.UUID) (map[string]any, stateData, domain.IDPEventType, error) {
	var data stateData

	claims, err := ref.tokenSigner.Verify(ctx, state)
	if err != nil {
		return nil, data, "", err
	}

	tokenType := domain.TokenType(claimString(claims, "token_type"))

	var eventType domain.IDPEventType

	switch tokenType {
	case domain.TokenTypeIDPSignin:
		eventType = domain.IDPEventTypeLogin
	case domain.TokenTypeIDPRegister:
		eventType = domain.IDPEventTypeRegister
	case domain.TokenTypeIDPLink:
		eventType = domain.IDPEventTypeLink
	default:
		return nil, data, "", &domain.InvalidJWTError{Message: "the state is not valid"}
	}

	// The event the token was signed for and the one its subject names must
	// agree, and the IdP must be the one the callback arrived at.
	if claimString(claims, "sub") != eventType.String() || claimString(claims, "idp") != idpID.String() {
		return nil, data, "", &domain.InvalidJWTError{Message: "the state is not valid"}
	}

	jti, err := uuid.Parse(claimString(claims, "jti"))
	if err != nil {
		return nil, data, "", &domain.InvalidJWTError{Message: "the state is not valid"}
	}

	expiresAt := time.Now().Add(idpRegistrationDuration)
	if exp, ok := claimExpiry(claims); ok {
		expiresAt = exp
	}

	// uuid.Nil() for the user: a state token's subject is the event, and in a
	// registration flow no account exists yet.
	firstUse, err := ref.revokedTokens.Consume(ctx, jti, uuid.Nil(), tokenType, expiresAt)
	if err != nil {
		return nil, data, "", err
	}

	if !firstUse {
		// The same wording every other bad state gets. A caller learns their
		// state was not accepted, never that it was accepted once already.
		slog.Warn("service.AuthnIDPs: an OAuth state was presented twice; the callback was refused", "jti", jti)

		return nil, data, "", &domain.InvalidJWTError{Message: "the state is not valid"}
	}

	if err := ref.unseal(claimString(claims, "data"), &data); err != nil {
		return nil, data, "", &domain.InvalidJWTError{Message: "the state is not valid"}
	}

	if data.Verifier == "" || data.Nonce == "" {
		return nil, data, "", &domain.InvalidJWTError{Message: "the state is not valid"}
	}

	return claims, data, eventType, nil
}

func (ref *AuthnIDPsService) seal(data stateData) (string, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return ref.cipher.EncryptString(raw)
}

func (ref *AuthnIDPsService) unseal(sealed string, data *stateData) error {
	if sealed == "" {
		return errors.New("the state carries no data")
	}

	raw, err := ref.cipher.DecryptString(sealed)
	if err != nil {
		return err
	}

	return json.Unmarshal(raw, data)
}

// randomToken is 32 random bytes, URL-safe: a PKCE verifier (43 characters,
// within RFC 7636's 43-128) and a nonce.
func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}

	return base64.RawURLEncoding.EncodeToString(b)
}

func isNotFound(err error) bool {
	if _, ok := errors.AsType[*domain.UserIdentityNotFoundError](err); ok {
		return true
	}

	if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
		return true
	}

	return false
}
