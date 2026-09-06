# Development Guidelines

This file is the single source of truth for agent instructions. `CLAUDE.md`,
`AGENTS.md`, `DEVELOPMENT_GUIDELINES.md`, `.cursorrules` and `.clinerules` are
symlinks to `.github/copilot-instructions.md` — edit that file, never a symlink.

**Keep this file short.** It holds the rules an agent needs on every task:
the stack, the layout, the style, the gates and the invariants that are easy
to break. The reasoning behind each rule — what shipped wrong, what was
measured, why the fix has the shape it has — lives under `docs/` and is
linked from the section that depends on it. When you add a rule here, add
one paragraph and put the story in the linked doc.

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

- Language: Go 1.27+ (`go 1.27.1` in `go.mod`; the patch version is load-bearing, CI and `govulncheck` resolve the toolchain from it)
- Framework: standard library (`net/http`, `slog`, `testing`)
- Build: `make` — every target is in the Makefile
- Live reload: `air`
- Database: PostgreSQL 18 (native `uuidv7()`, no extension); migrations with `goose`
- Cache / counters: Valkey (optional, `cache.enabled=false` is supported)
- Containers: `podman`
- API docs: Swagger/OpenAPI generated from handler annotations
- IDs: UUID v7 everywhere — `go run cmd/uuidgen/main.go -n 1 -v 7` to mint one
- Observability: OpenTelemetry, Prometheus, Grafana, Tempo (dev stack)
- Tests: `testing` + Testify; `go test -race`

## Project structure

**Hexagonal (Ports & Adapters).** The hard invariant: **nothing under
`internal/core/` imports infrastructure** (HTTP, database, cache, mail, OPA,
JWT, OTEL, ...). `TestCoreHasNoInfraImports` in `internal/core/arch_test.go`
breaks the build if it is violated.

```
internal/
├── adapter/
│   ├── driven/           ── outbound: cachevalkey, cipheraes, notifieremail,
│   │                        oauthidp, policyopa (embeds the rego bundle),
│   │                        ratelimit{breaker,memory,valkey}, repositorypg,
│   │                        throttlememory, tokenjwt
│   └── driving/http/     ── inbound: payload (wire shapes), handler,
│                            jwtvalidator, middleware, respond, server
├── app/                  ── composition root: builds adapters, wires ports
├── config/               ── flag/env loader
├── core/                 ── pure: domain, port/{driven,driving}, usecase
├── o11y/                 ── OTEL setup + helpers
└── version/              ── build metadata
```

### Where new code goes

| Adding a...                                    | Lives in                                                                                                                       |
| ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| Domain entity, validation rule, business error | `internal/core/domain/`                                                                                                        |
| Use-case method                                | `internal/core/usecase/<entity>.go`                                                                                            |
| HTTP route                                     | `internal/adapter/driving/http/handler/<entity>.go` + the `internal/core/port/driving/<entity>.go` interface                   |
| Repository method                              | `internal/adapter/driven/repositorypg/<entity>.go` + the `internal/core/port/driven/repository/<entity>.go` interface          |
| Outbound integration                           | port in `internal/core/port/driven/<concept>/`, adapter in `internal/adapter/driven/<concept><tech>/`                          |
| Database migration                             | `database/migrations/`, the next number in the sequence, never a gap                                                           |
| Mock                                           | `//go:generate go tool mockgen` stanza on the interface; output `mocks/{service,handler}/<entity>.go`                          |

Recipes with every step: [adding an entity](../docs/architecture/adding-an-entity.md),
[adding an adapter](../docs/architecture/adding-an-adapter.md).

### Hard rules

- **Use-cases never import adapters.** They depend on ports; `internal/app/` wires them.
- **Handlers never import use-cases.** They depend on driving ports; same wiring.
- **Domain types are pure.** Struct tags are fine; a transport import is not.
- **A handler that exists is registered.** No gate looks at `internal/app` and CI never runs the integration suite, so `products` once answered 404 on every route with every gate green. `TestEveryHandlerIsRegistered` fails for any `app.Handlers` field with no `RegisterRoutes` call in `server.go`.
- **File names follow the entity.** `rate_limits_<topic>.go` holds the `RateLimit` rule entity; `ratelimit_<topic>.go` holds the limiter mechanism. Two spellings, two things.

## Code style

Follow [Effective Go](https://golang.org/doc/effective_go.html) and the Google
[Go Style Guide](https://google.github.io/styleguide/go/guide),
[Decisions](https://google.github.io/styleguide/go/decisions) and
[Best Practices](https://google.github.io/styleguide/go/best-practices).
Small functions, meaningful names, dependency injection through ports.

### Go 1.27 baseline

`go fix -diff ./...` is clean; keep it that way and do not introduce older idioms.

- `any`; `min`/`max`; `slices`/`maps`; `for range n`; `new(expr)` for optional pointers.
- `errors.AsType[E](err)`, never `errors.As` — there are zero call sites and zero is the invariant.
- `iter.Seq`/`iter.Seq2` for streaming; `slog.NewMultiHandler` for a record with two sinks.
- **Standard-library `uuid`**, imported as `"uuid"`, not `github.com/google/uuid`. `uuid.NewV7()` returns no error; `uuid.Nil()` and `uuid.Max()` are functions (`&uuid.Nil` becomes `new(uuid.Nil())`); there is no `Version()` — use `domain.UUIDVersion`; `uuid.Parse` says only `invalid uuid`, so `handler.uuidParseReason` derives the wording. `domain.EnsureUUIDV7` still returns an always-nil error on purpose.
- **Never forward a library's error string into an API response.** A dependency bump would rewrite the published contract.
- `encoding/json` now sits on v2: error text changed and map keys are unsorted. Fixtures compare structurally (`assertJSONEqual`), keep it that way; `GOEXPERIMENT=nojsonv2` is a bug to fix, not a setting.
- No `godebug` line, no `//go:debug`; the `go` command fails on a removed setting.
- `testing/synctest` + `synctest.Sleep` + `httptest.NewTestServer` for anything time-dependent.
- Generic methods exist but cannot satisfy an interface, so ports stay non-generic.
- `go fix ./...` runs in the **same commit** as a `go` directive bump (the `embedlit` modernizer fires on `tokenjwt/adapter.go`).
- Every source-processing tool must be built by the running toolchain. `make tools` reinstalls what is missing or older; `betteralign` fails **silently** when stale.

The full list, with what each 1.27 change means here and the uuid API
differences, is in [`docs/development/go-127-baseline.md`](../docs/development/go-127-baseline.md).

## Configuration

- **Env var = flag with dots as underscores, uppercased.** `http.server.addr` is `HTTP_SERVER_ADDR`. No exceptions.
- **A switch is `.enabled`, never `.enable`.** A wrong guess stops the process (`flag provided but not defined`).
- **Every `Field` needs its flag registered by hand in `internal/app/configs.go`**; `TestEveryConfigFieldHasAFlag` catches a gap.
- **A default is written twice** (`NewField` and `setupFlags`); never borrow a neighbour's constant — `TestNoTwoSettingsShareADefaultConstant`.
- **Validation keys on the setting that selects the behaviour**, so the error names the setting to fix.
- **Token lifetimes and rate-limit budgets are database rows, not flags (migration `00016` seeds the lifetimes).** Do not add a fallback constant in Go; a replica that cannot read the row refuses to start.
- A mail transport belongs in `slashdevops/mailer`, not here.

Why each rule exists: [`docs/development/configuration-rules.md`](../docs/development/configuration-rules.md).
The operator's view: [`docs/operations/running-the-service.md`](../docs/operations/running-the-service.md).

## Security and authentication invariants

Each line is a rule that was broken once and measured. The linked doc has the
mechanism, the diagram and the measurement.

- **The middleware chain order is the contract**: `Recovery`, `SecurityHeaders`, transport, `MaxBody`, `RequireJSONBody`, pre-auth limiter, CORS. `TestEveryRequestIsRecoveredBoundedAndHeadered` pins it. A 500 says `internal server error`; `Location` comes from `respond.SetLocation`; `X-Request-ID` is minted, never taken. → [security.md](../docs/architecture/security.md)
- **`ReadHeaderTimeout` is on; `ReadTimeout`/`WriteTimeout` stay 0.** A write deadline would abort a legitimate long-running outbound call. → [http-server-timeouts.md](../docs/architecture/http-server-timeouts.md)
- **Forwarding headers are honoured only from `http.server.trusted.proxies`**, walked right to left; empty means `RemoteAddr`. → [rate-limiting.md](../docs/architecture/rate-limiting.md#client-ip-resolution)
- **One limiter, `rate_limits` rows are the only budgets, no fallback floor.** One rule per scope; the pre-auth stage must not trust `r.Pattern`; a broken rule is dropped and reported, never enforced or shadowing; a store fault is `RATE_LIMIT_UNAVAILABLE`, never "allowed"; buckets key on window parameters, not ids; every limiter registration goes through `RateLimitExemptions.Wrap`. → [rate-limiting.md](../docs/architecture/rate-limiting.md)
- **OPA is compiled once; the caller's grants travel in `input`.** `*` in a path is one uuid segment, `*` as an action is every method, the administrator rule is membership. `GrantGuard` runs before anything widens a grant; every grant change invalidates the same cache keys as its twin (`authzCacheKey`). A new path wildcard that is not `project_id` gets no membership check. → [authorization.md](../docs/architecture/authorization.md)
- **Token revocation fails closed, in Postgres.** Access tokens are denylisted through an in-memory mirror whose first load is fatal; refresh tokens rotate, carry their expiry, and a replay past the grace ends the chain; `Rotate` never overwrites a successor. → [authentication.md](../docs/architecture/authentication.md)
- **One JWT verifier, `tokenjwt.Signer.Verify`.** `iss`, `aud`, `exp` validated; `kid` is the RFC 7638 thumbprint; rotate keys with an overlap. A refused caller hears `Invalid or expired token`. A handler acts on the token the middleware verified, never one re-read from the body. **No credential travels in a URL or an error.** → [authentication.md](../docs/architecture/authentication.md)
- **A failed login says one thing, and one bcrypt compare runs every time.** The per-account throttle spends on `Attempt` and refunds on `Succeed`, delays but never locks, and is per replica. → [authentication.md](../docs/architecture/authentication.md)
- **An identity provider proves a subject, never an email.** Kind decides the protocol, issuer lives on the instance, identity is `(idp, subject)`, the API sets no cookie and issues no redirect. → [identity-providers.md](../docs/architecture/identity-providers.md)
- **Bodies are bounded before they are read** (1 MiB by default; give a large upload its own bound), must be `application/json`, and unknown fields are refused. Outbound requests dial through `safedial.Policy`. → [security.md](../docs/architecture/security.md)
- **Dev runs the shipped posture**: limiter on, shipped token lifetimes, `ratelimit.enabled=true`. `TestRunScriptAndAirAgree` keeps `run.sh` and `.air.toml` in step. A dev stack that disagrees with production hides production's bugs.

## SQL rules (repository layer)

- **Values go through `$n` placeholders**, never `fmt.Sprintf`. `repositorypg/roles.go` `UpdateByID` records the injection this stops.
- **Identifiers that must be interpolated** (a schema or table name computed at runtime) go through `pgx.Identifier{schema, table}.Sanitize()`.
- **A value that cannot be a placeholder** (distance operator, sort direction) is checked against an allow-list first.
- Existing `html/template` SQL builders are a wart, not a pattern; new builders use `text/template` plus explicit sanitisation.

→ [`docs/architecture/repository-sql.md`](../docs/architecture/repository-sql.md)

## Database migrations

- **A new file sorts after every applied one.** goose runs with no `AllowMissing`; a lower number stops startup. Until the first production deploy a column goes into the file that creates its table and the set is renumbered if needed; after it, migrations are additive only.
- **Editing an applied migration fails silently** — goose does not checksum. `make start-dev-env` recreates the database to pick a change up.
- **Seed data is insert-only.** System rows are guarded by one shared trigger function in `002_shared_functions.sql`; a Down must `DISABLE TRIGGER`, delete, re-enable.
- **Identifiers stay under 63 characters** and a constraint name referenced from Go (`handlePgError`) is a contract.
- **No speculative indexes.** Index a foreign key's leading column and what a query filters or sorts on.
- **Verify both directions** (`up`, `down-to 0`, `up`) against a live database before committing. A schema diff is `pg_dump --schema-only` with the `\restrict` lines filtered.
- **The `resources` rows in `00008_…` are generated** from `swagger.json` by `cmd/apiendpoints` — see Swagger below.

→ [`docs/architecture/database-migrations.md`](../docs/architecture/database-migrations.md)

## Documentation and Swagger

- **Every package has a package comment** saying why it exists, its contract, its invariants and the reason behind each non-obvious decision. `provider/doc.go` is the reference depth. godoc does not render mermaid.
- **Every architectural change updates `docs/architecture/`**, its `README.md` index and the root README list, with a mermaid diagram of the real mechanism including the failure paths.
- **Swagger annotations are the API contract.** `make build` regenerates `docs/api/` from them; never hand-edit it; every status a handler can write has a `@Success`/`@Failure`, including the middleware-supplied `401`/`403`/`413`/`415`/`429`; document no code the handler cannot return.
- **`@ID`, `@Summary`, `@Description`, `@Router` and route registrations feed the authz seed**: `make build && go run cmd/apiendpoints/main.go`, paste over the generated block in `00008_roles_policies_tables_upsert.sql`, diff the sorted row set. A new endpoint on a deployed database also needs a new migration.

→ [`docs/development/documentation-and-swagger.md`](../docs/development/documentation-and-swagger.md)

## Testing

- `make test` runs the unit suite (`-tags=unit`, race detector, coverage). Integration tests are `tests/integration/` behind `//go:build integration`, run against a live service: `go test -race -tags=integration ./tests/integration [-run TestXXX]`. Eval is behind `eval`.
- **`go build` and `make test` never compile the tagged suites.** After a sweeping change: `for t in unit integration eval; do go vet -tags=$t ./...; done`.
- **Never let a mock server encode the struct the client decodes.** External APIs are tested against golden JSON fixtures in `testdata/`, request and response.
- Time-dependent tests use `testing/synctest`, never real sleeps. Large debug output goes to `t.ArtifactDir()`. Parsers of untrusted input (the filter/sort/fields parsers) get `Fuzz` targets.
- Scenario names are lowercase snake_case: `t.Run("not_found", …)`. Integration helpers live in `tests/integration/helper_functions_test.go`.
- **A skipped test reports `ok`.** The `ratelimitvalkey` tests skip without a reachable Valkey and `go test` caches the skip; `make test` sets `VALKEY_TEST_CA` from `certs/dev/ca.crt`. On a coverage-gate failure straight after `make start-dev-env`, run `go clean -testcache` first. → [rate-limiting.md](../docs/architecture/rate-limiting.md#the-valkey-tests-need-a-reachable-valkey-or-they-skip)
- `make test-coverage` is a **ratchet** on what the unit suite actually reaches; regenerate floors by statements, and do not merge the integration profile into it.

## Running things locally

```bash
make dev-certs         # JWT pair, AES key, dev TLS CA and server pair under certs/;
                       # creates what is missing, NEVER overwrites (a new jwt.key
                       # invalidates every token, a new AES key every stored secret)
make start-dev-env     # Postgres, Valkey, Prometheus, Grafana, Tempo, Mailpit.
                       # DESTROYS the database — it is how a migration change is picked up
make stop-dev-env      # stop, keep volumes
make rm-dev-env        # remove entirely
air                    # build + run with live reload (flags from .air.toml)
pkill -f go-rest-api-service-template
make build && ./build/go-rest-api-service-template -ratelimit.enabled=false   # one-off flags
curl -s localhost:8080/api/v1/version   # check WHICH binary answers before trusting a result
make rename-project    # after cloning: owner/name from the origin remote, or
                       # GIT_REPOSITORY_OWNER=<o> GIT_REPOSITORY_NAME=<n>
```

Every `certs/` path `.air.toml` or `run.sh` names must be something `dev-certs`
generates (`TestEveryDevStackCertIsGenerated`); a named-but-missing file stops
the binary before it reads anything. `PROJECT_NAME` is parsed from the `module`
directive, never grepped. The frontend is
`~/git/github.com/slashdevops/go-rest-api-service-template-frontend`
(`pnpm run dev`; `pnpm run test:e2e` needs core and the dev env). An API shape
change is a two-repo change. `make go-mod-update` upgrades transitively and has
broken OPA once; run the three-tag `go vet` loop after it.
→ [`docs/development/dev-stack.md`](../docs/development/dev-stack.md)

## Post-change checklist

```bash
make tools               # rebuild any tool missing or built by an older Go
go fix -diff ./...       # must stay clean; `go fix ./...` applies
make go-fmt
make go-betteralign      # rewrites source; deliberately not part of build
make lint                # gofmt + vet + staticcheck + errcheck + gosec + opa check/fmt/test
make build               # also swag fmt + swag init → docs/api
make test                # unit, race, coverage
make vulncheck
make licenses-check
# touching tests/integration/:
make rm-dev-env && make start-dev-env && air   # other terminal
go test -race -tags=integration ./tests/integration
```

`gosec` findings are fixed or excluded with a reason in `.golangci.yaml`,
never silenced inline without one.

### CI

`pr.yaml` runs `go build ./...`, `make arch-test`, `make lint`, `make test`,
`make test-coverage` and nothing else. The Valkey container is deliberately **off** there, so the two Valkey-backed suites skip in CI and `.testcoverage.yml` carries lowered floors; run them locally. `security-scan.yaml` runs `govulncheck`
weekly; the release workflow runs `make build-dist`. **Nothing in CI catches
`docs/api` drifting from the annotations** — run `make build` locally. Do not
add a PR step that cannot answer differently because of the change under
review. `defaults.run.shell: bash` and `MAKE_STOP_ON_ERRORS: true` are
load-bearing. → [`docs/development/ci-gates.md`](../docs/development/ci-gates.md)

## Further reading

- [`docs/getting-started.md`](../docs/getting-started.md) — from an empty machine to a logged-in request
- [`docs/architecture/README.md`](../docs/architecture/README.md) — hexagon overview, request flow, index of every design doc
- [`docs/architecture/adding-an-entity.md`](../docs/architecture/adding-an-entity.md) — the recipe `products` follows
- [`docs/architecture/security.md`](../docs/architecture/security.md), [`authentication.md`](../docs/architecture/authentication.md), [`authorization.md`](../docs/architecture/authorization.md), [`identity-providers.md`](../docs/architecture/identity-providers.md), [`token-lifetimes.md`](../docs/architecture/token-lifetimes.md), [`rate-limiting.md`](../docs/architecture/rate-limiting.md), [`resource-limits.md`](../docs/architecture/resource-limits.md), [`http-server-timeouts.md`](../docs/architecture/http-server-timeouts.md), [`caching.md`](../docs/architecture/caching.md), [`health-probes.md`](../docs/architecture/health-probes.md), [`database-migrations.md`](../docs/architecture/database-migrations.md), [`repository-sql.md`](../docs/architecture/repository-sql.md)
- [`docs/development/`](../docs/development/) — Go 1.27 baseline, configuration rules, documentation and Swagger rules, CI gates, the dev stack in detail
- [`docs/operations/running-the-service.md`](../docs/operations/running-the-service.md) — the pre-flight checklist
