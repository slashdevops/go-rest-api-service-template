package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/slashdevops/go-rest-api-service-template/docs/api"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/jwtvalidator"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/server"
	"github.com/slashdevops/go-rest-api-service-template/internal/version"
)

// initHTTPServer initializes the HTTP server with all registered routes
func (a *App) initHTTPServer(ctx context.Context) error {
	// Configure server URL information
	serverProtocol := "http"
	if a.configs.HTTPServer.TLSEnabled.Value {
		serverProtocol = "https"
	}

	serverURL := fmt.Sprintf(
		"%s://%s:%d/%s",
		serverProtocol,
		a.configs.HTTPServer.Address.Value,
		a.configs.HTTPServer.Port.Value,
		apiPrefix,
	)

	healthStatusURL := fmt.Sprintf("%s/health/status", serverURL)
	versionURL := fmt.Sprintf("%s/version", serverURL)
	serverHost := fmt.Sprintf("%s:%d", a.configs.HTTPServer.Address.Value, a.configs.HTTPServer.Port.Value)
	swaggerURLIndex := fmt.Sprintf("%s/swagger/index.html", serverURL)

	slog.Info(
		"server endpoints",
		"api", serverURL,
		"health", healthStatusURL,
		"version", versionURL,
		"swagger", swaggerURLIndex,
	)

	if err := configureSwaggerMetadata(serverHost, apiPrefix, serverProtocol); err != nil {
		return fmt.Errorf("failed to configure swagger metadata: %w", err)
	}

	// Create a new router for API endpoints
	apiRouter := http.NewServeMux()

	// Location headers are built from this, never from a request header.
	respond.SetPublicBaseURL(a.configs.HTTPServer.PublicURL.Value)

	// Setup common middlewares. The order is the contract; middleware/doc.go
	// explains each position and TestEveryRequestIsRecoveredBoundedAndHeadered
	// pins it.
	apiCommonMdws := []middleware.Middleware{
		middleware.RequestID,
		middleware.Recovery,
		middleware.SecurityHeaders(a.securityHeadersOpts()),
		middleware.RewriteStandardErrorsAsJSON,
		middleware.Logging,
		middleware.HeaderAPIVersion(apiVersion),
		middleware.OtelTextMapPropagation,
		middleware.MaxBody(a.bodyLimits()),
		middleware.RequireJSONBody,
	}

	// The client IP resolver is needed by the limiter AND by the exemptions --
	// an excluded address must be exempt even when rate limiting is disabled --
	// and building it can fail, so it is built once, here.
	var clientIP *middleware.ClientIPResolver

	{
		var err error

		clientIP, err = middleware.NewClientIPResolver(a.configs.HTTPServer.TrustedProxiesList())
		if err != nil {
			// Config validation already rejects a malformed entry, so reaching
			// here means the two disagree; refuse to start rather than fall back
			// to a resolver that trusts nothing when an operator asked it to.
			return fmt.Errorf("http.server.trusted.proxies: %w", err)
		}
	}

	// Stated at startup because the two postures fail in OPPOSITE directions and
	// neither is visible from a request. Trusting nothing behind a proxy buckets
	// every client together; trusting a proxy that is not really in front of the
	// service lets a caller pick its own bucket by setting a header.
	//
	// This warning used to live in createRateLimiter, which is gone -- it must
	// not go with it. The measured consequence of getting it wrong: 30 password
	// guesses with a rotating X-Forwarded-For went from {401: 7, 429: 23} to
	// {401: 30}, which is not a weaker limit but no limit at all.
	if !clientIP.TrustsAnything() {
		slog.Warn(
			"the rate limiter is keyed on the peer address; X-Forwarded-For and X-Real-IP are ignored",
			"why", "http.server.trusted.proxies is empty",
			"note", "set it to the proxies in front of this service, or every client behind one shares a single bucket",
		)
	} else {
		slog.Info(
			"the rate limiter honours forwarding headers from trusted proxies only",
			"trusted_proxies", a.configs.HTTPServer.TrustedProxiesList(),
		)
	}

	// The exemptions are built above the limiter and wrap it. They used to live
	// INSIDE the limiter that only ran when rules were enabled -- which was not
	// the default -- so /health was rate limited and ratelimit.excluded.ips did
	// nothing at all.
	exemptions, err := a.rateLimitExemptions(clientIP)
	if err != nil {
		return err
	}

	// ONE limiter. There used to be two, mutually exclusive by an else -- the
	// rule limiter and a per-IP flag limiter whose budget was also the rule
	// limiter's fallback. That arrangement is gone: budgets live in the
	// rate_limits table and nowhere else, so there is nothing to choose between
	// and nothing to keep in step.
	if mdw := a.rateLimitMiddleware(middleware.RateLimitStagePreAuth, apiRouter, clientIP); mdw != nil {
		apiCommonMdws = append(apiCommonMdws, exemptions.Wrap(mdw))
	}

	// CORS goes AFTER the limiter. It answers a preflight itself, so placed
	// before the limiter every OPTIONS was answered for free -- an unmetered
	// request against any path. A rate-limited preflight gets a 429 without
	// CORS headers, which the browser treats as a refusal: the right answer.
	if a.configs.HTTPServer.CorsEnabled.Value {
		corsOpts := a.getCorsOptions()
		apiCommonMdws = append(apiCommonMdws, middleware.Cors(corsOpts))
	}

	// Create JWT validators
	jwtValidators := a.createJWTValidators()

	// Create middleware chains
	apiCommonMiddlewares := middleware.Chain(apiCommonMdws...)
	// The post-auth stage is appended to each authenticated chain below, AFTER
	// the token is verified: that is where the claims exist and where the mux
	// has already matched, so r.Pattern names the route template a rule targets.
	// Wrapped as well: an excluded address must be exempt in BOTH stages, or it
	// is exempt from the ip rule and limited by the user rule on the same
	// request. (The bypass prefixes are public routes with no post-auth chain,
	// so they never reach this one.)
	//
	// Kept nil when rules are off, rather than wrapped into a pass-through:
	// appendRateLimit treats nil as "no post-auth limiter", and a wrapper that
	// silently does nothing is exactly the inert-middleware shape that comment
	// exists to prevent.
	var postAuthRateLimit middleware.Middleware
	if mdw := a.rateLimitMiddleware(middleware.RateLimitStagePostAuth, apiRouter, clientIP); mdw != nil {
		postAuthRateLimit = exemptions.Wrap(mdw)
	}

	accessTokenMiddlewares := middleware.Chain(
		appendRateLimit([]middleware.Middleware{
			middleware.CheckAccessToken(jwtValidators, a.revokedAccessTokensCheckerOrNil()),
			middleware.CheckAuthz(a.services.Authz),
			// After the grant check: a project-scoped route also needs the
			// caller to be a member of THAT project, which the policy cannot
			// know. Routes without a project_id pass straight through.
			middleware.RequireProjectMembership(a.services.Projects),
		}, postAuthRateLimit)...,
	)
	// The same chain as accessTokenMiddlewares WITHOUT the revocation check.
	//
	// /auth/logout is the endpoint that revokes; gating it on the token not
	// already being revoked makes a second logout fail, and two tabs logging
	// out at once is ordinary. It is also the only endpoint where letting a
	// revoked token through costs nothing: logout is idempotent, and it can act
	// only on the caller's own tokens, which it verifies.
	logoutMiddlewares := middleware.Chain(
		appendRateLimit([]middleware.Middleware{
			middleware.CheckAccessToken(jwtValidators, nil),
			middleware.CheckAuthz(a.services.Authz),
		}, postAuthRateLimit)...,
	)

	// Special middleware chain for /me endpoints that checks user existence before authorization
	// This ensures proper 404 responses when a user has a valid JWT but has been deleted
	meEndpointMiddlewares := middleware.Chain(
		appendRateLimit([]middleware.Middleware{
			middleware.CheckAccessToken(jwtValidators, a.revokedAccessTokensCheckerOrNil()),
			middleware.CheckUserExists(a.services.Users),
			middleware.CheckAuthz(a.services.Authz),
		}, postAuthRateLimit)...,
	)
	// /auth/refresh is worth a token-scoped rule more than most endpoints: it
	// mints credentials, and rotation means a replayed token ends a session, so
	// a caller hammering it is either broken or hostile.
	refreshTokenMiddlewares := middleware.Chain(
		appendRateLimit([]middleware.Middleware{
			middleware.CheckRefreshToken(jwtValidators),
			middleware.CheckAuthz(a.services.Authz),
		}, postAuthRateLimit)...,
	)
	// Deliberately WITHOUT the post-auth limiter, here and on verification
	// below. Both are single-use flows reached from an emailed link, and the
	// account behind the token may not be usable yet -- a user- or token-scoped
	// rule would key on a subject that is mid-creation. The pre-auth IP rule is
	// the right control for these, and it already applies.
	passwordResetTokenMiddlewares := middleware.Chain(
		middleware.CheckPasswordResetToken(jwtValidators),
		middleware.CheckAuthz(a.services.Authz),
	)

	// Deliberately without CheckAuthz. The caller is proving an email address,
	// not exercising a permission: they have no roles yet, and their account is
	// disabled until this very call succeeds, so an authorisation check here
	// would refuse every legitimate verification. The token is the authority,
	// which is why it must be verified — and it is, by the middleware.
	verificationTokenMiddlewares := middleware.Chain(
		middleware.CheckVerificationToken(jwtValidators),
	)

	// Register public routes. The swagger UI is a development aid: it lists
	// every operation, field and example to anyone who asks, so a deployment
	// has to switch it on.
	if a.configs.HTTPServer.SwaggerEnabled.Value {
		slog.Warn("the swagger UI is served under /swagger/ to anyone; switch http.server.swagger.enabled off outside development")
		a.handlers.Swagger.RegisterRoutes(apiRouter)
	}
	// Liveness and status stay public -- an orchestrator carries no token. The
	// DETAILED view does not: it names every component, its configuration and
	// its timings, and it sat behind nothing on a path the limiter also exempts.
	a.handlers.Health.RegisterRoutes(apiRouter, middleware.Chain(), accessTokenMiddlewares)
	a.handlers.Version.RegisterRoutes(apiRouter)

	// Register protected routes
	a.handlers.Users.RegisterRoutes(apiRouter, accessTokenMiddlewares)
	a.handlers.Projects.RegisterRoutes(apiRouter, accessTokenMiddlewares)
	a.handlers.Products.RegisterRoutes(apiRouter, accessTokenMiddlewares)
	a.handlers.Policies.RegisterRoutes(apiRouter, accessTokenMiddlewares)
	a.handlers.Resources.RegisterRoutes(apiRouter, accessTokenMiddlewares)
	a.handlers.Roles.RegisterRoutes(apiRouter, accessTokenMiddlewares)
	a.handlers.Authn.RegisterRoutes(apiRouter, accessTokenMiddlewares, logoutMiddlewares, refreshTokenMiddlewares, passwordResetTokenMiddlewares, verificationTokenMiddlewares)
	a.handlers.IDPTypes.RegisterRoutes(apiRouter, accessTokenMiddlewares)
	a.handlers.IDPs.RegisterRoutes(apiRouter, accessTokenMiddlewares)
	a.handlers.AuthnIDPs.RegisterRoutes(apiRouter, accessTokenMiddlewares)
	a.handlers.ResourcesLimits.RegisterRoutes(apiRouter, accessTokenMiddlewares)
	a.handlers.RateLimits.RegisterRoutes(apiRouter, accessTokenMiddlewares)
	a.handlers.TokenLifetimes.RegisterRoutes(apiRouter, accessTokenMiddlewares)
	a.handlers.Me.RegisterRoutes(apiRouter, meEndpointMiddlewares)

	// Create the main router
	mainRouter := http.NewServeMux()
	mainRouter.Handle(
		fmt.Sprintf("/%s/", apiPrefix),
		http.StripPrefix(fmt.Sprintf("/%s", apiPrefix), apiCommonMiddlewares(apiRouter)),
	)

	// Create HTTP server
	a.httpServer = server.NewHTTPServer(server.HTTPServerConfig{
		Ctx:         ctx,
		HTTPHandler: mainRouter,
		Config:      a.configs.HTTPServer,
	})

	return nil
}

// configureSwaggerMetadata sets up the Swagger documentation metadata
func configureSwaggerMetadata(serverHost, apiPrefix, serverProtocol string) error {
	api.SwaggerInfo.Host = serverHost
	api.SwaggerInfo.BasePath = fmt.Sprintf("/%s", apiPrefix)
	api.SwaggerInfo.Schemes = []string{serverProtocol}
	api.SwaggerInfo.Version = version.Version

	return nil
}

// getCorsOptions creates CORS configuration options
func (a *App) getCorsOptions() middleware.CorsOpts {
	slog.Warn(
		"CORS enabled",
		"allowed_origins", a.configs.HTTPServer.CorsAllowedOrigins.Value,
		"allowed_methods", a.configs.HTTPServer.CorsAllowedMethods.Value,
		"allowed_headers", a.configs.HTTPServer.CorsAllowedHeaders.Value,
		"allow_credentials", a.configs.HTTPServer.CorsAllowCredentials.Value,
	)

	return middleware.CorsOpts{
		AllowedOrigins:   strings.Split(strings.Trim(a.configs.HTTPServer.CorsAllowedOrigins.Value, " "), ","),
		AllowedMethods:   strings.Split(strings.Trim(a.configs.HTTPServer.CorsAllowedMethods.Value, " "), ","),
		AllowedHeaders:   strings.Split(strings.Trim(a.configs.HTTPServer.CorsAllowedHeaders.Value, " "), ","),
		AllowCredentials: a.configs.HTTPServer.CorsAllowCredentials.Value,
	}
}

// createJWTValidators creates JWT validators for access and refresh tokens
// Uses cached public key from App.authKeys to avoid repeated file reads
func (a *App) createJWTValidators() map[jwtvalidator.ValidatorType]jwtvalidator.Validator {
	jwtValidators := make(map[jwtvalidator.ValidatorType]jwtvalidator.Validator)

	// The validators verify through the token signer rather than holding a
	// public key and a verification routine of their own. Returning an empty
	// map here is what makes a misconfiguration fail closed: checkToken finds
	// no validator for the type it needs and refuses the request.
	if a.tokenSigner == nil {
		slog.Error("token signer not initialised, cannot create JWT validators; every authenticated request will be refused")

		return jwtValidators
	}

	jwtValidators[jwtvalidator.ValidatorTypeAccessToken] = &jwtvalidator.AccessTokenValidator{
		Verifier: a.tokenSigner,
		ClientID: a.configs.Authn.Issuer.Value,
	}

	jwtValidators[jwtvalidator.ValidatorTypeRefreshToken] = &jwtvalidator.RefreshTokenValidator{
		Verifier: a.tokenSigner,
		ClientID: a.configs.Authn.Issuer.Value,
	}

	jwtValidators[jwtvalidator.ValidatorTypePasswordResetToken] = &jwtvalidator.PasswordResetTokenValidator{
		Verifier: a.tokenSigner,
		ClientID: a.configs.Authn.Issuer.Value,
	}

	jwtValidators[jwtvalidator.ValidatorTypeVerificationToken] = &jwtvalidator.VerificationTokenValidator{
		Verifier: a.tokenSigner,
		ClientID: a.configs.Authn.Issuer.Value,
	}

	return jwtValidators
}

// appendRateLimit adds the post-auth limiter to a chain, or returns the chain
// unchanged when rules are not enforced.
//
// A nil Middleware appended to a chain would be invoked and panic, so the check
// has to happen here rather than at each call site -- and doing it at each call
// site is how one of them ends up forgotten.
func appendRateLimit(chain []middleware.Middleware, mdw middleware.Middleware) []middleware.Middleware {
	if mdw == nil {
		return chain
	}

	return append(chain, mdw)
}
