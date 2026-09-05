# Requirements

Everything that has to exist on a machine before `go-rest-api-service-template` will build,
run or be developed on. It is the first page to read;
[running-the-service.md](./operations/running-the-service.md) is the next one.

## Software

| Requirement | Version | Needed for |
| --- | --- | --- |
| **Go** | `1.27.0`+ | Building and running |
| **macOS** | 13 Ventura+ | Building Go 1.27 itself, on Apple hardware |
| **podman** | any recent | The development environment and the container image |
| **OpenSSL** | 3.x | The JWT pair, the AES key and every TLS certificate |
| **PostgreSQL** | 18 | Supplied by the dev environment; a hard dependency in production. 18 specifically, for native `uuidv7()` |
| **Valkey** | any recent | Optional — `cache.enabled=false` is a supported mode |

### The Go patch version is load-bearing

CI resolves its toolchain from `go-version-file: ./go.mod`, and `govulncheck`
reports standard-library advisories against whatever toolchain it runs on. When
`make vulncheck` flags stdlib entries, bump the `go` directive before anything
else.

### Rebuild the tools after a toolchain upgrade

Every Go tool this repo uses is installed by the Makefile on demand, so this is
rarely needed by hand:

```bash
make tools
```

It matters after a Go upgrade. A tool that parses Go source cannot read a
standard library newer than the compiler that built it, and the two failures
look nothing alike: `govulncheck` errors loudly, while `betteralign` prints
`analysis skipped due to errors in package` and exits **0** — which reads
exactly like success. `make tools` rebuilds anything older than the current
toolchain and leaves everything else alone.

## podman on Apple Silicon

By default the podman machine mounts only `$HOME` into the VM. The development
environment writes outside it, so the machine has to be recreated with the extra
mounts.

> **This destroys the current machine and all its containers.**

```bash
podman machine stop
podman machine rm

mkdir -p $HOME/tmp
podman machine init -v $HOME:$HOME -v $HOME/tmp:$HOME/tmp:rw,security_model=none
podman machine start
```

Reference: <https://github.com/containers/podman/issues/14815>

## Key material

Three pieces are mandatory and **none has a working default** — the defaults
name files that do not exist. Generating them is covered in
[certificates.md](./certificates/certificates.md); which setting consumes each
and how it fails is in
[running-the-service.md](./operations/running-the-service.md).

| Material | Setting | Required |
| --- | --- | --- |
| EC private key (prime256v1 PEM) | `authn.private.key.file` | **yes** |
| EC public key (the public half) | `authn.public.key.file` | **yes** |
| AES key, 32/48/64 hex characters | `authn.symmetric.key.file` | **yes** |
| PostgreSQL TLS material | `database.ssl.*` | off by default |
| Valkey TLS material | `cache.tls.*` | off by default |
| HTTPS certificate for the listener | `http.server.tls.*` | off by default |

Mail is also not optional: account verification and password recovery both send,
and the service refuses to start without an SMTP host or an API URL.

## Two ways to run it, and they must agree

| | |
| --- | --- |
| `air` | Day-to-day. Rebuilds on every save. Arguments in `.air.toml`. |
| `./run.sh` | A plain `go run` with no file watching — e.g. under a debugger. |

**They pass the same flags, and `TestRunScriptAndAirAgree` fails the build if
they stop doing so.** They had drifted: `run.sh` ran the IP rate limiter at
200 req/s burst 400 while `.air.toml` ran the shipped 100/300, so a limit that
held under `air` could pass under `run.sh`, and neither matched what ships.

It is the same rule that removed the old 24-hour access-token override from both
files: **a dev stack that disagrees with production hides exactly the bugs
production will have.** Where a value has a shipped default, dev runs the
shipped default.

## Starting PostgreSQL and Valkey with TLS

The client half is two settings each. The server half is what people get stuck
on, and it is written out step by step in
[postgres-tls.md](./certificates/postgres-tls.md) and
[valkey-tls.md](./certificates/valkey-tls.md) — CA, server certificate, server
flags, and how to verify the connection is really encrypted. Those pages are the
source of truth; what follows is the entry point.

### The development stack does it for you

`make start-dev-env` generates one CA and one server certificate covering both
databases, and starts both with TLS on:

```bash
make start-dev-env
```

To generate the material without the stack — for a database you run yourself:

```bash
./dev-env/scripts/generate-dev-certs.sh ./certs/dev
# certs/dev/ca.crt      the CA both clients trust
# certs/dev/server.crt  presented by PostgreSQL and Valkey
# certs/dev/server.key  mode 0600
```

It is idempotent. Regenerating on every start would invalidate the CA the
running service already loaded, so it only regenerates when the certificate is
missing or has expired.

### Pointing the service at them

```bash
-database.ssl.mode=verify-full -database.ssl.root.cert.file=./certs/dev/ca.crt -cache.tls.enabled=true -cache.tls.ca.file=./certs/dev/ca.crt
```

`verify-full` is the only PostgreSQL mode that resists an active attacker.
`require` encrypts to *whoever answered*; `prefer` silently downgrades to
cleartext when the server does not offer TLS, and nothing reports it.

### Three things that bite

- **The server key must be `0600` and owned by the server's uid.** PostgreSQL
  refuses to start otherwise, which is why the dev pod pins `runAsUser: 999`.
  A key mounted with the wrong ownership produces a container that exits with
  `private key file has group or world access`.
- **`ssl=on` makes PostgreSQL *accept* TLS, not *require* it.** Requiring it is
  a `pg_hba.conf` decision: `hostssl` entries and no `host` fallback.
- **Valkey serves TLS and cleartext on different ports.** The dev pod puts TLS
  on the standard 6379 and leaves 6380 cleartext for the integration suite,
  which talks to Valkey directly to inspect keys. **A production server should
  pass `--port 0`** to refuse cleartext entirely.

TLS is on in development deliberately. A code path that only runs in production
is a code path nobody has tested.

## Ports the development environment uses

| Service | Port | Notes |
| --- | --- | --- |
| PostgreSQL | `5432` | `username` / `password` / `go-rest-api-service-template`, TLS on |
| Valkey | `6379`, `6380` | TLS on `6379`, cleartext on `6380` |
| Grafana | `3000` | dashboards provisioned from the repo |
| Prometheus | `9090` | OTLP receiver; alert rules loaded |
| Tempo | `3200`, `4317`, `4318` | traces |
| Mailpit | `1025` SMTP, `8025` UI | every email the service sends |
| The API | `8080` | run by `air`, not by podman |

## Getting to a running service

```bash
make tools                 # rebuild any tool older than the current Go
make start-dev-env         # every dependency, with TLS material generated
air                        # in another terminal
```

Then check it is alive:

```bash
curl -s localhost:8080/api/v1/health/live
```

## Further reading

- [Running the service](./operations/running-the-service.md) — every prerequisite in the order the service asks for it, and the real error each missing piece produces
- [Certificates and keys](./certificates/certificates.md) — generating the JWT pair and the AES key
- [PostgreSQL TLS](./certificates/postgres-tls.md) — generating the CA, configuring the server, choosing an SSL mode
- [Valkey TLS](./certificates/valkey-tls.md) — the same for the cache, and why that connection carries password hashes
- [Architecture](./architecture/README.md) — the hexagon, the request flow, the hard rules
