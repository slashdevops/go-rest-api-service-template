// Package middleware provides composable HTTP middleware used by API handlers.
//
// The package centers around the Middleware type:
//
//	type Middleware func(http.Handler) http.Handler
//
// and helper functions to compose wrappers predictably (Chain, Append, Then,
// ThenFunc, and Apply).
//
// Main responsibilities:
//   - Request identity: RequestID
//   - Panic recovery: Recovery
//   - Response hygiene: SecurityHeaders, RewriteStandardErrorsAsJSON
//   - Transport concerns: Logging, HeaderAPIVersion, OtelTextMapPropagation
//   - Request bounds: MaxBody, RequireJSONBody
//   - Request protection: RateLimit (rule-driven, two-stage)
//   - Cross-origin controls: Cors with CorsOpts
//   - Authentication and authorization: CheckAccessToken,
//     CheckRefreshToken, CheckPasswordResetToken, CheckPATokenActive, and CheckAuthz
//   - Resource existence guard: CheckUserExists
//
// The order the composition root uses, outermost first:
//  0. RequestID -- one id per request, on the response header, the log line
//     and every error body. Above Recovery so a recovered panic has one.
//  1. Recovery -- a panic anywhere below becomes a logged 500, not a dropped
//     connection. It was documented here as outermost for a long time while
//     wired nowhere; internal/app has a test that reads server.go so that
//     cannot happen again.
//  2. SecurityHeaders -- set before anything can write, so an early exit
//     (a 413, a 415, a 429) carries them too.
//  3. RewriteStandardErrorsAsJSON, Logging, HeaderAPIVersion, tracing.
//  4. MaxBody, RequireJSONBody -- the body is bounded and typed before any
//     handler reads it.
//  5. The pre-auth rate limiter, wrapped in its exemptions.
//  6. Cors -- AFTER the limiter, so a preflight is metered; before it, an
//     OPTIONS flood was free.
//  7. Per route: token validation, optional user existence, PA-token
//     liveness, authorization, the post-auth limiter.
//
// Notes:
//   - Token middlewares store validated claims in request context under JwtClaims.
//   - CheckUserExists is useful for /me-style endpoints where a valid token may
//     refer to a deleted user and should return 404.
//   - RewriteStandardErrorsAsJSON rewrites standard plain-text HTTP errors to
//     the project's structured JSON message format.
package middleware

// # The two rate limiters
//
// [RateLimit] and [IPRateLimiter] are MUTUALLY EXCLUSIVE, and the composition
// root keeps them so with an else. The rule limiter subsumes the flag one --
// its fallback IS the flag budget, applied when no rule set has loaded -- so
// running both charges every request twice against the same numbers, halving
// the effective limit for a reason nothing would explain.
//
// [RateLimit] is applied TWICE per protected route, at two stages, and the
// split is forced by where information exists rather than chosen for tidiness:
//
//   - [RateLimitStagePreAuth] runs in the common chain, which wraps the mux, so
//     r.Pattern is not yet the route -- it is the OUTER subtree mount
//     ("/api/v1/"). It resolves the route itself through the inner mux and
//     enforces ip- and global-scoped rules, before any token verification.
//   - [RateLimitStagePostAuth] runs inside a route's chain, where r.Pattern is
//     the route template and the claims exist, and enforces user-, token- and
//     project-scoped rules -- plus any rule whose audience is auth, whatever
//     its scope.
//
// Doing it all post-auth would put JWT verification and an authz lookup in
// front of the limiter, so a flood would cost that work before it could be
// refused. Doing it all pre-auth cannot work: there are no claims there.
//
// Both stages see every matched rule, so a stage filter stops an ip rule being
// charged in both. Without it every ip limit halves, silently, and only on
// routes that have a post-auth chain.
//
// That filter reads AUDIENCE as well as scope, and used not to. Scope alone
// sent every ip and global rule pre-auth, where nobody is authenticated and the
// audience check rejects an auth rule; post-auth the audience matched and the
// scope filter dropped it. An ip or global rule with audience=auth was
// therefore accepted, listed, resolved -- and charged in neither stage.
//
// # Three kinds of "cannot charge this", which are not interchangeable
//
//   - The SHARED counter could not answer. Unknown, never allowed;
//     ratelimit.store.fail.mode decides between refusing and falling back to
//     the per-replica limiter, and the refusal carries its own code.
//   - The RULE could not be built into a limiter. A misconfiguration, not an
//     outage: the bucket is skipped, the store is not implicated, and the fail
//     mode is NOT consulted -- it answers what an unreachable counter means,
//     and this one answered. Conflating the two drove rate_limit_store_up to
//     zero and, under fail-closed, turned one malformed row into a refusal of
//     every request the rule matched.
//   - The rule's SCOPE KEY is absent -- a user rule on an anonymous request.
//     Skipped, because bucketing them together would be one shared budget
//     wearing a per-user label.
//
// # Exemptions belong to neither limiter
//
// [RateLimitExemptions] holds the health/version bypass and the excluded-IP
// allowlist, and [RateLimitExemptions.Wrap] decorates WHICHEVER limiter the
// composition root chose. It is not inside either one, and that is the whole
// point: both exemptions used to live in [RateLimit], which runs only when
// ratelimit.rules.enabled is on -- and it is OFF by default. So in the shipped
// posture /health was rate limited (measured: 1 x 200 then 7 x 429 against a
// 1 req/s limiter) and ratelimit.excluded.ips did nothing at all.
//
// An exempt request SKIPS the limiter rather than being allowed by it, so no
// bucket is touched and no shared counter is consulted. Under
// ratelimit.store.fail.mode=closed that distinction is the difference between a
// readiness probe surviving a Valkey outage and the replica being evicted.
//
// # Address lists
//
// [ParseIPMatchers] is shared by [NewClientIPResolver]'s trusted-proxy list and
// [RateLimitExemptions]' excluded-IP list. Both take CIDR blocks or bare
// addresses, and a malformed entry is an error at construction rather than an
// entry that silently matches nothing -- which is what the excluded list did
// when it compared addresses as strings.
//
// See docs/architecture/rate-limiting.md for the whole picture.
