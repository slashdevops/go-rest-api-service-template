# Development Guidelines

This document contains the critical information about working with the project codebase.
Follows these guidelines precisely to ensure consistency and maintainability of the code.

> This file is the single source of truth for agent instructions. `CLAUDE.md`,
> `AGENTS.md`, `DEVELOPMENT_GUIDELINES.md`, `.cursorrules` and `.clinerules` are
> all symlinks to it — edit this file, never the symlinks.

## What this service is

`go-rest-api-service-template` is a **template**: a production-shaped starting
point for a Go HTTP REST API, meant to be cloned and cut down rather than
deployed as-is. Everything in it is real and works; nothing in it is a
placeholder.

What it ships:

| Area | What is here |
| --- | --- |
| **Architecture** | Hexagonal (Ports & Adapters), with the invariant enforced by a test |
| **Multi-tenancy** | Projects as the tenant boundary, with membership enforced in SQL |
| **Authentication** | JWT access + refresh with rotation, revocation denylist, login throttle, OAuth/OIDC IdPs |
| **Authorization** | RBAC through Open Policy Agent, with the resource catalogue generated from the OpenAPI spec |
| **Limits** | Database-backed rate limiting, and soft/hard resource limits resolved by scope |
| **Observability** | OpenTelemetry traces and metrics, Grafana/Tempo/Prometheus dev stack, alert rules with tests |

### `products` is the worked example

`products` is the entity to copy when adding your own. It is deliberately the
simplest thing that is still realistic — a project-scoped row with a name and a
description — and it exercises every convention in the repository:

| Layer | File |
| --- | --- |
| Domain + errors | `internal/core/domain/products{,_errors}.go` |
| Ports | `internal/core/port/driving/products.go`, `internal/core/port/driven/repository/products.go` |
| Use-case | `internal/core/usecase/products.go` |
| Repository | `internal/adapter/driven/repositorypg/products.go` |
| Handler + payload | `internal/adapter/driving/http/{handler,payload}/products.go` |
| Mock | `mocks/service/products.go` (generated) |
| Migration | `database/migrations/00009_products_tables.sql` |
| Composition root | `internal/app/{dependencies,repositories,services,handlers,server}.go` |
| Integration test | `tests/integration/api_products_test.go` |

Two things about it are worth reading before you copy it:

- **Its repository carries a project-membership predicate in the same statement
  as the row it guards.** OPA authorises the *path*, so a policy granting
  `/projects/*/products` matches every project. Without that predicate a caller
  could read and write a project they were never added to. Done as a separate
  `SELECT` it would be a TOCTOU window; done in the use-case it would have to be
  repeated at six call sites and forgotten at the seventh.
  `TestProductsProjectIsolation` fails if it is dropped.
- **It is deliberately not cached.** Its read is a function of the *caller* as
  well as the row, which leaves no key that is both tenant-safe and
  invalidatable. Any entity whose read is tenant-scoped in SQL inherits this;
  see `docs/architecture/caching.md`.

## Stack

- Language: Go (Go 1.27+)
- Framework: Go standard library
- Testing: Go's built-in testing package
- Build Tool: `make` using the Makefile with all the targets defined to build, test, and run the application
- Live reloading: `air` for development convenience
- Dependency Management: Go modules
- Version Control: Git
- Documentation: GoDoc
- Code Review: Pull requests on GitHub
- CI/CD: GitHub Actions
- Database: PostgreSQL 18 (for native `uuidv7()`; no extension required)
- Database Migrations: `goose` for schema migrations
- Logging: `slog` package from the standard library
- Containerization: `podman` for local development and testing
- API Documentation: Swagger/OpenAPI for API endpoints
- uuid version: The project is using UUID version 7 for generating unique identifiers.
- uuid generation tool: There is a command line tool `uuidgen` in the `cmd/uuidgen` package to generate UUIDs. you can use it executing `go run cmd/uuidgen/main.go -n 1 -v 7` to generate a new UUID. Where `-n` is the number of UUIDs to generate and `-v` is the UUID version. The generated UUIDs are in version 7 format, which is a time-based UUID format that provides better sorting and uniqueness guarantees.

## Project Structure

This project follows a **Hexagonal (Ports & Adapters)** architecture. The
hard invariant: **nothing under `internal/core/` may import infrastructure**
(HTTP, database, cache, mail, OPA, JWT, OTEL, ...). The
`TestCoreHasNoInfraImports` arch-test in `internal/core/arch_test.go`
runs as part of the regular suite and breaks the build if the
invariant is violated.

```
internal/
├── adapter/              ── infrastructure
│   ├── driven/           ── outbound: cache, mail, cipher, policy, persistence,
│   │   │                    OAuth, JWT signer, rate limiters, login throttle
│   │   ├── cachevalkey/         (wraps github.com/slashdevops/c3e + valkey-io)
│   │   ├── cipheraes/           (AES-GCM symmetric encryption)
│   │   ├── notifieremail/       (mailer + templates/ helper)
│   │   ├── oauthidp/            (golang.org/x/oauth2 + IDP UserInfo)
│   │   ├── policyopa/           (OPA Rego eval; embeds rego/ bundle)
│   │   ├── ratelimitbreaker/    (circuit breaker in front of the shared store)
│   │   ├── ratelimitmemory/     (per-replica token/leaky bucket)
│   │   ├── ratelimitvalkey/     (shared fixed-window counter)
│   │   ├── repositorypg/        (pgx-backed concrete repositories)
│   │   ├── throttlememory/      (per-identity login throttle)
│   │   └── tokenjwt/            (golang-jwt sign/verify)
│   └── driving/          ── inbound: HTTP transport
│       └── http/
│           ├── payload/          (request/response shapes — wire payloads)
│           ├── handler/          (route handlers; depend only on driving ports)
│           ├── jwtvalidator/     (incoming-token middleware helper)
│           ├── middleware/       (auth, CORS, rate limit, logging, ...)
│           ├── respond/          (JSON/error response helpers)
│           └── server/           (HTTP server lifecycle)
├── app/                  ── composition root: builds adapters, wires ports into use-cases
├── config/               ── flag/env loader
├── core/                 ── pure: no infra imports allowed
│   ├── domain/                   (entities, errors, validation, helpers)
│   ├── port/
│   │   ├── driven/               (interfaces use-cases consume)
│   │   │   ├── cache, cipher, notifier, oauth, policy, ratelimit,
│   │   │   ├── repository, throttle, token
│   │   └── driving/              (interfaces driving adapters consume)
│   └── usecase/                  (the business logic)
├── o11y/                 ── OTEL setup + helpers (cross-cutting infra)
└── version/              ── build metadata
```

### Where new code goes

| Adding a...                                        | Lives in                                                                                                                            |
| -------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| Domain entity, validation rule, business error     | `internal/core/domain/`                                                                                                             |
| New use-case method                                | `internal/core/usecase/<entity>.go`                                                                                                 |
| New HTTP route                                     | `internal/adapter/driving/http/handler/<entity>.go` (define/extend the matching `internal/core/port/driving/<entity>.go` interface) |
| New repository method                              | `internal/adapter/driven/repositorypg/<entity>.go` (and add to the `internal/core/port/driven/repository/<entity>.go` interface)    |
| New outbound integration (e.g. SMS, S3, an LLM provider) | Define a port in `internal/core/port/driven/<concept>/` and an adapter in `internal/adapter/driven/<concept>_<tech>/`               |
| Database migration                                 | `database/migrations/` (managed by `goose`) — **the next number in the sequence, never a gap**, see below                          |
| Mock for a port or service interface               | Add a `//go:generate go tool mockgen` stanza to the file declaring the interface; output to `mocks/{service,handler}/<entity>.go`   |

### Hard rules

- **Use-cases (`internal/core/usecase/`) never import adapters.** They depend on ports — `repository.Users`, `cache.Cache`, `oauth.Provider`, etc. The composition root in `internal/app/` does the wiring.
- **Handlers (`internal/adapter/driving/http/handler/`) never import use-cases directly.** They depend on driving ports — `driving.Authn`, `driving.Users`, etc. Same composition-root wiring.
- **Domain types are pure.** They may have struct tags (json, swagger), but they never import a transport package.
- **When generating handlers/code, use `uuidgen` for V7 IDs**: `go run cmd/uuidgen/main.go -n 1 -v 7`.
- **A handler that exists is registered.** The composition root is the one
  place an entity can be forgotten without a compile error, and `products`
  was: every layer existed and every route answered `404`, with every gate
  green, because no gate looks at `internal/app` and CI never runs the
  integration suite. `TestEveryHandlerIsRegistered` now fails for any field of
  `app.Handlers` with no `RegisterRoutes` call in `server.go`.

## Configuration

**Every setting's env var is the flag with dots replaced by underscores,
uppercased.** `http.server.*` used to be the one exception, mapping to
`SERVER_*` while its sibling `http.client.*` mapped to `HTTP_CLIENT_*`; it is
`HTTP_SERVER_*` now. Keep the rule mechanical — an operator should never have to
look one up.

**A switch is `.enabled`, never `.enable`.** A wrong guess is not a warning, it
is a stopped process: `flag provided but not defined: -database.migration.enabled`.

**A default is written twice** — in `NewField` (the env path) and in
`setupFlags` (the flag path) — and nothing connects them.
`TestNoTwoSettingsShareADefaultConstant` catches the mistake that follows:
borrowing a neighbouring field's constant, so the same setting answers
differently depending on how it was set. That had already happened to
`http.client.max.idle.conns.per.host`, invisibly, because both constants were
`100`.

**Validation must key on the setting that selects the behaviour.**
`MailConfig.Validate` used to require "either `mail.smtp.host` or
`mail.api.url`", which accepted a configuration the service could not run:
setting only the API URL passed, and startup then failed with `SMTPHost must be
between 1 and 255 characters` — an error naming a setting the operator
deliberately did not set. It keys on `mail.sender` now, so the message names the
setting to fix.

**A mail TRANSPORT belongs in `slashdevops/mailer`, not here.** `MailerMailgun`
was added there (v1.1.0) rather than in `notifieremail`, for the same reason the
rate limiter has one source of budgets: a second copy is a second thing to keep
in step.

**`log.level` accepts `ctrace` and `cfatal`** beyond the four it advertises, and
**`.air.toml` runs on `ctrace`**. Both are named in `ValidLogLevelHidden` and in
the flag help, so the dev stack no longer uses a value the help calls invalid.

## Rate limiting

There is **one** limiter, and the `rate_limits` table is the only source of
budgets. `ratelimit.enabled` (default `true`) is the only switch.

There used to be two, mutually exclusive by an `else`: a rule limiter and a
per-IP flag limiter (`http.server.ip.rate.limiter.*`) whose budget was *also*
the rule limiter's fallback floor. That arrangement caused two bugs on its own,
both measured live, and both invisible to every test:

- the exemptions lived in the limiter that did **not** run by default, so
  `/health` answered `429` and `ratelimit.excluded.ips` did nothing;
- `http.server.ip.rate.limiter.enabled=false` disabled nothing, because the
  branch reading it was unreachable whenever rules were on.

**The claim that the flags "cover the database being unreachable" was false.**
The service calls `Ping` at init and exits if it fails, so an unreachable
database means no replica is running to limit. What the floor actually covered
was the race between the HTTP server starting and the mirror's first load —
which is now closed by making that **first load synchronous and fatal**. The
invariant that replaces the floor: *if the service is serving, it has rules.*

**Do not reintroduce a fallback budget.** One number in two places is what let
the seeded row and the flags drift apart, and a floor nobody can see is worse
than an alert nobody has silenced: an empty rule set now means nothing is
limited, and `RateLimitNoRulesConfigured` says so.

The seeded default rule stays `system = FALSE` (editable). `system = TRUE` would
be permanently un-tunable, because the shared trigger refuses UPDATE as well as
DELETE — and it is now the *only* default, so it must be editable.

**`run.sh` and `.air.toml` set `ratelimit.enabled=true`**, which is also the
shipped default, so the dev stack exercises what is deployed.
`TestRunScriptAndAirAgree` fails if the two drift. The integration suite is the
one place that turns it off: it fires many requests from one address, and a
limit would make failures depend on test order.

The full picture — the precedence ladder, the two enforcement layers, every
failure mode — is in
[`docs/architecture/rate-limiting.md`](../docs/architecture/rate-limiting.md).
What follows is only what will bite while editing this code.

### Resolution returns one rule per SCOPE, not one winner

An `ip` rule bounds a source and a `project` rule bounds a tenant; both apply and
neither substitutes for the other. Collapsing them silently drops whichever bound
the operator was not thinking about.

Within a scope, kind dominates verb: an endpoint rule with `*` beats a prefix
rule naming a verb. Ties break by **name** — arbitrary, but *stable*, because a
winner that changes between reloads makes a limit appear to flap with no
diagnosable cause.

`HEAD` is covered by a `GET` rule. Without that, a `HEAD` request slips past
every rule written for the endpoint it hits.

### The pre-auth stage must NOT trust `r.Pattern`

The API mux is mounted on an outer router as a subtree, so by the time the
pre-auth stage runs `r.Pattern` is **already set to `/api/v1/`** — the mount
point, not the route. Trusting it makes every request look identical, so no
endpoint or prefix rule can ever match and the global rule silently wins.

Measured before the fix: a 5/minute rule on `/models` had no effect and the
response carried the global rule's headers. **Every unit test passed** — none set
`r.Pattern` to an outer mount. `pattern()` branches on the STAGE for this reason;
do not "simplify" it back to a nil check.

### A rule that cannot be enforced is dropped, and says so

Three states get confused here, and each was confused once:

- **A broken rule is not a store fault.** The per-replica limiter does no I/O,
  so the only error it can return is a rule it cannot build a limiter from.
  `charge` treated every error as the shared counter, so one malformed row drove
  `rate_limit_store_up` to zero, fired `RateLimitStoreDown` against a healthy
  Valkey, and under the default fail-closed refused every request the rule
  matched. Rule faults now carry `rate_limit_rule_faults_total` and their own
  alert, and **the fail mode is not consulted** — it answers what an
  *unreachable* counter means, and this one answered.
- **A broken rule must not refuse traffic.** It is a misconfiguration, not an
  availability incident. Refusing turns an operator's typo into an outage of the
  endpoint they were protecting. Dropping falls through to the next tier and to
  the flag floor, so traffic stays bounded by something.
- **A broken rule must not SHADOW a working one.** Resolution picks one winner
  *per scope* on specificity, so a malformed endpoint rule outranks a working
  global rule — letting it into resolution switches off a limit rather than
  adding none. `usecase.EnforceableRateLimits` filters first, and **every**
  caller that resolves uses it. Two call sites filtering differently is how
  `/rate_limits/effective` came to disagree with what was being enforced.

### The stage filter reads audience, not just scope

`ip` and `global` are decided pre-auth; `user`, `token` and `project` post-auth.
But audience cuts the other way, and the filter used to ignore it — so an `ip`
or `global` rule with `audience = auth` was rejected pre-auth (nobody is
authenticated before `CheckAccessToken`) *and* post-auth (wrong scope for the
stage). Accepted, listed, resolved, and charged nowhere.

An `auth` rule is charged post-auth whatever its scope. That is not a widening:
the pre-auth stage cannot match an `auth` rule at all, so nothing is charged
twice.

### The excluded-IP list takes CIDR, like every other address list

`ratelimit.excluded.ips` compared the resolved address to a map key as a
**string**, so a CIDR block was a key nothing could equal — accepted, visible,
exempting nobody. It shares `middleware.ParseIPMatchers` with
`http.server.trusted.proxies` now, and a malformed entry stops startup. Two
lists of the same kind of thing must accept the same syntax.

### The breaker changes the waiting, never the answer

`ratelimit.store.breaker.*` stops a failing shared counter being asked again on
every request. It returns an **error** while open, never "allowed" — so
`ratelimit.store.fail.mode` still decides what a fault means. Measured against a paused
Valkey: 93.1 ms per request without it, 24.1 ms with it.

**Its benefit depends on how the store fails.** A *stopped* Valkey refuses
instantly, so there is no timeout to save; the saving is real when the store
*hangs*. Do not quote the number without that caveat — the first measurement of
this feature showed no benefit at all, because it was taken against a stopped
container.

Half-open admits exactly **one** probe: closing optimistically sends the whole
waiting load back at a store that may still be down.

### A bucket is keyed on the window's PARAMETERS, never its id

`PUT` replaces a rule's window set wholesale and mints fresh uuids, so a bucket
keyed on the window id was reset by **any** edit — renaming a rule handed every
caller their full allowance back. Measured live: spend 4 of 6, edit the
description, and all of the next 4 were admitted. That is the trap PocketBase
falls into, reached by a different route.

The adapter's guard could not see it — it keeps a live bucket for an unchanged
budget under the same **key**, and the key was what changed. Keying on
`period:requests:burst` makes the documented intent true: rebuilt when, and only
when, the numbers actually change.

### Cross-replica notification is a SIGNAL, and the ticker is still the floor

A rule write publishes to a Valkey channel and every other replica reloads.
Proven with two replicas and a 10-minute reload interval: the write appeared and
was enforced on the other replica in under a second.

**Never put the rules in the payload.** A message says only "something changed",
so a lost one costs a delay and a duplicate costs a query. Rules in the message
would make delivery order load-bearing, and pub/sub offers no order across a
reconnect.

**Everything about it may fail** — no cache, a failed publish, a dropped
subscription — and the only consequence must be a change taking up to
`ratelimit.reload.interval`, which is the behaviour before it existed.

**The notifier needs its OWN Valkey client**: a subscribed connection accepts
nothing else, so sharing the counter's would stall every `INCR`.

### The valkey tests need a reachable Valkey, or they SKIP — and a SKIP reports "ok"

`ratelimitvalkey`'s tests connect to `127.0.0.1:6379` and skip when the ping
fails. **`make test` now sets `VALKEY_TEST_CA` for you** from
`certs/dev/ca.crt` when the dev stack has generated one, so the TLS-only dev
Valkey is reachable without remembering anything. Run a single package by hand
with an ABSOLUTE path, because `go test` runs in the package directory:

```bash
VALKEY_TEST_CA="$PWD/certs/dev/ca.crt" go test -tags=unit ./internal/adapter/driven/ratelimitvalkey/
```

Without the variable the client speaks plaintext, which is what **CI** uses: it
has no dev certs, so `pr.yaml` gives it a plaintext Valkey service container
instead of a certificate to generate and keep in step. Both shapes are
supported; which one runs depends on whether that CA file exists.

**A skipped test reports `ok`, and that is the whole hazard.** A suite that
verified nothing is indistinguishable from one that passed:

- a mutation test against a skipping suite reports "ok" and proves nothing —
  that happened while writing the notifier, twice;
- and the same silence broke the coverage gate for everyone. The package
  carries an 80% floor measured at 84.3% **with** a Valkey; skipping, it
  measures 17.6%, so `make test-coverage` failed on `main` on a package that
  had nothing wrong with it. The floor described an environment the gate never
  reproduced.

The rule that follows: **when a test can skip itself, something has to
guarantee the conditions it needs.** Documenting the variable was not enough —
a step you have to remember is a step that gets skipped, and this one is
silent.

### A store fault is UNKNOWN, never "allowed"

`ratelimit.store.fail.mode` decides what to do about it — `closed` refuses,
`local` falls back to the per-replica limiter — but the *port* never reports a
fault as an admitted request. Same reasoning as the token denylist: treating an
unreachable store as an empty one removes the limit exactly when the system is
least healthy.

A store-fault `429` carries `RATE_LIMIT_UNAVAILABLE`, not the ordinary budget
code. "Slow down" and "the rate limiter is broken and nobody is being limited
correctly" need different responses from whoever sees them.

**Health and version bypass the limiter in code**, before any rule lookup —
otherwise a Valkey outage with fail-closed evicts the replica from the load
balancer.

**The bypass and `ratelimit.excluded.ips` sit OUTSIDE both limiters.** They lived
inside the rule limiter, which ran only when rules were enabled —
and they were off by default. So in the shipped posture neither existed: `/health`
answered `429` under load and the excluded-IP list did nothing.
`middleware.RateLimitExemptions.Wrap` now decorates whichever limiter the
composition root chose, and `TestEveryLimiterRegistrationGoesThroughTheExemptionGate`
fails if a new registration skips it. An exempt request **skips** the limiter
rather than being allowed by it — under fail-closed, a request that still has to
ask an unreachable store is not exempt in the way that matters.

### The shared counter is a fixed window, deliberately

`INCR` + `PEXPIRE`, no Lua. The window index comes from the wall clock, so
replicas agree without coordinating. A fixed window admits up to 2N across a
boundary — and the per-replica token bucket in front of it is what smooths that.
**The layering is the answer to the fixed window's weakness**; removing either
half makes the other's trade worse.

`PEXPIRE` runs only on the first increment. Not because the window would
otherwise never roll — it rolls because the *key* changes — but because
re-pushing the TTL keeps every past window's key resident under load.

### Both strategies admit identically at equal parameters

Measured, and now tested —
`ratelimitmemory.TestBothStrategiesAdmitIdenticallyAtEqualParameters`.
`token_bucket` and `leaky_bucket` are duals.

**Do not write a test asserting that one paces where the other bursts.** It
cannot exist, and a test that appears to show it is measuring something else:
the first attempt varied the *burst* and passed with the strategy hardcoded. The column
records which question the operator was asking, and the UI must present it as
**budget vs pace** — never "bursty vs smooth", which would sell a difference that
does not exist. `token_bucket` is the default, and that default is what keeps
every existing rule behaving as it does today.

### Alert on the gauge, not the rate

`rate_limit_store_up` and `rate_limit_rules_staleness_seconds` are what to alert
on. With fail-local a sustained store fault is invisible — every request
succeeds, and each replica quietly enforces N × the limit. A fault *rate* that is
high but steady reads as a plateau; a gauge pinned at zero does not.

`rate_limit_rules_staleness_seconds` reports **`-1`** when the set has never
loaded. Zero would read as perfectly fresh.

## Client IP and the rate limiter

The per-IP rate limiter keys on [`ClientIPResolver`](../internal/adapter/driving/http/middleware/clientip.go),
which honours `X-Forwarded-For` / `X-Real-IP` **only when the peer is a
configured trusted proxy**. This is a security boundary, not a convenience:

- **`http.server.trusted.proxies` is empty by default**, and empty means the
  headers are ignored entirely and the bucket is keyed on `RemoteAddr`. A
  deployment behind a proxy must set it to the proxy's IPs or CIDR blocks, or
  every client behind that proxy shares one bucket.
- **Never read a forwarding header without checking the peer.** The resolver
  exists because the old code read `X-Forwarded-For` unconditionally: a caller
  rotating the header drew a fresh budget on every request, so the limiter did
  not weaken, it disappeared. Measured against the running API with the limiter
  at 5 req/s, 30 password guesses went from `{401: 7, 429: 23}` to `{401: 30}`.
- The chain is walked **right to left**, returning the first hop that is not
  itself trusted — a trusted proxy appends what it saw, so everything to the
  left of our own hops is client-supplied. An unreadable chain falls back to the
  peer, which over-limits rather than trusting a guess.
- The startup log states which posture is active, and warns when nothing is
  trusted. Keep that: neither posture is visible from a request, and they fail
  in opposite directions.

`make test` covers this in
`middleware/clientip_test.go` and `middleware/ratelimit_clientip_test.go`;
both were verified to fail when the peer check is removed.

**The limiter is on in `.air.toml`** (100 req/s, burst 300) so the dev stack
matches the shipped default. It was previously disabled there, which is why the
bypass was invisible locally. Two full integration runs see zero 429s at those
values — if you lower them, re-check.

## HTTP server timeouts

`driving/http/server/http.go` shipped for a long time with an `http.Server`
built from only `Addr` and `Handler`. Go applies **no header deadline of its
own**, so that left no bound at all on a Slowloris-style header trickle. All
five bounds are now configurable, and the defaults are the whole point:

| Setting                             | Default   | Why                                                                     |
| ----------------------------------- | --------- | ----------------------------------------------------------------------- |
| `http.server.read.header.timeout`   | `10s`     | the Slowloris bound; covers only the header read, never the handler      |
| `http.server.idle.timeout`          | `120s`    | reaps idle keep-alives; cannot interrupt a request in flight             |
| `http.server.max.header.bytes`      | `1 MiB`   | Go's own default, stated so it is tunable without a code change          |
| `http.server.read.timeout`          | **`0`**   | disabled — bulk ingest uploads an unbounded body                         |
| `http.server.write.timeout`         | **`0`**   | disabled — see below, this one will cut off real work                    |

**Do not "harden" the last two by switching them on.** `WriteTimeout` starts
when the request headers are read and covers the entire handler, so it caps
_total request duration_. A generation call is bounded by `http.client.timeout`
(120s) but may be retried `http.client.max.retries` (10) times, so a legitimate
request can run for roughly twenty minutes — a write deadline would abort
exactly the request an outbound integration exists to serve. `ReadTimeout` is
milder but still bounds how long a client may take to upload, which a bulk
endpoint accepting an arbitrary amount of text would feel first.

`ReadHeaderTimeout` is the one that closes the actual hole, and it is on by
default. The server logs a warning at startup if `WriteTimeout` is set, so an
operator sees the trade-off rather than discovering it as a truncated response.

Config lives in `config/httpserver.go`; the wiring is guarded by
`server.TestNewHTTPServerAppliesTimeouts`, which fails if a field stops reaching
the `http.Server` — the exact regression that let this ship unbounded. The full
reasoning, with the request-lifecycle diagram, is in
[`docs/architecture/http-server-timeouts.md`](../docs/architecture/http-server-timeouts.md).

## SQL rules (repository layer)

- **Always use `$n` placeholders for values.** Never `fmt.Sprintf` a value into a
  query. `repositorypg/roles.go` `UpdateByID` is the worked example, and its
  comment records the reachable injection it used to carry: role-name validation
  forbids control characters, HTML and null bytes but permits an apostrophe, so
  `x', description='injected` closed the literal and appended an
  attacker-controlled assignment. It was the only repository in the package not
  already using placeholders.
- **Identifiers that must be interpolated** (a schema or table name computed at
  runtime) go through `pgx.Identifier{schema, table}.Sanitize()` — never
  `strings.ReplaceAll` and never `fmt.Sprintf`. The hazard is less any single
  rule than two files in one package handling the same value under two different
  ones.
- **Values that cannot be placeholders** (a distance operator, a sort direction)
  must be validated against an allow-list before interpolation. The operator
  comes from `emb_vectors_functions.operator`, a column constrained only by NOT
  NULL and UNIQUE, so nothing in the database restricts it to a real operator.
- Neither of the two above is exploitable *today* — table names are derived from
  UUIDs and the operator rows are seeded — and that is exactly why both are
  enforced in code rather than argued about. "Safe because of where the value
  happens to come from" stops being true the first time someone adds a row.
- 22 files in the repository layer still import `html/template` to build SQL and
  use `template.HTML(...)` to defeat escaping. That is a historical wart, not a
  pattern to follow — new query builders should use `text/template` plus explicit
  sanitisation.

## Database migrations

The migration set was consolidated into a first-version schema: the increments
that had accumulated on top of it were folded back into the files they amended,
and the survivors were then **renumbered contiguously**, `00001` to `00016`, in
the order goose applies them. The old numbering (`200`, `1000`, … `20004`) left
gaps "to slot a file between two topics", and that invitation was the bug: a
file slotted below the highest applied version is a missing migration and stops
the service. Both steps were only possible because nothing is in production.
**From the first production deploy, migrations are additive again** and every
rule below about ordering applies without exception.

- **A new migration takes the next number, never a gap.** The service runs
  `goose.UpContext` with no `AllowMissing`, so a file numbered below the current
  DB version is rejected as a missing migration and **startup fails**. Whatever
  the topic, the new file goes at the end:
  `goose -dir database/migrations -s create <name> sql` writes it with the
  right number.
- **Renumbering means every existing database is recreated.** goose stores the
  version number, so a database at `20004` sees every file below it and refuses
  to start the service: `make rm-dev-env && make start-dev-env` after pulling
  the renumbering, and never renumber again once real data exists.
- **Editing an applied migration fails silently, it does not error.** goose
  tracks versions by number and does **not** checksum file contents, so an
  existing database neither re-runs a rewritten file nor notices a deleted one —
  it simply diverges from what the files say. That is why the consolidation
  required every developer to run `make rm-dev-env && make start-dev-env`, and
  why it cannot be repeated once real data exists.
- **Seed data is insert-only.** System rows (`system = TRUE`) are guarded by a
  `tr_restrict_delete_update_on_system_<table>` trigger that rejects **both**
  UPDATE and DELETE — so a down migration cannot clear the flag first. It must
  `ALTER TABLE … DISABLE TRIGGER`, delete, then re-enable.
- **All twenty of those triggers share one function**,
  `fn_restrict_delete_update_on_system()` in `00001_shared_functions.sql`, which
  names the table with `TG_TABLE_NAME`. Do not add a per-table copy; attach a
  trigger to the shared function. It is `00001` so it is created before the
  first table that needs it and dropped after the last — a shared object must
  sort below everything that depends on it, because goose applies Down in
  descending order.
- **Keep every identifier at 63 characters or fewer.** Postgres truncates
  silently, so a longer name means the name in the file is not the name in the
  database. That has bitten twice: a down migration had to hardcode a truncated
  trigger name, and `repositorypg` matched a constraint on its truncation to map
  a duplicate to 409. The schema currently has zero 63-character identifiers —
  `SELECT relname FROM pg_class WHERE relnamespace = 'public'::regnamespace AND
  length(relname) = 63` should stay empty.
- **A constraint name referenced from Go is a contract.** `handlePgError` in
  `repositorypg/embedding_config_indexes.go` turns a `23505` into a 409 by
  matching one; renaming it in the migration without updating the constant turns
  a documented 409 back into a 500.
- Generate ids with `go run cmd/uuidgen/main.go -n 1 -v 7`.
- **Verify both directions against a live DB before committing** — `goose up`,
  `goose down-to 0`, `goose up` — and check nothing is left behind: no tables, no
  functions, no enum types beyond `goose_db_version`. Down migrations used to be
  fiction here (empty bodies that reported OK, a `DROP EXTENSION IF NOT EXISTS`,
  a `DELETE FROM user`, a table dropped before the one referencing it, an enum
  never dropped so a second `up` failed). They work now; keep them working.
- **Do not add speculative indexes.** A `CREATE INDEX` on the `id` or a UNIQUE
  column duplicates the index the constraint already builds, and one on
  `created_at`/`updated_at` serves nothing this service queries. 130 such indexes
  were removed (288 → 160). Index a foreign key's leading column, index what a
  query actually filters or sorts on, and nothing else.

### Token revocation must fail closed

`revoked_tokens` is a denylist of `jti` claims, in **Postgres**, and the choice
of store is the load-bearing part:

- **Not Valkey.** `cache.enabled=false` is a supported mode, so a cache-backed
  denylist would silently not exist there and logout would go back to doing
  nothing. And the cache's documented invariant — a fault never fails a request
  — means answering "not revoked" when the truth is unknown.
- Postgres is a hard dependency of the service, so the denylist has the same
  availability as the service. A cache may sit in front of it, but only as an
  optimisation: a miss falls through to the truth, never to a decision.
- **An error from the store is fatal, never "not revoked."** Treating an
  unreachable denylist as an empty one lets a database blip re-validate every
  token anyone has logged out of.

`revoked_tokens.users_id` is deliberately **not** a foreign key to `users`:
deleting a user must not delete their revocations, or removing an account would
quietly re-validate every token it was ever issued.

**Access tokens ARE denylisted now**, and `authn.access.token.revocation.enabled`
defaults to `true`, so a logout ends the access token it was called with rather
than only the refresh token. This paragraph used to say the opposite, and the
reasoning it gave — that a lookup per request is too much to close a window the
lifetime already bounds — is why the denylist is an **in-memory mirror** rather
than a query: `initRevokedAccessTokens` builds it, the authn service adds to it
the moment it revokes, and the middleware consults memory.

Two things still follow from the old trade-off and have not changed:

- **the access-token lifetime stays short.** With revocation switched off it is
  once again the whole residual-access window after a logout, and the startup
  log says so explicitly — the absence is otherwise invisible, because every
  request still succeeds.
- **the first load is fatal.** Same rule as the rate-limit rules mirror: a
  denylist that failed to load is an empty one, and an empty denylist re-validates
  every token anyone has logged out of.

**Dev runs the shipped lifetimes.** `run.sh` and `.air.toml` used to override
the access token to 24h, which hid everything it touched: refresh was never
exercised locally, so the frontend bug that made `/auth/refresh` 404 survived,
and a logged-out token stayed usable for a day. Same rule as the IP rate
limiter — a dev stack that disagrees with production hides production's bugs.
Do not reintroduce the override to make a manual test more convenient.

**Logout revokes the refresh token supplied in its body**, and verifies it first:
signature, `token_type`, and that `sub` matches the caller. Without those checks
an ordinary logged-in user could revoke somebody else's token, which is a denial
of service against any account whose refresh token leaked into a log.

### Refresh tokens are rotated, and a replay ends the chain

Every refresh **spends** the token it was handed and issues a successor;
`revoked_tokens.replaced_by` records the link. Four rules follow, and each has
already been broken once in review:

- **Carry the expiry, never renew it.** Every link expires when the token that
  started the chain would have. Renewing on each refresh makes an active session
  immortal, which is a product decision and not one rotation gets to make.
- **Rotation without revocation is worse than no rotation** — it issues a second
  usable credential and retires nothing. With no store, or with
  `authn.refresh.token.rotation.enabled=false`, return the presented token
  unchanged.
- **Sign before recording.** A failure between the two must leave the old token
  working; recording first and then failing to sign locks the caller out with
  nothing in hand.
- **`Rotate` must never overwrite an existing successor.** `ON CONFLICT DO
  NOTHING`, not `DO UPDATE`: two requests racing on one token both reach it, and
  the successor written first is the one the chain actually went to. Overwriting
  strands the other one — still valid, and no longer reachable by the walk that
  would revoke it.

**A replay and a retry look identical; only time separates them.** Inside
`authn.refresh.token.rotation.grace` (30s) a spent token re-issues the successor
it already produced — a dropped response is not a theft, and an alarm that fires
on every lost packet is one nobody can act on. Past it, `RevokeChain` walks
`replaced_by` to the live tip and revokes it, ending the session for the
legitimate holder too, because nothing in the request says who is who.

**The grace path must check that the successor is still live.** Re-issuing a
successor that has since been revoked hands back a dead token and reports
success — the same bug logout was fixed for.

**Logout follows the chain as well.** A client that refreshed and then logged
out with the token it started the session with would otherwise revoke a link
that was already spent — a no-op that answers 200 while the session carries on.

**Every rejection says the ordinary revoked message.** Never tell a caller that
a replay was detected; that tells whoever stole the token how much is known. It
goes to a `WARN` log with the user, jti and tip instead.

**Rotation is a two-repo contract.** A client that keeps the token it just spent
presents a revoked credential next time, which reads as a replay and ends the
session. **The frontend deploys first** — storing the returned token is a no-op
against an API that does not rotate.

**The denylist now takes a row per refresh, not per logout.** `DeleteExpired`
had no caller for its whole existence; a sweeper runs it every
`authn.revoked.tokens.sweep.interval`. Because every lookup carries
`expires_at > NOW()`, the sweep can run late, early or never without changing an
answer.

### There is one JWT verification routine, and it is not the middleware's

`tokenjwt.Signer.Verify` verifies everything this service issues. The HTTP
middleware reaches it through `token.Signer`; `jwtvalidator` is a policy layer
(which classes require a `jti`) and holds no verification of its own.

**Do not add a second one.** There were two, and they disagreed: one pinned the
signing method and checked no `kid`, the other checked `kid` and pinned no
method, and neither checked `iss` or `aud`. Two verifiers that disagree are not
defence in depth — the weaker one is the one that decides.

- **`iss` and `aud` are validated**, not merely written. A token minted for
  another issuer used to be accepted on its signature alone; measured live, a
  token claiming `iss = aud = "https://attacker.example"` answered 200 with a
  full model list. Consequence to remember: **changing `authn.issuer`
  invalidates every token already issued.**
- **`exp` is required.** A token that omits it is refused, not treated as one
  that has not expired.
- **The keys are parsed once, in the constructor.** They used to be parsed per
  call, so a malformed key was found by the first request rather than at startup.

**A refused caller is told `Invalid or expired token` and nothing else**, decided
in the middleware, in one place. It used to write the validator's `err.Error()`,
which shipped `golang-jwt`'s own text — `crypto/ecdsa: verification error`,
`could not JSON decode header` — as part of this API's contract. This is the same
rule the stdlib-uuid migration wrote down: **never forward a library's error
string into an API response.** The reason goes to a `DEBUG` log.

**A handler that acts on a token must use the one the middleware verified.**
`middleware.JwtToken` carries it on the context beside the claims. `/auth/refresh`
used to take a *second* token from the request body and spend that instead, so
the token a request was authorised with and the token it acted on could differ —
measured live, a request authorised as one account came back with another
account's tokens. Never re-read a credential from the request when the middleware
has already verified one.

**A credential never travels in a URL.** The verification token was a path
segment on `GET /auth/verify/{token}`, so every verification wrote it into the
service's own request log (twice — `url=` and `path=`), the browser history and
the `Referer`. Email links point at the **frontend**, the page hands the token to
the API in an `Authorization` header, and there is no route that takes a token
from a path or a query string. `authn.user.verification.web.endpoint` is a
frontend URL, not an API one — the old name (`...api.endpoint`) is what led it
onto an API route to begin with.

**Never put the token in an error.** It is a live credential until it expires,
and an error message is the one place guaranteed to reach a log file.

**`kid` is the RFC 7638 thumbprint of the signing key**, and verification
resolves it against a keyset. It used to be `signKey.Params().N` — the P-256
group order, a public constant identical for every key — so the check compared a
constant to itself and two keys were indistinguishable.

- **Rotate with an overlap, never a swap.** Add the incoming key to
  `authn.additional.public.key.files` and deploy; then make it the signing pair
  and move the outgoing key to `additional`; only drop the old key once the
  longest-lived token signed by it has expired. That is a **personal access
  token, up to a year** — not the access token.
- **The canonical JWK JSON is a wire format, not a struct.** RFC 7638 fixes the
  member set and their order (`crv`, `kty`, `x`, `y`); `x` and `y` are
  fixed-length and left-padded. Building it with a marshaller that reorders or
  omits, or with `big.Int.Bytes()` which trims a leading zero, changes every
  `kid` — silently, for about one key in 256.
- **A mismatched key pair is fatal at startup.** It signs tokens the service
  cannot verify; the alternative is discovering it as a 401 on every request.

### Every config Field needs a flag registered by hand

`internal/config` declares a setting as a `Field` carrying its flag name and env
var name, but `setupFlags` in `internal/app/configs.go` registers the flag **one
line per setting**. Add a `Field` and forget the line and the setting works
through the environment while *stopping the process* when passed as a flag:
`flag provided but not defined: -authn.refresh.token.rotation.enabled`.

Six settings had drifted this way, including the switch that turns refresh-token
rotation off — the one an operator reaches for in a hurry, documented as
available, and guaranteed to refuse to start.
`TestEveryConfigFieldHasAFlag` walks the config by reflection and now fails on
any new gap; it recognises a `Field` structurally, so a new config group is
covered without being told about it.

When a caller genuinely needs to tell "expired, refresh" from "revoked, sign in",
design a deliberate signal — do not reach back for a library string.

### Brute force is bounded twice, and both are needed

Password guessing is limited per **source** by the IP rate limiter and per
**account** by `authn.login.throttle.*`. They are independent and neither
substitutes for the other: the IP limiter does nothing about guesses spread
across many addresses, and the account throttle does nothing about one source
hammering many accounts.

- **The throttle key comes from the address that was submitted**, hashed, before
  anything is looked up. Throttling only accounts that exist would answer "does
  this address have an account?" through the difference in behaviour — a failed
  attempt against an unknown address must cost exactly what a real one costs.
- **`Attempt` always spends; `Succeed` refunds.** Only failures accumulate, but
  the mechanism is consume-then-refund rather than a non-consuming check —
  because a non-consuming check is not expressible on a token bucket.
  `golang.org/x/time/rate` will not return a token whose time to act has already
  passed, so reserve-then-cancel silently spends on exactly the immediately
  available tokens a check would look at. Do not "optimise" it back.
- **It delays, it never locks.** Anyone who knows an address can spend its
  budget; a refilling ceiling is what stops that being an account they can keep
  shut. Do not replace it with a lockout that needs an operator to clear.
- **The budget is per replica.** Valkey is optional (`cache.enabled=false` is
  supported) so a cache-backed throttle would silently not exist, and Postgres
  would mean a disk write per unauthenticated request. N × a small number is
  still bounded, which the unthrottled path was not.

**A failed login says one thing.** Every reason a login does not succeed — no
such address, wrong password, disabled account, an account that authenticates
through an identity provider — answers `401` with
`domain.AuthnInvalidCredentials` and nothing else. Distinct messages let a caller
ask "does this address have an account?" and get an answer; the unknown-address
case used to echo the probed address straight back. The reason is recorded on
the span and in the log instead, so an operator can still tell a typo from a
disabled account.

**One bcrypt compare runs on every login**, found or not, whichever method was
asked for. An unknown address used to return before any hashing, so it answered
in a millisecond where a real one took fifty — a timing oracle that survives any
amount of care over the response body. `dummyPasswordHash` is the comparison
input when no account was found.

Measured against the running API, guessing at one account with a rotating
`X-Forwarded-For` — the attack the IP limiter alone cannot see:

```text
before any of this work:   {401: 30}          every guess evaluated
after the trusted-proxy fix + throttle: {401: 5, 429: 25}
```

### Verifying a migration change did not alter the schema

The strongest check is a before/after comparison of a database built from
scratch each way:

```bash
pg_dump --schema-only --no-owner --no-privileges | grep -vE '^\\(un)?restrict '
```

Two gotchas, both of which make the diff look broken when it is not: modern
`pg_dump` emits a **random `\restrict` token per run**, so two dumps of the same
quiet database never match until it is filtered; and row timestamps differ
between runs, so they must be normalised before a `--data-only` comparison.
`serial_id` also shifts whenever insert order changes, which is not a content
difference. An ordered digest avoids all three:

```sql
SELECT md5(string_agg(t::text, '' ORDER BY t.id)) FROM models t;
```

## New uuid generation

All the uuids should be version 7, you can use the command line tool `uuidgen` in the `cmd/uuidgen` package to generate UUIDs. You can execute the following command to generate a new UUID:

```bash
# -n is the number of UUIDs to generate, in this case 1
# -v is the UUID version, in this case 7
go run cmd/uuidgen/main.go -n 1 -v 7
```

### Use the standard library `uuid`, not `github.com/google/uuid`

Go 1.27 ships a `uuid` package, so the external dependency is gone. Import it as
plain `"uuid"`. The two packages agree on the things that matter — `uuid.UUID`
is `[16]byte` in both, so it stays comparable with `==` and usable as a map key;
`Parse` accepts the same five forms (canonical, uppercase, unhyphenated, braced,
`urn:uuid:`); and `pgx` encodes both to the identical 16 wire bytes and scans
back into either, even though the stdlib type has no `sql.Scanner` /
`driver.Valuer`. pgx matches it structurally as `[16]byte`, so the repository
layer needed no codec registration.

Where the APIs differ, and what this repo does about it:

| `github.com/google/uuid`        | standard library                   | Note                                                             |
| ------------------------------- | ---------------------------------- | ---------------------------------------------------------------- |
| `uuid.NewV7() (UUID, error)`    | `uuid.NewV7() UUID`                | no error to handle — it panics only if the CSPRNG fails          |
| `uuid.Must(uuid.NewV7())`       | `uuid.NewV7()`                     | `Must` does not exist and is no longer needed                    |
| `uuid.Nil` (variable)           | `uuid.Nil()` (function)            | `&uuid.Nil` has no equivalent — a call result is not addressable |
| `uuid.Max` (variable)           | `uuid.Max()` (function)            | only used by paginator tests                                     |
| `uuid.NewRandom()`              | `uuid.NewV4()`                     | same thing, better name                                          |
| `id.Version()` / `uuid.Version` | **absent** — use `domain.UUIDVersion` | the version is the high nibble of byte 6                      |
| `uuid.NewV6()`                  | **absent**                         | `cmd/uuidgen -v 6` is gone; nothing in the service generated v6  |

The missing `Version()` matters more than it looks: `domain.IsUUIDV7` and
`domain.ValidateUUID` enforce the v7-everywhere rule that the whole ID scheme
rests on. That check now lives in one place, `domain.UUIDVersion` — do not
re-derive the nibble at a call site.

Three things the swap taught, all of which will bite again:

- **A call result is not addressable.** `&uuid.Nil` has no direct translation;
  write `new(uuid.Nil())`, which is also the `new(expr)` idiom this repo already
  prefers over a throwaway variable.
- **`uuid.Parse` no longer explains itself.** The standard library collapses
  every failure into the string `invalid uuid`, where `github.com/google/uuid`
  distinguished a wrong length from wrong characters. That text was being
  forwarded to clients as the `Reason` of a `domain.InvalidUUIDError` and a
  `domain.InvalidJWTError`, so `handler.uuidParseReason` now derives the previous
  wording from the input instead. **Never forward a library's error string into
  an API response** — that is how a dependency bump silently rewrites a published
  contract.
- **`domain.EnsureUUIDV7` still returns an error that is always nil.** The
  signature was kept so the swap did not have to touch its 22 call sites and
  their error branches. Collapsing it to a single return is a worthwhile
  follow-up, not an oversight.

`format:"uuid"` in a struct tag is what makes swagger render an ID as
`{"type": "string", "format": "uuid"}` — 89 properties depend on it, and swag has
no special knowledge of either uuid package. After any change in this area run
`make build` and diff `docs/api/swagger.json`; a regression shows up as an ID
turning into an array of integers, which silently breaks every generated client.

## Code Style

- Follow Go's idiomatic style defined in
  - [Go Style Guide](https://google.github.io/styleguide/go/guide)
  - [Go Style Decisions](https://google.github.io/styleguide/go/decisions)
  - [Go Style Best Practices](https://google.github.io/styleguide/go/best-practices)
  - [Effective Go](https://golang.org/doc/effective_go.html)
- Use meaningful names for variables, functions, and packages.
- Keep functions small and focused on a single task.
- Use comments to explain complex logic or decisions.
- Use dependency injection for services and repositories to facilitate testing and maintainability.

### Go 1.27 baseline

The module is `go 1.27.0` and `go fix -diff ./...` is **clean** — the codebase is
already modernised, so do not introduce pre-1.21 idioms.

> The patch version in `go.mod` is load-bearing, not cosmetic. CI resolves its
> toolchain from `go-version-file: ./go.mod`, and `govulncheck` reports standard
> library advisories against whatever toolchain it runs on. Go 1.26.5 had seven
> _reachable_ stdlib vulnerabilities in this codebase — including `GO-2026-6089`
> on the `http.Server.ListenAndServe` path — all fixed in 1.26.6. When
> `make vulncheck` flags stdlib entries, bump this directive before anything else.

**Every source-processing tool must be rebuilt against the running toolchain.**
A binary built by Go 1.26 cannot parse a 1.27 stdlib, and the two tools this
repo uses fail in opposite ways:

- `govulncheck` fails **loudly** — `Int (function) is not a type` out of
  `math/rand/v2` plus a wall of `file requires newer Go version go1.27`. That is
  the tool, not a finding. `make vulncheck` reinstalls first, so it only bites a
  hand-run binary.
- `betteralign` fails **silently** — it prints `analysis skipped due to errors in
  package` for every package and exits 0, which reads exactly like "nothing to
  align". It has **no make target**, so nothing ever reinstalls it for you.

**The Makefile now handles this.** Every `install-*` target goes through the
`ensure_tool` macro, which reinstalls a tool only when it is missing or was
built by an **older** toolchain than `go env GOVERSION`. `make tools` refreshes
all nine at once. The comparison is "not older than" rather than "equal to" on
purpose: a tool whose own `go.mod` selects a newer toolchain is fine, and
demanding equality would reinstall it on every invocation.

This is also why the install targets no longer reinstall unconditionally — that
was hitting the network on every `make build` (for `swag` and `go-swagger`) and
still silently left a stale `betteralign` in place, because nothing invoked it.

Carried forward from the 1.26 baseline, all still current:

- `any` over `interface{}`; `min`/`max` builtins; `slices`/`maps` over hand-rolled loops.
- `for range n` over `for i := 0; i < n; i++` where the index is unused.
- `errors.AsType[E](err)` over `errors.As(err, &target)` — type-safe and faster.
  There are **zero** `errors.As` call sites; keep it that way. (The count of
  `AsType` uses is not worth recording — it only ever grows, and it is the zero
  that is the invariant.)
- `slog.NewMultiHandler` when a log record must reach more than one sink.
- `new(expr)` for optional pointer fields instead of a temporary variable.
- `iter.Seq` / `iter.Seq2` for streaming APIs rather than channels or callbacks.
- `os/signal.NotifyContext` sets a cancel **cause** — log it on shutdown.
- Green Tea GC is the default; do not set `GOEXPERIMENT=nogreenteagc`.

New in 1.27, and what each one means _here_:

- **Promoted fields can be set directly in a struct literal**, and the new
  `embedlit` modernizer rewrites code to match. The moment the `go 1.27.0`
  directive lands this fires on `tokenjwt/adapter.go`, flattening the embedded
  `jwt.RegisteredClaims{...}`. Run `go fix ./...` in the **same commit** as the
  directive bump or the `go fix -diff` CI gate goes red on the next PR.
- **The other new modernizers** are `atomictypes`, `slicesbackward` and
  `unsafefuncs`; `waitgroup` was renamed `waitgroupgo` and `fmtappendf` was
  removed. None of them match this repo today — it has no `sync/atomic`, no
  `sync.WaitGroup` and no `unsafe`. `go tool fix help` lists all 27 analyzers
  actually registered in the installed toolchain; trust it over any changelog.
- **Methods may now declare type parameters, but interfaces still cannot.** A
  generic method can never satisfy a port interface, so this does _not_ unlock
  generic repository or use-case ports. Do not try.
- **`encoding/json` is now implemented on top of `encoding/json/v2`.** Marshal
  and unmarshal behaviour is preserved, but **error text changes** and v2 does
  not sort map keys unless you pass `json.Deterministic`. Neither costs anything
  here today: the golden fixtures in `llmhttp/provider/*/testdata/` are compared
  **structurally** by `assertJSONEqual`, which unmarshals both sides before
  comparing, so key order never mattered — and `ollama.Options` is the only map
  that reaches a request body. Keep it that way; a fixture assertion that
  compares marshalled bytes would be brittle for a reason that has nothing to do
  with the wire contract. `GOEXPERIMENT=nojsonv2` exists as an escape hatch —
  treat reaching for it as a bug to fix, not a setting to adopt.
- **Time channels are unbuffered permanently**; the `asynctimerchan` GODEBUG is
  gone. This repo pins no GODEBUG at all — no `godebug` line in `go.mod`, no
  `//go:debug` comment — so nothing breaks. Keep it that way: from 1.27 the `go`
  command **fails outright** on a `godebug` naming a setting that was removed.
- **`http.Response.Body.Close` now drains unread content** so the connection can
  be reused. `provider/http.go` already reads every response to completion, so
  this costs nothing today; a future streaming path that abandons a large body
  must not assume `Close` is cheap.
- **New `net/http` server knobs**: `Server.MaxHeaderValueCount` bounds how many
  values a header may carry, and `Server.DisableClientPriority` opts out of the
  RFC 9218 HTTP/2 priority scheduling that is now on by default. The server in
  `driving/http/server/http.go` sets neither **deliberately** — an unset
  `MaxHeaderValueCount` already means `DefaultMaxHeaderValueCount` (500), so
  writing it out changes nothing, and there is no evidence a lower bound is
  wanted. The server's other timeouts _are_ set — see
  [HTTP server timeouts](#http-server-timeouts).
- **The `goroutineleak` pprof profile is GA** and the existing pprof server
  (`app/telemetry.go`) serves it at `/debug/pprof/goroutineleak` — not because a
  handler is registered for it, but because `/debug/pprof/` is a subtree pattern
  and `pprof.Index` serves every named profile under it. It finds goroutines
  blocked forever on a channel or mutex, which a plain `goroutine` dump cannot
  tell apart from an idle one. The pprof server is off by default and binds
  localhost:6060.
- **`testing/synctest.Sleep`** collapses `time.Sleep` + `synctest.Wait`, and
  **`httptest.NewTestServer`** returns an in-memory server that works _inside_ a
  synctest bubble, where `httptest.NewServer` cannot because it binds a real
  socket. Reach for the pair when you add a time-dependent test — but there is
  nothing to convert today: `provider.Caller` deliberately implements no retry,
  backoff or timeout (see `provider/doc.go`), so the provider suites are not
  time-dependent, and no unit test in the repo sleeps.
- **`strings.CutLast` / `bytes.CutLast`** cut around the _last_ separator. No
  call site needs them yet — the repo has zero `LastIndex` uses.
- **`go test` runs the `stdversion` vet check by default**, so using a stdlib
  symbol newer than the `go` directive now fails the test run instead of
  surfacing at build time somewhere else.
- **The standard library now ships a `uuid` package** and this repo uses it
  instead of `github.com/google/uuid` — see [New uuid generation](#new-uuid-generation)
  for the API differences that migration had to absorb.
- Building Go 1.27 itself requires **macOS 13 Ventura or later**.

### Documentation rules

Documentation is part of the change, not a follow-up. A PR that adds or reshapes
behaviour is incomplete without it.

### Every package carries a package comment

- One `doc.go` per package (or the comment on the primary file), starting
  `// Package <name> ...`.
- Say **why the package exists and what problem it solves**, not just what it
  contains — the shape of the code is already visible; the reasoning is not.
- Cover: the contract it exposes, the invariants callers must respect, the error
  types callers match on, and anything deliberately _not_ handled yet.
- **Record the reason behind a non-obvious decision.** A future reader who does
  not know why a constraint exists will remove it. `provider/doc.go` is the
  reference for the depth expected.
- Use godoc conventions: `#` headings, `[Symbol]` links, indented code blocks.
  godoc does **not** render mermaid — use a plain ASCII diagram there and keep
  mermaid for the markdown docs.

### Every architectural change updates `docs/`

- Update the affected file under `docs/architecture/`, or add one. The index in
  [`docs/architecture/README.md`](../docs/architecture/README.md) and the
  Documentation list in the root README must both stay accurate.
- **Diagram it with mermaid.** Prose alone does not convey a request path, a
  resolution chain, or how components relate. Reach for:
  - `flowchart` — component relationships, layering, resolution paths
  - `sequenceDiagram` — a request end to end, including the failure branches
  - `erDiagram` — entity relationships when the schema is involved
  - `stateDiagram-v2` — lifecycles and status transitions
- Diagrams must show the **real mechanism**, with actual type and function names,
  not a generic box-and-arrow sketch.
- Include the failure paths. A diagram that only shows the happy path hides
  exactly the part a reader needs.
- When behaviour is corrected, say what it used to do and why that was wrong —
  the same mistake is otherwise easy to reintroduce.
- Fence every block as ` ```mermaid `, and give other fenced blocks a
  language (`text`, `go`, `sql`, `bash`) — the markdown linter enforces this.

### Swagger annotations are the API contract, not comments

`make build` runs `swag fmt` + `swag init`, so everything under `docs/api/`
(including the compiled `docs.go`) is **generated from the handler annotations**.
Nothing validates them against the code. That inverts the usual risk: the
generated spec is always "in sync" — with whatever the comments happen to say —
so a wrong `@Failure` silently ships as the published contract and no gate fails.

- **Never hand-edit `docs/api/*`.** Change the annotation and re-run `make build`.
- **Every status code the handler can write needs a `@Success`/`@Failure`.** When
  you add an error branch, add the annotation in the same change. The codes most
  often missed are the ones added later to an existing handler: `403` from a
  `System*Error` guard, `404` from a `*NotFoundError`, `409` from an
  `*AlreadyExistsError`.
- **Do not document a code the handler cannot return.** A `@Failure 404` on an
  endpoint that treats not-found as idempotent success is worse than silence —
  it invents a branch for every generated client.
- **Match the real success shape.** If a handler calls `http.Redirect`, it is
  `@Success 302 {string} string` plus `@Header 302 {string} Location`, not a
  `{object}`. An `{object}` there makes clients parse a redirect as JSON.
- Middleware-supplied codes (`401`, `403`, `429`) will not appear in the handler
  body — annotate them from the middleware chain, not from a grep of the function.
- `@Accept` belongs on every POST/PUT/PATCH that reads a body.
- Auditing this is mechanical and worth redoing after a batch of handler work:
  compare `mux.Handle("METHOD /path"` registrations against `@Router`, and the
  `http.Status*` values written in each function body against its declared codes.
  Watch for two false positives — a status used as a _struct field_
  (`RedirectCode: http.StatusFound`) is not a response code, and a code emitted
  by a helper the handler calls will not appear inline.

### Swagger changes feed the authz seed data — regenerate it

The `resources` rows in
`database/migrations/00008_roles_policies_tables_upsert.sql` are **one row per API
endpoint**, generated from `docs/api/swagger.json`. Each row's id is the
operation's `@ID`, its name is `@Summary`, its description is `@Description`, and
its action/resource are the method and path. Those rows are what the policies
reference, so an endpoint missing here has no resource to authorise against.

So the chain is: **annotation → `make build` → `swagger.json` → `apiendpoints` →
migration**. After any change to `@ID`, `@Summary`, `@Description`, `@Router` or
a route registration:

```bash
make build                          # regenerate docs/api/swagger.json first
go run cmd/apiendpoints/main.go      # emits the resources rows
```

Paste the output over the generated block in
`00008_roles_policies_tables_upsert.sql` — it starts after the
`-- automatic generate with the program apiendpoints` marker (line 35) and runs
to the row ending in `;`. Leave everything above the marker alone.

- **Only these five annotations affect the output.** Changing `@Success`,
  `@Failure`, `@Accept`, `@Produce`, `@Param`, `@Tags` or `@Security` cannot
  change a single row — if the diff is non-empty after one of those, something
  else moved.
- **Diff the row _set_, not the file**, before concluding anything changed:
  `sort` both sides and compare. A row genuinely added or removed is the signal;
  anything else is noise.
- **The generator sorts on `(Path, Method)` and that ordering is load-bearing.**
  It previously sorted on `Path` alone with `sort.Slice`, which is not stable,
  fed from a randomised map walk — ten runs produced ten different orderings of
  the same 136 rows, so regenerating always produced a large diff and a real
  change was indistinguishable from churn. Do not weaken that comparator back to
  a single key.
- Because the file is an **already-applied migration**, edit it only when the row
  set genuinely changes, and remember existing databases will not re-run it — a
  new endpoint needs its resource row added by a _new_ migration as well, not
  only here. This file is what a freshly created database gets.

## Testing rules

- **Never write a test where the mock server encodes the same Go struct the
  client decodes.** That asserts nothing about the real wire contract and is how
  the broken Gemini/OpenAI/DeepSeek clients passed CI. For any external API,
  assert against **golden JSON fixtures in `testdata/`** captured from the real
  provider — on both the request body and the response.
- Use `testing/synctest` for anything time-dependent (cache TTL, rate limiting,
  retry/backoff, timeouts) instead of real sleeps. On Go 1.27 reach for
  `synctest.Sleep` (it is `time.Sleep` + `synctest.Wait` in one call) and
  `httptest.NewTestServer`, whose in-memory transport works inside a bubble —
  `httptest.NewServer` uses a real socket and does not. No existing unit test is
  time-dependent, so this is guidance for new tests, not a conversion backlog.
- Use `t.ArtifactDir()` (run with `-artifacts`) to dump debug output — rendered
  prompts, retrieved chunks — rather than `t.Log`ging large blobs.
- Parsers that consume untrusted input (`domain/qfv_parsers.go` — filter, sort,
  fields) should have `Fuzz` targets.

## Testing

### Integration Tests

Integration tests are used to test the api endpoints and the service functionality as a whole. They are located in the `tests/integration` package and are executed against a running instance of the service.

- The project is using the [Testify](https://pkg.go.dev/github.com/stretchr/testify) library for testing. But in most of the case the project is using the Go testing library. The project is using the `go test -race` command to run the tests.
- Use lowercase snake_case for test names scenarios, e.g. `t.Run("not_found",`, `t.Run("missing_authorization"`, etc.
- Write integration tests for all the handlers endpoints in the `internal/http/handler` package.
- Write integration tests in the `tests/integration` package to test the service endpoints.
- Use helper functions in the `tests/integration/helper_functions_test.go` file to avoid code duplication.
- If `air` tool is running, execute the tests using `go test -race -tags=integration ./tests/integration` amd optionally `-run TestXXX` where `TestXXX` is the name of the test to run.

### The normal gates never compile the integration suite

`tests/integration/` is behind `//go:build integration` and `tests/eval/` behind
`//go:build eval`. `go build ./...` uses no tags and `make test` uses
`-tags=unit`, so **neither ever type-checks those files**. A refactor can pass
`go build`, `go vet`, `golangci-lint` and `make test` and still leave the
integration suite uncompilable — the stdlib uuid migration did exactly that.

After any sweeping change, type-check every tag explicitly before believing a
green run:

```bash
for t in unit integration eval; do go vet -tags=$t ./... ; done
```

## Running things locally

The whole loop, in the order it is normally needed. Both repos matter: an API
change is usually a two-repo change, and the frontend is where a layout or
copy problem is actually visible.

### The core dev environment (Postgres, Valkey, Prometheus, Grafana, Tempo, Mailpit)

```bash
make dev-certs       # JWT pair, AES key and dev TLS CA under certs/; creates only
                     # what is missing, never overwrites. start-dev-env runs it
make start-dev-env   # provision and start it. Runs stop-dev-env first, and
                     # RECREATES the data -- see the warning below
make stop-dev-env    # stop the containers, keep the volumes
make rm-dev-env      # stop and remove the environment entirely
```

**`make dev-certs` never overwrites.** A regenerated `jwt.key` invalidates every
token issued, a regenerated AES key makes every stored IdP secret unreadable,
and a regenerated CA is not trusted by the service that already loaded the old
one. To rotate, delete the file and run it again; `jwt.pub` is re-derived from
whatever `jwt.key` is there, and the script warns when the pair does not match.

**`PROJECT_NAME` is split from the `module` directive, never grepped.** It used
to be `grep module go.mod | cut -d / -f 3`, which matches every line containing
the word "module" -- so the comment about "the module graph" added to go.mod
turned the name into two lines of prose, the backticks in it ran as a command,
and every dev-env target failed with `/bin/sh: -u: command not found` and
`Error 127`. A guard in the Makefile now refuses a name that is not exactly one
word. When a Makefile variable comes from a file, parse the directive, do not
grep for a word.

**`make rename-project` reads owner and name from the origin remote**, in both
the ssh and https shapes (the old `cut -d / -f 2` produced an empty name on an
https URL and would have replaced the template name with nothing). It rewrites
`slashdevops/go-rest-api-service-template`, then the bare name, then the
underscored name -- longest first, and only the full owner/name pair, because
go.mod also requires `github.com/slashdevops/{c3e,httpx,mailer,...}` which must
stay. `--null`, not `-Z`, on the grep: on macOS `-Z` means decompress, and the
file list reached xargs as one newline-joined name while the target still
printed ✅. With no remote: `GIT_REPOSITORY_OWNER=<o> GIT_REPOSITORY_NAME=<n>
make rename-project`.

**`start-dev-env` destroys the database.** It is how a migration change is
picked up (goose does not checksum, so an edited file is never re-applied to an
existing database), and it is the only way to be sure the schema matches the
files. Do not reach for it mid-investigation without saying so: any rule, user
or project created by hand goes with it.

### The core service

```bash
air                          # build and run with live reload, in the background
pkill -f "go-rest-api-service-template"    # stop it (and any binary started by hand)
```

`air` takes its flags from `.air.toml`, which is kept in step with `run.sh` by
`TestRunScriptAndAirAgree`. To run a one-off configuration, build and pass the
flags directly rather than editing either file:

```bash
make build && ./build/go-rest-api-service-template -ratelimit.enabled=false ...
```

**Check what is actually listening before trusting a result.** A stale process
from an earlier run answers on :8080 and will happily "prove" a change that is
not in it — `curl -s localhost:8080/api/v1/version` reports the commit and
branch it was built from.

### The frontend

```bash
cd ~/git/github.com/slashdevops/go-rest-api-service-template-frontend
cp .env.example .env   # ENDPOINT_API defaults to the local core
pnpm run dev           # http://localhost:5173
pkill -f vite          # stop it
```

`pnpm test` is vitest (unit and component). `pnpm run test:e2e` is Playwright,
which starts its own dev server and needs **both** the core service and the dev
environment up. `pnpm check`, `pnpm lint` and `pnpm build` are the other gates.

### Dependencies

```bash
make go-mod-update     # core: go get -u per direct dependency, then tidy
pnpm update            # frontend: within the ranges in package.json
```

`go get -u` upgrades the **transitive** closure too, which has broken the build
once: it pulled `gobwas/glob` past what OPA supports and nothing compiled.
`go mod tidy` is happy with that state — only a build or `go vet` catches it, so
run `for t in unit integration eval; do go vet -tags=$t ./... ; done` after any
dependency change.

## Post-Change Checklist

Prefer the Make targets that the repo already defines after making changes:

```bash
make tools               # Install/rebuild any tool that is missing or built by an older Go
go fix -diff ./...       # Preview Go 1.27 modernisers (currently clean — keep it that way)
go fix ./...             # Apply them
make go-fmt              # Format code (go fmt ./...)
make go-betteralign      # Re-align struct fields for optimal memory layout (rewrites source)
make lint                # Lint (formatting + vet + staticcheck SA* + errcheck + ineffassign + unused)
make build               # Build (also runs swag fmt + swag init + swagger markdown)
make test                # Unit tests with race detector + coverage
make vulncheck           # govulncheck — gated in CI
make licenses-check      # Third-party license allow-list check

# If touching tests/integration/ — bring the dev env up and run with the integration tag.
make rm-dev-env && make start-dev-env && air      # in another terminal
go test -race -tags=integration ./tests/integration
```

Notes:

- `betteralign` now has Make targets: `make go-betteralign` applies the
  rewrites and `make go-betteralign-check` only reports. It is deliberately
  **not** a dependency of `build` — a build target that silently edits your
  source is a bad surprise.
- `make build` runs `swag fmt` which can re-order imports in handlers
  if a package was renamed; re-run it whenever you rename a package
  used by an `@Param` / `@Success` swagger comment.
- `make test` excludes integration tests (it uses `-tags=unit`). The
  integration suite has its own runner shown above and a separate
  `make test-integration` target that builds a container instead.

### What CI gates on

**The PR workflow is deliberately cheap; the release workflow is thorough.**
That split is the rule to keep — a gate belongs on the PR only if it can
answer differently because of the change under review.

`.github/workflows/pr.yaml` runs, in order: `go build ./...`, `go fix -diff`
(must be empty), `make arch-test`, `make lint`, `make test`,
`make test-coverage` — plus two that run **only when their inputs moved**:

| Gate | Runs when | Why conditional |
| ---- | --------- | --------------- |
| `make vulncheck`    | `go.mod` or `go.sum` changed | a per-PR scan answers "did THIS change introduce a vulnerable dependency", and nothing else can change that answer |
| `make check-alerts` | a file under `dev-env/configuration/prometheus/` changed | it pulls `prom/prometheus`, ~250 MB, for a test that takes milliseconds |

**The changed-file query fails OPEN.** If it cannot tell what moved, both run.
A cost optimisation must never be able to silently drop a gate — keep that
property in anything added here.

**`go build ./...`, not `make build`.** `make build` installs `swag` and
`go-swagger` and regenerates `docs/api`, and the PR job then discards the
result; nothing diffs it, so it was never a gate on this path. The release
workflow runs the full `make build-dist`, which is where the generated docs
actually ship. The consequence to know: **nothing in CI catches `docs/api`
drifting from the handler annotations**, and nothing did before either — run
`make build` locally, as the checklist says.

**`.github/workflows/security-scan.yaml` runs `govulncheck` weekly.** It covers
what a per-PR scan structurally cannot: an advisory published against a
dependency nobody has touched. A quiet fortnight with no PRs was a fortnight
with no scan.

**A new push supersedes the run in flight** (`concurrency` + `cancel-in-progress`).
Without it, three pushes during a review bought three full runs, two of them
answering a question about code that no longer existed. Both repos have it.

**`~/go/bin` is cached**, keyed on the `Makefile`, because `install-*`
go-installs a tool whenever the binary is missing and on a fresh runner every
one is. Warm-cache floor, which is the floor and not the cost: `golangci-lint`
15s, `go-swagger` 6s, `go-test-coverage` 5s, `swag` 2s, `govulncheck` 1s.

**Do not add a step to the PR workflow without asking what it can catch that
the change under review could have caused.** Two of the removed steps —
a `go install scc` for a lines-of-code table, and a print of the bash and make
versions — gated nothing at all.

**`make test-coverage` is a CI gate now**, running straight after `make test`
because it reads the profile that step writes. It is a **ratchet**: it fails
when coverage drops, not when it fails to reach an aspiration.

It used to ask for 25% total and 20% per package against a measured **13.1%**,
so it could not pass, was wired into no workflow, and told nobody anything. The
thresholds now describe what the unit suite actually achieves — 13% total, and
per-package floors in `override` for the 21 packages unit tests genuinely
cover, rounded down to a ten so ordinary churn does not trip them. `package` is
`0` with those floors doing the work.

**Do not "fix" the low total by merging the integration profile.**
`tests/integration/` drives the API over HTTP against a **separately running
binary**, so nothing it exercises runs in a test process and no profile this
gate can read will ever attribute it. Raising the number honestly means making
out-of-process execution countable: `go build -cover`, run the suite against
that binary with `GOCOVERDIR` set, `go tool covdata textfmt`, then list both
profiles in `profile:`.

**Regenerate the floors by STATEMENTS, the way the tool measures.** A mean of
the per-file percentages is a different number — choosing floors from it put
`middleware` 0.3 points above anything it could reach, and the gate failed on
a package nothing was wrong with.

Two things about these workflows are load-bearing and should not be removed:

- **`defaults.run.shell: bash`** brings `pipefail`. Every step pipes `make` into
  `tee -a $GITHUB_STEP_SUMMARY`, and without it a failing `make` is masked by
  tee's exit code 0 — CI reports green on a broken build.
- **`MAKE_STOP_ON_ERRORS: true`** makes the Makefile's `exec_cmd` wrapper
  propagate failures instead of just printing a ❌.

## Further reading

- [`docs/architecture/README.md`](../docs/architecture/README.md) — hexagon overview, request flow, hard rules
- [`docs/architecture/adding-an-entity.md`](../docs/architecture/adding-an-entity.md) — recipe for a new domain entity
- [`docs/architecture/adding-an-adapter.md`](../docs/architecture/adding-an-adapter.md) — recipe for a new outbound integration
- [`docs/architecture/http-server-timeouts.md`](../docs/architecture/http-server-timeouts.md) — which bound covers which span of a request, and why two are off
- [`docs/architecture/caching.md`](../docs/architecture/caching.md) — the cache port, the fail-open invariant, dependency invalidation, and why json is the only encoder
- [`docs/architecture/rate-limiting.md`](../docs/architecture/rate-limiting.md) — the rule model, the two limiters, and the breaker
- [`docs/architecture/resource-limits.md`](../docs/architecture/resource-limits.md) — scopes, three-priority resolution, the counter signature
- [`docs/architecture/authentication.md`](../docs/architecture/authentication.md) — tokens, rotation, revocation, throttling
- [`docs/architecture/database-migrations.md`](../docs/architecture/database-migrations.md) — the file set and the rules that keep it applyable
- [`docs/architecture/health-probes.md`](../docs/architecture/health-probes.md) — which endpoint answers which question
- [`docs/operations/running-the-service.md`](../docs/operations/running-the-service.md) — the pre-flight checklist
