# The dev stack, in detail

The long form of the "Running things locally" section of `CLAUDE.md`: what
each target does, what it must never do, and the failure each note records.


The whole loop, in the order it is normally needed. Both repos matter: an API
change is usually a two-repo change, and the frontend is where a layout or
copy problem is actually visible.

## The core dev environment (Postgres, Valkey, Prometheus, Grafana, Tempo, Mailpit)

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

