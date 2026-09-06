# Go 1.27 baseline

The module is `go 1.27.1`. This page records what the toolchain bump changed
and the API differences the standard-library `uuid` migration had to absorb.
It is the long form of the "Go 1.27 baseline" bullets in `CLAUDE.md`.


The module is `go 1.27.1` and `go fix -diff ./...` is **clean** — the codebase is
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

