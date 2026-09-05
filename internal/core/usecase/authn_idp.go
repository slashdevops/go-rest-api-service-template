package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"uuid"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/oauth"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/repository"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/token"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

const (
	idpLoginDuration        = 15 * time.Minute
	idpRegistrationDuration = 25 * time.Minute
)

type AuthnServiceConsumer interface {
	LoginUser(ctx context.Context, input *domain.LoginUserInput) (*domain.LoginUserOutput, error)
	RegisterUser(ctx context.Context, input *domain.RegisterUserInput) error
}

type IDPsServiceConsumer interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.IDP, error)
}

type AuthnIDPsServiceConf struct {
	AuthnService  AuthnServiceConsumer
	IDPsService   IDPsServiceConsumer
	TokenSigner   token.Signer
	OAuth         oauth.Provider
	RevokedTokens repository.RevokedTokens
	OT            *o11y.OpenTelemetry
	Issuer        string
	MetricsPrefix string
}

type AuthnIDPsService struct {
	authnService    AuthnServiceConsumer
	idpsService     IDPsServiceConsumer
	tokenSigner     token.Signer
	oauth           oauth.Provider
	revokedTokens   repository.RevokedTokens
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

	if conf.TokenSigner == nil {
		return nil, &domain.InvalidTokenSignerError{Message: "TokenSigner is nil, but it is required for AuthnIDPsService"}
	}

	if conf.OAuth == nil {
		return nil, &domain.InvalidIdentityProvidersError{Message: "OAuth provider is nil, but it is required for AuthnIDPsService"}
	}

	if len(conf.Issuer) <= 2 || len(conf.Issuer) > 100 {
		return nil, &domain.InvalidIssuerError{Message: "Issuer is invalid, but it is required for AuthnIDPsService"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is nil, but it is required for AuthnIDPsService"}
	}

	ref := &AuthnIDPsService{
		authnService:  conf.AuthnService,
		idpsService:   conf.IDPsService,
		tokenSigner:   conf.TokenSigner,
		oauth:         conf.OAuth,
		revokedTokens: conf.RevokedTokens,
		issuer:        conf.Issuer,
		ot:            conf.OT,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "AuthnIDPs",
			Action: "NewAuthnIDPsService",
		},
	}
	if conf.MetricsPrefix != "" {
		ref.metricsPrefix = strings.ReplaceAll(conf.MetricsPrefix, "-", "_")
		ref.metricsPrefix += "_"
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
		metric.WithDescription(fmt.Sprintf("Duration of %s handler calls", AppLayer)),
		metric.WithUnit("s"), // Seconds
	)
	if err != nil {
		return nil, err
	}

	ref.metrics = &o11y.LayerMetrics{
		Counter:   callsCounter,
		Histogram: callsDuration,
	}

	return ref, nil
}

// validateCallbackInputs validates the basic inputs for the callback function.
func (ref *AuthnIDPsService) validateCallbackInputs(ctx context.Context, span trace.Span, attrs []attribute.KeyValue, idpID uuid.UUID, state, code string) error {
	start := time.Now()

	supported, err := ref.idpsService.GetByID(ctx, idpID)
	if err != nil {
		return o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	if supported == nil {
		return fmt.Errorf("IDP %s is not supported", idpID)
	}

	if state == "" {
		errorValue := &domain.InvalidIdentityProvidersError{Message: "state is empty"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if code == "" {
		errorValue := &domain.InvalidIdentityProvidersError{Message: "code is empty"}
		return o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	return nil
}

// validateAndParseClaims validates and parses JWT claims from the state.
func (ref *AuthnIDPsService) validateAndParseClaims(ctx context.Context, state string, idpID uuid.UUID) (map[string]any, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "validateAndParseClaims")
	defer span.End()

	claims, err := ref.tokenSigner.Verify(ctx, state)
	if err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	tokenType, ok := claims["token_type"].(string)
	if !ok {
		errorValue := &domain.InvalidJWTError{Message: "token_type claim is missing"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	if tokenType != domain.TokenTypeIDPSignin.String() && tokenType != domain.TokenTypeIDPRegister.String() {
		errorValue := &domain.InvalidJWTError{Message: fmt.Sprintf("unknown token type %s", tokenType)}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// sub in this case is login or register (domain.IDPEventType)
	eventType, ok := claims["sub"].(string)
	if !ok {
		errorValue := &domain.InvalidJWTError{Message: "sub claim is missing"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// validate event type
	if eventType != domain.IDPEventTypeLogin.String() && eventType != domain.IDPEventTypeRegister.String() {
		errorValue := &domain.InvalidJWTError{Message: fmt.Sprintf("unknown sub claim %s", eventType)}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// idp id is the same as the event type
	jwtIDPID, ok := claims["idp"].(string)
	if !ok {
		errorValue := &domain.InvalidJWTError{Message: "idp claim is missing"}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// validate idp
	if jwtIDPID != idpID.String() {
		errorValue := &domain.InvalidJWTError{Message: fmt.Sprintf("unknown idp claim %s", jwtIDPID)}
		return nil, o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// Spend the state.
	//
	// The state is the only thing binding this callback to the redirect that
	// started it, and until now it was reusable for its whole life: the same
	// state and code could be replayed, and a callback URL captured from a log,
	// a referrer or browser history stayed live. A single-use state is what the
	// parameter is for.
	//
	// It is spent HERE, before the authorization code is exchanged, and that is
	// deliberate: a state that survives a failed exchange is a state an attacker
	// can retry. The cost is that a transient failure at the provider means
	// starting the flow again, which is the right way round.
	if err := ref.consumeState(ctx, claims); err != nil {
		return nil, o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "service.AuthnIDPs.validateAndParseClaims")

	return claims, nil
}

// consumeState marks the state token spent, and refuses it if something already
// had. Without a store there is nothing to record it in, so the state stays
// replayable and says so in the log rather than pretending otherwise.
func (ref *AuthnIDPsService) consumeState(ctx context.Context, claims map[string]any) error {
	if ref.revokedTokens == nil {
		slog.Warn("service.AuthnIDPs: no revocation store, so the OAuth state is replayable until it expires")

		return nil
	}

	jti, err := uuid.Parse(claimString(claims, "jti"))
	if err != nil {
		return &domain.InvalidJWTError{Message: "jti claim is not a uuid"}
	}

	expiresAt := time.Now().Add(idpLoginDuration)
	if exp, ok := claimExpiry(claims); ok {
		expiresAt = exp
	}

	// uuid.Nil() for the user: a state token's subject is the event that
	// started the flow, and in a registration flow no account exists yet.
	// The state token's own token_type claim names what is being spent: an
	// idp_signin or idp_register state. Anything else was refused by the
	// verifier before reaching here.
	tokenType := domain.TokenType(claimString(claims, "token_type"))
	if !tokenType.IsValid() {
		return &domain.InvalidJWTError{Message: "token_type claim is invalid"}
	}

	firstUse, err := ref.revokedTokens.Consume(ctx, jti, uuid.Nil(), tokenType, expiresAt)
	if err != nil {
		return err
	}

	if !firstUse {
		// The same wording every other bad state gets. A caller learns their
		// state was not accepted, never that it was accepted once already,
		// which would confirm to whoever captured it that they had the real
		// thing.
		slog.Warn("service.AuthnIDPs: an OAuth state was presented twice; the callback was refused", "jti", jti)

		return &domain.InvalidJWTError{Message: "the state is not valid"}
	}

	return nil
}

func (ref *AuthnIDPsService) GetLoginURL(ctx context.Context, idpID uuid.UUID, eventType domain.IDPEventType) (string, error) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "GetLoginURL")
	defer span.End()

	if idpID == uuid.Nil() {
		errorValue := &domain.InvalidIdentityProvidersError{Message: "idpID is empty"}
		return "", o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	// Validate eventType
	if eventType != domain.IDPEventTypeLogin && eventType != domain.IDPEventTypeRegister {
		errorValue := &domain.InvalidIdentityProvidersError{Message: fmt.Sprintf("invalid event type: %s", eventType)}
		return "", o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)
	}

	jwtClaims := domain.JWTClaims{
		IDP:     idpID.String(),
		Subject: eventType.String(),
		Issuer:  ref.issuer,
	}

	switch eventType {
	case domain.IDPEventTypeLogin:
		jwtClaims.TokenType = domain.TokenTypeIDPSignin
		jwtClaims.TokenDuration = idpLoginDuration
	case domain.IDPEventTypeRegister:
		jwtClaims.TokenType = domain.TokenTypeIDPRegister
		jwtClaims.TokenDuration = idpRegistrationDuration
	}

	state, err := ref.tokenSigner.Sign(ctx, jwtClaims)
	if err != nil {
		slog.Error("service.AuthnIDPs.GetLoginURL", "error", err)
		e := &domain.InvalidIdentityProvidersError{Message: "failed to create IDP signin state"}
		return "", o11y.RecordError(ctx, span, start, e, ref.metrics, attrs)
	}

	idp, err := ref.idpsService.GetByID(ctx, idpID)
	if err != nil {
		return "", o11y.RecordError(ctx, span, start, &domain.InvalidIdentityProvidersError{Message: fmt.Sprintf("failed to get idp %s: %v", idpID, err)}, ref.metrics, attrs)
	}

	url, err := ref.oauth.LoginURL(ctx, idp, state)
	if err != nil {
		return "", o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	}

	slog.Debug("service.AuthnIDPs.GetLoginURL", "idpID", idpID, "eventType", eventType, "url", url)

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "service.AuthnIDPs.GetLoginURL")

	return url, nil
}

func (ref *AuthnIDPsService) Callback(ctx context.Context, idpID uuid.UUID, state, code string) domain.IDPCallbackResult {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTrace(ctx, ref.ot.Traces.Tracer, ref.metricsMetadata, "Callback")
	defer span.End()

	if err := ref.validateCallbackInputs(ctx, span, attrs, idpID, state, code); err != nil {
		return &domain.UnknownCallbackResult{Err: err}
	}

	claims, err := ref.validateAndParseClaims(ctx, state, idpID)
	if err != nil {
		return &domain.UnknownCallbackResult{Err: err}
	}

	idp, err := ref.idpsService.GetByID(ctx, idpID)
	if err != nil {
		errVal := &domain.InvalidIdentityProvidersError{Message: fmt.Sprintf("failed to get idp %s: %v", idpID, err)}
		return &domain.UnknownCallbackResult{Err: o11y.RecordError(ctx, span, start, errVal, ref.metrics, attrs)}
	}

	tokenType := claims["token_type"].(string)
	switch tokenType {
	case domain.TokenTypeIDPSignin.String(): // handle login callback for login
		slog.Debug("service.AuthnIDPs.Callback - login", "idpID", idpID, "tokenType", tokenType)

		userInfo, err := ref.oauth.Authenticate(ctx, idp, code)
		if err != nil {
			// Passed through rather than re-wrapped. The adapter already says
			// this in words this service owns and has logged the provider's own
			// text; wrapping it again would only prefix ours with ours.
			return &domain.LoginCallbackResult{
				Result: nil,
				Err:    o11y.RecordError(ctx, span, start, err, ref.metrics, attrs),
			}
		}

		input := &domain.LoginUserInput{
			Email:       userInfo.Email,
			LoginMethod: domain.LoginMethodOAuth,
		}

		out, err := ref.authnService.LoginUser(ctx, input)
		if err != nil {
			errorValue := &domain.InvalidAuthnServiceError{Message: fmt.Sprintf("failed to login user: %v", err)}
			return &domain.LoginCallbackResult{
				Result: nil,
				Err:    o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs),
			}
		}

		o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "service.AuthnIDPs.Callback")

		return &domain.LoginCallbackResult{
			Result: out,
			Err:    nil,
		}

	case domain.TokenTypeIDPRegister.String(): // handle registration callback for Register
		slog.Debug("service.AuthnIDPs.Callback", "event", "registration", "idpID", idpID, "userInfo", "fetching")

		userInfo, err := ref.oauth.Authenticate(ctx, idp, code)
		if err != nil {
			// See the login branch: the adapter owns this wording already.
			return &domain.RegisterCallbackResult{
				Result: nil,
				Err:    o11y.RecordError(ctx, span, start, err, ref.metrics, attrs),
			}
		}

		userID := uuid.NewV7()

		userPassword := uuid.NewV7()

		inputRegister := &domain.RegisterUserInput{
			ID:             userID,
			Email:          userInfo.Email,
			FirstName:      userInfo.FirstName,
			LastName:       userInfo.LastName,
			Password:       userPassword.String(), // random password, not used
			Disabled:       new(false),
			RegisterMethod: domain.RegisterMethodOAuth,
		}

		// Call the registration service
		if err := ref.authnService.RegisterUser(ctx, inputRegister); err != nil {
			errorValue := &domain.InvalidAuthnServiceError{Message: fmt.Sprintf("failed to register user: %v", err)}
			return &domain.RegisterCallbackResult{
				Result: nil,
				Err:    o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs),
			}
		}

		inputLogin := &domain.LoginUserInput{
			Email:       userInfo.Email,
			LoginMethod: domain.LoginMethodOAuth,
		}

		out, err := ref.authnService.LoginUser(ctx, inputLogin)
		if err != nil {
			errorValue := &domain.InvalidAuthnServiceError{Message: fmt.Sprintf("failed to login user: %v", err)}
			return &domain.RegisterCallbackResult{
				Result: nil,
				Err:    o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs),
			}
		}

		o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "service.AuthnIDPs.Callback")

		return &domain.RegisterCallbackResult{
			Result: out,
			Err:    nil,
		}

	default:
		errorValue := &domain.InvalidIdentityProvidersError{Message: fmt.Sprintf("unknown token type %s", tokenType)}
		return &domain.UnknownCallbackResult{Err: o11y.RecordError(ctx, span, start, errorValue, ref.metrics, attrs)}
	}
}
