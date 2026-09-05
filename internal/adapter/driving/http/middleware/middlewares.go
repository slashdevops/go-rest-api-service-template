package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/jwtvalidator"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/usecase"
)

// ContextKey is a type for context keys
type ContextKey string

func (k ContextKey) String() string {
	return string(k)
}

const (
	JwtClaims ContextKey = "jwt_claims"

	// JwtToken is the raw bearer token that checkToken verified, carried
	// alongside its claims.
	//
	// A handler that needs the token itself — /auth/refresh spends the very
	// token it was presented — must use THIS one and not re-read the request.
	// The refresh handler used to take a second token out of the request body
	// and act on that, so the token the middleware authorised and the token the
	// service acted on were two different values, and the validated one was
	// discarded.
	JwtToken ContextKey = "jwt_token"
)

// UsersServiceInterface defines the minimal interface needed for user existence checks
type UsersServiceInterface interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

// Middleware is a function that wraps an http.Handler
// to provide additional functionality
type Middleware func(http.Handler) http.Handler

// ThenFunc wraps an http.HandlerFunc with a middleware
// This is a convenience method to allow chaining middlewares
func (m Middleware) ThenFunc(h http.HandlerFunc) http.Handler {
	return m(http.HandlerFunc(h))
}

// Then wraps an http.Handler with a middleware
// This is a convenience method to allow chaining middlewares
func (m Middleware) Then(h http.Handler) http.Handler {
	return m(h)
}

// Apply applies the middleware to an http.Handler
func (m Middleware) Apply(h http.Handler) http.Handler {
	return m(h)
}

// Chain applies middlewares to an http.Handler
// in the order they are provided
func Chain(mws ...Middleware) Middleware {
	return func(h http.Handler) http.Handler {
		for i := range mws {
			h = mws[len(mws)-1-i](h)
		}
		return h
	}
}

// Append appends a middleware to the chain
func Append(m Middleware, mws ...Middleware) []Middleware {
	return append(mws, m)
}

// HeaderAPIVersion adds the API version to the response headers
func HeaderAPIVersion(version string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if version == "" {
				version = "v1"
			}

			w.Header().Set("X-API-Version", version)
			next.ServeHTTP(w, r)
		})
	}
}

// Logging middleware logs the request and response
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapped := newWrappedResponseWriter(w)

		next.ServeHTTP(wrapped, r)

		slog.Info("request", "method", r.Method, "path", r.URL.Path, "address", r.RemoteAddr, "status", wrapped.status)
	})
}

// OtelTextMapPropagation middleware propagates the OpenTelemetry context
// from incoming requests to outgoing requests
func OtelTextMapPropagation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(
			r.Context(), propagation.HeaderCarrier(r.Header),
		)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// customResponseWriter is a custom response writer that handles custom error responses.
type customResponseWriter struct {
	*wrappedResponseWriter
	method string
	path   string
}

// Write writes the response data.
func (w *customResponseWriter) Write(data []byte) (n int, err error) {
	var apiResponse payload.HTTPMessage

	switch w.status {
	case http.StatusNotFound:
		if err := json.Unmarshal(data, &apiResponse); err != nil {
			data, err = json.Marshal(
				payload.HTTPMessage{
					Timestamp:  time.Now().UTC(),
					StatusCode: http.StatusNotFound,
					Message:    "Not Found",
					Method:     w.method,
					Path:       w.path,
				},
			)
			if err != nil {
				return 0, err
			}
		}

	case http.StatusMethodNotAllowed:
		if err := json.Unmarshal(data, &apiResponse); err != nil {
			data, err = json.Marshal(
				payload.HTTPMessage{
					Timestamp:  time.Now().UTC(),
					StatusCode: http.StatusMethodNotAllowed,
					Message:    "Method Not Allowed",
					Method:     w.method,
					Path:       w.path,
				},
			)
			if err != nil {
				return 0, err
			}
		}
	}

	return w.wrappedResponseWriter.Write(data)
}

// RewriteStandardErrorsAsJSON is a middleware that rewrites standard HTTP errors as JSON responses.
func RewriteStandardErrorsAsJSON(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		newW := &customResponseWriter{
			wrappedResponseWriter: newWrappedResponseWriter(w),
			method:                r.Method,
			path:                  r.URL.Path,
		}

		h.ServeHTTP(newW, r)
	})
}

// CorsOpts is the configuration for the CORS middleware
// Options are:
// AllowedOrigins is a list of origins a cross-domain request can be executed from
// AllowedMethods is a list of methods the client is allowed to use with cross-domain requests
// AllowedHeaders is a list of non-simple headers the client is allowed to use with cross-domain requests
// AllowCredentials indicates whether the request can include user credentials like cookies, HTTP authentication or client side SSL certificates
type CorsOpts struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	AllowCredentials bool
}

// Cors middleware adds CORS headers to the response
func Cors(opts CorsOpts) Middleware {
	// Apply defaults before serving requests
	if len(opts.AllowedOrigins) == 0 {
		opts.AllowedOrigins = []string{"*"}
	}

	if len(opts.AllowedMethods) == 0 {
		opts.AllowedMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions}
	}

	if len(opts.AllowedHeaders) == 0 {
		opts.AllowedHeaders = []string{"Accept", "Content-Type", "Content-Length", "Accept-Encoding", "Authorization"}
	}

	// Per the CORS spec, credentials cannot be used with wildcard origins.
	// If AllowCredentials is true and a wildcard origin is configured, disable credentials.
	hasWildcard := slices.Contains(opts.AllowedOrigins, "*")
	if opts.AllowCredentials && hasWildcard {
		slog.Warn("CORS: AllowCredentials is incompatible with wildcard origin '*', disabling credentials")
		opts.AllowCredentials = false
	}

	methods := strings.Join(opts.AllowedMethods, ", ")
	headers := strings.Join(opts.AllowedHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if hasWildcard {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if slices.Contains(opts.AllowedOrigins, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				// Vary header is required when origin is not a wildcard
				w.Header().Add("Vary", "Origin")
			}

			w.Header().Set("Access-Control-Allow-Methods", methods)
			w.Header().Set("Access-Control-Allow-Headers", headers)

			if opts.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			// Handle CORS preflight requests (OPTIONS)
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// validateTokenType is a function type for token type validation
type validateTokenType func(tokenType string) bool

// RevokedAccessTokens is a generic token checking middleware that validates JWTs
// It extracts the token from the Authorization header, validates it using the provided validator,
// and checks the token_type claim using the provided validation function
// RevokedAccessTokens answers whether an access token has been revoked, from
// memory, with no I/O. See usecase.RevokedAccessTokens.
//
// It is an interface here so the middleware depends on the question, not on the
// mirror that answers it — and so a nil checker is a legible way to say the
// check is switched off.
type RevokedAccessTokens interface {
	Contains(jti uuid.UUID) bool
}

func checkToken(
	validators map[jwtvalidator.ValidatorType]jwtvalidator.Validator,
	validatorType jwtvalidator.ValidatorType,
	validateType validateTokenType,
	errorMsg string,
	revoked RevokedAccessTokens,
) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "Missing header: Authorization")
				return
			}

			// RFC 7235 makes the auth-scheme case-insensitive, so "bearer" and
			// "BEARER" are the same scheme as "Bearer". Matching the prefix
			// literally refused a request that was correctly formed, and the
			// message told the caller to send exactly what they had sent.
			scheme, credentials, found := strings.Cut(authHeader, " ")
			if !found || !strings.EqualFold(scheme, "Bearer") {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "Authorization header must use the Bearer scheme")
				return
			}

			token := strings.TrimSpace(credentials)
			if token == "" {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "Token is empty")
				return
			}

			if validators == nil {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "Unauthorized")
				return
			}

			// check validator has the required type
			validator, ok := validators[validatorType]
			if !ok {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "Unauthorized")
				return
			}

			claims, err := validator.Validate(r.Context(), token)
			if err != nil {
				// One message for every way verification can fail, and it is
				// ours. This used to write err.Error(), which carried the jwt
				// library's own text out to the client: callers received
				// "token is malformed: could not JSON decode header: invalid
				// character '\x9e'" and "crypto/ecdsa: verification error" —
				// a dependency's internals published as part of this API, one
				// upgrade away from silently changing.
				//
				// It is also the one place that decides what a refused caller
				// is told, which is why the validators below return opaque
				// errors rather than trying to phrase this themselves. When a
				// caller needs to tell "expired, go refresh" from "revoked, go
				// log in", that is a deliberate signal to design (Phase 3b),
				// not a library string to leak.
				slog.Debug("checkToken: token refused", "error", err, "path", r.URL.Path)
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "Invalid or expired token")
				return
			}

			if len(claims) == 0 {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "Claims is empty")
				return
			}

			// validate the token_type claim
			var tokenType any
			if tokenType, ok = claims["token_type"]; !ok {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "Token type field not found in claims")
				return
			}

			tokenTypeStr, ok := tokenType.(string)
			if !ok || !validateType(tokenTypeStr) {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, errorMsg)
				return
			}

			// The revocation check.
			//
			// Access tokens only. A refresh token is checked against the store
			// directly on the one endpoint that consumes it — exact, and
			// fail-closed — and a personal access token is governed by its own
			// row, which CheckPATokenActive reads. Adding a mirror to either
			// would substitute a stale answer for an exact one.
			//
			// A missing or unparseable jti on a token that got this far is a
			// token this service did not mint the way it mints them, so it is
			// refused rather than waved through: an access token with no jti
			// would otherwise be an access token that cannot be revoked.
			if revoked != nil && tokenTypeStr == domain.TokenTypeAccess.String() {
				jti, ok := claims["jti"].(string)
				if !ok {
					slog.Warn("checkToken: access token has no jti and therefore cannot be revoked", "path", r.URL.Path)
					respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "Invalid or expired token")

					return
				}

				tokenID, err := uuid.Parse(jti)
				if err != nil {
					slog.Warn("checkToken: access token has an unreadable jti", "path", r.URL.Path)
					respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "Invalid or expired token")

					return
				}

				if revoked.Contains(tokenID) {
					// The one 401 that carries a code, because it is the one a
					// client must not answer with a refresh-and-retry.
					respond.WriteJSONMessageWithCode(w, r, http.StatusUnauthorized,
						domain.CodeTokenRevoked, domain.AuthnTokenRevoked)

					return
				}
			}

			// Add the claims, and the token they came from, to the request
			// context. Both, because a handler that acts on the token itself
			// must act on the one that was actually verified.
			ctx := context.WithValue(r.Context(), JwtClaims, claims)
			ctx = context.WithValue(ctx, JwtToken, token)
			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}

// CheckAccessToken checks the JWTs created and signed by the application
// and validates the token_type claim
// The token_type claim is used to identify the type of token
// and this validates the "access" or "personal_access" token
func CheckAccessToken(validator map[jwtvalidator.ValidatorType]jwtvalidator.Validator, revoked RevokedAccessTokens) Middleware {
	return checkToken(
		validator,
		jwtvalidator.ValidatorTypeAccessToken,
		func(tokenType string) bool {
			return tokenType == domain.TokenTypeAccess.String() || tokenType == domain.TokenTypePersonalAccess.String()
		},
		"invalid token type access or personal_access",
		revoked,
	)
}

// CheckRefreshToken checks the JWTs created and signed by the application
func CheckRefreshToken(validator map[jwtvalidator.ValidatorType]jwtvalidator.Validator) Middleware {
	return checkToken(
		validator,
		jwtvalidator.ValidatorTypeRefreshToken,
		func(tokenType string) bool {
			return tokenType == domain.TokenTypeRefresh.String()
		},
		"Token type is not refresh",
		nil,
	)
}

// CheckVerificationToken checks the JWTs created and signed by the application
// by the application, taken from the Authorization header.
//
// The token used to arrive as a path segment — GET /auth/verify/{token} — which
// wrote a live credential into this service's own request log, and into the
// browser history and Referer of whoever clicked the link. It travels in a
// header now, the way the password-reset token already did.
func CheckVerificationToken(validator map[jwtvalidator.ValidatorType]jwtvalidator.Validator) Middleware {
	return checkToken(
		validator,
		jwtvalidator.ValidatorTypeVerificationToken,
		func(tokenType string) bool {
			return tokenType == domain.TokenTypeEmailVerification.String()
		},
		"Token type is not email_verification",
		nil,
	)
}

func CheckPasswordResetToken(validator map[jwtvalidator.ValidatorType]jwtvalidator.Validator) Middleware {
	return checkToken(
		validator,
		jwtvalidator.ValidatorTypePasswordResetToken,
		func(tokenType string) bool {
			return tokenType == domain.TokenTypePasswordReset.String()
		},
		"Token type is not password_reset",
		nil,
	)
}

// CheckAuthz middleware checks if the user_id (sub) in the JWT is authorized to access the resource
// through the OPA policy engine
func CheckAuthz(authz *usecase.AuthzService) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// get the sub claim from the context
			claims, ok := r.Context().Value(JwtClaims).(map[string]any)
			if !ok {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "claims not found in context")
				return
			}

			subStr, ok := claims["sub"].(string)
			if !ok {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "sub claim not found in claims")
				return
			}

			// sub to uuid
			sub, err := uuid.Parse(subStr)
			if err != nil {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "invalid sub claim")
				return
			}

			ok, err = authz.IsAuthorized(r.Context(), sub, r.Method, r.URL.Path)
			if err != nil {
				slog.Error(
					"authorization service error",
					"error", err,
					"sub", subStr,
					"method", r.Method,
					"path", r.URL.Path,
				)
				respond.WriteJSONMessage(w, r, http.StatusInternalServerError, "authorization service unavailable")
				return
			}

			if !ok {
				respond.WriteJSONMessage(w, r, http.StatusForbidden, fmt.Sprintf("access denied: %s %s", r.Method, r.URL.Path))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// secondsCeil rounds a non-negative duration up to whole seconds. Negative
// durations clamp to zero.
func secondsCeil(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int((d + time.Second - 1) / time.Second)
}

// CheckUserExists is a middleware that verifies the authenticated user exists in the database
// This should be used BEFORE authorization checks for /me endpoints to ensure proper 404 responses
// when a user has a valid JWT token but has been deleted from the database
func CheckUserExists(userService UsersServiceInterface) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Extract user ID from JWT claims (already validated by CheckAccessToken middleware)
			claims, ok := ctx.Value(JwtClaims).(map[string]any)
			if !ok {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "missing or invalid JWT claims")
				return
			}

			// Get the "sub" claim which contains the user ID
			sub, ok := claims["sub"]
			if !ok {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "missing user ID in token claims")
				return
			}

			subStr, ok := sub.(string)
			if !ok {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "invalid user ID format in token")
				return
			}

			userID, err := uuid.Parse(subStr)
			if err != nil {
				respond.WriteJSONMessage(w, r, http.StatusUnauthorized, "invalid user ID in token")
				return
			}

			// Check if user exists in database
			_, err = userService.GetByID(ctx, userID)
			if err != nil {
				if _, ok := errors.AsType[*domain.UserNotFoundError](err); ok {
					respond.WriteJSONMessage(w, r, http.StatusNotFound, "user not found")
					return
				}
				respond.WriteJSONMessage(w, r, http.StatusInternalServerError, "failed to verify user existence")
				return
			}

			// User exists, continue to next middleware
			next.ServeHTTP(w, r)
		})
	}
}

// Recovery middleware recovers from panics in downstream handlers,
// logs the panic, and returns a 500 Internal Server Error response.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error(
					"panic recovered",
					"error", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"remote_addr", r.RemoteAddr,
				)
				respond.WriteJSONMessage(w, r, http.StatusInternalServerError, "internal server error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}
