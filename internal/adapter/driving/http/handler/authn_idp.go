package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

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
func (ref *AuthnIDPsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/idp/{idp_id}/login", ref.login)
	mux.HandleFunc("GET /auth/idp/{idp_id}/callback", ref.callback)
	mux.HandleFunc("GET /auth/idp/{idp_id}/register", ref.registerUser)
	mux.HandleFunc("GET /auth/idp/available", ref.availableIDPs)
}

// login handles the login request for a specific IDP.
//
//	@Id				01988e60-89e5-72ab-adb4-3eef95d1afd3
//	@Summary		Initiate IDP login
//	@Description	Initiate authentication with specified Identity Provider and returns redirect URL for OAuth flow.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			idp_id	path		string						true	"Identity Provider unique identifier"	Format(uuid)
//	@Success		200		{object}	payload.IDPLoginResponse	"Login URL generated successfully. RedirectURL and RedirectCode are fields of the JSON body — this endpoint returns 200, it does not itself redirect."
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid IDP ID format or malformed request"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error during URL generation. NOTE: an unknown Identity Provider currently surfaces here rather than as a 404 — the handler has no not-found branch. Tracked as a behaviour defect; this annotation documents what the endpoint does today, not what it should do."
//	@Router			/auth/idp/{idp_id}/login [get]
func (ref *AuthnIDPsHandler) login(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "login")
	defer span.End()

	idpID, err := parseUUIDQueryParams(r.PathValue("idp_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	url, err := ref.service.GetLoginURL(ctx, idpID, domain.IDPEventTypeLogin)
	if err != nil {
		_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
		_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
		_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
		_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
		if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	outResponse := payload.IDPLoginResponse{
		IDPID:        idpID,
		RedirectURL:  url,
		RedirectCode: http.StatusFound,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "IDP found",
		attribute.String("idp.id", outResponse.IDPID.String()),
	)
}

// registerUser handles the user registration request for a specific IDP.
//
//	@Id				019894ba-6014-79cf-bff4-6668484cc7e3
//	@Summary		Initiate IDP registration
//	@Description	Initiate user registration with specified Identity Provider and returns redirect URL for OAuth registration flow.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			idp_id	path		string						true	"Identity Provider unique identifier"	Format(uuid)
//	@Success		200		{object}	payload.IDPRegisterResponse	"Registration URL generated successfully. RedirectURL and RedirectCode are fields of the JSON body — this endpoint returns 200, it does not itself redirect."
//	@Failure		400		{object}	payload.HTTPMessage			"Invalid IDP ID format or malformed request"
//	@Failure		500		{object}	payload.HTTPMessage			"Internal server error during URL generation. NOTE: an unknown Identity Provider currently surfaces here rather than as a 404 — the handler has no not-found branch. Tracked as a behaviour defect; this annotation documents what the endpoint does today, not what it should do."
//	@Router			/auth/idp/{idp_id}/register [get]
func (ref *AuthnIDPsHandler) registerUser(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "registerUser")
	defer span.End()

	idpID, err := parseUUIDQueryParams(r.PathValue("idp_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	url, err := ref.service.GetLoginURL(ctx, idpID, domain.IDPEventTypeRegister)
	if err != nil {
		_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
		_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
		_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
		_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
		if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	outResponse := payload.IDPRegisterResponse{
		IDPID:        idpID,
		RedirectURL:  url,
		RedirectCode: http.StatusFound,
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "IDP found",
		attribute.String("idp.id", outResponse.IDPID.String()),
	)

	// http.Redirect(w, r, url, http.StatusFound)
}

// callback handles the callback request from the IDP after authentication.
//
//	@Id				01988e60-89e5-72ee-9db4-db5cd7535717
//	@Summary		Handle IDP OAuth callback
//	@Description	Process OAuth callback from Identity Provider, validates state and authorization code.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			idp_id	path		string				true	"Identity Provider unique identifier"	Format(uuid)
//	@Param			state	query		string				true	"OAuth state parameter for CSRF protection"
//	@Param			code	query		string				true	"OAuth authorization code from IDP"
//	@Success		302		{string}	string				"Callback processed successfully. Authentication cookies are set and the caller is redirected to the IDP's configured login or register redirect URL. This endpoint never returns a JSON body on success — it always issues a redirect."
//	@Header			302		{string}	Location			"The IDP's configured LoginRedirectURL or RegisterRedirectURL"
//	@Failure		400		{object}	payload.HTTPMessage	"Invalid parameters, missing state/code, or invalid IDP ID format"
//	@Failure		404		{object}	payload.HTTPMessage	"Identity Provider not found"
//	@Failure		409		{object}	payload.HTTPMessage	"User already exists during registration"
//	@Failure		500		{object}	payload.HTTPMessage	"Internal server error during callback processing"
//	@Router			/auth/idp/{idp_id}/callback [get]
func (ref *AuthnIDPsHandler) callback(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "callback")
	defer span.End()

	idpID, err := parseUUIDQueryParams(r.PathValue("idp_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	// get the state from the url
	state := r.URL.Query().Get("state")
	if state == "" {
		e := o11y.RecordError(ctx, span, start, fmt.Errorf("missing state"), ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	// get the code from the url
	code := r.URL.Query().Get("code")
	if code == "" {
		e := o11y.RecordError(ctx, span, start, fmt.Errorf("missing code"), ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
		return
	}

	out := ref.service.Callback(ctx, idpID, state, code)

	switch out.GetEventType() {
	case domain.IDPEventTypeLogin:
		outResponse, err := out.GetLoginResponse()
		if err != nil {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		accessTokenExp, err := getJWTExpiration(outResponse.AccessToken)
		if err != nil {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		refreshTokenExp, err := getJWTExpiration(outResponse.RefreshToken)
		if err != nil {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		// set the cookies
		http.SetCookie(w, &http.Cookie{
			Name:     "auth.backend.accessToken",
			Value:    outResponse.AccessToken,
			Path:     "/",
			SameSite: http.SameSiteLaxMode,
			Secure:   true,
			HttpOnly: true,
			Expires:  accessTokenExp,
		})

		http.SetCookie(w, &http.Cookie{
			Name:     "auth.backend.refreshToken",
			Value:    outResponse.RefreshToken,
			Path:     "/",
			SameSite: http.SameSiteLaxMode,
			Secure:   true,
			HttpOnly: true,
			Expires:  refreshTokenExp,
		})

		http.SetCookie(w, &http.Cookie{
			Name:     "auth.backend.userId",
			Value:    outResponse.UserID.String(),
			Path:     "/",
			SameSite: http.SameSiteLaxMode,
			Secure:   true,
			HttpOnly: true,
			Expires:  refreshTokenExp,
		})

		idp, err := ref.idpsService.GetByID(ctx, idpID)
		if err != nil {
			_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
			_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
			_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
			_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
			if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
				e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
				respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
				return
			}

			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}

		o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user logged in",
			attribute.String("user.id", outResponse.UserID.String()),
		)

		http.Redirect(w, r, idp.LoginRedirectURL, http.StatusFound)

	case domain.IDPEventTypeRegister:
		outResponse, err := out.GetRegisterResponse()
		if err != nil {
			_, isInvalidMail := errors.AsType[*domain.InvalidEmailError](err)
			_, isInvalidPassword := errors.AsType[*domain.InvalidPasswordError](err)
			if isInvalidMail || isInvalidPassword {
				e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
				respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
				return
			}

			if _, ok := errors.AsType[*domain.UserAlreadyExistsError](err); ok {
				e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
				respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())
				return
			}

			_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
			_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
			_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
			_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
			if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
				e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
				respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
				return
			}

			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
			return
		}

		accessTokenExp, err := getJWTExpiration(outResponse.AccessToken)
		if err != nil {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		refreshTokenExp, err := getJWTExpiration(outResponse.RefreshToken)
		if err != nil {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		// set the cookies
		http.SetCookie(w, &http.Cookie{
			Name:     "auth.backend.accessToken",
			Value:    outResponse.AccessToken,
			Path:     "/",
			SameSite: http.SameSiteLaxMode,
			Secure:   true,
			HttpOnly: true,
			Expires:  accessTokenExp,
		})

		http.SetCookie(w, &http.Cookie{
			Name:     "auth.backend.refreshToken",
			Value:    outResponse.RefreshToken,
			Path:     "/",
			SameSite: http.SameSiteLaxMode,
			Secure:   true,
			HttpOnly: true,
			Expires:  refreshTokenExp,
		})

		http.SetCookie(w, &http.Cookie{
			Name:     "auth.backend.userId",
			Value:    outResponse.UserID.String(),
			Path:     "/",
			SameSite: http.SameSiteLaxMode,
			Secure:   true,
			HttpOnly: true,
			Expires:  refreshTokenExp,
		})

		idp, err := ref.idpsService.GetByID(ctx, idpID)
		if err != nil {
			_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
			_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
			_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
			_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
			if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
				e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
				respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
				return
			}

			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())
			return
		}

		o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "user created",
			attribute.String("user.id", outResponse.UserID.String()),
		)

		http.Redirect(w, r, idp.RegisterRedirectURL, http.StatusFound)

	case domain.IDPEventTypeUnknown:
		if err := out.GetUnknownResponse(); err != nil {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}
	}
}

// availableIDPs handles the request to get the list of available IDPs.
//
//	@Id				0198fb33-7333-76f9-bcb4-1af086de3e10
//	@Summary		List identity providers
//	@Description	Retrieve all identity providers configured and available for user authentication and registration.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	payload.ListIDPAvailableResponse	"List of available Identity Providers retrieved successfully"
//	@Failure		400	{object}	payload.HTTPMessage					"Malformed request"
//	@Failure		500	{object}	payload.HTTPMessage					"Internal server error retrieving IDPs"
//	@Router			/auth/idp/available [get]
func (ref *AuthnIDPsHandler) availableIDPs(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "availableIDPs")
	defer span.End()

	out, err := ref.idpsService.GetAvailableIDPs(ctx)
	if err != nil {
		_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
		_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
		_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
		_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
		if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())
			return
		}

		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	outResponse := payload.ListIDPAvailableResponse{
		Items: make([]payload.IDPAvailableResponse, len(out.Items)),
	}

	for i, idp := range out.Items {
		outResponse.Items[i] = payload.IDPAvailableResponse{
			ID:          idp.ID,
			Name:        idp.Name,
			Description: idp.Description,
			Logo:        idp.Logo,
			IDPType: payload.IDPTypesAvailable{
				ID:          idp.IDPType.ID,
				Name:        idp.IDPType.Name,
				Description: idp.IDPType.Description,
			},
		}
	}

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusInternalServerError, e.Error())
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "available IDPs retrieved")
}
