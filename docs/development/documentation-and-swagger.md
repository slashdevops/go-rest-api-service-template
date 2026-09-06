# Documentation and Swagger rules

Documentation is part of the change, not a follow-up. This page is the long
form of the "Documentation" and "Swagger" bullets in `CLAUDE.md`.

## Every package carries a package comment

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

## Every architectural change updates `docs/`

- Update the affected file under `docs/architecture/`, or add one. The index in
  [`docs/architecture/README.md`](../architecture/README.md) and the
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

## Swagger annotations are the API contract, not comments

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

## Swagger changes feed the authz seed data — regenerate it

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

