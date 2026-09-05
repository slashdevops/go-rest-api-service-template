package main

import (
	"context"
	"fmt"
	"log"
	"os"

	_ "net/http/pprof"

	"github.com/slashdevops/go-rest-api-service-template/internal/app"
)

// main is the entry point of the application.
//
//	@title			Go REST API Service Template API
//	@version		v1
//
//	@description	A production-shaped starting point for a Go HTTP REST API: hexagonal architecture, multi-tenant projects, RBAC through Open Policy Agent, database-backed rate limiting and resource limits, and OpenTelemetry traces and metrics throughout.
//	@description
//	@description	`products` is the worked example entity -- a project-scoped CRUD resource that exercises every convention here. Copy it when adding your own; see docs/architecture/adding-an-entity.md.
//	@description
//	@description	### Authentication
//	@description
//	@description	Four bearer credentials, each accepted by a different set of endpoints; see the security definitions below for which is which. Ordinary calls use **AccessToken**.
//	@description
//	@description	A logout revokes the access token it was called with, not only the refresh token, so it stops working immediately. That check can be switched off (`authn.access.token.revocation.enabled`), and where it is, a logged-out access token keeps working until it expires -- which is why the lifetime is short.
//	@description
//	@description	Refresh tokens are rotated: each refresh spends the token it was given and returns a successor, so a client **must** store what `POST /auth/refresh` returns. Presenting a spent one again reads as a replay and ends the session for whoever holds the live token too.
//	@description
//	@description	### Conventions
//	@description
//	@description	- **Identifiers are UUID v7.** Every `id` in this API, without exception.
//	@description	- **List endpoints share one shape**: `limit`, `next_token`, `prev_token`, `sort`, `filter` and `fields`, answering `{ items, paginator }`. Paginate with the tokens rather than an offset.
//	@description	- **Errors are `payload.HTTPMessage`**: a message, the method and path, and the status code. Some carry a machine-readable `code`; match on that and never on the prose, which may be reworded.
//	@description	- **Timestamps are RFC 3339**, and every entity carries `created_at` and `updated_at`.
//	@description
//	@description	### Rate limiting
//	@description
//	@description	Every endpoint may answer `429`, and the two reasons are not interchangeable:
//	@description
//	@description	- `RATE_LIMIT_EXCEEDED` — a budget was spent. `Retry-After` says when to come back.
//	@description	- `RATE_LIMIT_UNAVAILABLE` — the limiter's own shared counter is unreachable and the service is failing closed. Nobody is being limited *correctly*; this is an operator condition, not a signal to the caller to slow down.
//	@description
//	@description	`/health/*` and `/version` bypass the limiter, so an outage of it never makes the service look unreachable.
//	@description
//	@description	### Operations
//	@description
//	@description				- `GET /health/live` — liveness. Checks nothing else on purpose: restarting cannot fix a dependency that is down.
//	@description				- `GET /health/detailed` — readiness, per component. `200` healthy, `206` degraded, `503` a hard dependency is gone.
//	@description				- `GET /health/status` — a public, deliberately thin verdict for anyone who cannot present a token.
//	@description				- `GET /version` — the build this instance was made from.
//
//	@contact.name				API Support
//	@contact.url				https://goapitemplate.local
//	@contact.email				info@goapitemplate.local
//
//	@externalDocs.description	Architecture and operational documentation
//	@externalDocs.url			https://github.com/slashdevops/go-rest-api-service-template/tree/main/docs
//
// host, basePath and schemes are deliberately absent here: they are deployment
// facts, not source facts, and app.configureSwaggerMetadata sets all three at
// startup from configuration. Hardcoding them would bake one environment's
// address into every build.

//	@securityDefinitions.apikey	AccessToken
//	@in							header
//	@name						Authorization
//	@description				The ordinary credential: `Bearer <access_token>`, from `POST /auth/login` or `POST /auth/refresh`. Every endpoint outside `/auth`, `/health` and `/version` takes it. A personal access token is presented the same way and is accepted anywhere an access token is -- which is how a script authenticates without a login. Deliberately short-lived; refresh rather than lengthening it.

//	@securityDefinitions.apikey	RefreshToken
//	@in							header
//	@name						Authorization
//	@description				`Bearer <refresh_token>`, and accepted **only** by `POST /auth/refresh` and `DELETE /auth/logout`. Refreshing spends it and returns a successor: store what comes back. Presenting a spent one is treated as a replay and ends the whole session, including for whoever holds the live token.

//	@securityDefinitions.apikey	ResetPasswordToken
//	@in							header
//	@name						Authorization
//	@description				`Bearer <reset_password_token>`, accepted only by `POST /auth/password/reset`. Issued by `POST /auth/password/recover` and delivered by email, single use.

//	@securityDefinitions.apikey	VerificationToken
//	@in							header
//	@name						Authorization
//	@description				`Bearer <verification_token>`, accepted only by `POST /auth/verify/confirm`. It proves an email address rather than exercising a permission, so it is the one credential no authorization check is applied to -- the account it belongs to is still disabled at that point. It never travels in a URL: the emailed link points at the frontend, which presents the token in this header.

//	@tag.name			Authentication
//	@tag.description	Sign in, sign out, refresh, register, verify an address and recover a password. Also the identity-provider registry and the OAuth flows through it.

//	@tag.name			Me
//	@tag.description	The calling user's own account, effective permissions and resource limits. Everything here is scoped to whoever holds the token.

//	@tag.name			Products
//	@tag.description	The worked example entity: a project-scoped CRUD resource. Copy it when adding your own.
//
//	@tag.name			Projects
//	@tag.description	The tenant boundary. Every product, configuration and limit hangs off a project, and most paths in this API begin with one.

//	@tag.name			Users
//	@tag.description	Accounts, and their links to roles and projects. Both directions of every link are available.

//	@tag.name			Roles
//	@tag.description	Named bundles of policies, and the users they are granted to.

//	@tag.name			Policies
//	@tag.description	What a role may do, expressed as actions over resources. A policy names resources from the catalogue below.

//	@tag.name			Resources
//	@tag.description	The endpoint catalogue policies are written against -- one entry per operation in this API, generated from these annotations.

//	@tag.name			Resources Limits
//	@tag.description	How much a scope may create, and how much of it already exists. Read-only: limits are not editable through the API.

//	@tag.name			RateLimits
//	@tag.description	The rate-limit rules, and a preview of which would actually apply to a given method and endpoint.

//	@tag.name			Health
//	@tag.description	Liveness, readiness and a public status summary. All of it bypasses the rate limiter.

//	@tag.name			Authorization
//	@tag.description	What a caller may do, resolved. `/me/authz` answers it for the caller and `/users/{id}/authz` for anyone else; both return the permission set roles and policies add up to, rather than the roles and policies themselves.

//	@tag.name			Identity Providers (IDPs)
//	@tag.description	External OAuth providers, and the login and registration flows through them. The callback is browser-driven and answers a redirect, not JSON.

//	@tag.name			Identity Provider Types
//	@tag.description	The provider kinds an IdP can be configured as. Reference data.

//	@tag.name			Version
//	@tag.description	The build this instance was made from. Public, and the quickest way to tell which deployment answered.

// main function
func main() {
	ctx := context.Background()

	// Initialize application
	// Note: configuration loading happens inside NewApp, which sets up the logger
	application, err := app.NewApp(ctx)
	if err != nil {
		// Use fmt.Fprintf for errors before logger is initialized
		fmt.Fprintf(os.Stderr, "Failed to initialize application: %v\n", err)
		os.Exit(1)
	}

	// Run application
	if err := application.Run(ctx); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}
