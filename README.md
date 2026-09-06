# 🚀 go-rest-api-service-template

[![Go Reference](https://pkg.go.dev/badge/github.com/slashdevops/go-rest-api-service-template.svg)](https://pkg.go.dev/github.com/slashdevops/go-rest-api-service-template)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/slashdevops/go-rest-api-service-template)
[![Linted with golangci-lint](https://img.shields.io/badge/linter-golangci--lint-00ADD8?logo=go&logoColor=white)](./.golangci.yaml)
[![Pull Request](https://github.com/slashdevops/go-rest-api-service-template/actions/workflows/pr.yaml/badge.svg)](https://github.com/slashdevops/go-rest-api-service-template/actions/workflows/pr.yaml)
[![Security scan](https://github.com/slashdevops/go-rest-api-service-template/actions/workflows/security-scan.yaml/badge.svg)](https://github.com/slashdevops/go-rest-api-service-template/actions/workflows/security-scan.yaml)
[![Release](https://img.shields.io/github/v/release/slashdevops/go-rest-api-service-template?sort=semver)](https://github.com/slashdevops/go-rest-api-service-template/releases)
[![License](https://img.shields.io/github/license/slashdevops/go-rest-api-service-template)](./LICENSE)

A production-shaped starting point for a Go HTTP REST API service.

It is a **template**: create a repository from it, run `make rename-project`,
delete what you do not need, and keep the parts that would otherwise take
months to get right: the hexagon, the authentication, the limits, the
observability, the dev stack and the CI.

> 🧭 **New here?** [Getting started](./docs/getting-started.md) takes you from
> an empty machine to a logged-in request, one command at a time, and says
> what to check after each one.

---

## ⚡ Quick start

Five commands. Each can be run again; only step 4 has a side effect, it
recreates the stack's data.

```bash
make rename-project   # 1. once, right after creating your repository from the template
make tools            # 2. air, swag, goose, golangci-lint, ... (rebuilt whenever Go is upgraded)
make dev-certs        # 3. JWT signing pair, AES key, the server TLS pair and a dev TLS CA under certs/ (git-ignored)
make start-dev-env    # 4. PostgreSQL, Valkey, Grafana, Tempo, Prometheus and Mailpit, in a podman pod
air                   # 5. build and run the service with live reload
```

Then, from another terminal, prove it works:

```bash
curl -s localhost:8080/api/v1/health/live
```

🌐 **Log in from the browser.** Open Swagger UI at
<http://0.0.0.0:8080/api/v1/swagger/index.html> (`localhost` works too), expand
`POST /auth/login`, press **Try it out**, and send the administrator the
migrations seed for the development database:

```json
{ "email": "admin@goapitemplate.local", "password": "ThisIsApassw0rd.," }
```

Copy the `access_token` from the answer into the **Authorize** button as
`Bearer <token>`, and every other endpoint on that page works. The same call
from the shell:

```bash
curl -s -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@goapitemplate.local","password":"ThisIsApassw0rd.,"}'
```

📈 **Give the dashboards something to show.** The integration suite drives every
endpoint over HTTP, so a few passes of it fill the database, the metrics and the
traces with realistic activity:

```bash
cp tests/integration/integration.env.example tests/integration/integration.env   # once
go test -tags=integration ./tests/integration -count 3
```

Then open Grafana, Prometheus and Tempo below and watch the panels fill.

| 🔗 Where | URL |
| --- | --- |
| API | `http://localhost:8080/api/v1` |
| Swagger UI | `http://0.0.0.0:8080/api/v1/swagger/index.html` |
| Grafana (six dashboards, provisioned) | `http://localhost:3000` |
| Prometheus (metrics and alert rules) | `http://localhost:9090` |
| Tempo (traces, also queried from Grafana) | `http://localhost:3200` |
| Mailpit (every email the service sends) | `http://localhost:8025` |

🛑 **Stopping, and starting over.**

| I want to | Run |
| --- | --- |
| stop the service | `Ctrl+C` in the `air` terminal, or `pkill -f go-rest-api-service-template` |
| stop the stack and keep its data | `make stop-dev-env` |
| destroy the stack **and its data** | `make rm-dev-env` |
| start again from nothing | `make rm-dev-env && make start-dev-env`, then `air` |

Migrations run at startup, and `make start-dev-env` recreates the data every
time it runs. Skip step 1 when you are contributing to the template itself.

---

## ✨ What you get

**🔷 Architecture.** [Hexagonal (ports & adapters)](./docs/architecture/README.md),
with the invariant enforced by a test rather than by discipline:
`internal/core/…` may not import `net/http`, a database driver, a cache client,
an adapter, or the composition root. `make arch-test` runs it, and so does CI.

**📦 A worked example.** `products` is a project-scoped CRUD resource that
exercises every convention here: domain validation, both ports, a use-case, a
pgx repository with tenant scoping and keyset pagination, an HTTP handler with
swagger annotations, wire payloads, a generated mock, a migration, a seeded
resource limit, and an integration test. Copy it; see
[Adding a new entity](./docs/architecture/adding-an-entity.md).

**🔐 Authentication and authorization.**

- JWT access and refresh tokens, with **refresh rotation**: each refresh spends
  the token it was given and returns a successor; replaying a spent one ends the
  session.
- A **revocation denylist in PostgreSQL**, not in the cache. Logout revokes the
  access token it was called with, so it stops working immediately.
- Registration, email verification and password reset, with **uniform failure
  responses** so the endpoints do not disclose which accounts exist.
- A **login throttle** per identity, separate from the rate limiter.
- **OAuth / OIDC identity providers**, with client secrets encrypted at rest.
- **RBAC through Open Policy Agent**: roles, policies and a resource catalogue
  generated from the OpenAPI spec, evaluated by an embedded Rego bundle.

**🚦 Limits.**

- [**Rate limiting**](./docs/architecture/rate-limiting.md) with the rules in the
  database, not in flags: `rate_limits` + `rate_limit_windows`, matched by
  endpoint / prefix / global, keyed by ip / user / token / project / global, with
  token-bucket or leaky-bucket strategies and several windows per rule. A
  per-replica limiter sits in front of a shared Valkey counter, behind a circuit
  breaker, and the service **refuses to start** without a loadable rule set.
- [**Resource limits**](./docs/architecture/resource-limits.md): soft and hard
  ceilings on what a deployment, user or project may create, resolved by scope
  with an integrity signature on the counters.
- [**Token lifetimes**](./docs/architecture/token-lifetimes.md) as one database
  row rather than two flags: `GET`/`PUT /auth/token_lifetimes`, bounded and
  ordered (refresh strictly longer than access), mirrored per replica with a
  Valkey change signal, and applied to the next token issued.

**🔭 Observability.** OpenTelemetry traces and metrics throughout, with a
`Metadata{Layer, Domain, Action}` convention so every span and metric is named
consistently across handler, use-case and repository. Grafana, Tempo and
Prometheus in the dev stack, six dashboards, and Prometheus alert rules with
their own unit tests (`make check-alerts`).

**🛡️ Operations.** Three health endpoints with distinct jobs (liveness, detailed
readiness, a thin public verdict), [documented
timeouts](./docs/architecture/http-server-timeouts.md), TLS to PostgreSQL and
Valkey, mutual TLS, pprof behind a flag, and a `Containerfile` producing a
distroless multi-arch image.

**🧩 The API.** 49 paths across auth, users, projects, products, roles,
policies, resources, resource limits, rate limits, IdPs, `me`, health and
version. Every list endpoint shares one contract: `limit`, `next_token`,
`prev_token`, `sort`, `filter`, `fields`.

---

## 📋 Requirements

| | |
| --- | --- |
| **Go** | 1.27+. The `uuid` package is used from the **standard library**, not `github.com/google/uuid` |
| **PostgreSQL** | 18, for native `uuidv7()`; no extension needed. The dev stack runs it for you |
| **Valkey** | 8+, optional: `cache.enabled=false` is a supported mode. The dev stack runs it for you |
| **Podman** | for the dev stack and the container image |
| **OpenSSL 3 and envsubst** | for `make dev-certs` and the dev stack. On macOS: `brew install openssl gettext` |

Everything else (`air`, `swag`, `goose`, `golangci-lint`, ...) is a Go tool that
`make tools` installs. See [Requirements](./docs/requirements.md) for the
details and the macOS notes.

---

## 🛠️ Make targets

`make help` lists every target with its description. The ones you will use:

```bash
make rename-project    # rename module, binary, pod and database after creating a repo from the template
make tools             # install or rebuild every Go tool this repo uses
make dev-certs         # generate the JWT pair, the AES key, the server TLS pair and the dev TLS CA (idempotent)
make start-dev-env     # start the dev stack. WARNING: recreates its data
make stop-dev-env      # stop it, keep the data
make rm-dev-env        # stop it and delete the data

make build             # build the service (regenerates swagger first)
make test              # unit tests, with the race detector
make test-coverage     # coverage, gated by .testcoverage.yml
make test-integration  # integration tests, in a container, against a fresh stack

make lint              # golangci-lint
make vulncheck         # govulncheck
make arch-test         # the hexagonal invariant
make check-alerts      # validate and unit-test the Prometheus alert rules
make licenses-check    # third-party licence allowlist

make go-mod-update      # update every dependency, then tidy
make docs-swagger       # regenerate ./docs/api from the handler annotations
make docs-api-resources # regenerate the authz resource rows for migration 00008
```

---

## 📚 Documentation

| | |
| --- | --- |
| 🧭 [Getting started](./docs/getting-started.md) | from an empty machine to a logged-in request. **Start here** |
| 📋 [Requirements](./docs/requirements.md) | versions, key material, podman on macOS, the dev-stack ports |
| 🏃 [Running the service](./docs/operations/running-the-service.md) | every prerequisite in the order the service asks for it, and the error each missing one produces |
| 🔷 [Architecture](./docs/architecture/README.md) | the hexagon, the layout, the flow of a request |
| 📦 [Adding an entity](./docs/architecture/adding-an-entity.md) | the end-to-end recipe for a new resource |
| 🔌 [Adding an adapter](./docs/architecture/adding-an-adapter.md) | when you need a new external system |
| 🔐 [Authentication](./docs/architecture/authentication.md) | tokens, rotation, revocation, throttling |
| 🛡️ [Security](./docs/architecture/security.md) | the middleware chain and its order, response headers, body and content-type bounds, error hygiene, project membership, single-use links, the outbound address guard, supply chain |
| 🪪 [Identity providers](./docs/architecture/identity-providers.md) | OIDC and GitHub sign-in: a provider proves a subject, never an email; linking, provisioning, the callback contract |
| ⏳ [Token lifetimes](./docs/architecture/token-lifetimes.md) | the access/refresh lifetimes as a database row edited through the API |
| 🚦 [Rate limiting](./docs/architecture/rate-limiting.md) | the rule model and how it is enforced |
| 📏 [Resource limits](./docs/architecture/resource-limits.md) | scopes, resolution, the signature |
| 🗄️ [Caching](./docs/architecture/caching.md) | what is cached, and what must never be |
| 🧬 [Database migrations](./docs/architecture/database-migrations.md) | the file set and the rules |
| 🩺 [Health probes](./docs/architecture/health-probes.md) | which endpoint answers which question |
| ⏱️ [HTTP timeouts](./docs/architecture/http-server-timeouts.md) | which bound covers which span |
| 🔑 [Certificates](./docs/certificates/certificates.md) | keys, TLS to PostgreSQL and Valkey |
| 📖 [API reference](./docs/api/markdown.md) | generated from the OpenAPI spec |
| 🤖 [Development guidelines](./CLAUDE.md) | the conventions, the hard rules and the reasons behind them |

---

## 🐳 Container image

Every release tag builds the `Containerfile` (distroless, `linux/arm64` and
`linux/amd64`) from the release binaries and publishes a multi-arch manifest to
this repository's GitHub Container Registry:

```bash
podman pull ghcr.io/slashdevops/go-rest-api-service-template:latest
```

It takes the same flags as the binary. The image needs the key material and a
reachable PostgreSQL, so the smallest useful invocation against the dev stack
mounts `certs/` and joins the pod:

```bash
podman run --rm --pod go-rest-api-service-template -v "$PWD/certs:/certs:ro" \
  ghcr.io/slashdevops/go-rest-api-service-template:latest \
  -authn.private.key.file=/certs/jwt.key -authn.public.key.file=/certs/jwt.pub \
  -authn.symmetric.key.file=/certs/aes-256-symmetric-hex.key \
  -mail.smtp.host=localhost -mail.smtp.port=1025 -mail.smtp.username=welcome@goapitemplate.local \
  -mail.smtp.password=secret -mail.smtp.require.tls=false \
  -cache.tls.enabled=true -cache.tls.ca.file=/certs/dev/ca.crt \
  -database.ssl.mode=verify-full -database.ssl.root.cert.file=/certs/dev/ca.crt \
  -http.server.port=8081
```

Locally, `GO_OS=linux GO_ARCH=arm64 make build-dist` followed by
`CONTAINER_OS=linux CONTAINER_ARCH=arm64 make container-build` produces the same
image for your machine's architecture.

---

## 🗂️ Project layout

```text
cmd/                    service entrypoint + three small CLIs (uuidgen, saltpwd, apiendpoints)
database/migrations/    goose migrations, embedded and run at startup
dev-env/                the podman pod (PostgreSQL, Valkey, Grafana, Tempo, Prometheus, Mailpit)
                        and the scripts that generate the dev key material
docs/                   getting started, architecture, operations, certificates, generated API reference
internal/
├── adapter/
│   ├── driven/         outbound: repositorypg, cachevalkey, notifieremail, policyopa,
│   │                   tokenjwt, cipheraes, oauthidp, throttlememory, ratelimit{memory,valkey,breaker}
│   └── driving/http/   inbound: handler, payload, middleware, jwtvalidator, respond, server
├── app/                composition root: the only place that imports a port and its adapter
├── config/             flags and environment
├── core/               PURE: domain, port/{driven,driving}, usecase  ← no infra imports
├── o11y/               OpenTelemetry setup and helpers
└── version/            build metadata
mocks/                  generated with go.uber.org/mock
pkg/cslog/              slog Trace and Fatal levels
tests/integration/      the API suite, run against a separately started binary
```

---

## 🤝 Contributing

Pull requests run build, the `go fix` modernizers, `arch-test`, `lint`,
`vulncheck`, the unit suite and the coverage gate. Keep `internal/core/` free of
infrastructure imports and the rest tends to follow. The post-change checklist
is in [CLAUDE.md](./CLAUDE.md#post-change-checklist).

## 📄 License

[Apache License 2.0](./LICENSE).
