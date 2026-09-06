# CI gates and local checks

What the PR workflow gates on, what was removed from it and why, and the two
workflow settings that must never go. The long form of the "What CI gates on"
section that used to live in `CLAUDE.md`.

## The normal gates never compile the integration suite

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


## What CI gates on

**The PR workflow is deliberately cheap; the release workflow is thorough.**
That split is the rule to keep — a gate belongs on the PR only if it can
answer differently because of the change under review.

`.github/workflows/pr.yaml` runs, in order: `go build ./...`,
`make arch-test`, `make lint`, `make test`, `make test-coverage`. Nothing
else. The Valkey service container is deliberately **off** to keep the runner
cheap, so the two Valkey-backed suites (`ratelimitvalkey`,
`changenotifyvalkey`) skip in CI and `.testcoverage.yml` carries lowered
floors with the same note; run them locally against the dev stack, where the
Makefile sets `VALKEY_TEST_CA` for you.

Three gates were removed because each was cost with no answer the change
under review could alter:

| Removed gate        | Why it went, and where the check lives now |
| ------------------- | ------------------------------------------ |
| `go fix -diff`      | a modernizer drift is style, not a defect; it stays on the post-change checklist and `go fix ./...` is a one-liner locally |
| `make vulncheck`    | a per-PR scan only ever answered "did this PR add a vulnerable dependency"; the weekly `security-scan.yaml` covers that and the advisory published against a dependency nobody touched |
| `make check-alerts` | pulled a ~250 MB `prom/prometheus` image for a test that takes milliseconds; run `make check-alerts` locally whenever `dev-env/configuration/prometheus/` changes |

The changed-file query that gated the last two conditionally went with them,
and so did the `pull-requests: read` permission it needed.

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

