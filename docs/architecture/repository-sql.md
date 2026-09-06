# Repository SQL rules

The rules every query in `internal/adapter/driven/repositorypg/` follows, and
the reachable injection each one was written against.


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

