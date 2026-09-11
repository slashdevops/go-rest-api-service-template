# The dev stack, in detail

The long form of the "Running things locally" section of `CLAUDE.md`: what
each target does, what it must never do, and the failure each note records.


The whole loop, in the order it is normally needed. Both repos matter: an API
change is usually a two-repo change, and the frontend is where a layout or
copy problem is actually visible.

## The core dev environment (Postgres, Valkey, Prometheus, Grafana, Tempo, Loki, Mailpit)

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

**Every `certs/` path `.air.toml` or `run.sh` names must be something
`dev-certs` generates.** A file-valued flag is opened while the flags are
parsed, so a named-but-missing file is not a warning: the binary prints
`invalid value ... no such file or directory` and its usage before reading
anything else, even with the feature that uses the file switched off. That is
how a fresh checkout could not run `air` for as long as the HTTP server's
`goapitemplate.local.{crt,key}` was named and produced by nothing. The pair is
generated now (self-signed, `DEV_TLS_HOST` in the Makefile), and
`TestEveryDevStackCertIsGenerated` fails on the next such path.

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

## The core service

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

## The frontend

```bash
cd ~/git/github.com/slashdevops/go-rest-api-service-template-frontend
cp .env.example .env   # ENDPOINT_API defaults to the local core
pnpm run dev           # http://localhost:5173
pkill -f vite          # stop it
```

`pnpm test` is vitest (unit and component). `pnpm run test:e2e` is Playwright,
which starts its own dev server and needs **both** the core service and the dev
environment up. `pnpm check`, `pnpm lint` and `pnpm build` are the other gates.

## Dependencies

```bash
make go-mod-update     # core: go get -u per direct dependency, then tidy
pnpm update            # frontend: within the ranges in package.json
```

`go get -u` upgrades the **transitive** closure too, which has broken the build
once: it pulled `gobwas/glob` past what OPA supports and nothing compiled.
`go mod tidy` is happy with that state — only a build or `go vet` catches it, so
run `for t in unit integration eval; do go vet -tags=$t ./... ; done` after any
dependency change.

## After the laptop sleeps: `make dev-env-clock-sync`

Every backend in the dev stack — Postgres, Valkey, Prometheus, Loki, Tempo —
runs inside the podman virtual machine, and **the VM's clock does not move
while the host is asleep.** Close the lid for an afternoon and the VM wakes up
that far behind. chrony in the VM does correct it, but only at its next NTP
poll, which after a resume can be many minutes away.

Until then, everything the service sends is stamped with the host's clock and
judged by the VM's:

| Backend | What it does with data from "the future" |
| --- | --- |
| Loki | refuses it: `entry has timestamp too new` — now tolerated up to 24h by `creation_grace_period` |
| Prometheus | refuses it: `out of bounds: timestamp is too far in the future` — **no setting changes this**; the samples are gone |
| Postgres | `NOW()` is wrong by the skew, so anything the service or the integration tests compare against `time.Now()` — token lifetimes, rotation grace, throttle windows — misbehaves |
| Grafana | asks each backend for "the last hour" in *browser* time, which for Loki is entirely in its future: panels come back empty even for data it holds |

Measured in the core service: a VM 1h50m behind refused an entire integration
run — 261 log entries and five minutes of metrics — and the only symptoms were
empty dashboards and a test failure that could not be reproduced once the
clock had caught up. This stack is the same pod, so it fails the same way.

```bash
make dev-env-clock-check   # reports the skew; non-zero exit when it is over 5s
make dev-env-clock-sync    # steps the VM's clock to the host's, then re-checks
```

`start-dev-env` runs the sync itself, so a fresh stack always starts in step.
Run it by hand after a sleep — before `air`, and before any integration run.

## Where the three signals go

Each signal has its own exporter setting, and each can be off independently.
Losing any of them costs visibility and never service: the `telemetry`
component of `/health/detailed` goes `degraded`, the overall status stays
`healthy`, and every request still succeeds.

| Signal | Setting | Default | Dev stack | Typical production |
| --- | --- | --- | --- | --- |
| traces | `opentelemetry.trace.exporter` | `console` | `otlp-http` → Tempo `:4318` | `otlp-http` → a collector |
| metrics | `opentelemetry.metric.exporter` | `console` | `otlp-http` → Prometheus `:9090` | `otlp-http` or `prometheus` |
| logs | `opentelemetry.log.exporter` | **`noop`** | `otlp-http` → Loki `:3100` | `otlp-http` → Loki or a collector |

**Logs are the one that defaults to off, and that is not a gap.** Every record
still reaches `log.output` exactly as it always has; `noop` means only that
nothing is *also* shipped to a log store. It is what every deployment did
before the setting existed, so an upgrade changes nothing until you ask it to.

```bash
# ship logs to a Loki
./build/go-rest-api-service-template \
  -opentelemetry.log.exporter=otlp-http \
  -opentelemetry.log.endpoint=loki.internal \
  -opentelemetry.log.port=3100

# ship them to an OpenTelemetry Collector instead: same exporter, different path
./build/go-rest-api-service-template \
  -opentelemetry.log.exporter=otlp-http \
  -opentelemetry.log.endpoint=otel-collector.internal \
  -opentelemetry.log.port=4318 \
  -opentelemetry.log.path=/v1/logs
```

Set `opentelemetry.environment` when more than one deployment reports to the
same Grafana. It becomes `deployment.environment.name` on every trace, metric
and log, and Loki promotes it to a label, so it is the dimension that tells a
staging replica from a production one. It is empty by default and then omitted
entirely, because a wrong environment label is acted on while a missing one is
asked about.

Two things to know before turning it on in production:

- **`-log.level=ctrace` is safe to leave alone**, because TRACE records are
  never exported whatever the configuration. That level carries SQL statements
  with their arguments and outbound LLM request bodies; the exporter's floor is
  `DEBUG` and it is a constant, not a setting.
- **The connection is not encrypted.** Every OTLP exporter here dials with
  `WithInsecure()`, logs included. On an untrusted network, put a collector on
  the same host and let it do the TLS.

What a failing exporter looks like:

```console
$ curl -s localhost:8080/api/v1/health/detailed -H "Authorization: Bearer $TOKEN" | jq .components.telemetry
{
  "status": "degraded",
  "message": "the telemetry exporter is failing; traces, metrics and logs from this replica are not reaching their collectors",
  "details": {
    "log_collector": "localhost:3100",
    "log_exporter": "otlp-http",
    "export_errors": "1",
    "last_export_error": "Post \"http://localhost:3100/otlp/v1/logs\": EOF"
  }
}
```

`telemetry_export_errors_total` carries the same count as a metric, so it can
be alerted on. It is worth having for one case in particular: when only the
*log* collector is down, every dashboard keeps working and that counter is the
only thing that says so.

Full mechanism: [observability.md](../architecture/observability.md).
