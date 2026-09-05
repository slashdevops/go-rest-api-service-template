# go-rest-api-service-template

[![Go Report Card](https://goreportcard.com/badge/github.com/slashdevops/go-rest-api-service-template)](https://goreportcard.com/report/github.com/slashdevops/go-rest-api-service-template)
[![Go Reference](https://pkg.go.dev/badge/github.com/slashdevops/go-rest-api-service-template.svg)](https://pkg.go.dev/github.com/slashdevops/go-rest-api-service-template)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/slashdevops/go-rest-api-service-template)
[![Pull Request](https://github.com/slashdevops/go-rest-api-service-template/actions/workflows/pr.yaml/badge.svg)](https://github.com/slashdevops/go-rest-api-service-template/actions/workflows/pr.yaml)
[![Security scan](https://github.com/slashdevops/go-rest-api-service-template/actions/workflows/security-scan.yaml/badge.svg)](https://github.com/slashdevops/go-rest-api-service-template/actions/workflows/security-scan.yaml)
[![Release](https://img.shields.io/github/v/release/slashdevops/go-rest-api-service-template?sort=semver)](https://github.com/slashdevops/go-rest-api-service-template/releases)
[![License](https://img.shields.io/github/license/slashdevops/go-rest-api-service-template)](./LICENSE)

A production-shaped starting point for a Go HTTP REST API service.

It is a **template**: clone it, run `make rename-project`, delete what you do
not need, and keep the parts that would otherwise take months to get right —
the hexagon, the authentication, the limits, the observability, the dev stack
and the CI.

---

## What you get

**Architecture.** [Hexagonal (ports & adapters)](./docs/architecture/README.md),
with the invariant enforced by a test rather than by discipline:
`internal/core/…` may not import `net/http`, a database driver, a cache client,
an adapter, or the composition root. `make arch-test` runs it, and so does CI.

**A worked example.** `products` is a project-scoped CRUD resource that exercises
every convention here — domain validation, both ports, a use-case, a pgx
repository with tenant scoping and keyset pagination, an HTTP handler with
swagger annotations, wire payloads, a generated mock, a migration, a seeded
resource limit, and an integration test. Copy it; see
[Adding a new entity](./docs/architecture/adding-an-entity.md).

**Authentication and authorization.**

- JWT access and refresh tokens, with **refresh rotation** — each refresh spends
  the token it was given and returns a successor; replaying a spent one ends the
  session.
- A **revocation denylist in PostgreSQL**, not in the cache. Logout revokes the
  access token it was called with, so it stops working immediately.
- Registration, email verification and password reset, with **uniform failure
  responses** so the endpoints do not disclose which accounts exist.
- A **login throttle** per identity, separate from the rate limiter.
- **OAuth / OIDC identity providers**, with client secrets encrypted at rest.
- **RBAC through Open Policy Agent** — roles, policies and a resource catalogue
  generated from the OpenAPI spec, evaluated by an embedded Rego bundle.

**Limits.**

- [**Rate limiting**](./docs/architecture/rate-limiting.md) with the rules in the
  database, not in flags: `rate_limits` + `rate_limit_windows`, matched by
  endpoint / prefix / global, keyed by ip / user / token / project / global, with
  token-bucket or leaky-bucket strategies and several windows per rule. A
  per-replica limiter sits in front of a shared Valkey counter, behind a circuit
  breaker, and the service **refuses to start** without a loadable rule set.
- [**Resource limits**](./docs/architecture/resource-limits.md) — soft and hard
  ceilings on what a deployment, user or project may create, resolved by scope
  with an integrity signature on the counters.

**Observability.** OpenTelemetry traces and metrics throughout, with a
`Metadata{Layer, Domain, Action}` convention so every span and metric is named
consistently across handler, use-case and repository. Grafana, Tempo and
Prometheus in the dev stack, six dashboards, and Prometheus alert rules with
their own unit tests (`make check-alerts`).

**Operations.** Three health endpoints with distinct jobs (liveness, detailed
readiness, a thin public verdict), [documented
timeouts](./docs/architecture/http-server-timeouts.md), TLS to PostgreSQL and
Valkey, mutual TLS, pprof behind a flag, and a `Containerfile` producing a
distroless multi-arch image.

**The API.** 49 paths across auth, users, projects, products, roles, policies,
resources, resource limits, rate limits, IdPs, `me`, health and version. Every
list endpoint shares one contract: `limit`, `next_token`, `prev_token`, `sort`,
`filter`, `fields`.

---

## Requirements

| | |
| --- | --- |
| **Go** | 1.27+ — the `uuid` package is used from the **standard library**, not `github.com/google/uuid` |
| **PostgreSQL** | 18 — for native `uuidv7()`; no extension needed |
| **Valkey** | 8+ (optional; `cache.enabled=false` is a supported mode) |
| **Podman** | for the dev stack and container builds |

See [docs/requirements.md](./docs/requirements.md).

---

## Quick start

```bash
# 1. Create your repository from this template, then rename the module.
make rename-project

# 2. Install the Go tooling this repo uses (air, swag, goose, golangci-lint, ...)
make tools

# 3. Generate the signing and encryption keys the service needs.
#    (The PostgreSQL/Valkey dev CA is generated for you by step 4.)
mkdir -p certs
openssl ecparam -genkey -name prime256v1 -noout -out certs/jwt.key
openssl ec -in certs/jwt.key -pubout -out certs/jwt.pub
openssl rand -hex 32 | tr -d '\n' > certs/aes-256-symmetric-hex.key

# 4. Start PostgreSQL, Valkey, Grafana, Tempo, Prometheus and Mailpit
make start-dev-env

# 5. Run the service with hot reload
air
```

`certs/` is git-ignored. See [Certificates](./docs/certificates/certificates.md)
for what each key protects, and for the mTLS and production material.

The API is then on `http://localhost:8080`, Swagger UI on
`http://localhost:8080/swagger/index.html`, Grafana on `http://localhost:3000`
and Mailpit on `http://localhost:8025`.

Migrations run at startup. See
[Running the service](./docs/operations/running-the-service.md) for the full
list of what must be supplied before a deployment is real.

---

## Make targets

```bash
make help              # every target, with its description

make build             # build the service (regenerates swagger first)
make test              # unit tests
make test-integration  # integration tests against a running stack
make test-coverage     # coverage, gated by .testcoverage.yml

make lint              # golangci-lint
make vulncheck         # govulncheck
make arch-test         # the hexagonal invariant
make check-alerts      # validate + unit-test the Prometheus alert rules
make licenses-check    # third-party licence allowlist

make go-mod-update     # update every dependency, then tidy
make docs-swagger      # regenerate ./docs/api from the handler annotations
make docs-api-resources # regenerate the authz resource rows for migration 3100
```

---

## Documentation

| | |
| --- | --- |
| [Architecture](./docs/architecture/README.md) | the hexagon, the layout, the flow of a request |
| [Adding an entity](./docs/architecture/adding-an-entity.md) | the end-to-end recipe — start here |
| [Adding an adapter](./docs/architecture/adding-an-adapter.md) | when you need a new external system |
| [Authentication](./docs/architecture/authentication.md) | tokens, rotation, revocation, throttling |
| [Rate limiting](./docs/architecture/rate-limiting.md) | the rule model and how it is enforced |
| [Resource limits](./docs/architecture/resource-limits.md) | scopes, resolution, the signature |
| [Caching](./docs/architecture/caching.md) | what is cached, and what must never be |
| [Database migrations](./docs/architecture/database-migrations.md) | the file set and the rules |
| [Health probes](./docs/architecture/health-probes.md) | which endpoint answers which question |
| [HTTP timeouts](./docs/architecture/http-server-timeouts.md) | which bound covers which span |
| [Certificates](./docs/certificates/certificates.md) | keys, TLS to PostgreSQL and Valkey |
| [Running the service](./docs/operations/running-the-service.md) | the pre-flight checklist |
| [API reference](./docs/api/markdown.md) | generated from the OpenAPI spec |

---

## Project layout

```text
cmd/                    service entrypoint + three small CLIs (uuidgen, saltpwd, apiendpoints)
database/migrations/    goose migrations, embedded and run at startup
dev-env/                the Podman pod: PostgreSQL, Valkey, Grafana, Tempo, Prometheus, Mailpit
docs/                   architecture, operations, certificates, generated API reference
internal/
├── adapter/
│   ├── driven/         outbound: repositorypg, cachevalkey, notifieremail, policyopa,
│   │                   tokenjwt, cipheraes, oauthidp, throttlememory, ratelimit{memory,valkey,breaker}
│   └── driving/http/   inbound: handler, payload, middleware, jwtvalidator, respond, server
├── app/                composition root — the only place that imports a port and its adapter
├── config/             flags and environment
├── core/               PURE: domain, port/{driven,driving}, usecase  ← no infra imports
├── o11y/               OpenTelemetry setup and helpers
└── version/            build metadata
mocks/                  generated with go.uber.org/mock
pkg/cslog/              slog Trace and Fatal levels
tests/integration/      the API suite, run against a separately started binary
```

---

## Contributing

Pull requests run build, `go fix` modernizers, `arch-test`, `lint`,
`vulncheck`, `check-alerts`, the unit suite and the coverage gate. Keep
`internal/core/` free of infrastructure imports and the rest tends to follow.

## License

[Apache License 2.0](./LICENSE).
