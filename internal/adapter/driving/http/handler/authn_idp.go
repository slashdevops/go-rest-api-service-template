package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"uuid"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driving"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// AuthnIDPsHandlerConf is the configuration struct for the AuthnIDPsHandler.
type AuthnIDPsHandlerConf struct {
	Service       driving.AuthnIDPs
	IDPsService   driving.IDPs
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// AuthnIDPsHandler is the handler that will handle the authentication of users.
type AuthnIDPsHandler struct {
	service         driving.AuthnIDPs
	idpsService     driving.IDPs
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewAuthnIDPsHandler creates a new AuthnIDPsHandler.
func NewAuthnIDPsHandler(conf AuthnIDPsHandlerConf) (*AuthnIDPsHandler, error) {
	if conf.Service == nil {
		return nil, &domain.InvalidServiceError{Message: "driving.AuthnIDPs is required"}
	}

	if conf.IDPsService == nil {
		return nil, &domain.InvalidServiceError{Message: "IDPsService is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &AuthnIDPsHandler{
		service:       conf.Service,
		idpsService:   conf.IDPsService,
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
	}

	if conf.MetricsPrefix != "" {
		ref.metricsPrefix = strings.ReplaceAll(conf.MetricsPrefix, "-", "_")
		ref.metricsPrefix += "_"
	}

	ref.metricsMetadata = o11y.Metadata{
		Layer:  AppLayer,
		Domain: "AuthnIDPs",
		Action: "NewAuthnIDPsHandler",
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

// RegisterRoutes registers the routes for the AuthnIDPsHandler.
//
// The public routes start a flow and answer the callback; the three under
// protected take the caller's session: starting a link, listing and removing
// the caller's identities.
func (ref *AuthnIDPsHandler) RegisterRoutes(mux *http.ServeMux, protected ...middleware.Middleware) {
	mux.HandleFunc("GET /auth/idp/{idp_id}/login", ref.login)
	mux.HandleFunc("GET /auth/idp/{idp_id}/register", ref.registerUser)
	mux.HandleFunc("GET /auth/idp/{idp_id}/callback", ref.callback)
	mux.HandleFunc("GET /auth/idp/available", ref.availableIDPs)

	mdw := middleware.Chain(protected...)
	mux.Handle("GET /auth/idp/{idp_id}/link", mdw.ThenFunc(ref.link))
	mux.Handle("GET /me/identities", mdw.ThenFunc(ref.listIdentities))
	mux.Handle("DELETE /me/identities/{idp_id}", mdw.ThenFunc(ref.unlinkIdentity))
}

// login Start a sign-in through an identity provider
//
//	@ID				01988e60-89e5-72ab-adb4-3eef95d1afd3
//	@Summary		Initiate IDP login
//	@Description	Build the authorization URL for a sign-in. The caller sends the browser there; the provider returns it to the frontend callback route with a state and a code
//	@Tags			AuthnIDPs
//	@Produce		json
//	@Param			idp_id	path		string						true	"Identity provider id"	Format(uuid)
//	@Success		200		{object}	payload.IDPLoginResponse	"The URL to send the browser to"
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid identity provider id"
//	@Failure		404		{object}	payload.HTTPMessage			"Unknown or disabled identity provider"
//	@Failure		429		{object}	payload.HTTPMessage			"Too many requests"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error"
//	@Failure		503		{object}	payload.HTTPMessage			"The identity provider is not reachable"
//	@Router			/auth/idp/{idp_id}/login [get]
func (ref *AuthnIDPsHandler) login(w http.ResponseWriter, r *http.Request) {
	ref.start(w, r, "login", domain.IDPEventTypeLogin)
}

// registerUser Start a registration through an identity provider
//
//	@ID				019894ba-6014-79cf-bff4-6668484cc7e3
//	@Summary		Initiate IDP registration
//	@Description	Build the authorization URL for a registration. Same flow as a login; the callback creates the account when the provider vouches for the email and allows auto-provisioning
//	@Tags			AuthnIDPs
//	@Produce		json
//	@Param			idp_id	path		string						true	"Identity provider id"	Format(uuid)
//	@Success		200		{object}	payload.IDPRegisterResponse	"The URL to send the browser to"
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid identity provider id"
//	@Failure		404		{object}	payload.HTTPMessage			"Unknown or disabled identity provider"
//	@Failure		429		{object}	payload.HTTPMessage			"Too many requests"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error"
//	@Failure		503		{object}	payload.HTTPMessage			"The identity provider is not reachable"
//	@Router			/auth/idp/{idp_id}/register [get]
func (ref *AuthnIDPsHandler) registerUser(w http.ResponseWriter, r *http.Request) {
	ref.start(w, r, "registerUser", domain.IDPEventTypeRegister)
}

// link Start linking an identity provider to the caller's account
//
//	@ID				01a07319-1d32-7bd2-8ba0-f7da9aaaed0a
//	@Summary		Initiate IDP link
//	@Description	Build the authorization URL for linking a provider identity to the signed-in account. The only way an existing account gains one: the session proves the account, the provider proves the identity
//	@Tags			AuthnIDPs
//	@Produce		json
//	@Param			idp_id	path		string						true	"Identity provider id"	Format(uuid)
//	@Success		200		{object}	payload.IDPLoginResponse	"The URL to send the browser to"
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid identity provider id"
//	@Failure		401		{object}	payload.HTTPMessage			"Invalid or expired token"
//	@Failure		403		{object}	payload.HTTPMessage			"Not authorized"
//	@Failure		404		{object}	payload.HTTPMessage			"Unknown or disabled identity provider"
//	@Failure		429		{object}	payload.HTTPMessage			"Too many requests"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error"
//	@Failure		503		{object}	payload.HTTPMessage			"The identity provider is not reachable"
//	@Router			/auth/idp/{idp_id}/link [get]
//	@Security		AccessToken
func (ref *AuthnIDPsHandler) link(w http.ResponseWriter, r *http.Request) {
	ref.start(w, r, "link", domain.IDPEventTypeLink)
}

// start is the shared half of login, register and link.
func (ref *AuthnIDPsHandler) start(w http.ResponseWriter, r *http.Request, action string, eventType domain.IDPEventType) {
	startedAt := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, action)
	defer span.End()

	idpID, err := parseUUIDQueryParams(r.PathValue("idp_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return
	}

	userID := uuid.Nil()

	if eventType == domain.IDPEventTypeLink {
		userID, err = callerFromContext(r)
		if err != nil {
			e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)
			respond.WriteInternalError(w, r, e)

			return
		}
	}

	url, err := ref.service.GetLoginURL(ctx, idpID, eventType, userID)
	if err != nil {
		ref.writeStartError(w, r, span, startedAt, attrs, err)

		return
	}

	out := payload.IDPLoginResponse{IDPID: idpID, RedirectURL: url, RedirectCode: http.StatusFound}

	if eventType == domain.IDPEventTypeRegister {
		if err := respond.WriteJSONData(w, http.StatusOK, payload.IDPRegisterResponse(out)); err != nil {
			e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)
			respond.WriteInternalError(w, r, e)
		}

		return
	}

	if err := respond.WriteJSONData(w, http.StatusOK, out); err != nil {
		e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	o11y.RecordSuccess(ctx, span, startedAt, ref.metrics, attrs, "authorization URL built",
		attribute.String("idp.id", idpID.String()), attribute.String("event", eventType.String()))
}

// callback Complete a sign-in, registration or link started at an identity provider
//
//	@ID				01988e60-89e5-72ee-9db4-db5cd7535717
//	@Summary		Handle IDP OAuth callback
//	@Description	Called by the frontend callback route with the state and code from the provider, never by the browser: JSON answer, no cookie, no redirect. Spends the state, exchanges the code with PKCE, resolves the identity by subject
//	@Tags			AuthnIDPs
//	@Produce		json
//	@Param			idp_id	path		string						true	"Identity provider id"	Format(uuid)
//	@Param			state	query		string						true	"The state the provider echoed back"
//	@Param			code	query		string						false	"The authorization code, absent when the provider reports an error"
//	@Param			error	query		string						false	"The provider's error code, for example access_denied"
//	@Success		200		{object}	payload.IDPCallbackResponse	"The outcome"
//	@Failure		400		{object}	payload.HTTPMessage			"A state that is missing, spent, expired or minted for another provider, or a missing code"
//	@Failure		401		{object}	payload.HTTPMessage			"The provider refused the sign-in, or the identity is not linked to an account here"
//	@Failure		404		{object}	payload.HTTPMessage			"Unknown or disabled identity provider"
//	@Failure		409		{object}	payload.HTTPMessage			"The identity is already linked to an account"
//	@Failure		429		{object}	payload.HTTPMessage			"Too many requests"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error"
//	@Failure		503		{object}	payload.HTTPMessage			"The identity provider is not reachable"
//	@Router			/auth/idp/{idp_id}/callback [get]
func (ref *AuthnIDPsHandler) callback(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "callback")
	defer span.End()

	idpID, err := parseUUIDQueryParams(r.PathValue("idp_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return
	}

	q := r.URL.Query()

	// The provider says why the person did not arrive with a code: they
	// cancelled, the app is misconfigured. Answered as a refusal in this
	// service's words; the provider's description goes to the log.
	if providerErr := q.Get("error"); providerErr != "" {
		slog.Info("handler.AuthnIDPs.callback: the provider reported an error",
			"idp.id", idpID.String(), "error", providerErr, "description", q.Get("error_description"))

		e := o11y.RecordError(ctx, span, startedAt, &domain.InvalidAuthnServiceError{Message: "the identity provider did not complete the sign-in"}, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusUnauthorized, e.Error())

		return
	}

	state, code := q.Get("state"), q.Get("code")
	if state == "" || code == "" {
		e := o11y.RecordError(ctx, span, startedAt, &domain.InvalidInputError{Message: "state and code are required"}, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return
	}

	out, err := ref.service.Callback(ctx, idpID, state, code)
	if err != nil {
		ref.writeCallbackError(w, r, span, startedAt, attrs, err)

		return
	}

	resp := payload.IDPCallbackResponse{Event: out.EventType.String()}

	switch {
	case out.Login != nil:
		typedResources, err := payload.NewAuthzPermissions(out.Login.Resources)
		if err != nil {
			e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)
			respond.WriteInternalError(w, r, e)

			return
		}

		resp.Login = &payload.LoginUserResponse{
			AccessToken:  out.Login.AccessToken,
			RefreshToken: out.Login.RefreshToken,
			TokenType:    out.Login.TokenType,
			UserID:       out.Login.UserID,
			Resources:    typedResources,
		}
	case out.Linked != uuid.Nil():
		resp.LinkedTo = new(out.Linked)
	}

	if err := respond.WriteJSONData(w, http.StatusOK, resp); err != nil {
		e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	o11y.RecordSuccess(ctx, span, startedAt, ref.metrics, attrs, "callback completed",
		attribute.String("idp.id", idpID.String()), attribute.String("event", out.EventType.String()))
}

// listIdentities List the provider identities linked to the caller's account
//
//	@ID				01a07319-1d32-7ca7-834a-d8dcc6a19fa0
//	@Summary		List my linked identities
//	@Description	The identity providers the signed-in account can sign in through
//	@Tags			Me
//	@Produce		json
//	@Success		200	{object}	payload.ListUserIdentitiesResponse	"The linked identities"
//	@Failure		401	{object}	payload.HTTPMessage					"Invalid or expired token"
//	@Failure		403	{object}	payload.HTTPMessage					"Not authorized"
//	@Failure		429	{object}	payload.HTTPMessage					"Too many requests"
//	@Failure		500	{object}	payload.HTTPMessage					"Internal server error"
//	@Router			/me/identities [get]
//	@Security		AccessToken
func (ref *AuthnIDPsHandler) listIdentities(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "listIdentities")
	defer span.End()

	userID, err := callerFromContext(r)
	if err != nil {
		e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	items, err := ref.service.ListIdentities(ctx, userID)
	if err != nil {
		e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	resp := payload.ListUserIdentitiesResponse{Items: make([]payload.UserIdentityResponse, 0, len(items))}
	for _, it := range items {
		resp.Items = append(resp.Items, payload.UserIdentityResponse{
			IDPID: it.IDPID, IDPName: it.IDPName, IDPTypeName: it.IDPTypeName, Email: it.Email, LinkedAt: it.LinkedAt,
		})
	}

	if err := respond.WriteJSONData(w, http.StatusOK, resp); err != nil {
		e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	o11y.RecordSuccess(ctx, span, startedAt, ref.metrics, attrs, "identities listed", attribute.Int("count", len(items)))
}

// unlinkIdentity Remove a provider identity from the caller's account
//
//	@ID				01a07319-1d32-7cac-885b-6c544f5dbe08
//	@Summary		Unlink my identity
//	@Description	Remove the identity at one provider from the signed-in account. Refused when it is the only way into an account that has no password
//	@Tags			Me
//	@Produce		json
//	@Param			idp_id	path		string				true	"Identity provider id"	Format(uuid)
//	@Success		200		{object}	payload.HTTPMessage	"Identity unlinked"
//	@Failure		400		{object}	payload.HTTPMessage	"Invalid identity provider id"
//	@Failure		401		{object}	payload.HTTPMessage	"Invalid or expired token"
//	@Failure		403		{object}	payload.HTTPMessage	"Not authorized"
//	@Failure		404		{object}	payload.HTTPMessage	"No identity at that provider"
//	@Failure		409		{object}	payload.HTTPMessage	"It is the only way into the account"
//	@Failure		429		{object}	payload.HTTPMessage	"Too many requests"
//	@Failure		500		{object}	payload.HTTPMessage	"Internal server error"
//	@Router			/me/identities/{idp_id} [delete]
//	@Security		AccessToken
func (ref *AuthnIDPsHandler) unlinkIdentity(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "unlinkIdentity")
	defer span.End()

	idpID, err := parseUUIDQueryParams(r.PathValue("idp_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return
	}

	userID, err := callerFromContext(r)
	if err != nil {
		e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	if err := ref.service.UnlinkIdentity(ctx, userID, idpID); err != nil {
		e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)

		switch {
		case isType[*domain.UserIdentityNotFoundError](err):
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
		case isType[*domain.UserIdentityAlreadyLinkedError](err):
			respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
		case isType[*domain.ValidationErrors](err), isType[*domain.InvalidInputError](err):
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		default:
			respond.WriteInternalError(w, r, e)
		}

		return
	}

	respond.WriteJSONMessage(w, r, http.StatusOK, "identity unlinked")
	o11y.RecordSuccess(ctx, span, startedAt, ref.metrics, attrs, "identity unlinked", attribute.String("idp.id", idpID.String()))
}

// availableIDPs List the identity providers a visitor may sign in through
//
//	@ID				0198fb33-7333-76f9-bcb4-1af086de3e10
//	@Summary		List identity providers
//	@Description	Retrieve every ENABLED identity provider, for the login page. Disabled providers stay configured and are not offered
//	@Tags			AuthnIDPs
//	@Produce		json
//	@Success		200	{object}	payload.ListIDPAvailableResponse	"The providers"
//	@Failure		429	{object}	payload.HTTPMessage					"Too many requests"
//	@Failure		500	{object}	payload.HTTPMessage					"Internal server error"
//	@Router			/auth/idp/available [get]
func (ref *AuthnIDPsHandler) availableIDPs(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "availableIDPs")
	defer span.End()

	out, err := ref.idpsService.GetAvailableIDPs(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	outResponse := payload.ListIDPAvailableResponse{Items: make([]payload.IDPAvailableResponse, len(out.Items))}

	for i, idp := range out.Items {
		outResponse.Items[i] = payload.IDPAvailableResponse{
			ID:            idp.ID,
			Name:          idp.Name,
			Description:   idp.Description,
			Logo:          idp.Logo,
			AutoProvision: idp.AutoProvision,
			IDPType: payload.IDPTypesAvailable{
				ID:          idp.IDPType.ID,
				Name:        idp.IDPType.Name,
				Description: idp.IDPType.Description,
			},
		}
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, startedAt, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	o11y.RecordSuccess(ctx, span, startedAt, ref.metrics, attrs, "available IDPs retrieved")
}

// writeStartError maps a failure to build the authorization URL.
func (ref *AuthnIDPsHandler) writeStartError(w http.ResponseWriter, r *http.Request, span trace.Span, startedAt time.Time, attrs []attribute.KeyValue, err error) {
	e := o11y.RecordError(r.Context(), span, startedAt, err, ref.metrics, attrs)

	switch {
	case isType[*domain.IDPNotFoundError](err):
		respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
	case isType[*domain.InvalidAuthnServiceError](err):
		// The adapter's "not reachable": discovery failed.
		respond.WriteJSONMessage(w, r, http.StatusServiceUnavailable, e.Error())
	case isType[*domain.ValidationErrors](err), isType[*domain.InvalidInputError](err), isType[*domain.InvalidIdentityProvidersError](err):
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
	default:
		respond.WriteInternalError(w, r, e)
	}
}

// writeCallbackError maps a failed callback. Three refusals share the 401:
// the provider refused, the identity is not linked here, the account cannot
// sign in -- the frontend renders one message for all of them.
func (ref *AuthnIDPsHandler) writeCallbackError(w http.ResponseWriter, r *http.Request, span trace.Span, startedAt time.Time, attrs []attribute.KeyValue, err error) {
	e := o11y.RecordError(r.Context(), span, startedAt, err, ref.metrics, attrs)

	switch {
	case isType[*domain.InvalidJWTError](err), isType[*domain.InvalidInputError](err), isType[*domain.ValidationErrors](err), isType[*domain.InvalidIdentityProvidersError](err):
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
	case isType[*domain.IDPNotFoundError](err):
		respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
	case isType[*domain.UserIdentityAlreadyLinkedError](err):
		respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
	case isType[*domain.IDPIdentityNotLinkedError](err), isType[*domain.InvalidCredentialsError](err):
		respond.WriteJSONMessage(w, r, http.StatusUnauthorized, e.Error())
	case isType[*domain.IDPUnreachableError](err):
		respond.WriteJSONMessage(w, r, http.StatusServiceUnavailable, e.Error())
	case isType[*domain.InvalidAuthnServiceError](err):

		respond.WriteJSONMessage(w, r, http.StatusUnauthorized, e.Error())
	default:
		respond.WriteInternalError(w, r, e)
	}
}
